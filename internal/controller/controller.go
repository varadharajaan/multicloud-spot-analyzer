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
)

// Controller provides programmatic access to spot analysis APIs
type Controller struct {
	cfg    *config.Config
	logger *logging.Logger
}

// New creates a new Controller instance
func New() *Controller {
	logger, _ := logging.New(logging.Config{
		Level:       logging.INFO,
		LogDir:      config.Get().Logging.LogDir,
		EnableFile:  config.Get().Logging.EnableFile,
		EnableJSON:  config.Get().Logging.EnableJSON,
		EnableColor: config.Get().Logging.EnableColor,
		Component:   "controller",
		Version:     "1.0.0",
	})
	return &Controller{
		cfg:    config.Get(),
		logger: logger,
	}
}

// AnalyzeRequest represents a spot analysis request
type AnalyzeRequest struct {
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
	InstanceType string `json:"instanceType"`
	Region       string `json:"region"`
	RefreshCache bool   `json:"refreshCache,omitempty"`
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
	c.logger.Info("Starting analysis: region=%s, minVcpu=%d, enhanced=%v", req.Region, req.MinVCPU, req.Enhanced)

	// Handle cache refresh
	if req.RefreshCache {
		c.logger.Info("Force refreshing cache")
		provider.GetCacheManager().Clear()
	}

	// Set defaults
	if req.Region == "" {
		req.Region = c.cfg.AWS.DefaultRegion
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

	// Get providers
	c.logger.Debug("Creating providers for region: %s", req.Region)
	factory := provider.GetFactory()
	spotProvider, _ := factory.CreateSpotDataProvider(domain.AWS)
	specsProvider, _ := factory.CreateInstanceSpecsProvider(domain.AWS)

	var result *analyzer.EnhancedAnalysisResult
	var err error
	var usingRealPriceHistory bool

	if req.Enhanced {
		c.logger.Info("Running enhanced analysis with price history")
		priceProvider, _ := awsprovider.NewPriceHistoryProvider(req.Region)
		var enhancedAnalyzer *analyzer.EnhancedAnalyzer
		if priceProvider != nil && priceProvider.IsAvailable() {
			usingRealPriceHistory = true
			c.logger.Info("Using real AWS DescribeSpotPriceHistory data")
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)
		} else {
			c.logger.Warn("Price history provider not available, falling back to Spot Advisor only")
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzer(spotProvider, specsProvider)
		}
		result, err = enhancedAnalyzer.AnalyzeEnhanced(ctx, requirements)
	} else {
		c.logger.Info("Running basic analysis (Spot Advisor only)")
		smartAnalyzer := analyzer.NewSmartAnalyzer(spotProvider, specsProvider)
		basicResult, basicErr := smartAnalyzer.Analyze(ctx, requirements)
		err = basicErr
		if basicResult != nil {
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

	// Set data source
	if req.Enhanced && usingRealPriceHistory {
		resp.DataSource = "AWS DescribeSpotPriceHistory + Spot Advisor"
		resp.Insights = append(resp.Insights, "📊 Using real-time AWS DescribeSpotPriceHistory data")
	} else {
		resp.DataSource = "AWS Spot Advisor"
		resp.Insights = append(resp.Insights, "📋 Using AWS Spot Advisor data")
	}

	// Filter and convert instances
	instances := result.EnhancedInstances
	count := 0
	for _, inst := range instances {
		if count >= req.TopN {
			break
		}

		// Apply family filter if specified
		if len(req.Families) > 0 {
			family := extractFamily(inst.SpotData.InstanceType)
			if !containsFamily(req.Families, family) {
				continue
			}
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
	c.logger.Info("Starting AZ recommendation: instance=%s, region=%s", req.InstanceType, req.Region)

	// Handle cache refresh
	if req.RefreshCache {
		c.logger.Info("Clearing price history cache for AZ lookup")
		provider.GetCacheManager().DeletePrefix("aws:price_history:")
	}

	// Validate
	if req.InstanceType == "" {
		c.logger.Warn("Missing instanceType in AZ request")
		return nil, fmt.Errorf("instanceType is required")
	}
	if req.Region == "" {
		req.Region = c.cfg.AWS.DefaultRegion
	}

	// Create price history provider
	priceProvider, err := awsprovider.NewPriceHistoryProvider(req.Region)
	if err != nil {
		return &AZResponse{Success: false, Error: "Failed to create provider"}, nil
	}

	var predEngine *analyzer.PredictionEngine
	usingRealData := false

	if priceProvider != nil && priceProvider.IsAvailable() {
		usingRealData = true
		adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region)
	} else {
		predEngine = analyzer.NewPredictionEngine(nil, req.Region)
	}

	rec, err := predEngine.RecommendAZ(ctx, req.InstanceType)
	if err != nil {
		return &AZResponse{Success: false, Error: err.Error()}, nil
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
		})

		if az.Rank == 1 {
			resp.BestAZ = az.AvailabilityZone
		} else if az.Rank == 2 {
			resp.NextBestAZ = az.AvailabilityZone
		}
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
