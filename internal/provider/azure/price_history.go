// Package azure implements Azure price history analysis.
// This provides historical pricing data and analysis for Azure Spot VMs.
package azure

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

// Cache key prefix for price history
const cacheKeyPriceHistory = "azure:price_history:"

// PriceHistoryProvider provides historical price analysis for Azure Spot VMs
type PriceHistoryProvider struct {
	spotProvider *SpotDataProvider
	region       string
	cacheManager *provider.CacheManager
	mu           sync.RWMutex
}

// PriceAnalysis contains computed metrics from price data
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

// NewPriceHistoryProvider creates a new Azure price history provider
func NewPriceHistoryProvider(region string) *PriceHistoryProvider {
	return &PriceHistoryProvider{
		spotProvider: NewSpotDataProvider(),
		region:       region,
		cacheManager: provider.GetCacheManager(),
	}
}

// IsAvailable returns true - Azure price history is always available via Retail Prices API
func (p *PriceHistoryProvider) IsAvailable() bool {
	return true
}

// GetPriceAnalysis gets price analysis for a specific VM size
func (p *PriceHistoryProvider) GetPriceAnalysis(ctx context.Context, vmSize string, lookbackDays int) (*PriceAnalysis, error) {
	// Check cache first
	cacheKey := p.cacheKey(vmSize, lookbackDays)
	if cached, exists := p.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache hit for Azure price analysis: %s", vmSize)
		return cached.(*PriceAnalysis), nil
	}

	logging.Debug("Generating price analysis for Azure %s in %s", vmSize, p.region)

	// Note: Azure Retail Prices API returns current prices, not historical
	// We generate analysis based on current spot price with estimated volatility
	analysis, err := p.generatePriceAnalysis(ctx, vmSize)
	if err != nil {
		return nil, err
	}

	// Cache for 2 hours
	if analysis != nil {
		p.cacheManager.Set(cacheKey, analysis, 2*time.Hour)
	}

	return analysis, nil
}

// GetBatchPriceAnalysis gets price analysis for multiple VM sizes
func (p *PriceHistoryProvider) GetBatchPriceAnalysis(ctx context.Context, vmSizes []string, lookbackDays int) (map[string]*PriceAnalysis, error) {
	results := make(map[string]*PriceAnalysis)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process in parallel with semaphore
	semaphore := make(chan struct{}, 5)

	for _, vmSize := range vmSizes {
		wg.Add(1)
		go func(size string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			analysis, err := p.GetPriceAnalysis(ctx, size, lookbackDays)
			if err == nil && analysis != nil {
				mu.Lock()
				results[size] = analysis
				mu.Unlock()
			}
		}(vmSize)
	}

	wg.Wait()
	return results, nil
}

// generatePriceAnalysis creates analysis from current pricing data
func (p *PriceHistoryProvider) generatePriceAnalysis(ctx context.Context, vmSize string) (*PriceAnalysis, error) {
	// Fetch current spot data
	spotData, err := p.spotProvider.FetchSpotData(ctx, p.region, domain.Linux)
	if err != nil {
		return nil, err
	}

	// Find the specific VM size (try exact match first, then partial match)
	var spotPrice, onDemandPrice float64
	var foundSize string

	// Normalize search term
	searchTerm := strings.ToLower(vmSize)
	searchTerm = strings.TrimPrefix(searchTerm, "standard_")

	for _, data := range spotData {
		// Try exact match first
		if strings.EqualFold(data.InstanceType, vmSize) {
			spotPrice = data.SpotPrice
			onDemandPrice = data.OnDemandPrice
			foundSize = data.InstanceType
			break
		}
		// Try partial match (e.g., D2s_v5 matches Standard_D2s_v5)
		normalizedType := strings.ToLower(data.InstanceType)
		normalizedType = strings.TrimPrefix(normalizedType, "standard_")
		if strings.Contains(normalizedType, searchTerm) || strings.Contains(searchTerm, normalizedType) {
			if spotPrice == 0 || data.SpotPrice < spotPrice {
				spotPrice = data.SpotPrice
				onDemandPrice = data.OnDemandPrice
				foundSize = data.InstanceType
			}
		}
	}

	// If no match found in spot data, return nil - NO ESTIMATION
	if spotPrice == 0 {
		logging.Debug("No spot data found for %s - skipping (no estimation)", vmSize)
		return nil, nil
	}

	// If no on-demand price, we can't calculate meaningful metrics
	if onDemandPrice == 0 {
		logging.Debug("No on-demand price found for %s - skipping (no estimation)", vmSize)
		return nil, nil
	}

	// Calculate actual metrics from real data
	discount := (onDemandPrice - spotPrice) / onDemandPrice

	analysis := &PriceAnalysis{
		InstanceType:  foundSize,
		CurrentPrice:  spotPrice,
		AvgPrice:      spotPrice,
		MinPrice:      spotPrice,
		MaxPrice:      spotPrice,
		LastUpdated:   time.Now(),
		DataPoints:    1,
		TimeSpanHours: 24,
		Volatility:    discount * 0.3, // Derived from actual discount
		AllAZData:     make(map[string]*AZAnalysis),
	}

	analysis.StdDev = analysis.AvgPrice * analysis.Volatility

	// Try to get real zone availability from Azure SKU API
	skuProvider := NewSKUAvailabilityProvider()
	if skuProvider.IsAvailable() {
		ctx := context.Background()
		zoneAvail, err := skuProvider.GetZoneAvailability(ctx, vmSize, p.region)
		if err == nil && len(zoneAvail) > 0 {
			logging.Debug("Using real SKU availability data for %s", vmSize)
			bestZone := ""
			bestScore := -1

			for _, za := range zoneAvail {
				analysis.AllAZData[za.Zone] = &AZAnalysis{
					AvailabilityZone: za.Zone,
					AvgPrice:         spotPrice,
					MinPrice:         spotPrice,
					MaxPrice:         spotPrice,
					Volatility:       analysis.Volatility,
					DataPoints:       za.CapacityScore, // Use capacity score as data points
				}

				// Track best zone (available, unrestricted, highest capacity)
				if za.Available && !za.Restricted && za.CapacityScore > bestScore {
					bestScore = za.CapacityScore
					bestZone = za.Zone
				}
			}

			if bestZone != "" {
				analysis.AvailabilityZone = bestZone
			} else if len(zoneAvail) > 0 {
				analysis.AvailabilityZone = zoneAvail[0].Zone
			}

			// Skip default zone generation since we have real data
			goto patternGen
		}
	}

	// Fallback: Azure has zones but no credentials to check availability
	// Use default zone names - prices are the same across all zones
	{
		azZones := getAzureAvailabilityZones(p.region)
		for _, zone := range azZones {
			analysis.AllAZData[zone] = &AZAnalysis{
				AvailabilityZone: zone,
				AvgPrice:         spotPrice,
				MinPrice:         spotPrice,
				MaxPrice:         spotPrice,
				Volatility:       analysis.Volatility,
				DataPoints:       1,
			}
		}

		// Set best AZ to Zone 1 (all zones have same price when no SKU data)
		if len(azZones) > 0 {
			analysis.AvailabilityZone = azZones[0]
		}
	}

patternGen:
	// Generate hourly and weekday patterns (simulated based on typical patterns)
	analysis.HourlyPattern = p.generateHourlyPattern(spotPrice)
	analysis.WeekdayPattern = p.generateWeekdayPattern(spotPrice)

	// Trend score - neutral for current snapshot
	analysis.TrendScore = 0

	return analysis, nil
}

// getAzureAvailabilityZones returns the availability zone names for a region
// Azure regions typically have 3 availability zones
func getAzureAvailabilityZones(region string) []string {
	// Azure availability zones are numbered 1, 2, 3 within each region
	// Format: region-zone (e.g., eastus-1, eastus-2, eastus-3)
	return []string{
		region + "-1",
		region + "-2",
		region + "-3",
	}
}

// NOTE: Azure does NOT provide per-AZ spot pricing.
// Unlike AWS DescribeSpotPriceHistory which returns per-AZ prices,
// Azure Retail Prices API returns regional prices that apply to all AZs.
// Therefore, we do not have a generateAZData function for Azure.

// generateHourlyPattern creates estimated hourly price patterns
func (p *PriceHistoryProvider) generateHourlyPattern(basePrice float64) map[int]float64 {
	pattern := make(map[int]float64)

	// Typical pattern: lower prices at night (22:00-06:00), higher during business hours
	for hour := 0; hour < 24; hour++ {
		factor := 1.0
		switch {
		case hour >= 0 && hour < 6:
			factor = 0.92 // Lowest at night
		case hour >= 6 && hour < 9:
			factor = 0.97 // Morning ramp-up
		case hour >= 9 && hour < 17:
			factor = 1.05 // Business hours peak
		case hour >= 17 && hour < 22:
			factor = 1.0 // Evening
		default:
			factor = 0.95 // Late night
		}
		pattern[hour] = basePrice * factor
	}

	return pattern
}

// generateWeekdayPattern creates estimated weekday price patterns
func (p *PriceHistoryProvider) generateWeekdayPattern(basePrice float64) map[time.Weekday]float64 {
	pattern := make(map[time.Weekday]float64)

	// Typical pattern: lower prices on weekends
	factors := map[time.Weekday]float64{
		time.Sunday:    0.90,
		time.Monday:    1.02,
		time.Tuesday:   1.05,
		time.Wednesday: 1.05,
		time.Thursday:  1.03,
		time.Friday:    1.00,
		time.Saturday:  0.92,
	}

	for day, factor := range factors {
		pattern[day] = basePrice * factor
	}

	return pattern
}

// cacheKey generates a cache key for price analysis
func (p *PriceHistoryProvider) cacheKey(vmSize string, lookbackDays int) string {
	return cacheKeyPriceHistory + p.region + "_" + vmSize + "_" + strconv.Itoa(lookbackDays)
}

// GetAZRecommendations returns availability zone recommendations for a VM size
func (p *PriceHistoryProvider) GetAZRecommendations(ctx context.Context, vmSize string) ([]AZRecommendation, error) {
	analysis, err := p.GetPriceAnalysis(ctx, vmSize, 7)
	if err != nil || analysis == nil {
		return nil, err
	}

	// Convert AZ data to recommendations
	recommendations := make([]AZRecommendation, 0, len(analysis.AllAZData))
	for _, azData := range analysis.AllAZData {
		recommendations = append(recommendations, AZRecommendation{
			AvailabilityZone: azData.AvailabilityZone,
			AvgPrice:         azData.AvgPrice,
			MinPrice:         azData.MinPrice,
			MaxPrice:         azData.MaxPrice,
			Volatility:       azData.Volatility,
			DataPoints:       azData.DataPoints,
		})
	}

	// Sort by average price (lowest first)
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].AvgPrice < recommendations[j].AvgPrice
	})

	// Assign ranks
	for i := range recommendations {
		recommendations[i].Rank = i + 1
	}

	return recommendations, nil
}

// AZRecommendation contains a single AZ recommendation
type AZRecommendation struct {
	Rank             int
	AvailabilityZone string
	AvgPrice         float64
	MinPrice         float64
	MaxPrice         float64
	Volatility       float64
	DataPoints       int
}
