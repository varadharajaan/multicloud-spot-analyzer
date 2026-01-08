// Package nlp provides configurable natural language processing for workload requirements.
// Supports multiple LLM backends: Ollama, Hugging Face, OpenAI, and rule-based fallback.
package nlp

import (
	"context"
	"fmt"
	"strings"
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
	Confidence      float64 `json:"confidence,omitempty"`
	Provider        string `json:"provider,omitempty"` // Which provider was used
}

// Provider defines the interface for NLP providers
type Provider interface {
	// Name returns the provider name
	Name() string
	
	// IsAvailable checks if the provider is configured and accessible
	IsAvailable(ctx context.Context) bool
	
	// Parse analyzes natural language and returns workload requirements
	Parse(ctx context.Context, text string) (*WorkloadRequirements, error)
}

// ProviderType represents the type of NLP provider
type ProviderType string

const (
	ProviderOllama      ProviderType = "ollama"
	ProviderHuggingFace ProviderType = "huggingface"
	ProviderOpenAI      ProviderType = "openai"
	ProviderEmbedded    ProviderType = "embedded"  // Pure Go ML, no external deps
	ProviderRules       ProviderType = "rules"
	ProviderAuto        ProviderType = "auto" // Try providers in order
)

// Config holds NLP provider configuration
type Config struct {
	// Provider specifies which provider to use (ollama, huggingface, openai, rules, auto)
	Provider ProviderType `yaml:"provider" json:"provider"`
	
	// Ollama configuration
	Ollama OllamaConfig `yaml:"ollama" json:"ollama"`
	
	// HuggingFace configuration
	HuggingFace HuggingFaceConfig `yaml:"huggingface" json:"huggingface"`
	
	// OpenAI configuration
	OpenAI OpenAIConfig `yaml:"openai" json:"openai"`
	
	// Timeout in seconds for API calls
	TimeoutSeconds int `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// OllamaConfig holds Ollama-specific configuration
type OllamaConfig struct {
	// Endpoint is the Ollama API endpoint (default: http://localhost:11434)
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	
	// Model is the model to use (default: llama3.2, mistral, etc.)
	Model string `yaml:"model" json:"model"`
}

// HuggingFaceConfig holds HuggingFace-specific configuration
type HuggingFaceConfig struct {
	// APIKey is optional - free tier works without it but with rate limits
	APIKey string `yaml:"api_key" json:"api_key"`
	
	// Model is the model to use (default: facebook/bart-large-mnli for zero-shot)
	Model string `yaml:"model" json:"model"`
}

// OpenAIConfig holds OpenAI-specific configuration
type OpenAIConfig struct {
	// APIKey is required for OpenAI
	APIKey string `yaml:"api_key" json:"api_key"`
	
	// Model is the model to use (default: gpt-3.5-turbo)
	Model string `yaml:"model" json:"model"`
	
	// Endpoint allows using OpenAI-compatible APIs (Azure OpenAI, etc.)
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

// DefaultConfig returns the default NLP configuration
func DefaultConfig() Config {
	return Config{
		Provider:       ProviderAuto,
		TimeoutSeconds: 15,
		Ollama: OllamaConfig{
			Endpoint: "http://localhost:11434",
			Model:    "llama3.2",
		},
		HuggingFace: HuggingFaceConfig{
			Model: "facebook/bart-large-mnli",
		},
		OpenAI: OpenAIConfig{
			Model:    "gpt-3.5-turbo",
			Endpoint: "https://api.openai.com/v1",
		},
	}
}

// ParseProviderType parses a string into a ProviderType
func ParseProviderType(s string) ProviderType {
	switch strings.ToLower(s) {
	case "ollama":
		return ProviderOllama
	case "huggingface", "hf":
		return ProviderHuggingFace
	case "openai", "gpt":
		return ProviderOpenAI
	case "embedded", "local-ml", "ml":
		return ProviderEmbedded
	case "rules", "local", "fallback":
		return ProviderRules
	default:
		return ProviderAuto
	}
}

// String returns the string representation of the provider type
func (p ProviderType) String() string {
	return string(p)
}

// ErrProviderUnavailable is returned when no provider is available
var ErrProviderUnavailable = fmt.Errorf("no NLP provider available")

// ErrLowConfidence is returned when the AI confidence is too low
var ErrLowConfidence = fmt.Errorf("low confidence classification")
