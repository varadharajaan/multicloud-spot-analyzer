// Package analyzer provides AI-powered natural language parsing for workload requirements.
package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spot-analyzer/internal/nlp"
)

// WorkloadRequirements represents parsed workload specifications
type WorkloadRequirements struct {
	MinVCPU         int    `json:"minVcpu"`
	MaxVCPU         int    `json:"maxVcpu"`
	MinMemory       int    `json:"minMemory"`
	MaxMemory       int    `json:"maxMemory"`
	Architecture    string `json:"architecture,omitempty"`
	UseCase         string `json:"useCase,omitempty"`
	MaxInterruption int    `json:"maxInterruption"`
	Explanation     string `json:"explanation"`
	NeedsGPU        bool   `json:"needsGpu,omitempty"`
	GPUType         string `json:"gpuType,omitempty"`
}

// NLPParser provides AI-powered natural language parsing using configurable providers
// Supports: Ollama (local LLM), HuggingFace, OpenAI, and rule-based fallback
type NLPParser struct {
	httpClient *http.Client
	nlpManager *nlp.Manager
}

// NewNLPParser creates a new NLP parser with auto-detection of available providers
// Provider priority: Ollama > OpenAI > HuggingFace > Rules
func NewNLPParser() *NLPParser {
	return &NLPParser{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		nlpManager: nlp.NewDefaultManager(),
	}
}

// NewNLPParserWithConfig creates a parser with specific configuration
func NewNLPParserWithConfig(config nlp.Config) *NLPParser {
	return &NLPParser{
		httpClient: &http.Client{
			Timeout: time.Duration(config.TimeoutSeconds) * time.Second,
		},
		nlpManager: nlp.NewManager(config),
	}
}

// Parse analyzes natural language and returns workload requirements
// Uses the configured NLP provider (Ollama, OpenAI, HuggingFace, or rules)
func (p *NLPParser) Parse(text string) (*WorkloadRequirements, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Try using the NLP manager with configured providers
	result, err := p.nlpManager.Parse(ctx, text)
	if err == nil && result != nil {
		// Convert nlp.WorkloadRequirements to analyzer.WorkloadRequirements
		return &WorkloadRequirements{
			MinVCPU:         result.MinVCPU,
			MaxVCPU:         result.MaxVCPU,
			MinMemory:       result.MinMemory,
			MaxMemory:       result.MaxMemory,
			Architecture:    result.Architecture,
			UseCase:         result.UseCase,
			MaxInterruption: result.MaxInterruption,
			Explanation:     result.Explanation,
			NeedsGPU:        result.NeedsGPU,
			GPUType:         result.GPUType,
		}, nil
	}

	// If NLP manager fails, fall back to legacy parsing
	legacyResult, legacyErr := p.parseWithFreeAI(text)
	if legacyErr == nil && legacyResult != nil {
		return legacyResult, nil
	}

	// Final fallback to enhanced rule-based parsing
	return p.parseWithRules(text), nil
}

// GetAvailableProviders returns list of available NLP providers
func (p *NLPParser) GetAvailableProviders() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.nlpManager.GetAvailableProviders(ctx)
}

// parseWithFreeAI uses Hugging Face's free inference API for zero-shot classification
func (p *NLPParser) parseWithFreeAI(text string) (*WorkloadRequirements, error) {
	// Use Hugging Face's free zero-shot classification endpoint
	// This model classifies text into categories without needing an API key
	url := "https://api-inference.huggingface.co/models/facebook/bart-large-mnli"

	// Define workload categories for classification
	categories := []string{
		"high performance computing or quantum computing or scientific simulation",
		"machine learning training or deep learning or neural networks",
		"large language model or GPT or AI inference",
		"data analytics or big data processing",
		"video encoding or media processing",
		"gaming server or game hosting",
		"3D rendering or graphics processing",
		"CI/CD pipeline or build automation",
		"kubernetes cluster or container orchestration",
		"database server or data storage",
		"web server or API hosting",
		"small development or testing workload",
	}

	reqBody := map[string]interface{}{
		"inputs":     text,
		"parameters": map[string]interface{}{"candidate_labels": categories},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Labels []string  `json:"labels"`
		Scores []float64 `json:"scores"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(apiResp.Labels) == 0 || len(apiResp.Scores) == 0 {
		return nil, fmt.Errorf("no classification results")
	}

	// Get the top classification
	topLabel := apiResp.Labels[0]
	topScore := apiResp.Scores[0]

	// Only use AI result if confidence is high enough
	if topScore < 0.3 {
		return nil, fmt.Errorf("low confidence classification: %.2f", topScore)
	}

	// Map classification to workload requirements
	result := p.mapClassificationToRequirements(topLabel, topScore, text)
	return result, nil
}

// mapClassificationToRequirements converts AI classification to concrete specs
func (p *NLPParser) mapClassificationToRequirements(label string, score float64, originalText string) *WorkloadRequirements {
	result := &WorkloadRequirements{
		MinVCPU:         2,
		MaxVCPU:         0,
		MinMemory:       4,
		MaxInterruption: 2,
	}

	label = strings.ToLower(label)

	switch {
	case strings.Contains(label, "quantum") || strings.Contains(label, "high performance") || strings.Contains(label, "scientific"):
		result.MinVCPU = 32
		result.MaxVCPU = 96
		result.MinMemory = 128
		result.MaxMemory = 512
		result.UseCase = "hpc"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("[AI] HPC/Scientific workload (%.0f%% confidence): 32-96 vCPU, 128-512GB RAM recommended", score*100)

	case strings.Contains(label, "machine learning") || strings.Contains(label, "deep learning") || strings.Contains(label, "neural"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.MaxInterruption = 2
		result.NeedsGPU = true
		result.GPUType = "nvidia-t4"
		result.Explanation = fmt.Sprintf("[AI] ML Training workload (%.0f%% confidence): 16-64 vCPU, 64-256GB RAM, GPU recommended", score*100)

	case strings.Contains(label, "large language") || strings.Contains(label, "gpt") || strings.Contains(label, "inference"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.MaxInterruption = 1
		result.NeedsGPU = true
		result.GPUType = "nvidia-a10g"
		result.Explanation = fmt.Sprintf("[AI] LLM/Inference workload (%.0f%% confidence): 8-32 vCPU, 64-256GB RAM, GPU recommended", score*100)

	case strings.Contains(label, "data analytics") || strings.Contains(label, "big data"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 32
		result.MaxMemory = 128
		result.UseCase = "batch"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("[AI] Data Analytics workload (%.0f%% confidence): 8-32 vCPU, 32-128GB RAM", score*100)

	case strings.Contains(label, "video") || strings.Contains(label, "media"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 16
		result.MaxMemory = 64
		result.UseCase = "batch"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("[AI] Media Processing workload (%.0f%% confidence): 8-32 vCPU, 16-64GB RAM", score*100)

	case strings.Contains(label, "gaming") || strings.Contains(label, "game"):
		result.MinVCPU = 4
		result.MaxVCPU = 16
		result.MinMemory = 16
		result.MaxMemory = 64
		result.UseCase = "general"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("[AI] Gaming workload (%.0f%% confidence): 4-16 vCPU, 16-64GB RAM, low latency", score*100)

	case strings.Contains(label, "3d") || strings.Contains(label, "rendering") || strings.Contains(label, "graphics"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 32
		result.MaxMemory = 128
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.NeedsGPU = true
		result.GPUType = "nvidia-t4"
		result.Explanation = fmt.Sprintf("[AI] 3D Rendering workload (%.0f%% confidence): 16-64 vCPU, 32-128GB RAM, GPU", score*100)

	case strings.Contains(label, "ci") || strings.Contains(label, "cd") || strings.Contains(label, "build"):
		result.MinVCPU = 4
		result.MaxVCPU = 16
		result.MinMemory = 8
		result.MaxMemory = 32
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("[AI] CI/CD workload (%.0f%% confidence): 4-16 vCPU, 8-32GB RAM, cost-optimized", score*100)

	case strings.Contains(label, "kubernetes") || strings.Contains(label, "container"):
		result.MinVCPU = 2
		result.MaxVCPU = 8
		result.MinMemory = 4
		result.MaxMemory = 32
		result.UseCase = "kubernetes"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("[AI] Kubernetes workload (%.0f%% confidence): 2-8 vCPU, 4-32GB RAM, stable", score*100)

	case strings.Contains(label, "database") || strings.Contains(label, "storage"):
		result.MinVCPU = 2
		result.MaxVCPU = 16
		result.MinMemory = 8
		result.MaxMemory = 64
		result.UseCase = "database"
		result.MaxInterruption = 0
		result.Explanation = fmt.Sprintf("[AI] Database workload (%.0f%% confidence): 2-16 vCPU, 8-64GB RAM, max stability", score*100)

	case strings.Contains(label, "web") || strings.Contains(label, "api"):
		result.MinVCPU = 2
		result.MaxVCPU = 4
		result.MinMemory = 4
		result.MaxMemory = 16
		result.UseCase = "general"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("[AI] Web/API workload (%.0f%% confidence): 2-4 vCPU, 4-16GB RAM", score*100)

	case strings.Contains(label, "small") || strings.Contains(label, "development") || strings.Contains(label, "testing"):
		result.MinVCPU = 1
		result.MaxVCPU = 2
		result.MinMemory = 2
		result.MaxMemory = 8
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("[AI] Dev/Test workload (%.0f%% confidence): 1-2 vCPU, 2-8GB RAM, cost-optimized", score*100)

	default:
		result.Explanation = fmt.Sprintf("[AI] Analysis (%.0f%% confidence): Using balanced defaults", score*100)
	}

	// Extract any explicit numbers from original text
	p.extractExplicitNumbers(originalText, result)

	// Check for architecture preferences in original text
	p.extractArchitecture(originalText, result)

	return result
}

// extractExplicitNumbers looks for specific CPU/RAM numbers in text
func (p *NLPParser) extractExplicitNumbers(text string, result *WorkloadRequirements) {
	text = strings.ToLower(text)

	// Look for CPU patterns like "8 cpu", "8 vcpu", "8 cores", "8vcpu"
	cpuPatterns := []string{
		`(\d+)\s*(?:v?cpus?|cores?)`,
		`(\d+)\s*(?:v?cpu|core)`,
	}
	for _, pattern := range cpuPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			if num, err := strconv.Atoi(matches[1]); err == nil && num > 0 && num <= 192 {
				result.MinVCPU = num
				if result.MaxVCPU == 0 || result.MaxVCPU < num {
					result.MaxVCPU = num + 4
				}
			}
		}
	}

	// Look for RAM patterns like "16 gb", "16gb", "16 ram"
	ramPatterns := []string{
		`(\d+)\s*(?:gb|gib)(?:\s+(?:ram|memory))?`,
		`(\d+)\s*(?:gb|gib)`,
	}
	for _, pattern := range ramPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			if num, err := strconv.Atoi(matches[1]); err == nil && num > 0 && num <= 1024 {
				result.MinMemory = num
				if result.MaxMemory == 0 || result.MaxMemory < num {
					result.MaxMemory = num * 2
				}
			}
		}
	}
}

// extractArchitecture checks for architecture preferences
func (p *NLPParser) extractArchitecture(text string, result *WorkloadRequirements) {
	text = strings.ToLower(text)

	if strings.Contains(text, "intel") {
		result.Architecture = "intel"
	} else if strings.Contains(text, "amd") {
		result.Architecture = "amd"
	} else if strings.Contains(text, "arm") || strings.Contains(text, "graviton") {
		result.Architecture = "arm64"
	}

	// Check for explicit GPU mentions
	if strings.Contains(text, "gpu") || strings.Contains(text, "nvidia") || strings.Contains(text, "cuda") {
		result.NeedsGPU = true
		if strings.Contains(text, "a100") {
			result.GPUType = "nvidia-a100"
		} else if strings.Contains(text, "v100") {
			result.GPUType = "nvidia-v100"
		} else if strings.Contains(text, "a10") {
			result.GPUType = "nvidia-a10g"
		} else if result.GPUType == "" {
			result.GPUType = "nvidia-t4"
		}
	}
}

// containsWord checks if text contains a word as a whole word (not as part of another word)
func containsWord(text, word string) bool {
	text = strings.ToLower(text)
	word = strings.ToLower(word)

	// Use regex for word boundary matching
	pattern := `\b` + regexp.QuoteMeta(word) + `\b`
	re := regexp.MustCompile(pattern)
	return re.MatchString(text)
}

// WorkloadIntensity represents the detected intensity level
type WorkloadIntensity int

const (
	IntensityDefault WorkloadIntensity = iota
	IntensityLight
	IntensityMedium
	IntensityHeavy
	IntensityExtreme
)

// detectWorkloadIntensity analyzes text for intensity modifiers
func detectWorkloadIntensity(text string) (WorkloadIntensity, string) {
	text = strings.ToLower(text)
	
	// Extreme intensity keywords
	extremeKeywords := []string{
		"massive", "extreme", "planet-scale", "hyperscale",
		"petabyte", "exabyte", "thousands of", "millions of", "critical production",
		"real-time processing", "ultra high", "maximum performance",
	}
	for _, kw := range extremeKeywords {
		if strings.Contains(text, kw) {
			return IntensityExtreme, kw
		}
	}
	
	// Heavy intensity keywords
	heavyKeywords := []string{
		"heavy", "intensive", "production", "enterprise", "large-scale", "large scale",
		"high-performance", "high performance", "demanding", "complex", "serious",
		"professional", "commercial", "mission-critical", "mission critical",
		"enterprise-grade",
		"high-traffic", "high traffic", "high-load", "high load", "high volume",
		"heavy-duty", "heavy duty", "compute-intensive", "compute intensive",
		"memory-intensive", "memory intensive", "resource-intensive", "resource intensive",
		"big", "huge", "major", "significant", "substantial", "considerable",
		"tuning", "optimization", "forecasting", "prediction", "modeling",
		"processing pipeline", "data pipeline", "etl", "real-time", "realtime",
	}
	for _, kw := range heavyKeywords {
		if strings.Contains(text, kw) {
			return IntensityHeavy, kw
		}
	}
	
	// Medium intensity keywords
	mediumKeywords := []string{
		"moderate", "standard", "typical", "normal", "regular", "average",
		"medium", "mid-size", "midsize",
	}
	for _, kw := range mediumKeywords {
		if strings.Contains(text, kw) {
			return IntensityMedium, kw
		}
	}
	
	// Light intensity keywords
	lightKeywords := []string{
		"light", "small", "tiny", "minimal", "basic",
		"test", "testing", "poc", "proof of concept", "prototype", "experimental",
		"hobby", "personal", "tutorial", "demo", "sandbox",
		"low-traffic", "low traffic", "occasional",
	}
	for _, kw := range lightKeywords {
		if strings.Contains(text, kw) {
			return IntensityLight, kw
		}
	}
	
	return IntensityDefault, ""
}

// detectDomainWorkload detects domain-specific workloads that imply HPC/scientific computing
func detectDomainWorkload(text string) (bool, string, string) {
	text = strings.ToLower(text)
	
	// Scientific/HPC domain keywords
	scientificDomains := map[string]string{
		// Weather & Climate
		"weather":           "Weather modeling/forecasting",
		"climate":           "Climate simulation",
		"meteorolog":        "Meteorological computing",
		"forecast":          "Forecasting/prediction",
		"atmospheric":       "Atmospheric simulation",
		
		// Physics & Engineering
		"physics":           "Physics simulation",
		"cfd":               "Computational Fluid Dynamics",
		"fluid dynamics":    "Fluid dynamics simulation",
		"finite element":    "Finite Element Analysis",
		"fea":               "Finite Element Analysis",
		"molecular dynamics": "Molecular dynamics",
		"quantum":           "Quantum computing",
		"particle":          "Particle physics",
		
		// Life Sciences
		"genomic":           "Genomics processing",
		"genome":            "Genome analysis",
		"bioinformatics":    "Bioinformatics",
		"protein folding":   "Protein folding",
		"drug discovery":    "Drug discovery",
		"molecular":         "Molecular simulation",
		"dna":               "DNA sequencing",
		"rna":               "RNA analysis",
		"sequencing":        "Sequence analysis",
		
		// Engineering
		"crash simulation":  "Crash simulation",
		"structural analysis": "Structural analysis",
		"seismic":           "Seismic analysis",
		"aerodynamic":       "Aerodynamics simulation",
		"combustion":        "Combustion modeling",
		"reservoir":         "Reservoir simulation",
		"oil and gas":       "Oil & gas simulation",
		
		// Finance
		"monte carlo":       "Monte Carlo simulation",
		"risk calculation":  "Risk calculation",
		"option pricing":    "Option pricing",
		"quant":             "Quantitative finance",
		"trading":           "Trading systems",
		"backtesting":       "Backtesting",
		
		// Other HPC
		"render farm":       "Render farm",
		"ray tracing":       "Ray tracing",
		"path tracing":      "Path tracing",
		"distributed computing": "Distributed computing",
		"parallel processing": "Parallel processing",
		"mpi":               "MPI workload",
		"hpc":               "High Performance Computing",
		"supercomputer":     "Supercomputing workload",
	}
	
	for keyword, description := range scientificDomains {
		if strings.Contains(text, keyword) {
			return true, keyword, description
		}
	}
	
	return false, "", ""
}

// applyIntensityMultiplier adjusts specs based on detected intensity
func applyIntensityMultiplier(resp *WorkloadRequirements, intensity WorkloadIntensity, matchedKeyword string, explanations *[]string) {
	switch intensity {
	case IntensityExtreme:
		// Extreme: 4x multiplier
		resp.MinVCPU = max(resp.MinVCPU*4, 64)
		resp.MaxVCPU = max(resp.MaxVCPU*4, 192)
		resp.MinMemory = max(resp.MinMemory*4, 256)
		resp.MaxMemory = max(resp.MaxMemory*4, 1024)
		*explanations = append(*explanations, fmt.Sprintf("Extreme workload detected ('%s'): scaled to 64+ vCPU, 256+ GB RAM", matchedKeyword))
		
	case IntensityHeavy:
		// Heavy: 2-3x multiplier
		resp.MinVCPU = max(resp.MinVCPU*2, 16)
		resp.MaxVCPU = max(resp.MaxVCPU*2, 64)
		resp.MinMemory = max(resp.MinMemory*2, 64)
		resp.MaxMemory = max(resp.MaxMemory*2, 256)
		*explanations = append(*explanations, fmt.Sprintf("Heavy workload detected ('%s'): scaled to 16+ vCPU, 64+ GB RAM", matchedKeyword))
		
	case IntensityMedium:
		// Medium: 1.5x multiplier
		resp.MinVCPU = max(int(float64(resp.MinVCPU)*1.5), 4)
		resp.MaxVCPU = max(int(float64(resp.MaxVCPU)*1.5), 16)
		resp.MinMemory = max(int(float64(resp.MinMemory)*1.5), 16)
		resp.MaxMemory = max(int(float64(resp.MaxMemory)*1.5), 64)
		*explanations = append(*explanations, fmt.Sprintf("Medium workload detected ('%s'): balanced resources", matchedKeyword))
		
	case IntensityLight:
		// Light: reduce resources
		resp.MinVCPU = max(resp.MinVCPU/2, 1)
		resp.MaxVCPU = max(resp.MaxVCPU/2, 4)
		resp.MinMemory = max(resp.MinMemory/2, 2)
		resp.MaxMemory = max(resp.MaxMemory/2, 16)
		resp.MaxInterruption = 3 // Allow higher interruption for cost savings
		*explanations = append(*explanations, fmt.Sprintf("Light workload detected ('%s'): optimized for cost", matchedKeyword))
	}
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// parseWithRules uses rule-based parsing (fallback)
func (p *NLPParser) parseWithRules(text string) *WorkloadRequirements {
	originalText := text
	text = strings.ToLower(text)
	resp := &WorkloadRequirements{
		MinVCPU:         2,
		MaxVCPU:         0,
		MinMemory:       4,
		MaxInterruption: 2,
	}

	var explanations []string

	// STEP 1: Detect workload intensity FIRST (before specific workload detection)
	intensity, intensityKeyword := detectWorkloadIntensity(text)
	
	// STEP 2: Check for domain-specific HPC/scientific workloads
	isScientific, _, scientificDesc := detectDomainWorkload(text)
	if isScientific {
		resp.MinVCPU = 32
		resp.MaxVCPU = 96
		resp.MinMemory = 128
		resp.MaxMemory = 512
		resp.UseCase = "hpc"
		resp.MaxInterruption = 2
		explanations = append(explanations, fmt.Sprintf(" %s detected: HPC workload (32-96 vCPU, 128-512GB RAM)", scientificDesc))
		// Still apply intensity multiplier on top
		if intensity >= IntensityHeavy {
			applyIntensityMultiplier(resp, intensity, intensityKeyword, &explanations)
		}
		p.extractExplicitNumbers(originalText, resp)
		p.extractArchitecture(originalText, resp)
		resp.Explanation = strings.Join(explanations, " | ")
		return resp
	}

	// STEP 3: Parse high-performance/specialized workloads
	if strings.Contains(text, "quantum") || strings.Contains(text, "hpc") || strings.Contains(text, "high performance") ||
		strings.Contains(text, "scientific") || strings.Contains(text, "simulation") || strings.Contains(text, "research") {
		resp.MinVCPU = 32
		resp.MaxVCPU = 96
		resp.MinMemory = 128
		resp.MaxMemory = 512
		resp.UseCase = "hpc"
		resp.MaxInterruption = 2
		explanations = append(explanations, "HPC/Scientific workload: high compute (32-96 vCPU, 128-512GB RAM)")
	} else if strings.Contains(text, "machine learning") || strings.Contains(text, "ml training") ||
		strings.Contains(text, "deep learning") || strings.Contains(text, "neural network") ||
		strings.Contains(text, "ai training") || strings.Contains(text, "model training") ||
		strings.Contains(text, "neural") || strings.Contains(text, "deep") && strings.Contains(text, "training") {
		resp.MinVCPU = 16
		resp.MaxVCPU = 64
		resp.MinMemory = 64
		resp.MaxMemory = 256
		resp.UseCase = "ml"
		resp.MaxInterruption = 2
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-t4"
		explanations = append(explanations, "ML/AI Training workload: compute-optimized (16-64 vCPU, 64-256GB RAM, GPU)")
	} else if strings.Contains(text, "llm") || strings.Contains(text, "gpt") || strings.Contains(text, "transformer") ||
		strings.Contains(text, "large language") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 64
		resp.MaxMemory = 256
		resp.UseCase = "ml"
		resp.MaxInterruption = 1
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-a10g"
		explanations = append(explanations, "LLM/Transformer workload: needs GPU (8-32 vCPU, 64-256GB RAM)")
	} else if containsWord(text, "ai") || strings.Contains(text, "inference") {
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
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-t4"
		explanations = append(explanations, "3D Rendering: high compute, GPU recommended (16-64 vCPU, 32-128GB RAM)")
	} else if strings.Contains(text, "ci") || strings.Contains(text, "cd") || strings.Contains(text, "build") ||
		strings.Contains(text, "jenkins") || strings.Contains(text, "github actions") || strings.Contains(text, "pipeline") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 16
		resp.MinMemory = 8
		resp.MaxMemory = 32
		resp.UseCase = "batch"
		resp.MaxInterruption = 3
		explanations = append(explanations, "CI/CD Build: cost-optimized (4-16 vCPU, 8-32GB RAM)")
	} else if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") {
		// Kubernetes - but check intensity!
		resp.UseCase = "kubernetes"
		resp.MaxInterruption = 1
		if intensity >= IntensityHeavy {
			// Heavy Kubernetes workload
			resp.MinVCPU = 16
			resp.MaxVCPU = 64
			resp.MinMemory = 64
			resp.MaxMemory = 256
			explanations = append(explanations, "Heavy Kubernetes workload: production-grade (16-64 vCPU, 64-256GB RAM)")
		} else if intensity == IntensityMedium {
			resp.MinVCPU = 8
			resp.MaxVCPU = 32
			resp.MinMemory = 32
			resp.MaxMemory = 128
			explanations = append(explanations, "Medium Kubernetes workload: balanced (8-32 vCPU, 32-128GB RAM)")
		} else {
			resp.MinVCPU = 4
			resp.MaxVCPU = 16
			resp.MinMemory = 8
			resp.MaxMemory = 32
			explanations = append(explanations, "Kubernetes cluster: standard nodes (4-16 vCPU, 8-32GB RAM)")
		}
		// Skip intensity multiplier since we already handled it
		intensity = IntensityDefault
	} else if strings.Contains(text, "database") || strings.Contains(text, "db") || strings.Contains(text, "postgres") ||
		strings.Contains(text, "mysql") || strings.Contains(text, "mongo") || strings.Contains(text, "redis") {
		resp.UseCase = "database"
		resp.MaxInterruption = 0
		if intensity >= IntensityHeavy {
			resp.MinVCPU = 16
			resp.MaxVCPU = 64
			resp.MinMemory = 128
			resp.MaxMemory = 512
			explanations = append(explanations, "Heavy Database workload: high-memory (16-64 vCPU, 128-512GB RAM)")
		} else {
			resp.MinVCPU = 4
			resp.MaxVCPU = 16
			resp.MinMemory = 32
			resp.MaxMemory = 128
			explanations = append(explanations, "Database: memory-optimized with stability (4-16 vCPU, 32-128GB RAM)")
		}
		intensity = IntensityDefault
	} else {
		// No specific workload detected - parse by size keywords
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

	// Parse use cases (only if not set)
	if resp.UseCase == "" {
		if strings.Contains(text, "autoscaling") || strings.Contains(text, "asg") || strings.Contains(text, "auto scaling") {
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

	// STEP 4: Apply intensity multiplier (if not already handled by specific workload)
	if intensity != IntensityDefault && intensityKeyword != "" {
		applyIntensityMultiplier(resp, intensity, intensityKeyword, &explanations)
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

	// Check for GPU keywords
	if strings.Contains(text, "gpu") || strings.Contains(text, "nvidia") || strings.Contains(text, "cuda") {
		resp.NeedsGPU = true
		if strings.Contains(text, "a100") {
			resp.GPUType = "nvidia-a100"
		} else if strings.Contains(text, "v100") {
			resp.GPUType = "nvidia-v100"
		} else if strings.Contains(text, "a10") {
			resp.GPUType = "nvidia-a10g"
		} else {
			resp.GPUType = "nvidia-t4"
		}
		explanations = append(explanations, fmt.Sprintf("GPU required (%s)", resp.GPUType))
	}

	// Extract explicit numbers from text (override previous settings)
	p.extractExplicitNumbers(originalText, resp)

	if len(explanations) == 0 {
		resp.Explanation = "Using default settings: 2+ vCPU, 4GB+ RAM, moderate stability"
	} else {
		resp.Explanation = strings.Join(explanations, " | ")
	}

	return resp
}
