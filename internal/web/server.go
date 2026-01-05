package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
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

//go:embed static/*
var staticFiles embed.FS

// GetStaticFS returns the embedded static file system for use by Lambda handler
func GetStaticFS() fs.FS {
	return staticFiles
}

// Server represents the web UI server
type Server struct {
	port        int
	logger      *logging.Logger
	cfg         *config.Config
	rateLimiter *RateLimiter
	startTime   time.Time
}

// NewServer creates a new web server
func NewServer(port int) *Server {
	cfg := config.Get()

	logger, _ := logging.New(logging.Config{
		Level:       logging.INFO,
		LogDir:      "logs",
		EnableFile:  true,
		EnableJSON:  true, // Enable JSON logging for Athena/BigQuery
		EnableColor: true,
		Component:   "web",
		Version:     "1.0.0",
	})
	// Rate limit: 100 requests per minute per IP
	rateLimiter := NewRateLimiter(100, time.Minute)
	return &Server{port: port, logger: logger, cfg: cfg, rateLimiter: rateLimiter, startTime: time.Now()}
}

// Start starts the web server
func (s *Server) Start() error {
	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		s.logger.Error("Failed to load static files: %v", err)
		return err
	}

	// Create file server for static assets
	fileServer := http.FileServer(http.FS(staticFS))

	// Handle root path with UI version redirect
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Add no-cache headers for HTML files to prevent stale redirects
		if strings.HasSuffix(r.URL.Path, ".html") || r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		// Only redirect root path "/" to v2, NOT index.html
		if r.URL.Path == "/" {
			uiVersion := s.cfg.UI.Version
			if uiVersion == "v2" {
				http.Redirect(w, r, "/index-v2.html", http.StatusFound)
				return
			}
			// v1 mode - redirect to index.html
			http.Redirect(w, r, "/index.html", http.StatusFound)
			return
		}

		// Serve HTML files directly from embed FS to avoid http.FileServer redirect behavior
		if r.URL.Path == "/index.html" || r.URL.Path == "/index-v2.html" || r.URL.Path == "/swagger.html" {
			filename := strings.TrimPrefix(r.URL.Path, "/")
			content, err := fs.ReadFile(staticFS, filename)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.logger.Info("%s %s %s", r.Method, r.URL.Path, "0ms")
			w.Write(content)
			return
		}

		// Serve other static files normally
		s.logRequest(fileServer).ServeHTTP(w, r)
	})

	http.HandleFunc("/swagger-ui", s.handleSwaggerRedirect)
	http.HandleFunc("/api/health", s.handleHealth)
	http.HandleFunc("/api/analyze", s.rateLimiter.Middleware(s.handleAnalyze))
	http.HandleFunc("/api/az", s.rateLimiter.Middleware(s.handleAZRecommendation))
	http.HandleFunc("/api/smart-az", s.rateLimiter.Middleware(s.handleSmartAZRecommendation))
	http.HandleFunc("/api/parse-requirements", s.handleParseRequirements)
	http.HandleFunc("/api/presets", s.handlePresets)
	http.HandleFunc("/api/families", s.handleFamilies)
	http.HandleFunc("/api/instance-types", s.handleInstanceTypes)
	http.HandleFunc("/api/cache/status", s.handleCacheStatus)
	http.HandleFunc("/api/cache/refresh", s.rateLimiter.Middleware(s.handleCacheRefresh))
	http.HandleFunc("/api/openapi.json", s.handleOpenAPI)

	addr := fmt.Sprintf(":%d", s.port)
	uiVersion := s.cfg.UI.Version
	s.logger.Info("Starting web UI (version %s) at http://localhost%s", uiVersion, addr)
	fmt.Printf("🌐 Starting web UI (version %s) at http://localhost%s\n", uiVersion, addr)
	return http.ListenAndServe(addr, nil)
}

// handleSwaggerRedirect redirects /swagger-ui to /swagger.html
func (s *Server) handleSwaggerRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger.html", http.StatusMovedPermanently)
}

// handleHealth returns the health status of the service
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	checks := make(map[string]string)
	status := "healthy"

	// Check cache status
	cacheManager := provider.GetCacheManager()
	if cacheManager != nil {
		cacheStats := cacheManager.GetStats()
		if cacheStats.Items > 0 {
			checks["cache"] = "ok"
		} else {
			checks["cache"] = "empty"
		}
	} else {
		checks["cache"] = "unavailable"
	}

	// Check AWS credentials (optional)
	awsCreds := "not_configured"
	priceProvider, err := awsprovider.NewPriceHistoryProvider("us-east-1")
	if err == nil && priceProvider != nil && priceProvider.IsAvailable() {
		awsCreds = "configured"
	}
	checks["aws_credentials"] = awsCreds

	// Check Azure API availability (always available - public API)
	azureProvider := azureprovider.NewPriceHistoryProvider("eastus")
	if azureProvider != nil && azureProvider.IsAvailable() {
		checks["azure_api"] = "available"
	} else {
		checks["azure_api"] = "unavailable"
	}

	// Check GCP API availability (always available - uses pricing estimates)
	gcpProvider := gcpprovider.NewPriceHistoryProvider("us-central1")
	if gcpProvider != nil && gcpProvider.IsAvailable() {
		checks["gcp_api"] = "available"
	} else {
		checks["gcp_api"] = "unavailable"
	}

	// Uptime check
	uptime := time.Since(s.startTime)
	checks["uptime"] = uptime.Round(time.Second).String()

	resp := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "1.0.0",
		Checks:    checks,
	}

	json.NewEncoder(w).Encode(resp)
}

// logRequest wraps a handler with request logging
func (s *Server) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// AnalyzeRequest represents the API request
type AnalyzeRequest struct {
	CloudProvider   string   `json:"cloudProvider,omitempty"` // aws, azure
	MinVCPU         int      `json:"minVcpu"`
	MaxVCPU         int      `json:"maxVcpu"`
	MinMemory       int      `json:"minMemory"`
	MaxMemory       int      `json:"maxMemory"`
	Architecture    string   `json:"architecture"` // x86_64, arm64, intel, amd
	Region          string   `json:"region"`
	MaxInterruption int      `json:"maxInterruption"`
	UseCase         string   `json:"useCase"` // general, kubernetes, database, asg
	Enhanced        bool     `json:"enhanced"`
	TopN            int      `json:"topN"`
	Families        []string `json:"families,omitempty"` // Filter by instance families (t, m, c, r, etc.)
	RefreshCache    bool     `json:"refreshCache,omitempty"`
}

// AnalyzeResponse represents the API response
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
	Generation        string  `json:"generation"`
	HourlyPrice       string  `json:"hourlyPrice,omitempty"`
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}

	if r.Method != "POST" {
		s.logger.Warn("Invalid method: %s for /api/analyze", r.Method)
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: "Method not allowed"})
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("Failed to decode request: %v", err)
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: "Invalid request"})
		return
	}

	// Determine cloud provider
	cloudProvider := domain.ParseCloudProvider(req.CloudProvider)

	s.logger.Info("Analyze request: cloud=%s region=%s vcpu=%d-%d memory=%d-%d arch=%s useCase=%s enhanced=%v families=%v",
		cloudProvider, req.Region, req.MinVCPU, req.MaxVCPU, req.MinMemory, req.MaxMemory, req.Architecture, req.UseCase, req.Enhanced, req.Families)

	// Handle cache refresh request
	if req.RefreshCache {
		provider.GetCacheManager().Clear()
		s.logger.Info("Cache cleared per request")
	}

	// Set defaults based on cloud provider
	if req.Region == "" {
		req.Region = cloudProvider.DefaultRegion()
	}
	if req.TopN == 0 {
		req.TopN = 10
	}
	if req.MaxInterruption == 0 {
		req.MaxInterruption = 2
	}

	// Map architecture
	arch := ""
	switch strings.ToLower(req.Architecture) {
	case "intel", "amd", "x86_64", "x86":
		arch = "x86_64"
	case "arm", "arm64", "graviton":
		arch = "arm64"
	}

	// Apply use case presets
	requirements := s.applyUseCasePreset(req, arch)

	// Check cache state BEFORE analysis to track if we used cached data
	cacheManager := provider.GetCacheManager()
	cacheStatsBefore := cacheManager.GetStats()
	cachedItemsBefore := cacheStatsBefore.Items

	// Run analysis
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(cloudProvider)
	if err != nil {
		s.logger.Error("Failed to create spot provider: %v", err)
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: fmt.Sprintf("Failed to create provider: %v", err)})
		return
	}
	specsProvider, err := factory.CreateInstanceSpecsProvider(cloudProvider)
	if err != nil {
		s.logger.Error("Failed to create specs provider: %v", err)
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: fmt.Sprintf("Failed to create provider: %v", err)})
		return
	}

	var result *analyzer.EnhancedAnalysisResult
	var usingRealPriceHistory bool

	if req.Enhanced {
		var enhancedAnalyzer *analyzer.EnhancedAnalyzer

		switch cloudProvider {
		case domain.Azure:
			priceProvider := azureprovider.NewPriceHistoryProvider(req.Region)
			usingRealPriceHistory = true
			s.logger.Info("Using Azure Retail Prices API for enhanced analysis")
			adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)

		case domain.GCP:
			priceProvider := gcpprovider.NewPriceHistoryProvider(req.Region)
			usingRealPriceHistory = true
			s.logger.Info("Using GCP pricing data for enhanced analysis")
			adapter := gcpprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)

		default: // AWS
			priceProvider, _ := awsprovider.NewPriceHistoryProvider(req.Region)
			if priceProvider != nil && priceProvider.IsAvailable() {
				usingRealPriceHistory = true
				s.logger.Info("Using real AWS DescribeSpotPriceHistory for enhanced analysis")
				adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)
			} else {
				s.logger.Info("AWS credentials not available, using Spot Advisor data only")
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzer(spotProvider, specsProvider)
			}
		}
		result, err = enhancedAnalyzer.AnalyzeEnhanced(ctx, requirements)
	} else {
		smartAnalyzer := analyzer.NewSmartAnalyzer(spotProvider, specsProvider)
		basicResult, basicErr := smartAnalyzer.Analyze(ctx, requirements)
		err = basicErr
		if basicResult != nil {
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
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: err.Error()})
		return
	}

	// Check cache status - compare before/after to see if we used cached data
	cacheStatsAfter := cacheManager.GetStats()
	// If cache had items before and hits increased, we used cached data
	cachedData := cachedItemsBefore > 0 && cacheStatsAfter.Hits > cacheStatsBefore.Hits

	// Build response
	resp := AnalyzeResponse{
		Success:    true,
		Instances:  make([]InstanceResult, 0),
		Insights:   make([]string, 0),
		CachedData: cachedData,
		AnalyzedAt: time.Now().Format(time.RFC3339),
	}

	// Add data source insight based on cloud provider
	switch cloudProvider {
	case domain.Azure:
		if req.Enhanced && usingRealPriceHistory {
			resp.DataSource = "Azure Retail Prices API"
			resp.Insights = append(resp.Insights, "📊 Using real-time Azure Retail Prices API data")
		} else {
			resp.DataSource = "Azure Retail Prices API"
			resp.Insights = append(resp.Insights, "📋 Using Azure Retail Prices API data")
		}
	case domain.GCP:
		if req.Enhanced && usingRealPriceHistory {
			resp.DataSource = "GCP Pricing API"
			resp.Insights = append(resp.Insights, "📊 Using GCP Spot VM pricing data")
		} else {
			resp.DataSource = "GCP Pricing Estimates"
			resp.Insights = append(resp.Insights, "📋 Using GCP Spot VM pricing estimates")
		}
	default: // AWS
		if req.Enhanced {
			if usingRealPriceHistory {
				resp.DataSource = "AWS DescribeSpotPriceHistory + Spot Advisor"
				resp.Insights = append(resp.Insights, "📊 Using real-time AWS DescribeSpotPriceHistory data")
			} else {
				resp.DataSource = "AWS Spot Advisor"
				resp.Insights = append(resp.Insights, "📋 Using AWS Spot Advisor data (configure AWS credentials for price history)")
			}
		} else {
			resp.DataSource = "AWS Spot Advisor"
			resp.Insights = append(resp.Insights, "📋 Using AWS Spot Advisor data")
		}
	}

	// Use EnhancedInstances if available, otherwise fallback to TopInstances
	instances := result.EnhancedInstances
	if len(instances) == 0 && result.AnalysisResult != nil {
		for i := range result.TopInstances {
			inst := &result.TopInstances[i]
			instances = append(instances, &analyzer.EnhancedRankedInstance{
				InstanceAnalysis: inst,
				FinalScore:       inst.Score,
			})
		}
	}

	// For Azure, create SKU availability checker to filter out unavailable VMs
	var skuChecker *azureprovider.SKUAvailabilityProvider
	if cloudProvider == domain.Azure {
		skuChecker = azureprovider.NewSKUAvailabilityProvider()
		if !skuChecker.IsAvailable() {
			skuChecker = nil // No credentials, skip availability check
		}
	}

	count := 0
	skippedUnavailable := 0
	for _, inst := range instances {
		// For Azure, check if VM is actually available in the region
		if skuChecker != nil {
			if !skuChecker.IsVMAvailableInRegion(ctx, inst.InstanceAnalysis.Specs.InstanceType, req.Region) {
				skippedUnavailable++
				continue // Skip VMs not available in region
			}
		}

		// Apply family filter if specified - BEFORE checking count
		if len(req.Families) > 0 {
			family := extractInstanceFamily(inst.InstanceAnalysis.Specs.InstanceType)
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
			InstanceType:      inst.InstanceAnalysis.Specs.InstanceType,
			VCPU:              inst.InstanceAnalysis.Specs.VCPU,
			MemoryGB:          inst.InstanceAnalysis.Specs.MemoryGB,
			SavingsPercent:    inst.InstanceAnalysis.SpotData.SavingsPercent,
			InterruptionLevel: formatInterruption(inst.InstanceAnalysis.SpotData.InterruptionFrequency),
			Score:             inst.FinalScore,
			Architecture:      inst.InstanceAnalysis.Specs.Architecture,
			Generation:        strconv.Itoa(int(inst.InstanceAnalysis.Specs.Generation)),
		})
	}

	// Add insight about filtered VMs if any were skipped
	if skippedUnavailable > 0 {
		resp.Insights = append(resp.Insights, fmt.Sprintf("🔍 Filtered out %d VMs not available in %s", skippedUnavailable, req.Region))
	}

	if len(resp.Instances) > 0 {
		top := resp.Instances[0]
		resp.Summary = fmt.Sprintf("Top recommendation: %s with %d vCPU, %.0fGB RAM, %d%% savings",
			top.InstanceType, top.VCPU, top.MemoryGB, top.SavingsPercent)
		resp.Insights = append(resp.Insights, fmt.Sprintf("💰 Save up to %d%% compared to on-demand", top.SavingsPercent))
		resp.Insights = append(resp.Insights, fmt.Sprintf("⚡ %s interruption rate", top.InterruptionLevel))
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode response: %v", err)
		return
	}
	s.logger.Info("Response sent: %d instances", len(resp.Instances))
}

func (s *Server) applyUseCasePreset(req AnalyzeRequest, arch string) domain.UsageRequirements {
	requirements := domain.UsageRequirements{
		MinVCPU:         req.MinVCPU,
		MaxVCPU:         req.MaxVCPU,
		MinMemoryGB:     float64(req.MinMemory),
		MaxMemoryGB:     float64(req.MaxMemory),
		Architecture:    arch,
		Region:          req.Region,
		OS:              domain.Linux,
		MaxInterruption: domain.InterruptionFrequency(req.MaxInterruption),
		TopN:            req.TopN,
		AllowBurstable:  s.cfg.Analysis.AllowBurstable, // Use config default (true = include t-family)
		AllowBareMetal:  s.cfg.Analysis.AllowBareMetal, // Use config default
		Families:        req.Families,                  // Pass family filter to analyzer
	}

	// Apply use case specific settings
	switch strings.ToLower(req.UseCase) {
	case "kubernetes", "k8s":
		// K8s nodes need stable instances
		if requirements.MaxInterruption > 1 {
			requirements.MaxInterruption = 1
		}
		if requirements.MinMemoryGB == 0 {
			requirements.MinMemoryGB = 4
		}
	case "database", "db":
		// Databases need very stable instances
		requirements.MaxInterruption = 0
		requirements.PreferredCategory = domain.MemoryOptimized
	case "asg", "autoscaling":
		// ASG can handle interruptions better
		requirements.MaxInterruption = 2
		requirements.AllowBurstable = true
	case "weekend", "batch":
		// Batch jobs can use cheaper, less stable instances
		requirements.MaxInterruption = 3
		requirements.AllowBurstable = true
	}

	return requirements
}

// ParseRequirementsRequest for natural language parsing
type ParseRequirementsRequest struct {
	Text string `json:"text"`
}

// ParseRequirementsResponse returns parsed requirements
type ParseRequirementsResponse struct {
	MinVCPU         int    `json:"minVcpu"`
	MaxVCPU         int    `json:"maxVcpu"`
	MinMemory       int    `json:"minMemory"`
	MaxMemory       int    `json:"maxMemory"`
	Architecture    string `json:"architecture"`
	UseCase         string `json:"useCase"`
	MaxInterruption int    `json:"maxInterruption"`
	Explanation     string `json:"explanation"`
}

func (s *Server) handleParseRequirements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}

	var req ParseRequirementsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Try AI-powered parsing first, fall back to rules
	nlpParser := analyzer.NewNLPParser()
	result, err := nlpParser.Parse(req.Text)
	if err != nil {
		s.logger.Warn("NLP parsing failed: %v", err)
		// Fall back to local rules
		resp := parseNaturalLanguage(req.Text)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Convert NLP result to response format
	resp := ParseRequirementsResponse{
		MinVCPU:         result.MinVCPU,
		MaxVCPU:         result.MaxVCPU,
		MinMemory:       result.MinMemory,
		MaxMemory:       result.MaxMemory,
		Architecture:    result.Architecture,
		UseCase:         result.UseCase,
		MaxInterruption: result.MaxInterruption,
		Explanation:     result.Explanation,
	}
	json.NewEncoder(w).Encode(resp)
}

func parseNaturalLanguage(text string) ParseRequirementsResponse {
	text = strings.ToLower(text)
	resp := ParseRequirementsResponse{
		MinVCPU:         2,
		MaxVCPU:         0,
		MinMemory:       4,
		MaxInterruption: 2,
	}

	explanations := []string{}

	// Parse high-performance/specialized workloads FIRST (before size keywords)
	if strings.Contains(text, "quantum") || strings.Contains(text, "hpc") || strings.Contains(text, "high performance") ||
		strings.Contains(text, "scientific") || strings.Contains(text, "simulation") || strings.Contains(text, "research") {
		resp.MinVCPU = 32
		resp.MaxVCPU = 96
		resp.MinMemory = 128
		resp.MaxMemory = 512
		resp.UseCase = "ml"
		resp.MaxInterruption = 2
		explanations = append(explanations, "HPC/Scientific workload: high compute (32-96 vCPU, 128-512GB RAM)")
	} else if strings.Contains(text, "machine learning") || strings.Contains(text, "ml training") ||
		strings.Contains(text, "deep learning") || strings.Contains(text, "neural network") ||
		strings.Contains(text, "ai training") || strings.Contains(text, "model training") {
		resp.MinVCPU = 16
		resp.MaxVCPU = 64
		resp.MinMemory = 64
		resp.MaxMemory = 256
		resp.UseCase = "ml"
		resp.MaxInterruption = 2
		explanations = append(explanations, "ML/AI Training workload: compute-optimized (16-64 vCPU, 64-256GB RAM)")
	} else if strings.Contains(text, "ai") || strings.Contains(text, "inference") || strings.Contains(text, "llm") ||
		strings.Contains(text, "gpt") || strings.Contains(text, "transformer") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 32
		resp.MaxMemory = 128
		resp.UseCase = "ml"
		resp.MaxInterruption = 1
		explanations = append(explanations, "AI/Inference workload: balanced compute (8-32 vCPU, 32-128GB RAM)")
	} else if strings.Contains(text, "data science") || strings.Contains(text, "analytics") ||
		strings.Contains(text, "big data") || strings.Contains(text, "spark") || strings.Contains(text, "hadoop") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 32
		resp.MaxMemory = 128
		resp.UseCase = "batch"
		resp.MaxInterruption = 2
		explanations = append(explanations, "Data Science/Analytics: memory-optimized (8-32 vCPU, 32-128GB RAM)")
	} else if strings.Contains(text, "video") || strings.Contains(text, "encoding") || strings.Contains(text, "transcoding") ||
		strings.Contains(text, "streaming") || strings.Contains(text, "media") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 16
		resp.MaxMemory = 64
		resp.UseCase = "batch"
		resp.MaxInterruption = 2
		explanations = append(explanations, "Video/Media processing: compute-heavy (8-32 vCPU, 16-64GB RAM)")
	} else if strings.Contains(text, "gaming") || strings.Contains(text, "game server") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 16
		resp.MinMemory = 16
		resp.MaxMemory = 64
		resp.UseCase = "general"
		resp.MaxInterruption = 1
		explanations = append(explanations, "Gaming server: balanced with stability (4-16 vCPU, 16-64GB RAM)")
	} else if strings.Contains(text, "rendering") || strings.Contains(text, "3d") || strings.Contains(text, "graphics") {
		resp.MinVCPU = 16
		resp.MaxVCPU = 64
		resp.MinMemory = 32
		resp.MaxMemory = 128
		resp.UseCase = "batch"
		resp.MaxInterruption = 3
		explanations = append(explanations, "3D Rendering: high compute, cost-optimized (16-64 vCPU, 32-128GB RAM)")
	} else if strings.Contains(text, "ci") || strings.Contains(text, "cd") || strings.Contains(text, "build") ||
		strings.Contains(text, "jenkins") || strings.Contains(text, "github actions") || strings.Contains(text, "pipeline") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 16
		resp.MinMemory = 8
		resp.MaxMemory = 32
		resp.UseCase = "batch"
		resp.MaxInterruption = 3
		explanations = append(explanations, "CI/CD Build: cost-optimized (4-16 vCPU, 8-32GB RAM)")
	} else {
		// Parse CPU requirements by size keywords
		if strings.Contains(text, "small") || strings.Contains(text, "tiny") || strings.Contains(text, "micro") {
			resp.MinVCPU = 1
			resp.MaxVCPU = 2
			resp.MinMemory = 1
			resp.MaxMemory = 4
			explanations = append(explanations, "Small instance (1-2 vCPU)")
		} else if strings.Contains(text, "medium") || strings.Contains(text, "moderate") {
			resp.MinVCPU = 2
			resp.MaxVCPU = 4
			resp.MinMemory = 4
			resp.MaxMemory = 16
			explanations = append(explanations, "Medium instance (2-4 vCPU)")
		} else if strings.Contains(text, "large") || strings.Contains(text, "big") {
			resp.MinVCPU = 4
			resp.MaxVCPU = 8
			resp.MinMemory = 16
			resp.MaxMemory = 64
			explanations = append(explanations, "Large instance (4-8 vCPU)")
		} else if strings.Contains(text, "xlarge") || strings.Contains(text, "extra large") || strings.Contains(text, "huge") {
			resp.MinVCPU = 8
			resp.MaxVCPU = 32
			resp.MinMemory = 32
			explanations = append(explanations, "Extra large instance (8-32 vCPU)")
		}
	}

	// Extract specific CPU numbers (override if explicitly mentioned)
	for _, word := range strings.Fields(text) {
		if num, err := strconv.Atoi(strings.TrimSuffix(word, "vcpu")); err == nil {
			if strings.Contains(text, "core") || strings.Contains(text, "cpu") || strings.Contains(text, "vcpu") {
				resp.MinVCPU = num
				resp.MaxVCPU = num + 2
				explanations = append(explanations, fmt.Sprintf("Detected %d vCPU requirement", num))
			}
		}
		// Check for memory like "8gb" or "16 gb"
		if strings.HasSuffix(word, "gb") {
			numStr := strings.TrimSuffix(word, "gb")
			if num, err := strconv.Atoi(numStr); err == nil {
				resp.MinMemory = num
				explanations = append(explanations, fmt.Sprintf("Detected %dGB memory requirement", num))
			}
		}
	}

	// Parse use cases (only if not already set by workload detection)
	if resp.UseCase == "" {
		if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") || strings.Contains(text, "cluster") {
			resp.UseCase = "kubernetes"
			resp.MaxInterruption = 1
			explanations = append(explanations, "Kubernetes use case: prioritizing stability")
		} else if strings.Contains(text, "database") || strings.Contains(text, "db") || strings.Contains(text, "postgres") ||
			strings.Contains(text, "mysql") || strings.Contains(text, "mongo") || strings.Contains(text, "redis") {
			resp.UseCase = "database"
			resp.MaxInterruption = 0
			explanations = append(explanations, "Database use case: maximum stability required")
		} else if strings.Contains(text, "autoscaling") || strings.Contains(text, "asg") || strings.Contains(text, "auto scaling") {
			resp.UseCase = "asg"
			resp.MaxInterruption = 2
			explanations = append(explanations, "Auto-scaling use case: balanced cost/stability")
		} else if strings.Contains(text, "weekend") || strings.Contains(text, "batch") || strings.Contains(text, "job") ||
			strings.Contains(text, "temporary") || strings.Contains(text, "short") {
			resp.UseCase = "batch"
			resp.MaxInterruption = 3
			explanations = append(explanations, "Batch/temporary use case: prioritizing cost savings")
		} else if strings.Contains(text, "web") || strings.Contains(text, "api") || strings.Contains(text, "server") {
			resp.UseCase = "general"
			resp.MaxInterruption = 2
			explanations = append(explanations, "Web/API use case: balanced approach")
		}
	}

	// Parse architecture
	if strings.Contains(text, "intel") {
		resp.Architecture = "intel"
		explanations = append(explanations, "Intel architecture selected")
	} else if strings.Contains(text, "amd") {
		resp.Architecture = "amd"
		explanations = append(explanations, "AMD architecture selected")
	} else if strings.Contains(text, "arm") || strings.Contains(text, "graviton") {
		resp.Architecture = "arm64"
		explanations = append(explanations, "ARM/Graviton architecture: better cost efficiency")
	}

	// Parse scale keywords
	if strings.Contains(text, "scale") {
		if strings.Contains(text, "small") {
			resp.MinVCPU = 2
			resp.MaxVCPU = 4
		} else if strings.Contains(text, "large") || strings.Contains(text, "high") {
			resp.MinVCPU = 8
			resp.MaxVCPU = 32
		}
	}

	if len(explanations) == 0 {
		resp.Explanation = "Using default settings: 2+ vCPU, 4GB+ RAM, moderate stability"
	} else {
		resp.Explanation = strings.Join(explanations, " | ")
	}

	return resp
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	presets := []map[string]interface{}{
		{
			"id":           "kubernetes",
			"name":         "Kubernetes Cluster",
			"description":  "Stable nodes for K8s workloads",
			"icon":         "☸️",
			"minVcpu":      2,
			"minMemory":    4,
			"interruption": 1,
		},
		{
			"id":           "database",
			"name":         "Database Server",
			"description":  "Maximum stability for data workloads",
			"icon":         "🗄️",
			"minVcpu":      2,
			"minMemory":    8,
			"interruption": 0,
		},
		{
			"id":           "asg",
			"name":         "Auto Scaling Group",
			"description":  "Balanced cost/stability for ASG",
			"icon":         "📈",
			"minVcpu":      2,
			"minMemory":    4,
			"interruption": 2,
		},
		{
			"id":           "batch",
			"name":         "Batch/Weekend Jobs",
			"description":  "Maximum savings for temporary workloads",
			"icon":         "⏰",
			"minVcpu":      2,
			"minMemory":    4,
			"interruption": 3,
		},
		{
			"id":           "web",
			"name":         "Web Server/API",
			"description":  "General purpose web workloads",
			"icon":         "🌐",
			"minVcpu":      2,
			"minMemory":    4,
			"interruption": 2,
		},
		{
			"id":           "ml",
			"name":         "ML Training",
			"description":  "Compute-optimized for ML workloads",
			"icon":         "🤖",
			"minVcpu":      8,
			"minMemory":    32,
			"interruption": 2,
		},
	}

	json.NewEncoder(w).Encode(presets)
}

func formatInterruption(level domain.InterruptionFrequency) string {
	switch level {
	case 0:
		return "<5%"
	case 1:
		return "5-10%"
	case 2:
		return "10-15%"
	case 3:
		return "15-20%"
	case 4:
		return ">20%"
	default:
		return "Unknown"
	}
}

// CacheStatusResponse represents cache status
type CacheStatusResponse struct {
	Hits        int64    `json:"hits"`
	Misses      int64    `json:"misses"`
	Items       int      `json:"items"`
	Keys        []string `json:"keys,omitempty"`
	LastRefresh string   `json:"lastRefresh"`
	TTLHours    float64  `json:"ttlHours"`
}

// handleCacheStatus returns current cache statistics
func (s *Server) handleCacheStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cache := provider.GetCacheManager()
	stats := cache.GetStats()

	lastRefresh := cache.GetLastRefresh()
	lastRefreshStr := "never"
	if !lastRefresh.IsZero() {
		lastRefreshStr = lastRefresh.Format(time.RFC3339)
	}

	resp := CacheStatusResponse{
		Hits:        stats.Hits,
		Misses:      stats.Misses,
		Items:       stats.Items,
		Keys:        cache.Keys(),
		LastRefresh: lastRefreshStr,
		TTLHours:    cache.GetTTL().Hours(),
	}

	s.logger.Info("Cache status: items=%d hits=%d misses=%d", stats.Items, stats.Hits, stats.Misses)
	json.NewEncoder(w).Encode(resp)
}

// handleCacheRefresh clears the cache forcing fresh data on next request
func (s *Server) handleCacheRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed, use POST",
		})
		return
	}

	cache := provider.GetCacheManager()
	itemsBefore := len(cache.Keys())
	cache.Refresh()

	s.logger.Info("Cache refreshed: cleared %d items", itemsBefore)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("Cache cleared: %d items removed", itemsBefore),
		"itemsCleared": itemsBefore,
		"refreshTime":  time.Now().Format(time.RFC3339),
	})
}

// AZRequest for availability zone recommendations
type AZRequest struct {
	CloudProvider string `json:"cloudProvider,omitempty"` // aws, azure
	InstanceType  string `json:"instanceType"`
	Region        string `json:"region"`
}

// AZResponse for availability zone recommendations
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
	Confidence        float64            `json:"confidence"`
	DataSources       []string           `json:"dataSources,omitempty"`
	Error             string             `json:"error,omitempty"`
}

// AZRecommendation for a single AZ
type AZRecommendation struct {
	Rank              int     `json:"rank"`
	AvailabilityZone  string  `json:"availabilityZone"`
	CombinedScore     float64 `json:"combinedScore"`     // 0-100 overall score
	CapacityScore     float64 `json:"capacityScore"`     // 0-100 capacity score
	AvailabilityScore float64 `json:"availabilityScore"` // 0-100 availability score
	PriceScore        float64 `json:"priceScore"`        // 0-100 price score
	AvgPrice          float64 `json:"avgPrice"`
	MinPrice          float64 `json:"minPrice"`
	MaxPrice          float64 `json:"maxPrice"`
	CurrentPrice      float64 `json:"currentPrice"`
	PricePredicted    bool    `json:"pricePredicted"` // True if price was estimated
	Volatility        float64 `json:"volatility"`
	InterruptionRate  float64 `json:"interruptionRate"` // Estimated %
	Stability         string  `json:"stability"`
	CapacityLevel     string  `json:"capacityLevel"` // High, Medium, Low
	Available         bool    `json:"available"`
}

// handleAZRecommendation handles availability zone recommendations
func (s *Server) handleAZRecommendation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(AZResponse{Success: false, Error: "Method not allowed"})
		return
	}

	var req AZRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(AZResponse{Success: false, Error: "Invalid request"})
		return
	}

	// Determine cloud provider
	cloudProvider := domain.ParseCloudProvider(req.CloudProvider)

	if req.InstanceType == "" {
		json.NewEncoder(w).Encode(AZResponse{Success: false, Error: "instanceType is required"})
		return
	}

	if req.Region == "" {
		req.Region = cloudProvider.DefaultRegion()
	}

	s.logger.Info("AZ recommendation request: cloud=%s instance=%s region=%s", cloudProvider, req.InstanceType, req.Region)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var predEngine *analyzer.PredictionEngine
	usingRealData := false
	isAzure := false

	switch cloudProvider {
	case domain.Azure:
		priceProvider := azureprovider.NewPriceHistoryProvider(req.Region)
		isAzure = true
		s.logger.Info("Using Azure Retail Prices API for AZ recommendations")
		adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
			WithCloudProvider("azure").
			WithZoneProvider(azureprovider.NewZoneProviderAdapter(req.Region)).
			WithCapacityProvider(azureprovider.NewCapacityProviderAdapter(req.Region))

	case domain.GCP:
		priceProvider := gcpprovider.NewPriceHistoryProvider(req.Region)
		s.logger.Info("Using GCP pricing data for AZ recommendations")
		adapter := gcpprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
			WithCloudProvider("gcp").
			WithZoneProvider(gcpprovider.NewZoneProviderAdapter(req.Region))
		usingRealData = true // GCP always has pricing data available

	default: // AWS
		priceProvider, err := awsprovider.NewPriceHistoryProvider(req.Region)
		if err != nil {
			json.NewEncoder(w).Encode(AZResponse{Success: false, Error: "Failed to create provider"})
			return
		}

		if priceProvider != nil && priceProvider.IsAvailable() {
			usingRealData = true
			s.logger.Info("Using real AWS DescribeSpotPriceHistory for AZ recommendations")
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
				WithCloudProvider("aws").
				WithZoneProvider(awsprovider.NewZoneProviderAdapter(req.Region)).
				WithCapacityProvider(awsprovider.NewCapacityProviderAdapter(req.Region))
		} else {
			s.logger.Info("AWS credentials not available for AZ recommendations")
			predEngine = analyzer.NewPredictionEngine(nil, req.Region).
				WithCloudProvider("aws").
				WithZoneProvider(awsprovider.NewZoneProviderAdapter(req.Region)).
				WithCapacityProvider(awsprovider.NewCapacityProviderAdapter(req.Region))
		}
	}

	// Use the smart AZ selector for recommendations
	smartRec, err := predEngine.SmartRecommendAZ(ctx, req.InstanceType, analyzer.DefaultWeights())
	if err != nil {
		// Fall back to traditional method if smart selector fails
		s.logger.Warn("Smart AZ selector failed, falling back to traditional method: %v", err)
		rec, err := predEngine.RecommendAZ(ctx, req.InstanceType)
		if err != nil {
			json.NewEncoder(w).Encode(AZResponse{Success: false, Error: err.Error()})
			return
		}

		// Handle traditional response
		if isAzure {
			usingRealData = rec.UsingRealSKUData
		}
		s.sendTraditionalAZResponse(w, req, rec, usingRealData)
		return
	}

	// Use smart recommendation insights
	usingRealData = smartRec.Confidence > 0.5 // Higher confidence means we have real data

	// Get config for AZ recommendation count
	cfg := config.Get()
	maxAZRecommendations := cfg.Analysis.AZRecommendations

	// Build response from smart recommendation
	resp := AZResponse{
		Success:           true,
		InstanceType:      req.InstanceType,
		Region:            req.Region,
		Recommendations:   make([]AZRecommendation, 0),
		Insights:          smartRec.Insights,
		PriceDifferential: 0, // Calculate from smart results
		UsingRealData:     usingRealData,
		Confidence:        smartRec.Confidence,
		DataSources:       smartRec.DataSources,
		BestAZ:            smartRec.BestAZ,
		NextBestAZ:        smartRec.NextBestAZ,
	}

	// Calculate price differential
	if len(smartRec.Rankings) >= 2 {
		bestPrice := smartRec.Rankings[0].SpotPrice
		worstPrice := smartRec.Rankings[len(smartRec.Rankings)-1].SpotPrice
		if bestPrice > 0 {
			resp.PriceDifferential = ((worstPrice - bestPrice) / bestPrice) * 100
		}
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

		// Determine capacity level
		capacityLevel := "Low"
		if rank.CapacityScore >= 80 {
			capacityLevel = "High"
		} else if rank.CapacityScore >= 50 {
			capacityLevel = "Medium"
		}

		resp.Recommendations = append(resp.Recommendations, AZRecommendation{
			AvailabilityZone:  rank.Zone,
			Rank:              rank.Rank,
			CombinedScore:     rank.CombinedScore,
			CapacityScore:     rank.CapacityScore,
			AvailabilityScore: rank.AvailabilityScore,
			PriceScore:        rank.PriceScore,
			AvgPrice:          rank.SpotPrice,
			MinPrice:          rank.SpotPrice * 0.9, // Estimate
			MaxPrice:          rank.SpotPrice * 1.1, // Estimate
			CurrentPrice:      rank.SpotPrice,
			PricePredicted:    rank.PricePredicted,
			Volatility:        rank.Volatility,
			InterruptionRate:  rank.InterruptionRate,
			Stability:         stability,
			CapacityLevel:     capacityLevel,
			Available:         rank.Available,
		})
	}

	json.NewEncoder(w).Encode(resp)
}

// sendTraditionalAZResponse sends the traditional AZ response (fallback)
func (s *Server) sendTraditionalAZResponse(w http.ResponseWriter, req AZRequest, rec *analyzer.AZRecommendation, usingRealData bool) {
	cfg := config.Get()
	maxAZRecommendations := cfg.Analysis.AZRecommendations

	resp := AZResponse{
		Success:           true,
		InstanceType:      req.InstanceType,
		Region:            req.Region,
		Recommendations:   make([]AZRecommendation, 0),
		Insights:          rec.Insights,
		PriceDifferential: rec.PriceDifferential,
		UsingRealData:     usingRealData,
		BestAZ:            rec.BestAZ,
		Confidence:        0.5, // Medium confidence for traditional method
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
			AvailabilityZone:  az.AvailabilityZone,
			Rank:              az.Rank,
			CombinedScore:     az.Score * 100,
			CapacityScore:     50, // Unknown in traditional method
			AvailabilityScore: 100,
			PriceScore:        az.Score * 100,
			AvgPrice:          az.AvgPrice,
			MinPrice:          az.MinPrice,
			MaxPrice:          az.MaxPrice,
			CurrentPrice:      az.AvgPrice,
			Volatility:        az.Volatility,
			Stability:         stability,
			CapacityLevel:     "Unknown",
			Available:         true,
		})
	}

	json.NewEncoder(w).Encode(resp)
}

// handleSmartAZRecommendation handles smart availability zone recommendations with full details
// This endpoint provides more detailed scoring breakdown than the standard /api/az endpoint
func (s *Server) handleSmartAZRecommendation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		InstanceType  string `json:"instanceType"`
		Region        string `json:"region"`
		CloudProvider string `json:"cloudProvider"`
		OptimizeFor   string `json:"optimizeFor"` // "balanced", "capacity", "cost"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}

	cloudProvider := domain.ParseCloudProvider(req.CloudProvider)

	if req.InstanceType == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "instanceType is required"})
		return
	}

	if req.Region == "" {
		req.Region = cloudProvider.DefaultRegion()
	}

	// Select weights based on optimization goal
	weights := analyzer.DefaultWeights()
	switch req.OptimizeFor {
	case "capacity":
		weights = analyzer.HighCapacityWeights()
	case "cost":
		weights = analyzer.LowCostWeights()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var predEngine *analyzer.PredictionEngine

	switch cloudProvider {
	case domain.Azure:
		priceProvider := azureprovider.NewPriceHistoryProvider(req.Region)
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
			WithZoneProvider(gcpprovider.NewZoneProviderAdapter(req.Region)).
			WithCapacityProvider(gcpprovider.NewCapacityProviderAdapter(req.Region))

	default: // AWS
		priceProvider, err := awsprovider.NewPriceHistoryProvider(req.Region)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create provider"})
			return
		}

		var adapter analyzer.PriceHistoryProvider
		if priceProvider != nil && priceProvider.IsAvailable() {
			adapter = awsprovider.NewPriceHistoryAdapter(priceProvider)
		}
		predEngine = analyzer.NewPredictionEngine(adapter, req.Region).
			WithCloudProvider("aws").
			WithZoneProvider(awsprovider.NewZoneProviderAdapter(req.Region)).
			WithCapacityProvider(awsprovider.NewCapacityProviderAdapter(req.Region))
	}

	result, err := predEngine.SmartRecommendAZ(ctx, req.InstanceType, weights)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	// Return full smart result
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// handleFamilies returns available instance families derived from instance specs (cached)
func (s *Server) handleFamilies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get cloud provider from query parameter
	cp := domain.ParseCloudProvider(r.URL.Query().Get("cloud"))
	ctx := context.Background()

	// Check cache first
	cacheKey := fmt.Sprintf("families:%s", cp)
	cacheManager := provider.GetCacheManager()
	if cached, exists := cacheManager.Get(cacheKey); exists {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// Try to get families dynamically from instance specs
	factory := provider.GetFactory()
	specsProvider, err := factory.CreateInstanceSpecsProvider(cp)
	if err == nil {
		allSpecs, specErr := specsProvider.GetAllInstanceSpecs(ctx)
		if specErr == nil && len(allSpecs) > 0 {
			// Derive unique families from specs
			familyMap := make(map[string]string) // family -> description
			for _, spec := range allSpecs {
				family := extractInstanceFamily(spec.InstanceType)
				if family != "" {
					if _, exists := familyMap[family]; !exists {
						familyMap[family] = getCategoryDescription(spec.Category)
					}
				}
			}

			// Convert to response format
			families := make([]map[string]string, 0, len(familyMap))
			for name, desc := range familyMap {
				families = append(families, map[string]string{
					"name":        name,
					"description": desc,
				})
			}

			// Sort families alphabetically
			sort.Slice(families, func(i, j int) bool {
				return families[i]["name"] < families[j]["name"]
			})

			if len(families) > 0 {
				// Cache for 24 hours (families don't change often)
				cacheManager.Set(cacheKey, families, 24*time.Hour)
				json.NewEncoder(w).Encode(families)
				return
			}
		}
	}

	// Fallback to hardcoded families if dynamic fetch fails
	var fallbackFamilies []map[string]string
	if cp == domain.Azure {
		fallbackFamilies = []map[string]string{
			{"name": "B", "description": "Burstable"},
			{"name": "D", "description": "General Purpose"},
			{"name": "Das", "description": "General Purpose (AMD)"},
			{"name": "Dps", "description": "General Purpose (ARM)"},
			{"name": "Ds", "description": "General Purpose (SSD)"},
			{"name": "E", "description": "Memory Optimized"},
			{"name": "Eas", "description": "Memory Optimized (AMD)"},
			{"name": "Eps", "description": "Memory Optimized (ARM)"},
			{"name": "Es", "description": "Memory Optimized (SSD)"},
			{"name": "F", "description": "Compute Optimized"},
			{"name": "Fs", "description": "Compute Optimized (SSD)"},
			{"name": "Fx", "description": "Compute Optimized (High Freq)"},
			{"name": "HB", "description": "High Performance (HPC)"},
			{"name": "HC", "description": "High Performance (Compute)"},
			{"name": "L", "description": "Storage Optimized"},
			{"name": "Ls", "description": "Storage Optimized (SSD)"},
			{"name": "M", "description": "Memory Optimized (Large)"},
			{"name": "NC", "description": "GPU Compute"},
			{"name": "ND", "description": "GPU Deep Learning"},
			{"name": "NV", "description": "GPU Visualization"},
		}
	} else {
		// Return AWS families from config
		cfg := config.Get()
		// Convert config families to map format for consistency
		for _, f := range cfg.InstanceFamilies.Available {
			fallbackFamilies = append(fallbackFamilies, map[string]string{
				"name":        f.Name,
				"description": f.Description,
			})
		}
	}

	// Cache the fallback families
	if len(fallbackFamilies) > 0 {
		cacheManager.Set(cacheKey, fallbackFamilies, 24*time.Hour)
	}
	json.NewEncoder(w).Encode(fallbackFamilies)
}

// getCategoryDescription returns a human-readable description for instance category
func getCategoryDescription(cat domain.InstanceCategory) string {
	switch cat {
	case domain.GeneralPurpose:
		return "General Purpose"
	case domain.ComputeOptimized:
		return "Compute Optimized"
	case domain.MemoryOptimized:
		return "Memory Optimized"
	case domain.StorageOptimized:
		return "Storage Optimized"
	case domain.AcceleratedComputing:
		return "GPU/Accelerated"
	case domain.HighPerformance:
		return "High Performance"
	default:
		return "General Purpose"
	}
}

// InstanceTypeInfo contains basic info for autocomplete
type InstanceTypeInfo struct {
	InstanceType string  `json:"instanceType"`
	Family       string  `json:"family"`
	VCPU         int     `json:"vcpu"`
	MemoryGB     float64 `json:"memoryGb"`
	Architecture string  `json:"architecture"`
}

// handleInstanceTypes returns available instance types for autocomplete
func (s *Server) handleInstanceTypes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get query parameter for filtering
	query := strings.ToLower(r.URL.Query().Get("q"))
	family := strings.ToLower(r.URL.Query().Get("family"))
	cp := domain.ParseCloudProvider(r.URL.Query().Get("cloud"))
	limitStr := r.URL.Query().Get("limit")
	limit := 50 // Default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	// Get all instance specs
	ctx := context.Background()
	factory := provider.GetFactory()
	specsProvider, err := factory.CreateInstanceSpecsProvider(cp)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get instance specs",
		})
		return
	}

	allSpecs, err := specsProvider.GetAllInstanceSpecs(ctx)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to fetch instance specs",
		})
		return
	}

	// Filter and build response
	results := make([]InstanceTypeInfo, 0)
	for _, spec := range allSpecs {
		// Filter by family if specified
		if family != "" {
			specFamily := extractInstanceFamily(spec.InstanceType)
			if !strings.EqualFold(specFamily, family) {
				continue
			}
		}

		// Filter by query if specified
		if query != "" {
			if !strings.Contains(strings.ToLower(spec.InstanceType), query) {
				continue
			}
		}

		results = append(results, InstanceTypeInfo{
			InstanceType: spec.InstanceType,
			Family:       extractInstanceFamily(spec.InstanceType),
			VCPU:         spec.VCPU,
			MemoryGB:     spec.MemoryGB,
			Architecture: spec.Architecture,
		})

		if len(results) >= limit {
			break
		}
	}

	// Sort by instance type
	sort.Slice(results, func(i, j int) bool {
		return results[i].InstanceType < results[j].InstanceType
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"count":     len(results),
		"instances": results,
	})
}

// handleOpenAPI serves the OpenAPI specification
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Read the openapi.json file from embedded filesystem or disk
	data, err := os.ReadFile("api/openapi.json")
	if err != nil {
		// Try relative to executable
		exeDir := getExecutableDir()
		data, err = os.ReadFile(exeDir + "/api/openapi.json")
		if err != nil {
			http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
			return
		}
	}
	w.Write(data)
}

func getExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := exe[:len(exe)-len("/spot-web.exe")]
	if dir == exe {
		dir = exe[:len(exe)-len("\\spot-web.exe")]
	}
	return dir
}

// extractInstanceFamily extracts the family prefix from an instance type
// AWS: "m5.large" -> "m", "c6i.xlarge" -> "c", "t3a.medium" -> "t"
// Azure: "Standard_D2s_v5" -> "D", "Standard_E4s_v5" -> "E"
func extractInstanceFamily(instanceType string) string {
	// Handle Azure format: Standard_D2s_v5 -> D
	if strings.HasPrefix(instanceType, "Standard_") {
		rest := strings.TrimPrefix(instanceType, "Standard_")
		// Extract letters before first digit
		for i, c := range rest {
			if c >= '0' && c <= '9' {
				return rest[:i]
			}
		}
		// No digit found, return full rest
		return rest
	}

	// Handle AWS format: m5.large -> m
	for i, c := range instanceType {
		if c >= '0' && c <= '9' {
			return instanceType[:i]
		}
	}
	return instanceType
}

// containsFamily checks if a family is in the list of allowed families
func containsFamily(families []string, family string) bool {
	for _, f := range families {
		if strings.EqualFold(f, family) {
			return true
		}
	}
	return false
}
