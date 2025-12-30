package domain

import (
	"testing"
)

func TestInterruptionFrequencyString(t *testing.T) {
	tests := []struct {
		name     string
		freq     InterruptionFrequency
		expected string
	}{
		{"VeryLow", VeryLow, "<5%"},
		{"Low", Low, "5-10%"},
		{"Medium", Medium, "10-15%"},
		{"High", High, "15-20%"},
		{"VeryHigh", VeryHigh, ">20%"},
		{"Unknown", InterruptionFrequency(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.freq.String()
			if result != tt.expected {
				t.Errorf("InterruptionFrequency.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCloudProviderConstants(t *testing.T) {
	if AWS != "aws" {
		t.Errorf("AWS constant = %v, want aws", AWS)
	}
	if Azure != "azure" {
		t.Errorf("Azure constant = %v, want azure", Azure)
	}
	if GCP != "gcp" {
		t.Errorf("GCP constant = %v, want gcp", GCP)
	}
}

func TestOperatingSystemConstants(t *testing.T) {
	if Linux != "Linux" {
		t.Errorf("Linux constant = %v, want Linux", Linux)
	}
	if Windows != "Windows" {
		t.Errorf("Windows constant = %v, want Windows", Windows)
	}
}

func TestInstanceCategoryConstants(t *testing.T) {
	categories := []struct {
		name     string
		category InstanceCategory
		expected string
	}{
		{"GeneralPurpose", GeneralPurpose, "general_purpose"},
		{"ComputeOptimized", ComputeOptimized, "compute_optimized"},
		{"MemoryOptimized", MemoryOptimized, "memory_optimized"},
		{"StorageOptimized", StorageOptimized, "storage_optimized"},
		{"AcceleratedComputing", AcceleratedComputing, "accelerated_computing"},
		{"HighPerformance", HighPerformance, "high_performance"},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.category) != tt.expected {
				t.Errorf("InstanceCategory %s = %v, want %v", tt.name, tt.category, tt.expected)
			}
		})
	}
}

func TestInstanceGenerationConstants(t *testing.T) {
	if Current != 0 {
		t.Errorf("Current generation = %v, want 0", Current)
	}
	if Previous != 1 {
		t.Errorf("Previous generation = %v, want 1", Previous)
	}
	if Legacy != 2 {
		t.Errorf("Legacy generation = %v, want 2", Legacy)
	}
	if Deprecated != 3 {
		t.Errorf("Deprecated generation = %v, want 3", Deprecated)
	}
}

func TestInstanceSpecs(t *testing.T) {
	specs := InstanceSpecs{
		InstanceType:  "m5.large",
		VCPU:          2,
		MemoryGB:      8.0,
		HasGPU:        false,
		Architecture:  "x86_64",
		Category:      GeneralPurpose,
		Generation:    Current,
		IsDeprecated:  false,
		IsBurstable:   false,
		CloudProvider: AWS,
	}

	if specs.InstanceType != "m5.large" {
		t.Errorf("InstanceType = %v, want m5.large", specs.InstanceType)
	}
	if specs.VCPU != 2 {
		t.Errorf("VCPU = %v, want 2", specs.VCPU)
	}
	if specs.MemoryGB != 8.0 {
		t.Errorf("MemoryGB = %v, want 8.0", specs.MemoryGB)
	}
	if specs.CloudProvider != AWS {
		t.Errorf("CloudProvider = %v, want AWS", specs.CloudProvider)
	}
}

func TestUsageRequirements(t *testing.T) {
	req := UsageRequirements{
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemoryGB:     4,
		MaxMemoryGB:     32,
		Region:          "us-east-1",
		OS:              Linux,
		Architecture:    "x86_64",
		AllowBurstable:  true,
		MaxInterruption: Medium,
		Families:        []string{"m", "c"},
	}

	if req.MinVCPU != 2 {
		t.Errorf("MinVCPU = %v, want 2", req.MinVCPU)
	}
	if req.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", req.Region)
	}
	if len(req.Families) != 2 {
		t.Errorf("Families length = %v, want 2", len(req.Families))
	}
}
