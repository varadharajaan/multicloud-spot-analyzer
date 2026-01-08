// Package nlp provides configurable natural language processing for workload requirements.
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

// OllamaProvider implements NLP using local Ollama LLM
type OllamaProvider struct {
	endpoint   string
	model      string
	httpClient *http.Client
}

// NewOllamaProvider creates a new Ollama-based NLP provider
// endpoint: Ollama API endpoint (default: http://localhost:11434)
// model: Model to use (e.g., "llama3.2", "mistral", "gemma2")
func NewOllamaProvider(endpoint, model string) *OllamaProvider {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.2" // Default to Llama 3.2
	}

	return &OllamaProvider{
		endpoint: endpoint,
		model:    model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // LLMs can be slow
		},
	}
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama/%s", p.model)
}

// IsAvailable checks if Ollama is running and the model is available
func (p *OllamaProvider) IsAvailable(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/tags", p.endpoint)

	// Use a short timeout for availability check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// Ollama not running - this is expected if not installed
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Check if our model is available
	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return false
	}

	for _, m := range tagsResp.Models {
		// Model names can be "llama3.2:latest" or just "llama3.2"
		if strings.HasPrefix(m.Name, p.model) {
			return true
		}
	}

	// Model not found - could suggest pulling it
	return false
}

// GetInstallInstructions returns platform-specific install instructions
func (p *OllamaProvider) GetInstallInstructions() string {
	return `Ollama not detected. To enable local LLM parsing:

  Windows:   winget install Ollama.Ollama
  macOS:     brew install ollama
  Linux:     curl -fsSL https://ollama.com/install.sh | sh

Then pull a model:
  ollama pull llama3.2

The parser will automatically use Ollama when available.
Without Ollama, the enhanced rule-based parser is used (still works well).`
}

// Parse uses Ollama to analyze natural language and extract workload requirements
func (p *OllamaProvider) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	prompt := p.buildPrompt(text)

	reqBody := map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
		"format": "json",
		"options": map[string]interface{}{
			"temperature": 0.1, // Low temperature for consistent, deterministic output
			"num_predict": 500, // Limit response length
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	// Parse the JSON response from the LLM
	result, err := p.parseResponse(ollamaResp.Response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workload requirements: %w", err)
	}

	return result, nil
}

// buildPrompt creates a detailed prompt for the LLM
func (p *OllamaProvider) buildPrompt(text string) string {
	return fmt.Sprintf(`You are an expert cloud infrastructure analyst. Analyze the following natural language description and extract cloud compute instance requirements.

USER INPUT: "%s"

Analyze the workload described and determine:
1. What type of workload is this? (web, database, ML/AI, HPC/scientific, kubernetes, batch, etc.)
2. How resource-intensive is this workload? (light, medium, heavy, extreme)
3. What compute resources are needed?
4. What stability/interruption tolerance is appropriate?

Consider domain-specific workloads:
- Weather/climate modeling ? HPC, needs 32+ vCPU, 128+ GB RAM
- Genomics/bioinformatics ? HPC, needs 32+ vCPU, 128+ GB RAM
- ML training/deep learning ? Compute-heavy, needs GPU, 16+ vCPU, 64+ GB RAM
- Database (production) ? Memory-optimized, needs stability
- Kubernetes (production/heavy) ? Scale based on "heavy", "production", "enterprise" keywords
- CI/CD ? Cost-optimized, can tolerate interruptions

Respond ONLY with a valid JSON object (no markdown, no explanation):
{
  "minVcpu": <integer, minimum vCPU cores>,
  "maxVcpu": <integer, maximum vCPU cores or 0 for flexible>,
  "minMemory": <integer, minimum RAM in GB>,
  "maxMemory": <integer, maximum RAM in GB or 0 for flexible>,
  "architecture": <string: "x86_64", "arm64", "intel", "amd", or "" for any>,
  "useCase": <string: "kubernetes", "database", "batch", "ml", "hpc", "general", "asg">,
  "maxInterruption": <integer 0-4: 0=<5%%, 1=5-10%%, 2=10-15%%, 3=15-20%%, 4=>20%%>,
  "needsGpu": <boolean>,
  "gpuType": <string: "nvidia-t4", "nvidia-a10g", "nvidia-a100", "nvidia-v100", or "">,
  "explanation": <string: brief explanation of your analysis>
}

Guidelines for sizing:
- Light/dev/test: 1-2 vCPU, 2-8 GB RAM, maxInterruption=3
- Medium/standard: 4-8 vCPU, 16-32 GB RAM, maxInterruption=2
- Heavy/production: 16-64 vCPU, 64-256 GB RAM, maxInterruption=1
- Extreme/HPC: 32-96+ vCPU, 128-512+ GB RAM, maxInterruption=2
- Database: Always maxInterruption=0 for stability

IMPORTANT: Output ONLY the JSON object, nothing else.`, text)
}

// parseResponse extracts WorkloadRequirements from LLM response
func (p *OllamaProvider) parseResponse(response string) (*WorkloadRequirements, error) {
	response = strings.TrimSpace(response)

	// Find JSON in response (LLMs sometimes add extra text)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no valid JSON found in response: %s", response)
	}
	jsonStr := response[start : end+1]

	var result WorkloadRequirements
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w (response: %s)", err, jsonStr)
	}

	// Validate and apply defaults
	if result.MinVCPU <= 0 {
		result.MinVCPU = 2
	}
	if result.MinMemory <= 0 {
		result.MinMemory = 4
	}
	if result.MaxInterruption < 0 || result.MaxInterruption > 4 {
		result.MaxInterruption = 2
	}

	// Add provider info to explanation
	result.Explanation = fmt.Sprintf("?? [%s] %s", p.model, result.Explanation)

	return &result, nil
}
