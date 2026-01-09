package nlp

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RulesProvider implements NLP using rule-based pattern matching (fallback)
type RulesProvider struct{}

// NewRulesProvider creates a new rules-based NLP provider
func NewRulesProvider() *RulesProvider {
	return &RulesProvider{}
}

func (p *RulesProvider) Name() string {
	return "rules"
}

func (p *RulesProvider) IsAvailable(ctx context.Context) bool {
	return true // Always available
}

func (p *RulesProvider) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	return p.parseWithRules(text), nil
}

// parseWithRules uses enhanced rule-based parsing
func (p *RulesProvider) parseWithRules(text string) *WorkloadRequirements {
	originalText := text
	text = strings.ToLower(text)
	
	resp := &WorkloadRequirements{
		MinVCPU:         2,
		MaxVCPU:         0,
		MinMemory:       4,
		MaxInterruption: 2,
	}

	var explanations []string

	// STEP 1: Detect workload intensity
	intensity, intensityKeyword := p.detectIntensity(text)

	// STEP 2: Check for domain-specific HPC workloads
	if isHPC, _, desc := p.detectHPCDomain(text); isHPC {
		resp.MinVCPU = 32
		resp.MaxVCPU = 96
		resp.MinMemory = 128
		resp.MaxMemory = 512
		resp.UseCase = "hpc"
		resp.MaxInterruption = 2
		explanations = append(explanations, fmt.Sprintf("%s detected: HPC workload (32-96 vCPU, 128-512GB RAM)", desc))
		
		if intensity >= intensityHeavy {
			p.applyIntensityMultiplier(resp, intensity, intensityKeyword, &explanations)
		}
		p.extractNumbers(originalText, resp)
		p.extractArchitecture(text, resp)
		resp.Explanation = strings.Join(explanations, " | ")
		return resp
	}

	// STEP 3: Match workload patterns
	matched := p.matchWorkloadPatterns(text, resp, intensity, intensityKeyword, &explanations)

	// STEP 4: Apply intensity if not already handled
	if !matched && intensity != intensityDefault && intensityKeyword != "" {
		p.applyIntensityMultiplier(resp, intensity, intensityKeyword, &explanations)
	}

	// STEP 5: Parse architecture and GPU
	p.extractArchitecture(text, resp)
	p.extractGPU(text, resp, &explanations)

	// STEP 6: Extract explicit numbers (override)
	p.extractNumbers(originalText, resp)

	if len(explanations) == 0 {
		resp.Explanation = "[Rules] Default settings: 2+ vCPU, 4GB+ RAM"
	} else {
		resp.Explanation = "[Rules] " + strings.Join(explanations, " | ")
	}

	return resp
}

type workloadIntensity int

const (
	intensityDefault workloadIntensity = iota
	intensityLight
	intensityMedium
	intensityHeavy
	intensityExtreme
)

func (p *RulesProvider) detectIntensity(text string) (workloadIntensity, string) {
	// Extreme
	for _, kw := range []string{"massive", "extreme", "planet-scale", "hyperscale", "petabyte", "exabyte"} {
		if strings.Contains(text, kw) {
			return intensityExtreme, kw
		}
	}
	
	// Heavy
	for _, kw := range []string{
		"heavy", "intensive", "production", "enterprise", "large-scale",
		"high-performance", "demanding", "mission-critical", "commercial",
		"high-traffic", "high-load", "compute-intensive", "memory-intensive",
		"tuning", "optimization", "forecasting", "prediction", "modeling",
		"real-time", "realtime", "etl", "data pipeline",
	} {
		if strings.Contains(text, kw) {
			return intensityHeavy, kw
		}
	}
	
	// Medium
	for _, kw := range []string{"moderate", "standard", "typical", "normal", "medium"} {
		if strings.Contains(text, kw) {
			return intensityMedium, kw
		}
	}
	
	// Light
	for _, kw := range []string{
		"light", "small", "tiny", "minimal", "basic", "simple",
		"dev", "development", "test", "testing", "poc", "prototype",
		"hobby", "personal", "learning", "demo", "sandbox",
	} {
		if strings.Contains(text, kw) {
			return intensityLight, kw
		}
	}
	
	return intensityDefault, ""
}

func (p *RulesProvider) detectHPCDomain(text string) (bool, string, string) {
	domains := map[string]string{
		"weather":            "Weather modeling",
		"climate":            "Climate simulation",
		"forecast":           "Forecasting",
		"genomic":            "Genomics",
		"bioinformatics":     "Bioinformatics",
		"protein folding":    "Protein folding",
		"molecular dynamics": "Molecular dynamics",
		"cfd":                "CFD simulation",
		"fluid dynamics":     "Fluid dynamics",
		"finite element":     "FEA",
		"monte carlo":        "Monte Carlo",
		"quantum":            "Quantum computing",
		"physics":            "Physics simulation",
		"seismic":            "Seismic analysis",
		"reservoir":          "Reservoir simulation",
		"hpc":                "HPC",
		"supercomputer":      "Supercomputing",
	}
	
	for kw, desc := range domains {
		if strings.Contains(text, kw) {
			return true, kw, desc
		}
	}
	return false, "", ""
}

func (p *RulesProvider) matchWorkloadPatterns(text string, resp *WorkloadRequirements, 
	intensity workloadIntensity, intensityKw string, explanations *[]string) bool {
	
	// ML/AI patterns
	if strings.Contains(text, "machine learning") || strings.Contains(text, "deep learning") ||
		strings.Contains(text, "neural network") || strings.Contains(text, "ml training") {
		resp.MinVCPU = 16
		resp.MaxVCPU = 64
		resp.MinMemory = 64
		resp.MaxMemory = 256
		resp.UseCase = "ml"
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-t4"
		*explanations = append(*explanations, "ML Training: 16-64 vCPU, 64-256GB, GPU")
		return true
	}
	
	if strings.Contains(text, "llm") || strings.Contains(text, "gpt") || 
		strings.Contains(text, "transformer") || strings.Contains(text, "large language") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 64
		resp.MaxMemory = 256
		resp.UseCase = "ml"
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-a10g"
		resp.MaxInterruption = 1
		*explanations = append(*explanations, "LLM Inference: 8-32 vCPU, 64-256GB, GPU")
		return true
	}

	// Kubernetes - intensity-aware
	if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") {
		resp.UseCase = "kubernetes"
		resp.MaxInterruption = 1
		
		if intensity >= intensityHeavy {
			resp.MinVCPU = 16
			resp.MaxVCPU = 64
			resp.MinMemory = 64
			resp.MaxMemory = 256
			*explanations = append(*explanations, "Heavy K8s: 16-64 vCPU, 64-256GB")
		} else if intensity == intensityMedium {
			resp.MinVCPU = 8
			resp.MaxVCPU = 32
			resp.MinMemory = 32
			resp.MaxMemory = 128
			*explanations = append(*explanations, "Medium K8s: 8-32 vCPU, 32-128GB")
		} else if intensity == intensityLight {
			resp.MinVCPU = 2
			resp.MaxVCPU = 4
			resp.MinMemory = 4
			resp.MaxMemory = 16
			resp.MaxInterruption = 2
			*explanations = append(*explanations, "Light K8s: 2-4 vCPU, 4-16GB")
		} else {
			resp.MinVCPU = 4
			resp.MaxVCPU = 16
			resp.MinMemory = 8
			resp.MaxMemory = 32
			*explanations = append(*explanations, "Standard K8s: 4-16 vCPU, 8-32GB")
		}
		return true
	}

	// Database - intensity-aware
	if strings.Contains(text, "database") || strings.Contains(text, "postgres") ||
		strings.Contains(text, "mysql") || strings.Contains(text, "mongo") || strings.Contains(text, "redis") {
		resp.UseCase = "database"
		resp.MaxInterruption = 0
		
		if intensity >= intensityHeavy {
			resp.MinVCPU = 16
			resp.MaxVCPU = 64
			resp.MinMemory = 128
			resp.MaxMemory = 512
			*explanations = append(*explanations, "Heavy DB: 16-64 vCPU, 128-512GB, max stability")
		} else {
			resp.MinVCPU = 4
			resp.MaxVCPU = 16
			resp.MinMemory = 32
			resp.MaxMemory = 128
			*explanations = append(*explanations, "Standard DB: 4-16 vCPU, 32-128GB, max stability")
		}
		return true
	}

	// Analytics/Big Data
	if strings.Contains(text, "analytics") || strings.Contains(text, "big data") ||
		strings.Contains(text, "spark") || strings.Contains(text, "hadoop") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 32
		resp.MaxMemory = 128
		resp.UseCase = "batch"
		*explanations = append(*explanations, "Analytics: 8-32 vCPU, 32-128GB")
		return true
	}

	// Video/Media
	if strings.Contains(text, "video") || strings.Contains(text, "encoding") ||
		strings.Contains(text, "transcoding") || strings.Contains(text, "media") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 16
		resp.MaxMemory = 64
		resp.UseCase = "batch"
		*explanations = append(*explanations, "Media processing: 8-32 vCPU, 16-64GB")
		return true
	}

	// CI/CD
	if strings.Contains(text, "ci/cd") || strings.Contains(text, "jenkins") ||
		strings.Contains(text, "github actions") || strings.Contains(text, "build pipeline") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 16
		resp.MinMemory = 8
		resp.MaxMemory = 32
		resp.UseCase = "batch"
		resp.MaxInterruption = 3
		*explanations = append(*explanations, "CI/CD: 4-16 vCPU, 8-32GB, cost-optimized")
		return true
	}

	// Gaming
	if strings.Contains(text, "gaming") || strings.Contains(text, "game server") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 16
		resp.MinMemory = 16
		resp.MaxMemory = 64
		resp.UseCase = "general"
		resp.MaxInterruption = 1
		*explanations = append(*explanations, "Gaming: 4-16 vCPU, 16-64GB, low latency")
		return true
	}

	// 3D Rendering
	if strings.Contains(text, "rendering") || strings.Contains(text, "3d") {
		resp.MinVCPU = 16
		resp.MaxVCPU = 64
		resp.MinMemory = 32
		resp.MaxMemory = 128
		resp.UseCase = "batch"
		resp.NeedsGPU = true
		resp.GPUType = "nvidia-t4"
		*explanations = append(*explanations, "3D Rendering: 16-64 vCPU, 32-128GB, GPU")
		return true
	}

	// Web/API
	if strings.Contains(text, "web") || strings.Contains(text, "api") || 
		strings.Contains(text, "microservice") {
		resp.MinVCPU = 2
		resp.MaxVCPU = 8
		resp.MinMemory = 4
		resp.MaxMemory = 16
		resp.UseCase = "general"
		*explanations = append(*explanations, "Web/API: 2-8 vCPU, 4-16GB")
		return true
	}

	return false
}

func (p *RulesProvider) applyIntensityMultiplier(resp *WorkloadRequirements, 
	intensity workloadIntensity, keyword string, explanations *[]string) {
	
	switch intensity {
	case intensityExtreme:
		resp.MinVCPU = maxInt(resp.MinVCPU*4, 64)
		resp.MaxVCPU = maxInt(resp.MaxVCPU*4, 192)
		resp.MinMemory = maxInt(resp.MinMemory*4, 256)
		resp.MaxMemory = maxInt(resp.MaxMemory*4, 1024)
		*explanations = append(*explanations, fmt.Sprintf("Extreme ('%s'): 64+ vCPU, 256+ GB", keyword))
		
	case intensityHeavy:
		resp.MinVCPU = maxInt(resp.MinVCPU*2, 16)
		resp.MaxVCPU = maxInt(resp.MaxVCPU*2, 64)
		resp.MinMemory = maxInt(resp.MinMemory*2, 64)
		resp.MaxMemory = maxInt(resp.MaxMemory*2, 256)
		*explanations = append(*explanations, fmt.Sprintf("Heavy ('%s'): 16+ vCPU, 64+ GB", keyword))
		
	case intensityLight:
		resp.MinVCPU = maxInt(resp.MinVCPU/2, 1)
		resp.MaxVCPU = maxInt(resp.MaxVCPU/2, 4)
		resp.MinMemory = maxInt(resp.MinMemory/2, 2)
		resp.MaxMemory = maxInt(resp.MaxMemory/2, 16)
		resp.MaxInterruption = 3
		*explanations = append(*explanations, fmt.Sprintf("Light ('%s'): cost-optimized", keyword))
	}
}

func (p *RulesProvider) extractNumbers(text string, resp *WorkloadRequirements) {
	text = strings.ToLower(text)

	// CPU patterns
	cpuRe := regexp.MustCompile(`(\d+)\s*(?:v?cpus?|cores?)`)
	if m := cpuRe.FindStringSubmatch(text); len(m) > 1 {
		if num, _ := strconv.Atoi(m[1]); num > 0 && num <= 192 {
			resp.MinVCPU = num
			if resp.MaxVCPU == 0 || resp.MaxVCPU < num {
				resp.MaxVCPU = num + 4
			}
		}
	}

	// Memory patterns
	memRe := regexp.MustCompile(`(\d+)\s*(?:gb|gib)`)
	if m := memRe.FindStringSubmatch(text); len(m) > 1 {
		if num, _ := strconv.Atoi(m[1]); num > 0 && num <= 1024 {
			resp.MinMemory = num
			if resp.MaxMemory == 0 || resp.MaxMemory < num {
				resp.MaxMemory = num * 2
			}
		}
	}
}

func (p *RulesProvider) extractArchitecture(text string, resp *WorkloadRequirements) {
	if strings.Contains(text, "intel") {
		resp.Architecture = "intel"
	} else if strings.Contains(text, "amd") {
		resp.Architecture = "amd"
	} else if strings.Contains(text, "arm") || strings.Contains(text, "graviton") {
		resp.Architecture = "arm64"
	}
}

func (p *RulesProvider) extractGPU(text string, resp *WorkloadRequirements, explanations *[]string) {
	if strings.Contains(text, "gpu") || strings.Contains(text, "nvidia") || strings.Contains(text, "cuda") {
		resp.NeedsGPU = true
		if strings.Contains(text, "a100") {
			resp.GPUType = "nvidia-a100"
		} else if strings.Contains(text, "v100") {
			resp.GPUType = "nvidia-v100"
		} else if strings.Contains(text, "a10") {
			resp.GPUType = "nvidia-a10g"
		} else if resp.GPUType == "" {
			resp.GPUType = "nvidia-t4"
		}
		*explanations = append(*explanations, fmt.Sprintf("GPU: %s", resp.GPUType))
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
