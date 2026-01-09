package nlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HuggingFaceProvider implements NLP using HuggingFace's free inference API
type HuggingFaceProvider struct {
	token      string // Optional - works without but rate-limited
	model      string
	httpClient *http.Client
}

// NewHuggingFaceProvider creates a new HuggingFace provider
func NewHuggingFaceProvider(token, model string, timeout time.Duration) *HuggingFaceProvider {
	if model == "" {
		model = "facebook/bart-large-mnli" // Good for zero-shot classification
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &HuggingFaceProvider{
		token: token,
		model: model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *HuggingFaceProvider) Name() string {
	return "huggingface"
}

func (p *HuggingFaceProvider) IsAvailable(ctx context.Context) bool {
	// HuggingFace is generally available (free tier)
	// We could ping the API but it's rate-limited
	return true
}

func (p *HuggingFaceProvider) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	// Use zero-shot classification
	url := fmt.Sprintf("https://api-inference.huggingface.co/models/%s", p.model)

	// Define workload categories
	categories := []string{
		"high performance computing, scientific simulation, weather forecasting, genomics",
		"machine learning training, deep learning, neural network training",
		"large language model inference, GPT, AI inference",
		"heavy production kubernetes cluster, enterprise container orchestration",
		"light development kubernetes, testing cluster",
		"heavy production database, enterprise data storage",
		"light database, development database",
		"data analytics, big data processing, ETL pipeline",
		"video encoding, media processing, streaming",
		"CI/CD pipeline, build automation, Jenkins",
		"web server, API hosting, microservices",
		"small development, testing, prototype",
	}

	reqBody := map[string]interface{}{
		"inputs":     text,
		"parameters": map[string]interface{}{"candidate_labels": categories},
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HuggingFace API error (%d): %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Labels []string  `json:"labels"`
		Scores []float64 `json:"scores"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Labels) == 0 {
		return nil, fmt.Errorf("no classification results")
	}

	// Map classification to requirements
	return p.mapToRequirements(apiResp.Labels[0], apiResp.Scores[0])
}

func (p *HuggingFaceProvider) mapToRequirements(label string, score float64) (*WorkloadRequirements, error) {
	if score < 0.25 {
		return nil, fmt.Errorf("low confidence: %.2f", score)
	}

	label = strings.ToLower(label)
	result := &WorkloadRequirements{
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		MaxMemory:       32,
		MaxInterruption: 2,
		Confidence:      score,
	}

	switch {
	case strings.Contains(label, "high performance") || strings.Contains(label, "scientific") ||
		strings.Contains(label, "weather") || strings.Contains(label, "genomics"):
		result.MinVCPU = 32
		result.MaxVCPU = 96
		result.MinMemory = 128
		result.MaxMemory = 512
		result.UseCase = "hpc"
		result.Explanation = fmt.Sprintf("?? [HuggingFace] HPC/Scientific workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "machine learning") || strings.Contains(label, "deep learning"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.NeedsGPU = true
		result.GPUType = "nvidia-t4"
		result.Explanation = fmt.Sprintf("?? [HuggingFace] ML Training workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "language model") || strings.Contains(label, "gpt") || strings.Contains(label, "inference"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "ml"
		result.NeedsGPU = true
		result.GPUType = "nvidia-a10g"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("?? [HuggingFace] LLM Inference workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "heavy") && strings.Contains(label, "kubernetes"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 64
		result.MaxMemory = 256
		result.UseCase = "kubernetes"
		result.MaxInterruption = 1
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Heavy Kubernetes workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "light") && strings.Contains(label, "kubernetes"):
		result.MinVCPU = 2
		result.MaxVCPU = 8
		result.MinMemory = 4
		result.MaxMemory = 16
		result.UseCase = "kubernetes"
		result.MaxInterruption = 2
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Light Kubernetes workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "heavy") && strings.Contains(label, "database"):
		result.MinVCPU = 16
		result.MaxVCPU = 64
		result.MinMemory = 128
		result.MaxMemory = 512
		result.UseCase = "database"
		result.MaxInterruption = 0
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Heavy Database workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "light") && strings.Contains(label, "database"):
		result.MinVCPU = 2
		result.MaxVCPU = 8
		result.MinMemory = 8
		result.MaxMemory = 32
		result.UseCase = "database"
		result.MaxInterruption = 0
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Light Database workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "analytics") || strings.Contains(label, "big data") || strings.Contains(label, "etl"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 32
		result.MaxMemory = 128
		result.UseCase = "batch"
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Data Analytics workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "video") || strings.Contains(label, "media") || strings.Contains(label, "streaming"):
		result.MinVCPU = 8
		result.MaxVCPU = 32
		result.MinMemory = 16
		result.MaxMemory = 64
		result.UseCase = "batch"
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Media Processing workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "ci") || strings.Contains(label, "cd") || strings.Contains(label, "build"):
		result.MinVCPU = 4
		result.MaxVCPU = 16
		result.MinMemory = 8
		result.MaxMemory = 32
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("?? [HuggingFace] CI/CD workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "web") || strings.Contains(label, "api") || strings.Contains(label, "microservice"):
		result.MinVCPU = 2
		result.MaxVCPU = 8
		result.MinMemory = 4
		result.MaxMemory = 16
		result.UseCase = "general"
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Web/API workload (%.0f%% confidence)", score*100)

	case strings.Contains(label, "small") || strings.Contains(label, "development") || strings.Contains(label, "testing"):
		result.MinVCPU = 1
		result.MaxVCPU = 2
		result.MinMemory = 2
		result.MaxMemory = 8
		result.UseCase = "batch"
		result.MaxInterruption = 3
		result.Explanation = fmt.Sprintf("?? [HuggingFace] Dev/Test workload (%.0f%% confidence)", score*100)

	default:
		result.Explanation = fmt.Sprintf("?? [HuggingFace] General workload (%.0f%% confidence)", score*100)
	}

	return result, nil
}
