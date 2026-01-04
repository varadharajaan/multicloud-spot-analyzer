// Package domain contains cloud provider abstractions for multi-cloud support.
// This module implements the Strategy pattern for cloud providers, making it
// easy to add new providers (AWS, Azure, GCP) with consistent interfaces.
package domain

import (
	"context"
	"fmt"
	"sync"
)

// ===============================================
// Cloud Provider Registry (Dependency Injection)
// ===============================================

// CloudProviderRegistry manages cloud provider implementations
// Uses the Service Locator pattern with lazy initialization
type CloudProviderRegistry struct {
	mu        sync.RWMutex
	providers map[CloudProvider]CloudProviderServices
}

// CloudProviderServices bundles all services for a cloud provider
type CloudProviderServices struct {
	Name             CloudProvider
	SpotProvider     SpotDataProvider
	SpecsProvider    InstanceSpecsProvider
	ZoneProvider     ZoneAvailabilityProvider
	CapacityProvider CapacityEstimator
	PriceProvider    PriceHistoryProvider
	FamilyExtractor  FamilyExtractor
}

// NewCloudProviderRegistry creates a new provider registry
func NewCloudProviderRegistry() *CloudProviderRegistry {
	return &CloudProviderRegistry{
		providers: make(map[CloudProvider]CloudProviderServices),
	}
}

// Register adds a cloud provider's services to the registry
func (r *CloudProviderRegistry) Register(services CloudProviderServices) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[services.Name] = services
}

// Get retrieves a cloud provider's services
func (r *CloudProviderRegistry) Get(provider CloudProvider) (CloudProviderServices, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if svc, ok := r.providers[provider]; ok {
		return svc, nil
	}
	return CloudProviderServices{}, fmt.Errorf("provider %s not registered", provider)
}

// GetAll returns all registered providers
func (r *CloudProviderRegistry) GetAll() []CloudProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	providers := make([]CloudProvider, 0, len(r.providers))
	for p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// ===============================================
// Zone & AZ Interfaces (Provider-Specific)
// ===============================================

// ZoneAvailabilityProvider provides zone availability information
// Each cloud implements this differently:
// - AWS: Uses DescribeAvailabilityZones + SpotPriceHistory
// - Azure: Uses Compute SKUs API
// - GCP: Will use Compute Zones API
type ZoneAvailabilityProvider interface {
	// GetZoneAvailability returns zones where a VM type is available
	GetZoneAvailability(ctx context.Context, vmType, region string) ([]ZoneInfo, error)
	
	// GetAllZones returns all zones in a region
	GetAllZones(ctx context.Context, region string) ([]string, error)
	
	// IsAvailable returns true if the provider has valid credentials
	IsAvailable() bool
	
	// GetProviderName returns the cloud provider
	GetProviderName() CloudProvider
}

// ZoneInfo contains zone availability data
type ZoneInfo struct {
	Zone             string  `json:"zone"`
	Available        bool    `json:"available"`
	Restricted       bool    `json:"restricted"`
	RestrictionReason string `json:"restriction_reason,omitempty"`
	CapacityScore    int     `json:"capacity_score"` // 0-100
}

// CapacityEstimator estimates zone capacity
// Each cloud has different signals:
// - AWS: Spot price volatility, interruption rates
// - Azure: VM type diversity in zone
// - GCP: Preemptible VM quotas
type CapacityEstimator interface {
	// GetCapacityScore returns 0-100 capacity estimate for VM in zone
	GetCapacityScore(ctx context.Context, vmType, zone string) (int, error)
	
	// GetZoneCapacityScores returns capacity scores for all zones in region
	GetZoneCapacityScores(ctx context.Context, region string) (map[string]int, error)
}

// PriceHistoryProvider provides historical pricing data
// - AWS: DescribeSpotPriceHistory (per-AZ, 90 days)
// - Azure: Retail Prices API (regional only, current)
// - GCP: Will need BigQuery or Pricing API
type PriceHistoryProvider interface {
	// GetPriceHistory returns price history for a VM type
	GetPriceHistory(ctx context.Context, vmType, region string) ([]PricePoint, error)
	
	// GetCurrentPrice returns current spot/preemptible price
	GetCurrentPrice(ctx context.Context, vmType, region, zone string) (float64, error)
	
	// HasPerZonePricing returns true if provider has per-zone pricing
	HasPerZonePricing() bool
	
	// GetProviderName returns the cloud provider
	GetProviderName() CloudProvider
}

// PricePoint represents a price at a point in time
type PricePoint struct {
	Timestamp   int64   `json:"timestamp"`
	Price       float64 `json:"price"`
	Zone        string  `json:"zone,omitempty"`
	SpotPrice   float64 `json:"spot_price"`
	OnDemand    float64 `json:"on_demand"`
}

// ===============================================
// Instance Naming Interfaces
// ===============================================

// FamilyExtractor extracts instance family from instance type name
// Each cloud has different naming:
// - AWS: m5.large -> "m", c6i.xlarge -> "c"  
// - Azure: Standard_D4s_v5 -> "D", Standard_B2s -> "B"
// - GCP: n2-standard-4 -> "n2", e2-medium -> "e2"
type FamilyExtractor interface {
	// ExtractFamily returns the family/series from instance type
	ExtractFamily(instanceType string) string
	
	// NormalizeName normalizes instance type name for comparison
	NormalizeName(instanceType string) string
	
	// GetProviderName returns the cloud provider
	GetProviderName() CloudProvider
}

// ===============================================
// AZ Recommendation Interfaces
// ===============================================

// AZRecommender provides availability zone recommendations
type AZRecommender interface {
	// RecommendAZ returns ranked AZ recommendations
	RecommendAZ(ctx context.Context, vmType, region string) (*AZRecommendation, error)
	
	// GetProviderName returns the cloud provider
	GetProviderName() CloudProvider
}

// AZRecommendation contains the AZ recommendation result
type AZRecommendation struct {
	VMType       string           `json:"vm_type"`
	Region       string           `json:"region"`
	Rankings     []AZRanking      `json:"rankings"`
	BestAZ       string           `json:"best_az"`
	NextBestAZ   string           `json:"next_best_az,omitempty"`
	Confidence   string           `json:"confidence"` // high, medium, low
	DataSources  []string         `json:"data_sources"`
	Insights     []string         `json:"insights"`
}

// AZRanking contains ranking for a single AZ
type AZRanking struct {
	Zone              string  `json:"zone"`
	Rank              int     `json:"rank"`
	CombinedScore     float64 `json:"combined_score"`
	AvailabilityScore float64 `json:"availability_score"`
	CapacityScore     float64 `json:"capacity_score"`
	PriceScore        float64 `json:"price_score"`
	StabilityScore    float64 `json:"stability_score"`
	InterruptionScore float64 `json:"interruption_score"`
	SpotPrice         float64 `json:"spot_price"`
	PricePredicted    bool    `json:"price_predicted"`
	InterruptionRate  float64 `json:"interruption_rate"`
	CapacityLevel     string  `json:"capacity_level"` // high, medium, low
	Available         bool    `json:"available"`
	Stability         string  `json:"stability"`
	Explanation       string  `json:"explanation,omitempty"`
}

// ===============================================
// Region & Naming Helpers
// ===============================================

// RegionMapper maps regions between clouds and formats
type RegionMapper interface {
	// GetDisplayName returns human-readable region name
	GetDisplayName(region string) string
	
	// GetZoneFormat returns zone naming format for region
	// AWS: us-east-1a, Azure: eastus-1, GCP: us-east1-b
	GetZoneFormat(region string, zoneIndex int) string
	
	// GetAllRegions returns all supported regions
	GetAllRegions() []string
	
	// GetProviderName returns the cloud provider
	GetProviderName() CloudProvider
}

// ===============================================
// Score Weights (Configurable per use case)
// ===============================================

// ScoreWeights defines weights for AZ scoring factors
type ScoreWeights struct {
	Availability float64 `json:"availability"` // Zone availability weight
	Capacity     float64 `json:"capacity"`     // Capacity score weight
	Price        float64 `json:"price"`        // Price weight
	Stability    float64 `json:"stability"`    // Volatility weight
	Interruption float64 `json:"interruption"` // Interruption rate weight
}

// DefaultScoreWeights returns default balanced weights
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.25,
		Capacity:     0.25,
		Price:        0.20,
		Stability:    0.15,
		Interruption: 0.15,
	}
}

// HighAvailabilityWeights returns weights for HA workloads
func HighAvailabilityWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.35,
		Capacity:     0.30,
		Price:        0.10,
		Stability:    0.15,
		Interruption: 0.10,
	}
}

// CostOptimizedWeights returns weights for cost-sensitive workloads
func CostOptimizedWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.15,
		Capacity:     0.15,
		Price:        0.40,
		Stability:    0.15,
		Interruption: 0.15,
	}
}
