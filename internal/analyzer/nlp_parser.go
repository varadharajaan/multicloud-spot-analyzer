// Package analyzer provides AI-powered natural language parsing for workload requirements.
package analyzer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// NLPParser provides AI-powered natural language parsing using free public APIs
type NLPParser struct {
	httpClient *http.Client
}

// NewNLPParser creates a new NLP parser (no API key required)
func NewNLPParser() *NLPParser {
	return &NLPParser{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Parse analyzes natural language and returns workload requirements
// Uses free Hugging Face zero-shot classification, falls back to enhanced rule-based parsing
func (p *NLPParser) Parse(text string) (*WorkloadRequirements, error) {
	// Try AI classification first
	result, err := p.parseWithFreeAI(text)
	if err == nil && result != nil {
		return result, nil
	}

	// Fall back to enhanced rule-based parsing
	return p.parseWithRules(text), nil
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
		result.Explanation = fmt.Sprintf("🤖 AI detected HPC/Scientific workload (%.0f%% confidence): 32-96 vCPU, 128-512GB RAM recommended", score*100)

	case strings.Contains(label, "machine learning") || strings.Contains(label, "deep learning") || strings.Contains(label, "neural"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.MaxInterruption = 2
		result.NeedsGPU = true
		result.GPUType = "nvidia-t4"
		result.Explanation = fmt.Sprintf("🤖 AI detected ML Training workload (%.0f%% confidence): 16-64 vCPU, 64-256GB RAM, GPU recommended", score*100)

	case strings.Contains(label, "large language") || strings.Contains(label, "gpt") || strings.Contains(label, "inference"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.MaxInterruption = 1
		result.NeedsGPU = true
		result.GPUType = "nvidia-a10g"
		result.Explanation = fmt.Sprintf("🤖 AI detected LLM/Inference workload (%.0f%% confidence): 8-32 vCPU, 64-256GB RAM, GPU recommended", score*100)

	case strings.Contains(label, "data analytics") || strings.Contains(label, "big data"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 32
		result.MaxMemory = 128
		result.UseCase = "batch"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("🤖 AI detected Data Analytics workload (%.0f%% confidence): 8-32 vCPU, 32-128GB RAM", score*100)

	case strings.Contains(label, "video") || strings.Contains(label, "media"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 16
		result.MaxMemory = 64
		result.UseCase = "batch"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("🤖 AI detected Media Processing workload (%.0f%% confidence): 8-32 vCPU, 16-64GB RAM", score*100)

	case strings.Contains(label, "gaming") || strings.Contains(label, "game"):
		result.MinVCPU = 4
		result.MaxVCPU = 16
		result.MinMemory = 16
		result.MaxMemory = 64
		result.UseCase = "general"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("🤖 AI detected Gaming workload (%.0f%% confidence): 4-16 vCPU, 16-64GB RAM, low latency", score*100)

	case strings.Contains(label, "3d") || strings.Contains(label, "rendering") || strings.Contains(label, "graphics"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 32
		result.MaxMemory = 128
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.NeedsGPU = true
		result.GPUType = "nvidia-t4"
		result.Explanation = fmt.Sprintf("🤖 AI detected 3D Rendering workload (%.0f%% confidence): 16-64 vCPU, 32-128GB RAM, GPU", score*100)

	case strings.Contains(label, "ci") || strings.Contains(label, "cd") || strings.Contains(label, "build"):
		result.MinVCPU = 4
		result.MaxVCPU = 16
		result.MinMemory = 8
		result.MaxMemory = 32
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("🤖 AI detected CI/CD workload (%.0f%% confidence): 4-16 vCPU, 8-32GB RAM, cost-optimized", score*100)

	case strings.Contains(label, "kubernetes") || strings.Contains(label, "container"):
		result.MinVCPU = 2
		result.MaxVCPU = 8
		result.MinMemory = 4
		result.MaxMemory = 32
		result.UseCase = "kubernetes"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("🤖 AI detected Kubernetes workload (%.0f%% confidence): 2-8 vCPU, 4-32GB RAM, stable", score*100)

	case strings.Contains(label, "database") || strings.Contains(label, "storage"):
		result.MinVCPU = 2
		result.MaxVCPU = 16
		result.MinMemory = 8
		result.MaxMemory = 64
		result.UseCase = "database"
		result.MaxInterruption = 0
		result.Explanation = fmt.Sprintf("🤖 AI detected Database workload (%.0f%% confidence): 2-16 vCPU, 8-64GB RAM, max stability", score*100)

	case strings.Contains(label, "web") || strings.Contains(label, "api"):
		result.MinVCPU = 2
		result.MaxVCPU = 4
		result.MinMemory = 4
		result.MaxMemory = 16
		result.UseCase = "general"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("🤖 AI detected Web/API workload (%.0f%% confidence): 2-4 vCPU, 4-16GB RAM", score*100)

	case strings.Contains(label, "small") || strings.Contains(label, "development") || strings.Contains(label, "testing"):
		result.MinVCPU = 1
		result.MaxVCPU = 2
		result.MinMemory = 2
		result.MaxMemory = 8
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("🤖 AI detected Dev/Test workload (%.0f%% confidence): 1-2 vCPU, 2-8GB RAM, cost-optimized", score*100)

	default:
		result.Explanation = fmt.Sprintf("🤖 AI analysis (%.0f%% confidence): Using balanced defaults", score*100)
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

	//

	var explanations []string

	// Parse high-performance/specialized workloads FIRST
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
		strings.Contains(text, "ai training") || strings.Contains(text, "model training") {
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
	} else {
		// Parse by size keywords
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
		if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") {
			resp.UseCase = "kubernetes"
			resp.MinMemory = 4
			resp.MaxInterruption = 1
			explanations = append(explanations, "Kubernetes use case: prioritizing stability")
		} else if strings.Contains(text, "database") || strings.Contains(text, "db") || strings.Contains(text, "postgres") ||
			strings.Contains(text, "mysql") || strings.Contains(text, "mongo") || strings.Contains(text, "redis") {
			resp.UseCase = "database"
			resp.MinMemory = 8
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
