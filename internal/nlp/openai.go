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

// OpenAIProvider implements NLP using OpenAI's API (or compatible APIs like Azure OpenAI)
type OpenAIProvider struct {
	apiKey     string
	model      string
	endpoint   string
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI-based NLP provider
func NewOpenAIProvider(apiKey, model, endpoint string, timeout time.Duration) *OpenAIProvider {
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: strings.TrimSuffix(endpoint, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *OpenAIProvider) Name() string {
	return fmt.Sprintf("openai/%s", p.model)
}

func (p *OpenAIProvider) IsAvailable(ctx context.Context) bool {
	return p.apiKey != ""
}

func (p *OpenAIProvider) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	systemPrompt := `You are an expert cloud infrastructure analyst. Analyze workload descriptions and extract compute requirements.

Output ONLY a JSON object with these fields:
- minVcpu: integer (minimum vCPU cores)
- maxVcpu: integer (maximum vCPU, 0 for flexible)
- minMemory: integer (minimum RAM in GB)
- maxMemory: integer (maximum RAM in GB, 0 for flexible)
- architecture: string ("x86_64", "arm64", "intel", "amd", or "")
- useCase: string ("kubernetes", "database", "batch", "ml", "hpc", "general", "asg")
- maxInterruption: integer 0-4 (0=<5%, 1=5-10%, 2=10-15%, 3=15-20%, 4=>20%)
- needsGpu: boolean
- gpuType: string ("nvidia-t4", "nvidia-a10g", "nvidia-a100", or "")
- explanation: string (brief explanation)

Sizing guidelines:
- HPC/Scientific (weather, genomics, physics): 32-96 vCPU, 128-512GB RAM
- ML Training: 16-64 vCPU, 64-256GB RAM, GPU
- Heavy Production K8s: 16-64 vCPU, 64-256GB RAM
- Heavy Database: 16-64 vCPU, 128-512GB RAM, maxInterruption=0
- Light/Dev: 1-4 vCPU, 2-16GB RAM, maxInterruption=3`

	reqBody := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("Analyze this workload: %s", text)},
		},
		"temperature":   0.1,
		"response_format": map[string]string{"type": "json_object"},
	}

	jsonBody, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/chat/completions", p.endpoint)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error (%d): %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	content := apiResp.Choices[0].Message.Content
	return p.parseResponse(content)
}

func (p *OpenAIProvider) parseResponse(content string) (*WorkloadRequirements, error) {
	content = strings.TrimSpace(content)

	// Find JSON in response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON in response")
	}
	jsonStr := content[start : end+1]

	var result WorkloadRequirements
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, err
	}

	// Defaults
	if result.MinVCPU <= 0 {
		result.MinVCPU = 2
	}
	if result.MinMemory <= 0 {
		result.MinMemory = 4
	}

	result.Explanation = fmt.Sprintf("?? [%s] %s", p.model, result.Explanation)
	result.Confidence = 0.95 // OpenAI is generally high confidence

	return &result, nil
}
