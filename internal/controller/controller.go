// Package controller provides programmatic API access to spot analysis.
// This package exposes the same functionality as the web API but for
// direct Go code integration.
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/config"
	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
	awsprovider "github.com/spot-analyzer/internal/provider/aws"
	azureprovider "github.com/spot-analyzer/internal/provider/azure"
	gcpprovider "github.com/spot-analyzer/internal/provider/gcp"
)

// Controller provides programmatic access to spot analysis APIs
type Controller struct {
	cfg    *config.Config
	logger *logging.Logger
}

// New creates a new Controller instance
func New() *Controller {
	logger, err := logging.New(logging.Config{
		Level:       logging.INFO,
		LogDir:      config.Get().Logging.LogDir,
		EnableFile:  config.Get().Logging.EnableFile,
		EnableJSON:  config.Get().Logging.EnableJSON,
		EnableColor: config.Get().Logging.EnableColor,
		Component:   "controller",
		Version:     "1.0.0",
	})
	if err != nil || logger == nil {
		// Fallback to default logger
		logger = logging.GetDefault()
	}
	return &Controller{
		cfg:    config.Get(),
		logger: logger,
	}
}

// AnalyzeRequest represents a spot analysis request
type AnalyzeRequest struct {
	CloudProvider   string   `json:"cloudProvider,omitempty"` // aws, azure
	MinVCPU         int      `json:"minVcpu"`
	MaxVCPU         int      `json:"maxVcpu"`
	MinMemory       int      `json:"minMemory"`
	MaxMemory       int      `json:"maxMemory"`
	Architecture    string   `json:"architecture"` // x86_64, arm64, intel, amd
	Region          string   `json:"region"`
	MaxInterruption int      `json:"maxInterruption"`
	UseCase         string   `json:"useCase"`
	Enhanced        bool     `json:"enhanced"`
	TopN            int      `json:"topN"`
	Families        []string `json:"families,omitempty"`       // Filter by instance families (t, m, c, r, etc.)
	AllowBurstable  *bool    `json:"allowBurstable,omitempty"` // Include burstable instances (t-family), nil = use config default
	RefreshCache    bool     `json:"refreshCache,omitempty"`
}

// AnalyzeResponse represents the analysis result
type AnalyzeResponse struct {
	Success    bool             `json:"success"`
	Instances  []InstanceResult `json:"instances"`
	Summary    string           `json:"summary"`
	Insights   []string         `json:"insights"`
	DataSource string           `json:"dataSource"`
	CachedData bool             `json:"cachedData"`
	AnalyzedAt string           `json:"analyzedAt"`
	Error      string           `json:"error,omitempty"`
}

// InstanceResult represents a single instance recommendation
type InstanceResult struct {
	Rank              int     `json:"rank"`
	InstanceType      string  `json:"instanceType"`
	VCPU              int     `json:"vcpu"`
	MemoryGB          float64 `json:"memoryGb"`
	SavingsPercent    int     `json:"savingsPercent"`
	InterruptionLevel string  `json:"interruptionLevel"`
	Score             float64 `json:"score"`
	Architecture      string  `json:"architecture"`
	Family            string  `json:"family"`
	HourlyPrice       string  `json:"hourlyPrice,omitempty"`
}

// AZRequest represents an AZ recommendation request
type AZRequest struct {
	CloudProvider string `json:"cloudProvider,omitempty"` // aws, azure
	InstanceType  string `json:"instanceType"`
	Region        string `json:"region"`
	RefreshCache  bool   `json:"refreshCache,omitempty"`
}

// AZResponse represents AZ recommendations
type AZResponse struct {
	Success           bool               `json:"success"`
	InstanceType      string             `json:"instanceType"`
	Region            string             `json:"region"`
	Recommendations   []AZRecommendation `json:"recommendations"`
	Insights          []string           `json:"insights"`
	PriceDifferential float64            `json:"priceDifferential"`
	BestAZ            string             `json:"bestAz"`
	NextBestAZ        string             `json:"nextBestAz,omitempty"`
	UsingRealData     bool               `json:"usingRealData"`
	Error             string             `json:"error,omitempty"`
}

// AZRecommendation represents a single AZ recommendation
type AZRecommendation struct {
	Rank             int     `json:"rank"`
	AvailabilityZone string  `json:"availabilityZone"`
	AvgPrice         float64 `json:"avgPrice"`
	MinPrice         float64 `json:"minPrice"`
	MaxPrice         float64 `json:"maxPrice"`
	CurrentPrice     float64 `json:"currentPrice"`
	Volatility       float64 `json:"volatility"`
	Stability        string  `json:"stability"`
	CombinedScore    float64 `json:"combinedScore"` // 0-100 overall score
	CapacityScore    float64 `json:"capacityScore"` // 0-100 capacity score
}

// CacheStatus represents cache statistics
type CacheStatus struct {
	Items    int     `json:"items"`
	Hits     int64   `json:"hits"`
	Misses   int64   `json:"misses"`
	TTLHours float64 `json:"ttlHours"`
}

// Analyze performs spot instance analysis
func (c *Controller) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResponse, error) {
	startTime := time.Now()

	// Determine cloud provider
	cloudProvider := domain.ParseCloudProvider(req.CloudProvider)

	c.logger.Info("Starting analysis: cloud=%s region=%s, minVcpu=%d, enhanced=%v", cloudProvider, req.Region, req.MinVCPU, req.Enhanced)

	// Handle cache refresh
	if req.RefreshCache {
		c.logger.Info("Force refreshing cache")
		provider.GetCacheManager().Clear()
	}

	// Set defaults based on cloud provider
	if req.Region == "" {
		req.Region = cloudProvider.DefaultRegion()
		// Allow config override if set
		if cloudProvider == domain.Azure && c.cfg.Azure.DefaultRegion != "" {
			req.Region = c.cfg.Azure.DefaultRegion
		} else if cloudProvider == domain.AWS && c.cfg.AWS.DefaultRegion != "" {
			req.Region = c.cfg.AWS.DefaultRegion
		}
	}
	if req.TopN == 0 {
		req.TopN = c.cfg.Analysis.DefaultTopN
	}
	if req.MaxInterruption == 0 {
		req.MaxInterruption = c.cfg.Analysis.DefaultMaxInterruption
	}

	// Validate
	if req.MinVCPU <= 0 {
		c.logger.Warn("Invalid minVcpu: %d", req.MinVCPU)
		return nil, fmt.Errorf("minVcpu must be greater than 0")
	}

	// Track cache state before
	cacheManager := provider.GetCacheManager()
	cacheStatsBefore := cacheManager.GetStats()

	// Build requirements
	requirements := domain.UsageRequirements{
		MinVCPU:         req.MinVCPU,
		MaxVCPU:         req.MaxVCPU,
		MinMemoryGB:     float64(req.MinMemory),
		MaxMemoryGB:     float64(req.MaxMemory),
		MaxInterruption: domain.InterruptionFrequency(req.MaxInterruption),
		Region:          req.Region,
		OS:              domain.Linux,
		TopN:            req.TopN,
	}

	// Handle AllowBurstable - use config default if not specified
	if req.AllowBurstable != nil {
		requirements.AllowBurstable = *req.AllowBurstable
	} else {
		requirements.AllowBurstable = c.cfg.Analysis.AllowBurstable
	}
	c.logger.Debug("AllowBurstable=%v (from config: %v)", requirements.AllowBurstable, req.AllowBurstable == nil)

	// Map architecture
	switch req.Architecture {
	case "intel", "amd", "x86_64", "x86":
		requirements.Architecture = "x86_64"
	case "arm", "arm64", "graviton":
		requirements.Architecture = "arm64"
	}

	// Get providers based on cloud provider
	c.logger.Debug("Creating providers for %s region: %s", cloudProvider, req.Region)
	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(cloudProvider)
	if err != nil {
		c.logger.Error("Failed to create spot provider: %v", err)
		return &AnalyzeResponse{Success: false, Error: fmt.Sprintf("Failed to create spot provider: %v", err)}, nil
	}
	specsProvider, err := factory.CreateInstanceSpecsProvider(cloudProvider)
	if err != nil {
		c.logger.Error("Failed to create specs provider: %v", err)
		return &AnalyzeResponse{Success: false, Error: fmt.Sprintf("Failed to create specs provider: %v", err)}, nil
	}

	var result *analyzer.EnhancedAnalysisResult
	var usingRealPriceHistory bool

	if req.Enhanced {
		c.logger.Info("Running enhanced analysis with price history")
		var enhancedAnalyzer *analyzer.EnhancedAnalyzer

		switch cloudProvider {
		case domain.Azure:
			priceProvider := azureprovider.NewPriceHistoryProvider(req.Region)
			usingRealPriceHistory = true
			c.logger.Info("Using Azure Retail Prices API")
			adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)

		default: // AWS
			priceProvider, _ := awsprovider.NewPriceHistoryProvider(req.Region)
			if priceProvider != nil && priceProvider.IsAvailable() {
				usingRealPriceHistory = true
				c.logger.Info("Using real AWS DescribeSpotPriceHistory data")
				adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)
			} else {
				c.logger.Warn("Price history provider not available, falling back to Spot Advisor only")
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzer(spotProvider, specsProvider)
			}
		}
		result, err = enhancedAnalyzer.AnalyzeEnhanced(ctx, requirements)
	} else {
		c.logger.Info("Running basic analysis")
		smartAnalyzer := analyzer.NewSmartAnalyzer(spotProvider, specsProvider)
		basicResult, basicErr := smartAnalyzer.Analyze(ctx, requirements)
		err = basicErr
		if basicResult != nil {
			// Update cloud provider in result
			basicResult.CloudProvider = cloudProvider
			result = &analyzer.EnhancedAnalysisResult{
				AnalysisResult:    basicResult,
				EnhancedInstances: make([]*analyzer.EnhancedRankedInstance, 0),
			}
			for i := range basicResult.TopInstances {
				inst := &basicResult.TopInstances[i]
				result.EnhancedInstances = append(result.EnhancedInstances, &analyzer.EnhancedRankedInstance{
					InstanceAnalysis: inst,
					FinalScore:       inst.Score,
				})
			}
		}
	}

	if err != nil {
		c.logger.Error("Analysis failed: %v", err)
		return &AnalyzeResponse{Success: false, Error: err.Error()}, nil
	}

	// Check cache usage
	cacheStatsAfter := cacheManager.GetStats()
	cachedData := cacheStatsBefore.Items > 0 && cacheStatsAfter.Hits > cacheStatsBefore.Hits

	// Build response
	resp := &AnalyzeResponse{
		Success:    true,
		Instances:  make([]InstanceResult, 0),
		Insights:   make([]string, 0),
		CachedData: cachedData,
		AnalyzedAt: time.Now().Format(time.RFC3339),
	}

	// Set data source based on cloud provider
	switch cloudProvider {
	case domain.Azure:
		if req.Enhanced && usingRealPriceHistory {
			resp.DataSource = "Azure Retail Prices API"
			resp.Insights = append(resp.Insights, "📊 Using real-time Azure Retail Prices API data")
		} else {
			resp.DataSource = "Azure Retail Prices API"
			resp.Insights = append(resp.Insights, "📋 Using Azure Retail Prices API data")
		}
	default: // AWS
		if req.Enhanced && usingRealPriceHistory {
			resp.DataSource = "AWS DescribeSpotPriceHistory + Spot Advisor"
			resp.Insights = append(resp.Insights, "📊 Using real-time AWS DescribeSpotPriceHistory data")
		} else {
			resp.DataSource = "AWS Spot Advisor"
			resp.Insights = append(resp.Insights, "📋 Using AWS Spot Advisor data")
		}
	}

	// Filter and convert instances
	instances := result.EnhancedInstances

	// For Azure, create SKU availability checker to filter out unavailable VMs
	// This is mandatory to ensure we only return VMs that are actually available
	var skuChecker *azureprovider.SKUAvailabilityProvider
	if cloudProvider == domain.Azure {
		skuChecker = azureprovider.NewSKUAvailabilityProvider()
		if !skuChecker.IsAvailable() {
			c.logger.Warn("Azure SKU check unavailable - no credentials configured")
			skuChecker = nil // No credentials, skip availability check
		} else {
			c.logger.Info("Azure SKU availability check enabled (mandatory)")
		}
	}

	count := 0
	notInSKUAPI := 0
	for _, inst := range instances {
		// For Azure, check if VM is in SKU API for zone availability info
		// Note: VMs with spot pricing ARE available even if not in SKU API yet
		// (SKU API often lags behind Retail Prices API for new VM series)
		if skuChecker != nil {
			if !skuChecker.IsVMAvailableInRegion(ctx, inst.Specs.InstanceType, req.Region) {
				// Don't filter out - just track for informational purposes
				// VMs with spot pricing are available, SKU API just doesn't have metadata yet
				notInSKUAPI++
			}
		}

		// Apply family filter if specified - BEFORE checking count
		if len(req.Families) > 0 {
			family := extractFamily(inst.SpotData.InstanceType)
			if !containsFamily(req.Families, family) {
				continue
			}
		}

		// Check count limit AFTER family filter
		if count >= req.TopN {
			break
		}

		count++
		resp.Instances = append(resp.Instances, InstanceResult{
			Rank:              count,
			InstanceType:      inst.SpotData.InstanceType,
			VCPU:              inst.Specs.VCPU,
			MemoryGB:          inst.Specs.MemoryGB,
			SavingsPercent:    inst.SpotData.SavingsPercent,
			InterruptionLevel: inst.SpotData.InterruptionFrequency.String(),
			Score:             inst.FinalScore,
			Architecture:      inst.Specs.Architecture,
			Family:            extractFamily(inst.SpotData.InstanceType),
		})
	}

	// Add insight about VMs not in SKU API (informational only, not filtered)
	if notInSKUAPI > 0 {
		resp.Insights = append(resp.Insights, fmt.Sprintf("ℹ️ %d VMs have spot pricing but lack zone availability data (new VM series)", notInSKUAPI))
		c.logger.WithFields(logging.Fields{
			"not_in_sku_api": notInSKUAPI,
			"region":         req.Region,
		}).Info("VMs with spot pricing not yet in SKU API")
	}

	// Summary
	if len(resp.Instances) > 0 {
		top := resp.Instances[0]
		resp.Summary = fmt.Sprintf("Top recommendation: %s with %d vCPU, %.0fGB RAM, %d%% savings",
			top.InstanceType, top.VCPU, top.MemoryGB, top.SavingsPercent)
		resp.Insights = append(resp.Insights, fmt.Sprintf("💰 Save up to %d%% compared to on-demand", top.SavingsPercent))
		resp.Insights = append(resp.Insights, fmt.Sprintf("⚡ %s interruption rate", top.InterruptionLevel))
	}

	duration := time.Since(startTime)
	c.logger.WithFields(logging.Fields{
		"duration_ms":   duration.Milliseconds(),
		"region":        req.Region,
		"results_count": len(resp.Instances),
		"cached_data":   resp.CachedData,
		"data_source":   resp.DataSource,
	}).Info("Analysis completed: found %d instances in %v", len(resp.Instances), duration)

	return resp, nil
}

// RecommendAZ gets AZ recommendations for an instance type
func (c *Controller) RecommendAZ(ctx context.Context, req AZRequest) (*AZResponse, error) {
	startTime := time.Now()

	// Determine cloud provider
	cloudProvider := domain.ParseCloudProvider(req.CloudProvider)

	c.logger.Info("Starting AZ recommendation: cloud=%s instance=%s, region=%s", cloudProvider, req.InstanceType, req.Region)

	// Handle cache refresh
	if req.RefreshCache {
		c.logger.Info("Clearing price history cache for AZ lookup")
		provider.GetCacheManager().DeletePrefix(cloudProvider.CacheKeyPrefix() + "price_history:")
	}

	// Validate
	if req.InstanceType == "" {
		c.logger.Warn("Missing instanceType in AZ request")
		return nil, fmt.Errorf("instanceType is required")
	}
	if req.Region == "" {
		req.Region = cloudProvider.DefaultRegion()
		// Allow config override
		if cloudProvider == domain.Azure && c.cfg.Azure.DefaultRegion != "" {
			req.Region = c.cfg.Azure.DefaultRegion
		} else if cloudProvider == domain.AWS && c.cfg.AWS.DefaultRegion != "" {
			req.Region = c.cfg.AWS.DefaultRegion
		}
	}

	var predEngine *analyzer.PredictionEngine
	usingRealData := false
	isAzure := false

	switch cloudProvider {
	case domain.Azure:
		priceProvider := azureprovider.NewPriceHistoryProvider(req.Region)
		isAzure = true
		adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
			WithCloudProvider("azure").
			WithZoneProvider(azureprovider.NewZoneProviderAdapter(req.Region)).
			WithCapacityProvider(azureprovider.NewCapacityProviderAdapter(req.Region))

	case domain.GCP:
		priceProvider := gcpprovider.NewPriceHistoryProvider(req.Region)
		adapter := gcpprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
			WithCloudProvider("gcp").
			WithZoneProvider(gcpprovider.NewZoneProviderAdapter(req.Region))
		usingRealData = true // GCP always has pricing data available

	default: // AWS
		priceProvider, err := awsprovider.NewPriceHistoryProvider(req.Region)
		if err != nil {
			return &AZResponse{Success: false, Error: "Failed to create provider"}, nil
		}

		if priceProvider != nil && priceProvider.IsAvailable() {
			usingRealData = true
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
				WithCloudProvider("aws").
				WithZoneProvider(awsprovider.NewZoneProviderAdapter(req.Region)).
				WithCapacityProvider(awsprovider.NewCapacityProviderAdapter(req.Region))
		} else {
			predEngine = analyzer.NewPredictionEngine(nil, req.Region).
				WithCloudProvider("aws").
				WithZoneProvider(awsprovider.NewZoneProviderAdapter(req.Region)).
				WithCapacityProvider(awsprovider.NewCapacityProviderAdapter(req.Region))
		}
	}

	// Use smart AZ selector for recommendations
	smartRec, err := predEngine.SmartRecommendAZ(ctx, req.InstanceType, analyzer.DefaultWeights())
	if err != nil {
		// Fall back to traditional method if smart selector fails
		c.logger.Warn("Smart AZ selector failed, falling back to traditional method: %v", err)
		rec, err := predEngine.RecommendAZ(ctx, req.InstanceType)
		if err != nil {
			return &AZResponse{Success: false, Error: err.Error()}, nil
		}
		return c.buildTraditionalAZResponse(req, rec, usingRealData, isAzure)
	}

	// Use smart recommendation
	usingRealData = smartRec.Confidence > 0.5

	maxAZRecommendations := c.cfg.Analysis.AZRecommendations

	// Calculate price differential from smart results
	priceDifferential := 0.0
	if len(smartRec.Rankings) >= 2 {
		bestPrice := smartRec.Rankings[0].SpotPrice
		worstPrice := smartRec.Rankings[len(smartRec.Rankings)-1].SpotPrice
		if bestPrice > 0 {
			priceDifferential = ((worstPrice - bestPrice) / bestPrice) * 100
		}
	}

	resp := &AZResponse{
		Success:           true,
		InstanceType:      req.InstanceType,
		Region:            req.Region,
		Recommendations:   make([]AZRecommendation, 0),
		Insights:          smartRec.Insights,
		PriceDifferential: priceDifferential,
		UsingRealData:     usingRealData,
		BestAZ:            smartRec.BestAZ,
		NextBestAZ:        smartRec.NextBestAZ,
	}

	for i, rank := range smartRec.Rankings {
		if i >= maxAZRecommendations {
			break
		}

		stability := "Low"
		if rank.Volatility < 0.05 {
			stability = "Very Stable"
		} else if rank.Volatility < 0.1 {
			stability = "Stable"
		} else if rank.Volatility < 0.2 {
			stability = "Moderate"
		} else {
			stability = "High Volatility"
		}

		resp.Recommendations = append(resp.Recommendations, AZRecommendation{
			Rank:             rank.Rank,
			AvailabilityZone: rank.Zone,
			AvgPrice:         rank.SpotPrice,
			MinPrice:         rank.SpotPrice * 0.9, // Estimate
			MaxPrice:         rank.SpotPrice * 1.1, // Estimate
			CurrentPrice:     rank.SpotPrice,
			Volatility:       rank.Volatility,
			Stability:        stability,
			CombinedScore:    rank.CombinedScore,
			CapacityScore:    rank.CapacityScore,
		})
	}

	if !usingRealData {
		resp.Insights = append(resp.Insights, "⚠️ Configure AWS credentials for real-time AZ pricing data")
	}

	duration := time.Since(startTime)
	c.logger.WithFields(map[string]interface{}{
		"duration_ms":   duration.Milliseconds(),
		"instance_type": req.InstanceType,
		"region":        req.Region,
		"az_count":      len(resp.Recommendations),
		"best_az":       resp.BestAZ,
		"using_real":    usingRealData,
	}).Info("AZ recommendation completed")

	return resp, nil
}

// GetCacheStatus returns current cache statistics
func (c *Controller) GetCacheStatus() CacheStatus {
	c.logger.Debug("Getting cache status")
	cache := provider.GetCacheManager()
	stats := cache.GetStats()

	c.logger.WithFields(map[string]interface{}{
		"items":  stats.Items,
		"hits":   stats.Hits,
		"misses": stats.Misses,
	}).Debug("Cache status retrieved")

	return CacheStatus{
		Items:    stats.Items,
		Hits:     stats.Hits,
		Misses:   stats.Misses,
		TTLHours: cache.GetTTL().Hours(),
	}
}

// RefreshCache clears the cache and returns stats
func (c *Controller) RefreshCache() (int, error) {
	c.logger.Info("Refreshing cache")
	cache := provider.GetCacheManager()
	stats := cache.GetStats()
	itemsBefore := stats.Items
	cache.Clear()

	c.logger.WithFields(map[string]interface{}{
		"items_cleared": itemsBefore,
	}).Info("Cache cleared successfully")

	return itemsBefore, nil
}

// GetAvailableFamilies returns available instance families
func (c *Controller) GetAvailableFamilies() []config.InstanceFamily {
	families := c.cfg.InstanceFamilies.Available
	c.logger.WithFields(map[string]interface{}{
		"family_count": len(families),
	}).Debug("Retrieved available instance families")
	return families
}

// Helper functions
func extractFamily(instanceType string) string {
	// Extract family from instance type (e.g., "m5.large" -> "m")
	for i, c := range instanceType {
		if c >= '0' && c <= '9' {
			return instanceType[:i]
		}
	}
	return instanceType
}

func containsFamily(families []string, family string) bool {
	for _, f := range families {
		if f == family {
			return true
		}
	}
	return false
}

// buildTraditionalAZResponse creates an AZ response from the traditional RecommendAZ result
func (c *Controller) buildTraditionalAZResponse(req AZRequest, rec *analyzer.AZRecommendation, usingRealData, isAzure bool) (*AZResponse, error) {
	// For Azure, use the UsingRealSKUData flag from the recommendation
	if isAzure {
		usingRealData = rec.UsingRealSKUData
	}

	maxAZRecommendations := c.cfg.Analysis.AZRecommendations

	resp := &AZResponse{
		Success:           true,
		InstanceType:      req.InstanceType,
		Region:            req.Region,
		Recommendations:   make([]AZRecommendation, 0),
		Insights:          rec.Insights,
		PriceDifferential: rec.PriceDifferential,
		UsingRealData:     usingRealData,
		BestAZ:            rec.BestAZ,
		NextBestAZ:        rec.WorstAZ, // Use worst as fallback
	}

	for i, az := range rec.Recommendations {
		if i >= maxAZRecommendations {
			break
		}

		stability := "Low"
		if az.Volatility < 0.05 {
			stability = "Very Stable"
		} else if az.Volatility < 0.1 {
			stability = "Stable"
		} else if az.Volatility < 0.2 {
			stability = "Moderate"
		} else {
			stability = "High Volatility"
		}

		resp.Recommendations = append(resp.Recommendations, AZRecommendation{
			Rank:             az.Rank,
			AvailabilityZone: az.AvailabilityZone,
			AvgPrice:         az.AvgPrice,
			MinPrice:         az.MinPrice,
			MaxPrice:         az.MaxPrice,
			CurrentPrice:     az.AvgPrice,
			Volatility:       az.Volatility,
			Stability:        stability,
			CombinedScore:    az.Score * 100, // Convert 0-1 to 0-100
			CapacityScore:    50,             // Unknown in traditional method
		})

		if az.Rank == 1 {
			resp.BestAZ = az.AvailabilityZone
		} else if az.Rank == 2 {
			resp.NextBestAZ = az.AvailabilityZone
		}
	}

	if !usingRealData {
		resp.Insights = append(resp.Insights, "⚠️ Configure cloud credentials for real-time AZ pricing data")
	}

	return resp, nil
}
