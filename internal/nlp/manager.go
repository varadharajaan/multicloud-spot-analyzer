// Package nlp provides a unified NLP manager with multiple provider support
package nlp

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Manager manages NLP providers and routing
type Manager struct {
	config    Config
	providers []Provider
}

// NewManager creates a new NLP manager with the given configuration
func NewManager(config Config) *Manager {
	m := &Manager{config: config}
	m.initProviders()
	return m
}

// NewDefaultManager creates a manager with default configuration
func NewDefaultManager() *Manager {
	return NewManager(NewDefaultConfig())
}

// NewDefaultConfig returns the default NLP configuration from environment
func NewDefaultConfig() Config {
	return Config{
		Provider:       ProviderAuto,
		TimeoutSeconds: 60,
		Ollama: OllamaConfig{
			Endpoint: getEnvOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"),
			Model:    getEnvOrDefault("OLLAMA_MODEL", "llama3.2"),
		},
		HuggingFace: HuggingFaceConfig{
			APIKey: os.Getenv("HUGGINGFACE_TOKEN"),
			Model:  getEnvOrDefault("HUGGINGFACE_MODEL", "facebook/bart-large-mnli"),
		},
		OpenAI: OpenAIConfig{
			APIKey:   os.Getenv("OPENAI_API_KEY"),
			Model:    getEnvOrDefault("OPENAI_MODEL", "gpt-3.5-turbo"),
			Endpoint: getEnvOrDefault("OPENAI_ENDPOINT", "https://api.openai.com/v1"),
		},
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// initProviders initializes available providers based on configuration
func (m *Manager) initProviders() {
	timeout := time.Duration(m.config.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	switch m.config.Provider {
	case ProviderOllama:
		m.providers = []Provider{
			NewOllamaProvider(m.config.Ollama.Endpoint, m.config.Ollama.Model),
		}

	case ProviderHuggingFace:
		m.providers = []Provider{
			NewHuggingFaceProvider(m.config.HuggingFace.APIKey, m.config.HuggingFace.Model, timeout),
		}

	case ProviderOpenAI:
		if m.config.OpenAI.APIKey != "" {
			m.providers = []Provider{
				NewOpenAIProvider(m.config.OpenAI.APIKey, m.config.OpenAI.Model, m.config.OpenAI.Endpoint, timeout),
			}
		}

	case ProviderRules:
		m.providers = []Provider{
			NewRulesProvider(),
		}

	case ProviderAuto:
		fallthrough
	default:
		// Auto mode: try providers in order of preference
		// 1. Ollama (local, free, best quality)
		// 2. OpenAI (if API key configured)
		// 3. HuggingFace (free but rate-limited)
		// 4. Rules (always available)
		m.providers = []Provider{
			NewOllamaProvider(m.config.Ollama.Endpoint, m.config.Ollama.Model),
		}
		if m.config.OpenAI.APIKey != "" {
			m.providers = append(m.providers, 
				NewOpenAIProvider(m.config.OpenAI.APIKey, m.config.OpenAI.Model, m.config.OpenAI.Endpoint, timeout))
		}
		m.providers = append(m.providers,
			NewHuggingFaceProvider(m.config.HuggingFace.APIKey, m.config.HuggingFace.Model, timeout),
			NewRulesProvider(),
		)
	}
}

// Parse analyzes text and returns workload requirements
// Tries providers in order until one succeeds
func (m *Manager) Parse(ctx context.Context, text string) (*WorkloadRequirements, error) {
	var lastErr error

	for _, provider := range m.providers {
		// Check if provider is available
		if !provider.IsAvailable(ctx) {
			continue
		}

		// Try to parse
		result, err := provider.Parse(ctx, text)
		if err == nil && result != nil {
			return result, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all NLP providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no NLP providers available")
}

// GetAvailableProviders returns list of available provider names
func (m *Manager) GetAvailableProviders(ctx context.Context) []string {
	var available []string
	for _, p := range m.providers {
		if p.IsAvailable(ctx) {
			available = append(available, p.Name())
		}
	}
	return available
}

// GetStatus returns a status summary of all providers
func (m *Manager) GetStatus(ctx context.Context) map[string]interface{} {
	status := make(map[string]interface{})
	
	for _, p := range m.providers {
		providerStatus := map[string]interface{}{
			"available": p.IsAvailable(ctx),
		}
		
		// Add install instructions for Ollama if not available
		if ollama, ok := p.(*OllamaProvider); ok {
			if !p.IsAvailable(ctx) {
				providerStatus["install_hint"] = ollama.GetInstallInstructions()
			}
		}
		
		status[p.Name()] = providerStatus
	}
	
	return status
}
