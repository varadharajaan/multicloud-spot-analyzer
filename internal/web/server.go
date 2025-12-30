package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
	awsprovider "github.com/spot-analyzer/internal/provider/aws"
)

//go:embed static/*
var staticFiles embed.FS

// Server represents the web UI server
type Server struct {
	port   int
	logger *logging.Logger
}

// NewServer creates a new web server
func NewServer(port int) *Server {
	logger, _ := logging.New(logging.Config{
		Level:       logging.INFO,
		LogDir:      "logs",
		EnableFile:  true,
		EnableColor: true,
	})
	return &Server{port: port, logger: logger}
}

// Start starts the web server
func (s *Server) Start() error {
	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		s.logger.Error("Failed to load static files: %v", err)
		return err
	}

	http.Handle("/", s.logRequest(http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/api/analyze", s.handleAnalyze)
	http.HandleFunc("/api/parse-requirements", s.handleParseRequirements)
	http.HandleFunc("/api/presets", s.handlePresets)

	addr := fmt.Sprintf(":%d", s.port)
	s.logger.Info("Starting web UI at http://localhost%s", addr)
	fmt.Printf("🌐 Starting web UI at http://localhost%s\n", addr)
	return http.ListenAndServe(addr, nil)
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
	MinVCPU         int    `json:"minVcpu"`
	MaxVCPU         int    `json:"maxVcpu"`
	MinMemory       int    `json:"minMemory"`
	MaxMemory       int    `json:"maxMemory"`
	Architecture    string `json:"architecture"` // x86_64, arm64, intel, amd
	Region          string `json:"region"`
	MaxInterruption int    `json:"maxInterruption"`
	UseCase         string `json:"useCase"` // general, kubernetes, database, asg
	Enhanced        bool   `json:"enhanced"`
	TopN            int    `json:"topN"`
}

// AnalyzeResponse represents the API response
type AnalyzeResponse struct {
	Success   bool             `json:"success"`
	Instances []InstanceResult `json:"instances"`
	Summary   string           `json:"summary"`
	Insights  []string         `json:"insights"`
	Error     string           `json:"error,omitempty"`
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

	s.logger.Info("Analyze request: region=%s vcpu=%d-%d memory=%d-%d arch=%s useCase=%s enhanced=%v",
		req.Region, req.MinVCPU, req.MaxVCPU, req.MinMemory, req.MaxMemory, req.Architecture, req.UseCase, req.Enhanced)

	// Set defaults
	if req.Region == "" {
		req.Region = "us-east-1"
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

	// Run analysis
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	factory := provider.GetFactory()
	spotProvider, _ := factory.CreateSpotDataProvider(domain.AWS)
	specsProvider, _ := factory.CreateInstanceSpecsProvider(domain.AWS)

	var result *analyzer.EnhancedAnalysisResult
	var err error

	if req.Enhanced {
		priceProvider, _ := awsprovider.NewPriceHistoryProvider(req.Region)
		var enhancedAnalyzer *analyzer.EnhancedAnalyzer
		if priceProvider != nil && priceProvider.IsAvailable() {
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(spotProvider, specsProvider, adapter, req.Region)
		} else {
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzer(spotProvider, specsProvider)
		}
		result, err = enhancedAnalyzer.AnalyzeEnhanced(ctx, requirements)
	} else {
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
		json.NewEncoder(w).Encode(AnalyzeResponse{Success: false, Error: err.Error()})
		return
	}

	// Build response
	resp := AnalyzeResponse{
		Success:   true,
		Instances: make([]InstanceResult, 0),
		Insights:  make([]string, 0),
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

	for i, inst := range instances {
		if i >= req.TopN {
			break
		}
		resp.Instances = append(resp.Instances, InstanceResult{
			Rank:              i + 1,
			InstanceType:      inst.InstanceAnalysis.Specs.InstanceType,
			VCPU:              inst.InstanceAnalysis.Specs.VCPU,
			MemoryGB:          inst.InstanceAnalysis.Specs.MemoryGB,
			SavingsPercent:    inst.InstanceAnalysis.SpotData.SavingsPercent,
			InterruptionLevel: formatInterruption(inst.InstanceAnalysis.SpotData.InterruptionFrequency),
			Score:             inst.FinalScore,
			Architecture:      inst.InstanceAnalysis.Specs.Architecture,
			Generation:        string(inst.InstanceAnalysis.Specs.Generation),
		})
	}

	if len(resp.Instances) > 0 {
		top := resp.Instances[0]
		resp.Summary = fmt.Sprintf("Top recommendation: %s with %d vCPU, %.0fGB RAM, %d%% savings",
			top.InstanceType, top.VCPU, top.MemoryGB, top.SavingsPercent)
		resp.Insights = append(resp.Insights, fmt.Sprintf("💰 Save up to %d%% compared to on-demand", top.SavingsPercent))
		resp.Insights = append(resp.Insights, fmt.Sprintf("⚡ %s interruption rate", top.InterruptionLevel))
	}

	json.NewEncoder(w).Encode(resp)
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

	resp := parseNaturalLanguage(req.Text)
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

	// Parse CPU requirements
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

	// Extract specific CPU numbers
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

	// Parse use cases
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
