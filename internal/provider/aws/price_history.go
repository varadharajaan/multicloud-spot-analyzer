package aws

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/spot-analyzer/internal/provider"
)

// Cache key prefix for price history
const cacheKeyPriceHistory = "aws:price_history:"

// PriceHistoryProvider fetches real historical spot prices from AWS
type PriceHistoryProvider struct {
	client       *ec2.Client
	region       string
	cacheManager *provider.CacheManager
	available    bool
}

// PriceAnalysis contains computed metrics from historical price data
type PriceAnalysis struct {
	InstanceType     string
	AvailabilityZone string
	CurrentPrice     float64
	AvgPrice         float64
	MinPrice         float64
	MaxPrice         float64
	StdDev           float64
	Volatility       float64 // StdDev / AvgPrice (coefficient of variation)
	TrendSlope       float64 // Positive = prices rising, Negative = falling
	TrendScore       float64 // Normalized -1 to 1
	DataPoints       int
	TimeSpanHours    float64
	HourlyPattern    map[int]float64          // Hour -> Avg price
	WeekdayPattern   map[time.Weekday]float64 // Weekday -> Avg price
	LastUpdated      time.Time
	AllAZData        map[string]*AZAnalysis // All availability zone data
}

// AZAnalysis contains per-AZ price analysis
type AZAnalysis struct {
	AvailabilityZone string
	AvgPrice         float64
	MinPrice         float64
	MaxPrice         float64
	Volatility       float64
	DataPoints       int
}

// NewPriceHistoryProvider creates a provider with default AWS credentials
func NewPriceHistoryProvider(region string) (*PriceHistoryProvider, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		return &PriceHistoryProvider{
			region:       region,
			available:    false,
			cacheManager: provider.GetCacheManager(),
		}, nil // Return provider but mark as unavailable
	}

	client := ec2.NewFromConfig(cfg)

	// Test credentials with a minimal call
	priceProvider := &PriceHistoryProvider{
		client:       client,
		region:       region,
		available:    true,
		cacheManager: provider.GetCacheManager(),
	}

	// Quick validation - try to fetch one data point
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.DescribeSpotPriceHistory(ctx, &ec2.DescribeSpotPriceHistoryInput{
		MaxResults: aws.Int32(1),
	})
	if err != nil {
		priceProvider.available = false
	}

	return priceProvider, nil
}

// IsAvailable returns true if AWS credentials are configured and working
func (p *PriceHistoryProvider) IsAvailable() bool {
	return p.available
}

// GetPriceAnalysis fetches and analyzes historical prices for an instance type
func (p *PriceHistoryProvider) GetPriceAnalysis(ctx context.Context, instanceType string, lookbackDays int) (*PriceAnalysis, error) {
	if !p.available {
		return nil, nil
	}

	// Check global cache (2 hour TTL for price history)
	cacheKey := fmt.Sprintf("%s%s_%s_%d", cacheKeyPriceHistory, p.region, instanceType, lookbackDays)
	if cached, exists := p.cacheManager.Get(cacheKey); exists {
		return cached.(*PriceAnalysis), nil
	}

	// Fetch historical prices
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(lookbackDays) * 24 * time.Hour)

	input := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       []types.InstanceType{types.InstanceType(instanceType)},
		ProductDescriptions: []string{"Linux/UNIX"},
		StartTime:           aws.Time(startTime),
		EndTime:             aws.Time(endTime),
		MaxResults:          aws.Int32(1000),
	}

	var allPrices []types.SpotPrice
	paginator := ec2.NewDescribeSpotPriceHistoryPaginator(p.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		allPrices = append(allPrices, page.SpotPriceHistory...)

		// Limit to reasonable amount of data
		if len(allPrices) >= 5000 {
			break
		}
	}

	if len(allPrices) == 0 {
		return nil, nil
	}

	analysis := p.analyzePrices(instanceType, allPrices)

	// Update global cache with 2-hour TTL
	p.cacheManager.Set(cacheKey, analysis, 2*time.Hour)

	return analysis, nil
}

// GetBatchPriceAnalysis fetches analysis for multiple instance types efficiently
func (p *PriceHistoryProvider) GetBatchPriceAnalysis(ctx context.Context, instanceTypes []string, lookbackDays int) (map[string]*PriceAnalysis, error) {
	if !p.available {
		return nil, nil
	}

	results := make(map[string]*PriceAnalysis)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process in batches to avoid rate limiting
	batchSize := 10
	semaphore := make(chan struct{}, 5) // Max 5 concurrent requests

	for i := 0; i < len(instanceTypes); i += batchSize {
		end := i + batchSize
		if end > len(instanceTypes) {
			end = len(instanceTypes)
		}
		batch := instanceTypes[i:end]

		for _, instType := range batch {
			wg.Add(1)
			go func(it string) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				analysis, err := p.GetPriceAnalysis(ctx, it, lookbackDays)
				if err == nil && analysis != nil {
					mu.Lock()
					results[it] = analysis
					mu.Unlock()
				}
			}(instType)
		}
	}

	wg.Wait()
	return results, nil
}

func (p *PriceHistoryProvider) analyzePrices(instanceType string, prices []types.SpotPrice) *PriceAnalysis {
	if len(prices) == 0 {
		return nil
	}

	// Sort by timestamp (oldest first)
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Timestamp.Before(*prices[j].Timestamp)
	})

	// Extract price values
	var priceValues []float64
	hourlyPrices := make(map[int][]float64)
	weekdayPrices := make(map[time.Weekday][]float64)

	for _, sp := range prices {
		if sp.SpotPrice == nil {
			continue
		}
		price := parsePrice(*sp.SpotPrice)
		if price > 0 {
			priceValues = append(priceValues, price)
			hour := sp.Timestamp.Hour()
			weekday := sp.Timestamp.Weekday()
			hourlyPrices[hour] = append(hourlyPrices[hour], price)
			weekdayPrices[weekday] = append(weekdayPrices[weekday], price)
		}
	}

	if len(priceValues) == 0 {
		return nil
	}

	// Calculate statistics
	avg := mean(priceValues)
	stdDev := standardDeviation(priceValues, avg)
	minPrice, maxPrice := minMax(priceValues)
	volatility := 0.0
	if avg > 0 {
		volatility = stdDev / avg
	}

	// Calculate trend using linear regression
	trendSlope, trendScore := calculateTrend(priceValues)

	// Calculate time-based patterns
	hourlyPattern := make(map[int]float64)
	for hour, vals := range hourlyPrices {
		hourlyPattern[hour] = mean(vals)
	}

	weekdayPattern := make(map[time.Weekday]float64)
	for day, vals := range weekdayPrices {
		weekdayPattern[day] = mean(vals)
	}

	// Time span
	timeSpan := prices[len(prices)-1].Timestamp.Sub(*prices[0].Timestamp)

	// Best availability zone (lowest avg price) and all AZ data
	bestAZ, allAZData := analyzeAllAZs(prices)

	return &PriceAnalysis{
		InstanceType:     instanceType,
		AvailabilityZone: bestAZ,
		CurrentPrice:     priceValues[len(priceValues)-1],
		AvgPrice:         avg,
		MinPrice:         minPrice,
		MaxPrice:         maxPrice,
		StdDev:           stdDev,
		Volatility:       volatility,
		TrendSlope:       trendSlope,
		TrendScore:       trendScore,
		DataPoints:       len(priceValues),
		TimeSpanHours:    timeSpan.Hours(),
		HourlyPattern:    hourlyPattern,
		WeekdayPattern:   weekdayPattern,
		LastUpdated:      time.Now(),
		AllAZData:        allAZData,
	}
}

func parsePrice(s string) float64 {
	var price float64
	_, _ = fmt.Sscanf(s, "%f", &price)
	return price
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func standardDeviation(values []float64, avg float64) float64 {
	if len(values) < 2 {
		return 0
	}
	sumSquares := 0.0
	for _, v := range values {
		diff := v - avg
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(values)-1))
}

func minMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func calculateTrend(values []float64) (slope float64, normalizedScore float64) {
	n := len(values)
	if n < 2 {
		return 0, 0
	}

	// Simple linear regression
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	nf := float64(n)
	denominator := nf*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0, 0
	}

	slope = (nf*sumXY - sumX*sumY) / denominator

	// Normalize slope to -1 to 1 range based on price magnitude
	avgPrice := sumY / nf
	if avgPrice > 0 {
		// Percentage change per data point
		normalizedScore = (slope / avgPrice) * 100
		// Clamp to -1 to 1
		if normalizedScore > 1 {
			normalizedScore = 1
		} else if normalizedScore < -1 {
			normalizedScore = -1
		}
	}

	return slope, normalizedScore
}

// analyzeAllAZs analyzes price data for all availability zones
func analyzeAllAZs(prices []types.SpotPrice) (string, map[string]*AZAnalysis) {
	azPrices := make(map[string][]float64)
	for _, sp := range prices {
		if sp.SpotPrice == nil || sp.AvailabilityZone == nil {
			continue
		}
		price := parsePrice(*sp.SpotPrice)
		if price > 0 {
			azPrices[*sp.AvailabilityZone] = append(azPrices[*sp.AvailabilityZone], price)
		}
	}

	allAZData := make(map[string]*AZAnalysis)
	bestAZ := ""
	lowestAvg := math.MaxFloat64

	for az, vals := range azPrices {
		avg := mean(vals)
		minP, maxP := minMax(vals)
		stdDev := standardDeviation(vals, avg)
		volatility := 0.0
		if avg > 0 {
			volatility = stdDev / avg
		}

		allAZData[az] = &AZAnalysis{
			AvailabilityZone: az,
			AvgPrice:         avg,
			MinPrice:         minP,
			MaxPrice:         maxP,
			Volatility:       volatility,
			DataPoints:       len(vals),
		}

		if avg < lowestAvg {
			lowestAvg = avg
			bestAZ = az
		}
	}

	return bestAZ, allAZData
}
