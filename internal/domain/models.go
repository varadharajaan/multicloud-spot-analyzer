// Package domain contains the core domain models for the spot analyzer.
// These models are cloud-agnostic and represent the business logic entities.
package domain

import (
	"strings"
	"time"
)

// CloudProvider represents supported cloud providers
type CloudProvider string

const (
	AWS   CloudProvider = "aws"
	Azure CloudProvider = "azure"
	GCP   CloudProvider = "gcp"
)

// ParseCloudProvider parses a string into a CloudProvider.
// Returns AWS as default if the string doesn't match any known provider.
func ParseCloudProvider(s string) CloudProvider {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "azure":
		return Azure
	case "gcp":
		return GCP
	default:
		return AWS
	}
}

// String returns the string representation of the cloud provider
func (c CloudProvider) String() string {
	return string(c)
}

// IsValid checks if the cloud provider is a known valid provider
func (c CloudProvider) IsValid() bool {
	switch c {
	case AWS, Azure, GCP:
		return true
	default:
		return false
	}
}

// DefaultRegion returns the default region for the cloud provider
func (c CloudProvider) DefaultRegion() string {
	switch c {
	case Azure:
		return "eastus"
	case GCP:
		return "us-central1"
	default:
		return "us-east-1"
	}
}

// CacheKeyPrefix returns the cache key prefix for the cloud provider
func (c CloudProvider) CacheKeyPrefix() string {
	return string(c) + ":"
}

// OperatingSystem represents the OS type
type OperatingSystem string

const (
	Linux   OperatingSystem = "Linux"
	Windows OperatingSystem = "Windows"
)

// InstanceCategory represents the general category of an instance
type InstanceCategory string

const (
	GeneralPurpose       InstanceCategory = "general_purpose"
	ComputeOptimized     InstanceCategory = "compute_optimized"
	MemoryOptimized      InstanceCategory = "memory_optimized"
	StorageOptimized     InstanceCategory = "storage_optimized"
	AcceleratedComputing InstanceCategory = "accelerated_computing"
	HighPerformance      InstanceCategory = "high_performance"
)

// InterruptionFrequency represents how often spot instances are interrupted
type InterruptionFrequency int

const (
	VeryLow  InterruptionFrequency = 0 // <5%
	Low      InterruptionFrequency = 1 // 5-10%
	Medium   InterruptionFrequency = 2 // 10-15%
	High     InterruptionFrequency = 3 // 15-20%
	VeryHigh InterruptionFrequency = 4 // >20%
)

// InterruptionLabel returns the human-readable label for the interruption frequency
func (i InterruptionFrequency) String() string {
	switch i {
	case VeryLow:
		return "<5%"
	case Low:
		return "5-10%"
	case Medium:
		return "10-15%"
	case High:
		return "15-20%"
	case VeryHigh:
		return ">20%"
	default:
		return "Unknown"
	}
}

// InstanceGeneration represents the generation/age of an instance type
type InstanceGeneration int

const (
	Current    InstanceGeneration = 0
	Previous   InstanceGeneration = 1
	Legacy     InstanceGeneration = 2
	Deprecated InstanceGeneration = 3
)

// InstanceSpecs contains the hardware specifications of an instance type
type InstanceSpecs struct {
	InstanceType    string             `json:"instance_type"`
	VCPU            int                `json:"vcpu"`
	MemoryGB        float64            `json:"memory_gb"`
	HasGPU          bool               `json:"has_gpu"`
	GPUCount        int                `json:"gpu_count"`
	GPUType         string             `json:"gpu_type,omitempty"`
	GPUMemoryGB     float64            `json:"gpu_memory_gb,omitempty"`
	NetworkMbps     int                `json:"network_mbps"`
	StorageGB       float64            `json:"storage_gb"`
	StorageType     string             `json:"storage_type"`
	Architecture    string             `json:"architecture"`
	Category        InstanceCategory   `json:"category"`
	Generation      InstanceGeneration `json:"generation"`
	IsDeprecated    bool               `json:"is_deprecated"`
	IsBurstable     bool               `json:"is_burstable"`
	IsBareMetal     bool               `json:"is_bare_metal"`
	Hypervisor      string             `json:"hypervisor,omitempty"`
	ProcessorFamily string             `json:"processor_family,omitempty"`
	CloudProvider   CloudProvider      `json:"cloud_provider"`
}

// SpotData contains spot pricing and interruption data for an instance type
type SpotData struct {
	InstanceType          string                `json:"instance_type"`
	Region                string                `json:"region"`
	OS                    OperatingSystem       `json:"os"`
	SavingsPercent        int                   `json:"savings_percent"`
	InterruptionFrequency InterruptionFrequency `json:"interruption_frequency"`
	SpotPrice             float64               `json:"spot_price,omitempty"`
	OnDemandPrice         float64               `json:"on_demand_price,omitempty"`
	CloudProvider         CloudProvider         `json:"cloud_provider"`
	LastUpdated           time.Time             `json:"last_updated"`
}

// InstanceAnalysis represents the combined analysis of an instance type
type InstanceAnalysis struct {
	Specs          InstanceSpecs  `json:"specs"`
	SpotData       SpotData       `json:"spot_data"`
	Score          float64        `json:"score"`
	Rank           int            `json:"rank"`
	ScoreBreakdown ScoreBreakdown `json:"score_breakdown"`
	Recommendation string         `json:"recommendation"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// ScoreBreakdown provides detailed scoring information
type ScoreBreakdown struct {
	SavingsScore      float64 `json:"savings_score"`
	StabilityScore    float64 `json:"stability_score"`
	PerformanceScore  float64 `json:"performance_score"`
	ValueScore        float64 `json:"value_score"`
	FitnessScore      float64 `json:"fitness_score"`
	GenerationPenalty float64 `json:"generation_penalty"`
}

// UsageRequirements defines user's workload requirements
type UsageRequirements struct {
	MinVCPU           int                   `json:"min_vcpu"`
	MaxVCPU           int                   `json:"max_vcpu,omitempty"`
	MinMemoryGB       float64               `json:"min_memory_gb,omitempty"`
	MaxMemoryGB       float64               `json:"max_memory_gb,omitempty"`
	RequiresGPU       bool                  `json:"requires_gpu"`
	MinGPUCount       int                   `json:"min_gpu_count,omitempty"`
	GPUType           string                `json:"gpu_type,omitempty"`
	MinStorageGB      float64               `json:"min_storage_gb,omitempty"`
	PreferredCategory InstanceCategory      `json:"preferred_category,omitempty"`
	Architecture      string                `json:"architecture,omitempty"` // x86_64, arm64
	Region            string                `json:"region"`
	OS                OperatingSystem       `json:"os"`
	MaxInterruption   InterruptionFrequency `json:"max_interruption"`
	MinSavingsPercent int                   `json:"min_savings_percent,omitempty"`
	AllowBurstable    bool                  `json:"allow_burstable"`
	AllowBareMetal    bool                  `json:"allow_bare_metal"`
	Families          []string              `json:"families,omitempty"` // Filter by instance families (t, m, c, r, etc.)
	TopN              int                   `json:"top_n"`
}

// AnalysisResult contains the complete analysis output
type AnalysisResult struct {
	Requirements  UsageRequirements  `json:"requirements"`
	TopInstances  []InstanceAnalysis `json:"top_instances"`
	TotalAnalyzed int                `json:"total_analyzed"`
	FilteredOut   int                `json:"filtered_out"`
	AnalyzedAt    time.Time          `json:"analyzed_at"`
	Region        string             `json:"region"`
	CloudProvider CloudProvider      `json:"cloud_provider"`
}

// NewDefaultRequirements creates default requirements
func NewDefaultRequirements() UsageRequirements {
	return UsageRequirements{
		MinVCPU:         2,
		OS:              Linux,
		Region:          "us-east-1",
		MaxInterruption: Medium,
		AllowBurstable:  false,
		AllowBareMetal:  false,
		TopN:            10,
	}
}
