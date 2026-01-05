// Package gcp implements GCP price history analysis.
// This provides pricing data and analysis for GCP Spot VMs (Preemptible VMs).
package gcp

import (
	"context"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

// Cache key prefix for price history
const cacheKeyPriceHistory = "gcp:price_history:"

// PriceHistoryProvider provides historical price analysis for GCP Spot VMs
type PriceHistoryProvider struct {
	spotProvider  *SpotDataProvider
	billingClient *BillingCatalogClient
	region        string
	cacheManager  *provider.CacheManager
	mu            sync.RWMutex
}

// PriceAnalysis contains computed metrics from price data
type PriceAnalysis struct {
	InstanceType     string
	Zone             string
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
	AllZoneData      map[string]*ZoneAnalysis // Zone-specific data
	UsingRealSKUData bool                     // True if real cloud SKU API data was used
}

// ZoneAnalysis contains per-zone price analysis
type ZoneAnalysis struct {
	Zone       string
	AvgPrice   float64
	MinPrice   float64
	MaxPrice   float64
	Volatility float64
	DataPoints int
}

// NewPriceHistoryProvider creates a new GCP price history provider
func NewPriceHistoryProvider(region string) *PriceHistoryProvider {
	return &PriceHistoryProvider{
		spotProvider:  NewSpotDataProvider(),
		region:        region,
		cacheManager:  provider.GetCacheManager(),
		billingClient: NewBillingCatalogClient(),
	}
}

// IsAvailable returns true - GCP price data is available via spot provider
func (p *PriceHistoryProvider) IsAvailable() bool {
	return true
}

// HasRealPricingData returns true if real Billing Catalog API data is available
func (p *PriceHistoryProvider) HasRealPricingData() bool {
	return p.billingClient != nil && p.billingClient.IsAvailable()
}

// GetProviderName returns the cloud provider
func (p *PriceHistoryProvider) GetProviderName() domain.CloudProvider {
	return domain.GCP
}

// GetPriceAnalysis gets price analysis for a specific machine type
func (p *PriceHistoryProvider) GetPriceAnalysis(ctx context.Context, machineType string, lookbackDays int) (*PriceAnalysis, error) {
	// Check cache first
	cacheKey := p.cacheKey(machineType, lookbackDays)
	if cached, exists := p.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache hit for GCP price analysis: %s", machineType)
		return cached.(*PriceAnalysis), nil
	}

	logging.Debug("Generating price analysis for GCP %s in %s", machineType, p.region)

	// Generate analysis based on current pricing
	analysis, err := p.generatePriceAnalysis(ctx, machineType)
	if err != nil {
		return nil, err
	}

	// Cache for 2 hours
	if analysis != nil {
		p.cacheManager.Set(cacheKey, analysis, 2*time.Hour)
	}

	return analysis, nil
}

// GetBatchPriceAnalysis gets price analysis for multiple machine types
func (p *PriceHistoryProvider) GetBatchPriceAnalysis(ctx context.Context, machineTypes []string, lookbackDays int) (map[string]*PriceAnalysis, error) {
	results := make(map[string]*PriceAnalysis)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process in parallel with semaphore
	semaphore := make(chan struct{}, 5)

	for _, machineType := range machineTypes {
		wg.Add(1)
		go func(mt string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			analysis, err := p.GetPriceAnalysis(ctx, mt, lookbackDays)
			if err == nil && analysis != nil {
				mu.Lock()
				results[mt] = analysis
				mu.Unlock()
			}
		}(machineType)
	}

	wg.Wait()
	return results, nil
}

// generatePriceAnalysis creates analysis from current pricing data
func (p *PriceHistoryProvider) generatePriceAnalysis(ctx context.Context, machineType string) (*PriceAnalysis, error) {
	// Fetch current spot data
	spotData, err := p.spotProvider.FetchSpotData(ctx, p.region, domain.Linux)
	if err != nil {
		return nil, err
	}

	// Find the specific machine type
	var currentSpot *domain.SpotData
	for i := range spotData {
		if spotData[i].InstanceType == machineType {
			currentSpot = &spotData[i]
			break
		}
	}

	if currentSpot == nil {
		// Try to generate for unknown machine type
		specsProvider := NewInstanceSpecsProvider()
		specs, err := specsProvider.GetInstanceSpecs(ctx, machineType)
		if err != nil {
			return nil, domain.NewInstanceSpecsError(machineType, domain.ErrNotFound)
		}

		// Calculate spot data for this machine type
		spotProvider := NewSpotDataProvider()
		calculatedSpot := spotProvider.calculateSpotPricing(*specs, p.region, domain.Linux)
		currentSpot = calculatedSpot
	}

	if currentSpot == nil {
		return nil, domain.NewInstanceSpecsError(machineType, domain.ErrNotFound)
	}

	// Generate analysis
	// GCP doesn't provide historical pricing, so we estimate volatility
	family := extractFamily(machineType)
	volatility := p.estimateVolatility(family, p.region)

	analysis := &PriceAnalysis{
		InstanceType:  machineType,
		Zone:          p.region,
		CurrentPrice:  currentSpot.SpotPrice,
		AvgPrice:      currentSpot.SpotPrice,
		MinPrice:      currentSpot.SpotPrice * 0.95, // Estimate 5% variance
		MaxPrice:      currentSpot.SpotPrice * 1.05,
		StdDev:        currentSpot.SpotPrice * volatility,
		Volatility:    volatility,
		TrendSlope:    0, // GCP prices are relatively stable
		TrendScore:    0,
		DataPoints:    1,
		TimeSpanHours: 0,
		LastUpdated:   time.Now(),
		AllZoneData:   p.generateZoneData(currentSpot.SpotPrice, volatility),
	}

	return analysis, nil
}

// estimateVolatility estimates price volatility for a machine family
func (p *PriceHistoryProvider) estimateVolatility(family, region string) float64 {
	// GCP Spot VM prices are generally more stable than AWS spot prices
	// Base volatility
	baseVolatility := 0.05 // 5%

	// Adjust by family
	switch family {
	case "e2", "n2":
		baseVolatility = 0.04 // Common instances, stable
	case "c2", "c2d", "c3":
		baseVolatility = 0.06 // Compute instances
	case "m2", "m3":
		baseVolatility = 0.05 // Memory instances
	case "a2", "a3", "g2":
		baseVolatility = 0.10 // GPU instances, higher volatility
	case "t2a", "t2d":
		baseVolatility = 0.03 // Tau instances, very stable
	}

	// Adjust by region popularity
	if isHighDemandRegion(region) {
		baseVolatility *= 1.2
	}

	return baseVolatility
}

// generateZoneData generates per-zone price data
func (p *PriceHistoryProvider) generateZoneData(basePrice, volatility float64) map[string]*ZoneAnalysis {
	zones := getZonesForRegion(p.region)
	zoneData := make(map[string]*ZoneAnalysis)

	for i, zone := range zones {
		// Slight price variance between zones
		zoneMultiplier := 1.0 + (float64(i) * 0.01) // 1% increment per zone
		zonePrice := basePrice * zoneMultiplier

		zoneData[zone] = &ZoneAnalysis{
			Zone:       zone,
			AvgPrice:   zonePrice,
			MinPrice:   zonePrice * 0.95,
			MaxPrice:   zonePrice * 1.05,
			Volatility: volatility,
			DataPoints: 1,
		}
	}

	return zoneData
}

// getZonesForRegion returns zones for a GCP region
func getZonesForRegion(region string) []string {
	// Most GCP regions have 3 zones (a, b, c)
	// Some have fewer or more
	zoneMap := map[string][]string{
		"us-central1":          {"us-central1-a", "us-central1-b", "us-central1-c", "us-central1-f"},
		"us-east1":             {"us-east1-b", "us-east1-c", "us-east1-d"},
		"us-east4":             {"us-east4-a", "us-east4-b", "us-east4-c"},
		"us-west1":             {"us-west1-a", "us-west1-b", "us-west1-c"},
		"us-west2":             {"us-west2-a", "us-west2-b", "us-west2-c"},
		"us-west3":             {"us-west3-a", "us-west3-b", "us-west3-c"},
		"us-west4":             {"us-west4-a", "us-west4-b", "us-west4-c"},
		"europe-west1":         {"europe-west1-b", "europe-west1-c", "europe-west1-d"},
		"europe-west2":         {"europe-west2-a", "europe-west2-b", "europe-west2-c"},
		"europe-west3":         {"europe-west3-a", "europe-west3-b", "europe-west3-c"},
		"europe-west4":         {"europe-west4-a", "europe-west4-b", "europe-west4-c"},
		"europe-north1":        {"europe-north1-a", "europe-north1-b", "europe-north1-c"},
		"asia-east1":           {"asia-east1-a", "asia-east1-b", "asia-east1-c"},
		"asia-east2":           {"asia-east2-a", "asia-east2-b", "asia-east2-c"},
		"asia-northeast1":      {"asia-northeast1-a", "asia-northeast1-b", "asia-northeast1-c"},
		"asia-northeast2":      {"asia-northeast2-a", "asia-northeast2-b", "asia-northeast2-c"},
		"asia-northeast3":      {"asia-northeast3-a", "asia-northeast3-b", "asia-northeast3-c"},
		"asia-south1":          {"asia-south1-a", "asia-south1-b", "asia-south1-c"},
		"asia-southeast1":      {"asia-southeast1-a", "asia-southeast1-b", "asia-southeast1-c"},
		"asia-southeast2":      {"asia-southeast2-a", "asia-southeast2-b", "asia-southeast2-c"},
		"australia-southeast1": {"australia-southeast1-a", "australia-southeast1-b", "australia-southeast1-c"},
		"australia-southeast2": {"australia-southeast2-a", "australia-southeast2-b", "australia-southeast2-c"},
	}

	if zones, ok := zoneMap[region]; ok {
		return zones
	}

	// Default: generate a, b, c zones
	return []string{region + "-a", region + "-b", region + "-c"}
}

// cacheKey generates a cache key for price analysis
func (p *PriceHistoryProvider) cacheKey(machineType string, lookbackDays int) string {
	return cacheKeyPriceHistory + p.region + ":" + machineType
}

// GetPriceHistory returns price history points (limited for GCP)
func (p *PriceHistoryProvider) GetPriceHistory(ctx context.Context, vmType, region string) ([]domain.PricePoint, error) {
	// GCP doesn't provide historical spot pricing
	// Return current price as a single data point
	analysis, err := p.GetPriceAnalysis(ctx, vmType, 1)
	if err != nil {
		return nil, err
	}

	return []domain.PricePoint{
		{
			Timestamp: time.Now().Unix(),
			Price:     analysis.CurrentPrice,
			SpotPrice: analysis.CurrentPrice,
			OnDemand:  analysis.CurrentPrice / (1 - 0.7), // Estimate on-demand (70% discount)
		},
	}, nil
}

// GetCurrentPrice returns current spot price
func (p *PriceHistoryProvider) GetCurrentPrice(ctx context.Context, vmType, region, zone string) (float64, error) {
	analysis, err := p.GetPriceAnalysis(ctx, vmType, 1)
	if err != nil {
		return 0, err
	}
	return analysis.CurrentPrice, nil
}

// HasPerZonePricing returns true - GCP has per-zone pricing
func (p *PriceHistoryProvider) HasPerZonePricing() bool {
	return true
}
