// Package domain contains interfaces that define contracts for the application.
package domain

import (
	"context"
)

// SpotDataProvider defines the interface for fetching spot instance data
// from any cloud provider. Implementations must be thread-safe.
type SpotDataProvider interface {
	// FetchSpotData retrieves spot instance data for a specific region and OS
	FetchSpotData(ctx context.Context, region string, os OperatingSystem) ([]SpotData, error)
	
	// GetSupportedRegions returns all regions supported by the provider
	GetSupportedRegions(ctx context.Context) ([]string, error)
	
	// GetProviderName returns the cloud provider identifier
	GetProviderName() CloudProvider
	
	// RefreshData forces a refresh of cached data
	RefreshData(ctx context.Context) error
}

// InstanceSpecsProvider defines the interface for fetching instance specifications
type InstanceSpecsProvider interface {
	// GetInstanceSpecs returns specifications for a specific instance type
	GetInstanceSpecs(ctx context.Context, instanceType string) (*InstanceSpecs, error)
	
	// GetAllInstanceSpecs returns specifications for all instance types
	GetAllInstanceSpecs(ctx context.Context) ([]InstanceSpecs, error)
	
	// GetInstancesByVCPU returns instances matching the vCPU requirement
	GetInstancesByVCPU(ctx context.Context, minVCPU, maxVCPU int) ([]InstanceSpecs, error)
	
	// GetProviderName returns the cloud provider identifier
	GetProviderName() CloudProvider
}

// InstanceAnalyzer defines the interface for analyzing and scoring instances
type InstanceAnalyzer interface {
	// Analyze performs analysis on spot instances based on requirements
	Analyze(ctx context.Context, requirements UsageRequirements) (*AnalysisResult, error)
	
	// ScoreInstance calculates the score for a single instance
	ScoreInstance(specs InstanceSpecs, spot SpotData, requirements UsageRequirements) (float64, ScoreBreakdown)
}

// InstanceFilter defines the interface for filtering instances
type InstanceFilter interface {
	// Filter removes instances that don't match requirements
	Filter(instances []InstanceSpecs, spots map[string]SpotData, requirements UsageRequirements) []InstanceSpecs
	
	// IsEligible checks if a single instance matches requirements
	IsEligible(specs InstanceSpecs, spot *SpotData, requirements UsageRequirements) (bool, []string)
}

// RecommendationEngine defines the interface for generating recommendations
type RecommendationEngine interface {
	// GenerateRecommendation creates a recommendation string for an instance
	GenerateRecommendation(analysis InstanceAnalysis, requirements UsageRequirements) string
	
	// GenerateWarnings identifies potential issues with an instance choice
	GenerateWarnings(analysis InstanceAnalysis, requirements UsageRequirements) []string
}

// CacheProvider defines the interface for caching data
type CacheProvider interface {
	// Get retrieves a value from cache
	Get(key string) (interface{}, bool)
	
	// Set stores a value in cache with expiration
	Set(key string, value interface{}, ttlSeconds int)
	
	// Delete removes a value from cache
	Delete(key string)
	
	// Clear removes all values from cache
	Clear()
}

// CloudProviderFactory creates cloud provider instances
type CloudProviderFactory interface {
	// CreateSpotDataProvider creates a spot data provider for the given cloud
	CreateSpotDataProvider(provider CloudProvider) (SpotDataProvider, error)
	
	// CreateInstanceSpecsProvider creates an instance specs provider for the given cloud
	CreateInstanceSpecsProvider(provider CloudProvider) (InstanceSpecsProvider, error)
}

// Logger defines the logging interface used throughout the application
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}
