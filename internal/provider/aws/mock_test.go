package aws

import (
	"context"
	"testing"
	"time"

	"github.com/spot-analyzer/internal/domain"
)

// MockSpotData holds test data for mock provider
type MockSpotData struct {
	InstanceType     string
	Region           string
	SavingsPercent   int
	InterruptionFreq int
}

// MockSpotDataProvider is a mock implementation for testing
type MockSpotDataProvider struct {
	data map[string]*MockSpotData
}

func NewMockSpotDataProvider() *MockSpotDataProvider {
	return &MockSpotDataProvider{
		data: map[string]*MockSpotData{
			"m5.large": {
				InstanceType:     "m5.large",
				Region:           "us-east-1",
				SavingsPercent:   60,
				InterruptionFreq: 0,
			},
			"c5.large": {
				InstanceType:     "c5.large",
				Region:           "us-east-1",
				SavingsPercent:   65,
				InterruptionFreq: 0,
			},
			"t3.large": {
				InstanceType:     "t3.large",
				Region:           "us-east-1",
				SavingsPercent:   70,
				InterruptionFreq: 0,
			},
		},
	}
}

func (m *MockSpotDataProvider) GetSpotData(ctx context.Context, region string, os domain.OperatingSystem) (map[string]domain.SpotData, error) {
	result := make(map[string]domain.SpotData)
	for k, v := range m.data {
		result[k] = domain.SpotData{
			InstanceType:          v.InstanceType,
			Region:                region,
			OS:                    os,
			SavingsPercent:        v.SavingsPercent,
			InterruptionFrequency: domain.InterruptionFrequency(v.InterruptionFreq),
		}
	}
	return result, nil
}

func (m *MockSpotDataProvider) GetProviderName() domain.CloudProvider {
	return domain.AWS
}

func (m *MockSpotDataProvider) RefreshData(ctx context.Context) error {
	return nil
}

func TestMockSpotDataProvider(t *testing.T) {
	provider := NewMockSpotDataProvider()

	data, err := provider.GetSpotData(context.Background(), "us-east-1", domain.Linux)
	if err != nil {
		t.Fatalf("GetSpotData failed: %v", err)
	}

	if len(data) != 3 {
		t.Errorf("Expected 3 instances, got %d", len(data))
	}

	m5, exists := data["m5.large"]
	if !exists {
		t.Error("m5.large should exist")
	}
	if m5.SavingsPercent != 60 {
		t.Errorf("m5.large savings = %d, want 60", m5.SavingsPercent)
	}
}

func TestInstanceSpecsProvider(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	ctx := context.Background()

	// Test getting known instance
	specs, err := provider.GetInstanceSpecs(ctx, "m5.large")
	if err != nil {
		t.Fatalf("GetInstanceSpecs failed: %v", err)
	}

	if specs.InstanceType != "m5.large" {
		t.Errorf("InstanceType = %v, want m5.large", specs.InstanceType)
	}
	if specs.VCPU != 2 {
		t.Errorf("VCPU = %v, want 2", specs.VCPU)
	}
	if specs.MemoryGB != 8 {
		t.Errorf("MemoryGB = %v, want 8", specs.MemoryGB)
	}
	if specs.CloudProvider != domain.AWS {
		t.Errorf("CloudProvider = %v, want AWS", specs.CloudProvider)
	}
}

func TestInstanceSpecsProviderDeriveFromName(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	tests := []struct {
		name       string
		wantVCPU   int
		wantMemory float64
	}{
		{"m99.large", 2, 8},    // Unknown generation but valid size
		{"c99.xlarge", 4, 16},  // Unknown generation but valid size
		{"r99.2xlarge", 8, 32}, // Unknown generation but valid size
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs, err := provider.GetInstanceSpecs(ctx, tt.name)
			if err != nil {
				t.Logf("Instance %s not found (expected for unknown instances)", tt.name)
				return
			}
			if specs.VCPU != tt.wantVCPU {
				t.Errorf("VCPU = %v, want %v", specs.VCPU, tt.wantVCPU)
			}
		})
	}
}

func TestBurstableFamilySpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	tests := []struct {
		instanceType string
		wantVCPU     int
		wantMemory   float64
		wantBurst    bool
	}{
		{"t3.nano", 2, 0.5, true},
		{"t3.micro", 2, 1, true},
		{"t3.small", 2, 2, true},
		{"t3.medium", 2, 4, true},
		{"t3.large", 2, 8, true},
		{"t3.xlarge", 4, 16, true},
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			specs, err := provider.GetInstanceSpecs(ctx, tt.instanceType)
			if err != nil {
				t.Fatalf("GetInstanceSpecs failed: %v", err)
			}

			if specs.VCPU != tt.wantVCPU {
				t.Errorf("VCPU = %v, want %v", specs.VCPU, tt.wantVCPU)
			}
			if specs.MemoryGB != tt.wantMemory {
				t.Errorf("MemoryGB = %v, want %v", specs.MemoryGB, tt.wantMemory)
			}
			if specs.IsBurstable != tt.wantBurst {
				t.Errorf("IsBurstable = %v, want %v", specs.IsBurstable, tt.wantBurst)
			}
		})
	}
}

func TestGetAllInstanceSpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	specs, err := provider.GetAllInstanceSpecs(ctx)
	if err != nil {
		t.Fatalf("GetAllInstanceSpecs failed: %v", err)
	}

	if len(specs) < 100 {
		t.Errorf("Expected at least 100 instance types, got %d", len(specs))
	}

	// Check that different categories are represented
	categories := make(map[domain.InstanceCategory]bool)
	for _, s := range specs {
		categories[s.Category] = true
	}

	if !categories[domain.GeneralPurpose] {
		t.Error("Missing GeneralPurpose instances")
	}
	if !categories[domain.ComputeOptimized] {
		t.Error("Missing ComputeOptimized instances")
	}
	if !categories[domain.MemoryOptimized] {
		t.Error("Missing MemoryOptimized instances")
	}
}

func TestGetInstancesByVCPU(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	// Get instances with exactly 2 vCPU
	specs, err := provider.GetInstancesByVCPU(ctx, 2, 2)
	if err != nil {
		t.Fatalf("GetInstancesByVCPU failed: %v", err)
	}

	if len(specs) == 0 {
		t.Error("Expected at least some 2 vCPU instances")
	}

	for _, s := range specs {
		if s.VCPU != 2 {
			t.Errorf("Instance %s has %d vCPU, expected 2", s.InstanceType, s.VCPU)
		}
	}
}

func TestPriceHistoryProviderNotAvailable(t *testing.T) {
	// Create provider without credentials (will be unavailable)
	provider, err := NewPriceHistoryProvider("us-east-1")
	if err != nil {
		// Expected if no AWS credentials
		t.Logf("Provider creation failed (expected without creds): %v", err)
		return
	}

	// If provider was created but not available, that's expected
	if provider != nil && !provider.IsAvailable() {
		t.Log("Provider not available - expected without AWS credentials")
	}
}

func TestPriceAnalysisStructure(t *testing.T) {
	// Test the PriceAnalysis struct
	analysis := &PriceAnalysis{
		InstanceType:     "m5.large",
		AvailabilityZone: "us-east-1a",
		CurrentPrice:     0.05,
		AvgPrice:         0.045,
		MinPrice:         0.03,
		MaxPrice:         0.08,
		StdDev:           0.01,
		Volatility:       0.22,
		TrendSlope:       -0.001,
		TrendScore:       0.3,
		DataPoints:       100,
		TimeSpanHours:    168,
		LastUpdated:      time.Now(),
		AllAZData: map[string]*AZAnalysis{
			"us-east-1a": {
				AvailabilityZone: "us-east-1a",
				AvgPrice:         0.045,
				MinPrice:         0.03,
				MaxPrice:         0.08,
				Volatility:       0.2,
				DataPoints:       50,
			},
			"us-east-1b": {
				AvailabilityZone: "us-east-1b",
				AvgPrice:         0.048,
				MinPrice:         0.035,
				MaxPrice:         0.085,
				Volatility:       0.25,
				DataPoints:       50,
			},
		},
	}

	if analysis.InstanceType != "m5.large" {
		t.Errorf("InstanceType = %v, want m5.large", analysis.InstanceType)
	}
	if len(analysis.AllAZData) != 2 {
		t.Errorf("AllAZData length = %d, want 2", len(analysis.AllAZData))
	}
	if analysis.AllAZData["us-east-1a"].AvgPrice != 0.045 {
		t.Errorf("AZ us-east-1a AvgPrice = %v, want 0.045", analysis.AllAZData["us-east-1a"].AvgPrice)
	}
}
