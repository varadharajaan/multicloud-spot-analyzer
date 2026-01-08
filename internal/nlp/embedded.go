// Package nlp provides an embedded NLP provider using pure Go ML
// This implementation uses a lightweight text classification approach
// without external dependencies like Ollama or cloud APIs.
package nlp

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// EmbeddedProvider implements NLP using pure Go text classification
// Uses TF-IDF based classification with predefined workload categories
type EmbeddedProvider struct {
	classifier *WorkloadClassifier
}

// NewEmbeddedProvider creates a new embedded NLP provider
func NewEmbeddedProvider() *EmbeddedProvider {
	return &EmbeddedProvider{
		classifier: NewWorkloadClassifier(),
	}
}

func (p *EmbeddedProvider) Name() string {
	return "embedded"
}

func (p *EmbeddedProvider) IsAvailable(ctx context.Context) bool {
	return true // Always available - no external dependencies
}

func (p *EmbeddedProvider) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	// Classify the workload
	classification := p.classifier.Classify(text)
	
	// Map classification to requirements
	result := p.mapClassificationToRequirements(classification, text)
	
	return result, nil
}

// WorkloadClassifier uses TF-IDF based text classification
type WorkloadClassifier struct {
	categories []WorkloadCategory
	idfWeights map[string]float64
}

// WorkloadCategory represents a workload type with its characteristics
type WorkloadCategory struct {
	Name           string
	Keywords       []string
	KeywordWeights map[string]float64
	MinVCPU        int
	MaxVCPU        int
	MinMemory      int
	MaxMemory      int
	UseCase        string
	MaxInterruption int
	NeedsGPU       bool
	GPUType        string
}

// NewWorkloadClassifier creates a trained workload classifier
func NewWorkloadClassifier() *WorkloadClassifier {
	c := &WorkloadClassifier{
		idfWeights: make(map[string]float64),
	}
	
	// Define workload categories with weighted keywords
	c.categories = []WorkloadCategory{
		{
			Name: "hpc",
			Keywords: []string{
				"hpc", "high performance", "scientific", "simulation", "research",
				"quantum", "supercomputer", "parallel", "mpi", "cluster computing",
				"weather", "climate", "forecast", "atmospheric", "meteorolog",
				"genomic", "genome", "bioinformatics", "protein", "sequencing", "dna", "rna",
				"physics", "cfd", "fluid dynamics", "finite element", "fea", "molecular",
				"monte carlo", "seismic", "reservoir", "aerodynamic", "combustion",
				"ray tracing", "path tracing", "render farm",
			},
			KeywordWeights: map[string]float64{
				"quantum": 2.0, "hpc": 2.0, "supercomputer": 2.0,
				"weather": 1.8, "climate": 1.8, "forecast": 1.5,
				"genomic": 1.8, "bioinformatics": 1.8,
				"monte carlo": 1.5, "simulation": 1.3,
			},
			MinVCPU: 32, MaxVCPU: 96, MinMemory: 128, MaxMemory: 512,
			UseCase: "hpc", MaxInterruption: 2,
		},
		{
			Name: "ml_training",
			Keywords: []string{
				"machine learning", "ml training", "deep learning", "neural network",
				"ai training", "model training", "tensorflow", "pytorch", "keras",
				"training pipeline", "hyperparameter", "gradient descent",
				"backpropagation", "epoch", "batch size", "loss function",
			},
			KeywordWeights: map[string]float64{
				"deep learning": 2.0, "neural network": 1.8,
				"machine learning": 1.5, "training": 1.3,
			},
			MinVCPU: 16, MaxVCPU: 64, MinMemory: 64, MaxMemory: 256,
			UseCase: "ml", MaxInterruption: 2, NeedsGPU: true, GPUType: "nvidia-t4",
		},
		{
			Name: "llm_inference",
			Keywords: []string{
				"llm", "gpt", "transformer", "large language", "inference",
				"chatbot", "text generation", "embedding", "vector database",
				"rag", "retrieval", "prompt", "fine-tuning", "lora",
			},
			KeywordWeights: map[string]float64{
				"llm": 2.0, "gpt": 2.0, "transformer": 1.8,
				"large language": 1.8, "inference": 1.3,
			},
			MinVCPU: 8, MaxVCPU: 32, MinMemory: 64, MaxMemory: 256,
			UseCase: "ml", MaxInterruption: 1, NeedsGPU: true, GPUType: "nvidia-a10g",
		},
		{
			Name: "kubernetes",
			Keywords: []string{
				"kubernetes", "k8s", "container", "pod", "deployment",
				"helm", "kubectl", "ingress", "service mesh", "istio",
				"node pool", "cluster", "orchestration",
			},
			KeywordWeights: map[string]float64{
				"kubernetes": 2.0, "k8s": 2.0,
				"container": 1.3, "pod": 1.3,
			},
			MinVCPU: 4, MaxVCPU: 16, MinMemory: 8, MaxMemory: 32,
			UseCase: "kubernetes", MaxInterruption: 1,
		},
		{
			Name: "database",
			Keywords: []string{
				"database", "db", "postgres", "postgresql", "mysql", "mariadb",
				"mongodb", "redis", "elasticsearch", "cassandra", "dynamodb",
				"sql", "nosql", "replication", "sharding", "backup",
			},
			KeywordWeights: map[string]float64{
				"database": 2.0, "postgres": 1.8, "mysql": 1.8,
				"mongodb": 1.8, "redis": 1.5,
			},
			MinVCPU: 4, MaxVCPU: 16, MinMemory: 32, MaxMemory: 128,
			UseCase: "database", MaxInterruption: 0,
		},
		{
			Name: "analytics",
			Keywords: []string{
				"analytics", "data science", "big data", "spark", "hadoop",
				"etl", "data pipeline", "warehouse", "olap", "business intelligence",
				"data lake", "presto", "athena", "redshift", "snowflake",
			},
			KeywordWeights: map[string]float64{
				"analytics": 1.8, "big data": 1.8, "spark": 1.5,
				"data science": 1.5, "etl": 1.3,
			},
			MinVCPU: 8, MaxVCPU: 32, MinMemory: 32, MaxMemory: 128,
			UseCase: "batch", MaxInterruption: 2,
		},
		{
			Name: "media",
			Keywords: []string{
				"video", "encoding", "transcoding", "streaming", "media",
				"ffmpeg", "audio", "codec", "broadcast", "live streaming",
				"vod", "cdn", "hls", "dash",
			},
			KeywordWeights: map[string]float64{
				"video": 1.8, "encoding": 1.5, "transcoding": 1.5,
				"streaming": 1.3, "media": 1.2,
			},
			MinVCPU: 8, MaxVCPU: 32, MinMemory: 16, MaxMemory: 64,
			UseCase: "batch", MaxInterruption: 2,
		},
		{
			Name: "cicd",
			Keywords: []string{
				"ci", "cd", "cicd", "build", "pipeline", "jenkins", "github actions",
				"gitlab", "circleci", "travis", "automation", "deploy",
				"continuous integration", "continuous delivery",
			},
			KeywordWeights: map[string]float64{
				"ci": 1.5, "cd": 1.5, "build": 1.3,
				"pipeline": 1.3, "jenkins": 1.5,
			},
			MinVCPU: 4, MaxVCPU: 16, MinMemory: 8, MaxMemory: 32,
			UseCase: "batch", MaxInterruption: 3,
		},
		{
			Name: "gaming",
			Keywords: []string{
				"gaming", "game server", "multiplayer", "game hosting",
				"minecraft", "steam", "dedicated server", "game engine",
			},
			KeywordWeights: map[string]float64{
				"gaming": 2.0, "game server": 2.0,
				"multiplayer": 1.5, "game": 1.2,
			},
			MinVCPU: 4, MaxVCPU: 16, MinMemory: 16, MaxMemory: 64,
			UseCase: "general", MaxInterruption: 1,
		},
		{
			Name: "rendering",
			Keywords: []string{
				"rendering", "3d", "graphics", "blender", "maya", "arnold",
				"gpu rendering", "vray", "octane", "unreal", "unity",
			},
			KeywordWeights: map[string]float64{
				"rendering": 2.0, "3d": 1.8, "graphics": 1.5,
				"blender": 1.5, "gpu rendering": 2.0,
			},
			MinVCPU: 16, MaxVCPU: 64, MinMemory: 32, MaxMemory: 128,
			UseCase: "batch", MaxInterruption: 3, NeedsGPU: true, GPUType: "nvidia-t4",
		},
		{
			Name: "web",
			Keywords: []string{
				"web", "api", "server", "http", "rest", "graphql",
				"nginx", "apache", "load balancer", "microservice",
				"frontend", "backend", "application server",
			},
			KeywordWeights: map[string]float64{
				"web": 1.5, "api": 1.5, "server": 1.2,
				"microservice": 1.3,
			},
			MinVCPU: 2, MaxVCPU: 8, MinMemory: 4, MaxMemory: 16,
			UseCase: "general", MaxInterruption: 2,
		},
		{
			Name: "dev_test",
			Keywords: []string{
				"development", "dev", "testing", "test", "staging",
				"sandbox", "poc", "prototype", "demo", "learning",
				"tutorial", "hobby", "personal", "experimental",
			},
			KeywordWeights: map[string]float64{
				"development": 1.5, "testing": 1.5, "dev": 1.3,
				"poc": 1.5, "sandbox": 1.5,
			},
			MinVCPU: 1, MaxVCPU: 4, MinMemory: 2, MaxMemory: 8,
			UseCase: "batch", MaxInterruption: 3,
		},
	}
	
	// Pre-compute IDF weights
	c.computeIDFWeights()
	
	return c
}

// computeIDFWeights calculates inverse document frequency for keywords
func (c *WorkloadClassifier) computeIDFWeights() {
	docFreq := make(map[string]int)
	totalDocs := len(c.categories)
	
	for _, cat := range c.categories {
		seen := make(map[string]bool)
		for _, kw := range cat.Keywords {
			words := strings.Fields(strings.ToLower(kw))
			for _, w := range words {
				if !seen[w] {
					docFreq[w]++
					seen[w] = true
				}
			}
		}
	}
	
	for word, freq := range docFreq {
		c.idfWeights[word] = math.Log(float64(totalDocs+1) / float64(freq+1))
	}
}

// ClassificationResult holds the classification output
type ClassificationResult struct {
	Category   string
	Confidence float64
	Scores     map[string]float64
	Intensity  string
}

// Classify determines the workload category for the given text
func (c *WorkloadClassifier) Classify(text string) ClassificationResult {
	text = strings.ToLower(text)
	words := tokenize(text)
	
	// Calculate TF for input text
	tf := make(map[string]float64)
	for _, w := range words {
		tf[w]++
	}
	for w := range tf {
		tf[w] = 1 + math.Log(tf[w]) // Log normalization
	}
	
	// Score each category
	scores := make(map[string]float64)
	
	for _, cat := range c.categories {
		score := 0.0
		
		// Check for keyword matches
		for _, kw := range cat.Keywords {
			kwLower := strings.ToLower(kw)
			
			// Phrase matching (for multi-word keywords)
			if strings.Contains(text, kwLower) {
				weight := 1.0
				if w, ok := cat.KeywordWeights[kw]; ok {
					weight = w
				}
				score += weight * 2.0 // Boost for exact phrase match
			}
			
			// Individual word matching with TF-IDF
			kwWords := strings.Fields(kwLower)
			for _, w := range kwWords {
				if tfVal, ok := tf[w]; ok {
					idf := c.idfWeights[w]
					if idf == 0 {
						idf = 1.0
					}
					score += tfVal * idf
				}
			}
		}
		
		scores[cat.Name] = score
	}
	
	// Find best category
	var bestCat string
	var bestScore float64
	for cat, score := range scores {
		if score > bestScore {
			bestScore = score
			bestCat = cat
		}
	}
	
	// Calculate confidence (normalized score)
	totalScore := 0.0
	for _, score := range scores {
		totalScore += score
	}
	confidence := 0.0
	if totalScore > 0 {
		confidence = bestScore / totalScore
	}
	
	// Detect intensity
	intensity := c.detectIntensity(text)
	
	return ClassificationResult{
		Category:   bestCat,
		Confidence: confidence,
		Scores:     scores,
		Intensity:  intensity,
	}
}

// detectIntensity determines the workload intensity level
func (c *WorkloadClassifier) detectIntensity(text string) string {
	extremeKeywords := []string{
		"massive", "extreme", "planet-scale", "hyperscale", "petabyte", "exabyte",
	}
	heavyKeywords := []string{
		"heavy", "intensive", "production", "enterprise", "large-scale",
		"high-performance", "demanding", "mission-critical", "high-traffic",
		"compute-intensive", "memory-intensive", "real-time",
	}
	lightKeywords := []string{
		"light", "small", "tiny", "minimal", "basic", "simple",
		"dev", "development", "test", "testing", "poc", "prototype",
		"sandbox", "hobby", "personal", "learning", "demo",
	}
	
	for _, kw := range extremeKeywords {
		if strings.Contains(text, kw) {
			return "extreme"
		}
	}
	for _, kw := range heavyKeywords {
		if strings.Contains(text, kw) {
			return "heavy"
		}
	}
	for _, kw := range lightKeywords {
		if strings.Contains(text, kw) {
			return "light"
		}
	}
	
	return "default"
}

// tokenize splits text into words and cleans them
func tokenize(text string) []string {
	// Remove punctuation
	re := regexp.MustCompile(`[^\w\s]`)
	text = re.ReplaceAllString(text, " ")
	
	// Split into words
	words := strings.Fields(strings.ToLower(text))
	
	// Filter stop words (minimal set)
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "my": true, "your": true, "his": true,
		"her": true, "its": true, "our": true, "their": true,
		"need": true, "want": true, "like": true,
	}
	
	var filtered []string
	for _, w := range words {
		if len(w) > 1 && !stopWords[w] {
			filtered = append(filtered, w)
		}
	}
	
	return filtered
}

// mapClassificationToRequirements converts classification to concrete specs
func (p *EmbeddedProvider) mapClassificationToRequirements(result ClassificationResult, text string) *WorkloadRequirements {
	req := &WorkloadRequirements{
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		MaxMemory:       16,
		MaxInterruption: 2,
		Confidence:      result.Confidence,
		Provider:        "embedded",
	}
	
	// Find matching category
	classifier := p.classifier
	var matchedCat *WorkloadCategory
	for i := range classifier.categories {
		if classifier.categories[i].Name == result.Category {
			matchedCat = &classifier.categories[i]
			break
		}
	}
	
	if matchedCat != nil {
		req.MinVCPU = matchedCat.MinVCPU
		req.MaxVCPU = matchedCat.MaxVCPU
		req.MinMemory = matchedCat.MinMemory
		req.MaxMemory = matchedCat.MaxMemory
		req.UseCase = matchedCat.UseCase
		req.MaxInterruption = matchedCat.MaxInterruption
		req.NeedsGPU = matchedCat.NeedsGPU
		req.GPUType = matchedCat.GPUType
	}
	
	// Apply intensity multiplier
	switch result.Intensity {
	case "extreme":
		req.MinVCPU = maxInt(req.MinVCPU*4, 64)
		req.MaxVCPU = maxInt(req.MaxVCPU*4, 192)
		req.MinMemory = maxInt(req.MinMemory*4, 256)
		req.MaxMemory = maxInt(req.MaxMemory*4, 1024)
	case "heavy":
		req.MinVCPU = maxInt(req.MinVCPU*2, 16)
		req.MaxVCPU = maxInt(req.MaxVCPU*2, 64)
		req.MinMemory = maxInt(req.MinMemory*2, 64)
		req.MaxMemory = maxInt(req.MaxMemory*2, 256)
	case "light":
		req.MinVCPU = maxInt(req.MinVCPU/2, 1)
		req.MaxVCPU = maxInt(req.MaxVCPU/2, 4)
		req.MinMemory = maxInt(req.MinMemory/2, 2)
		req.MaxMemory = maxInt(req.MaxMemory/2, 16)
		req.MaxInterruption = 3
	}
	
	// Extract architecture
	textLower := strings.ToLower(text)
	if strings.Contains(textLower, "intel") {
		req.Architecture = "intel"
	} else if strings.Contains(textLower, "amd") {
		req.Architecture = "amd"
	} else if strings.Contains(textLower, "arm") || strings.Contains(textLower, "graviton") {
		req.Architecture = "arm64"
	}
	
	// Extract explicit numbers
	extractNumbers(text, req)
	
	// Build explanation
	req.Explanation = buildExplanation(result, req)
	
	return req
}

// extractNumbers extracts explicit CPU/memory numbers from text
func extractNumbers(text string, req *WorkloadRequirements) {
	text = strings.ToLower(text)
	
	// CPU patterns
	cpuRe := regexp.MustCompile(`(\d+)\s*(?:v?cpus?|cores?)`)
	if m := cpuRe.FindStringSubmatch(text); len(m) > 1 {
		if num := parseInt(m[1]); num > 0 && num <= 192 {
			req.MinVCPU = num
			if req.MaxVCPU < num {
				req.MaxVCPU = num + 4
			}
		}
	}
	
	// Memory patterns
	memRe := regexp.MustCompile(`(\d+)\s*(?:gb|gib)`)
	if m := memRe.FindStringSubmatch(text); len(m) > 1 {
		if num := parseInt(m[1]); num > 0 && num <= 1024 {
			req.MinMemory = num
			if req.MaxMemory < num {
				req.MaxMemory = num * 2
			}
		}
	}
}

// parseInt parses a string to int, returns 0 on error
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// buildExplanation creates a human-readable explanation
func buildExplanation(result ClassificationResult, req *WorkloadRequirements) string {
	categoryNames := map[string]string{
		"hpc":           "HPC/Scientific",
		"ml_training":   "ML Training",
		"llm_inference": "LLM/AI Inference",
		"kubernetes":    "Kubernetes",
		"database":      "Database",
		"analytics":     "Data Analytics",
		"media":         "Media Processing",
		"cicd":          "CI/CD",
		"gaming":        "Gaming",
		"rendering":     "3D Rendering",
		"web":           "Web/API",
		"dev_test":      "Dev/Test",
	}
	
	catName := categoryNames[result.Category]
	if catName == "" {
		catName = "General"
	}
	
	intensityEmoji := ""
	switch result.Intensity {
	case "extreme":
		intensityEmoji = "?? Extreme"
	case "heavy":
		intensityEmoji = "?? Heavy"
	case "light":
		intensityEmoji = "?? Light"
	default:
		intensityEmoji = "?? Standard"
	}
	
	// Get top 3 scores for insight
	type scoreEntry struct {
		cat   string
		score float64
	}
	var entries []scoreEntry
	for cat, score := range result.Scores {
		if score > 0 {
			entries = append(entries, scoreEntry{cat, score})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})
	
	gpuNote := ""
	if req.NeedsGPU {
		gpuNote = " | GPU: " + req.GPUType
	}
	
	return fmt.Sprintf("?? [Embedded ML] %s workload (%.0f%% confidence) | %s | %d-%d vCPU, %d-%dGB RAM%s",
		catName, result.Confidence*100, intensityEmoji,
		req.MinVCPU, req.MaxVCPU, req.MinMemory, req.MaxMemory, gpuNote)
}
