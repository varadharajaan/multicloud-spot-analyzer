// Package provider contains the cloud provider factory and registry.
package provider

import (
	"fmt"
	"sync"

	"github.com/spot-analyzer/internal/domain"
)

// Factory implements the CloudProviderFactory interface
// using the Abstract Factory pattern for creating cloud-specific providers.
type Factory struct {
	mu                    sync.RWMutex
	spotProviders         map[domain.CloudProvider]domain.SpotDataProvider
	specsProviders        map[domain.CloudProvider]domain.InstanceSpecsProvider
	registeredProviders   map[domain.CloudProvider]bool
}

// ProviderCreator is a function type for creating providers
type SpotProviderCreator func() (domain.SpotDataProvider, error)
type SpecsProviderCreator func() (domain.InstanceSpecsProvider, error)

var (
	globalFactory      *Factory
	globalFactoryOnce  sync.Once
	spotCreators       = make(map[domain.CloudProvider]SpotProviderCreator)
	specsCreators      = make(map[domain.CloudProvider]SpecsProviderCreator)
	creatorsMu         sync.RWMutex
)

// GetFactory returns the global factory instance (Singleton pattern)
func GetFactory() *Factory {
	globalFactoryOnce.Do(func() {
		globalFactory = &Factory{
			spotProviders:       make(map[domain.CloudProvider]domain.SpotDataProvider),
			specsProviders:      make(map[domain.CloudProvider]domain.InstanceSpecsProvider),
			registeredProviders: make(map[domain.CloudProvider]bool),
		}
	})
	return globalFactory
}

// RegisterSpotProviderCreator registers a creator function for a cloud provider's spot data provider
func RegisterSpotProviderCreator(provider domain.CloudProvider, creator SpotProviderCreator) {
	creatorsMu.Lock()
	defer creatorsMu.Unlock()
	spotCreators[provider] = creator
}

// RegisterSpecsProviderCreator registers a creator function for a cloud provider's specs provider
func RegisterSpecsProviderCreator(provider domain.CloudProvider, creator SpecsProviderCreator) {
	creatorsMu.Lock()
	defer creatorsMu.Unlock()
	specsCreators[provider] = creator
}

// CreateSpotDataProvider creates or returns a cached spot data provider
func (f *Factory) CreateSpotDataProvider(provider domain.CloudProvider) (domain.SpotDataProvider, error) {
	f.mu.RLock()
	if p, exists := f.spotProviders[provider]; exists {
		f.mu.RUnlock()
		return p, nil
	}
	f.mu.RUnlock()

	creatorsMu.RLock()
	creator, exists := spotCreators[provider]
	creatorsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedProvider, provider)
	}

	p, err := creator()
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.spotProviders[provider] = p
	f.registeredProviders[provider] = true
	f.mu.Unlock()

	return p, nil
}

// CreateInstanceSpecsProvider creates or returns a cached instance specs provider
func (f *Factory) CreateInstanceSpecsProvider(provider domain.CloudProvider) (domain.InstanceSpecsProvider, error) {
	f.mu.RLock()
	if p, exists := f.specsProviders[provider]; exists {
		f.mu.RUnlock()
		return p, nil
	}
	f.mu.RUnlock()

	creatorsMu.RLock()
	creator, exists := specsCreators[provider]
	creatorsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedProvider, provider)
	}

	p, err := creator()
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.specsProviders[provider] = p
	f.mu.Unlock()

	return p, nil
}

// GetSupportedProviders returns all registered cloud providers
func (f *Factory) GetSupportedProviders() []domain.CloudProvider {
	creatorsMu.RLock()
	defer creatorsMu.RUnlock()

	providers := make([]domain.CloudProvider, 0)
	seen := make(map[domain.CloudProvider]bool)

	for p := range spotCreators {
		if !seen[p] {
			providers = append(providers, p)
			seen[p] = true
		}
	}

	return providers
}

// IsProviderSupported checks if a cloud provider is registered
func (f *Factory) IsProviderSupported(provider domain.CloudProvider) bool {
	creatorsMu.RLock()
	defer creatorsMu.RUnlock()

	_, hasSpot := spotCreators[provider]
	_, hasSpecs := specsCreators[provider]

	return hasSpot && hasSpecs
}
