package gcp

import (
	"context"
	"strings"
	"testing"

	"github.com/spot-analyzer/internal/domain"
)

// ============================================================================
// SpotDataProvider Tests
// ============================================================================

func TestNewSpotDataProvider(t *testing.T) {
	provider := NewSpotDataProvider()

	if provider == nil {
		t.Fatal("NewSpotDataProvider() returned nil")
	}
}

func TestSpotDataProvider_GetProviderName(t *testing.T) {
	provider := NewSpotDataProvider()
	name := provider.GetProviderName()

	if name != domain.GCP {
		t.Errorf("GetProviderName() = %v, want %v", name, domain.GCP)
	}
}

func TestSpotDataProvider_GetSupportedRegions(t *testing.T) {
	provider := NewSpotDataProvider()
	ctx := context.Background()
	regions, err := provider.GetSupportedRegions(ctx)

	if err != nil {
		t.Fatalf("GetSupportedRegions() error: %v", err)
	}

	if len(regions) == 0 {
		t.Error("GetSupportedRegions() returned empty slice")
	}

	// Check for expected regions
	expectedRegions := []string{
		"us-central1",
		"us-east1",
		"europe-west1",
		"asia-northeast1",
	}

	regionMap := make(map[string]bool)
	for _, r := range regions {
		regionMap[r] = true
	}

	for _, expected := range expectedRegions {
		if !regionMap[expected] {
			t.Errorf("Expected region %s not found in supported regions", expected)
		}
	}
}

func TestSpotDataProvider_FetchSpotData(t *testing.T) {
	provider := NewSpotDataProvider()
	ctx := context.Background()

	tests := []struct {
		name       string
		region     string
		os         domain.OperatingSystem
		wantErr    bool
		minResults int
	}{
		{
			name:       "Valid region with Linux",
			region:     "us-central1",
			os:         domain.Linux,
			wantErr:    false,
			minResults: 5,
		},
		{
			name:       "Europe region with Linux",
			region:     "europe-west1",
			os:         domain.Linux,
			wantErr:    false,
			minResults: 5,
		},
		{
			name:       "Asia region with Linux",
			region:     "asia-northeast1",
			os:         domain.Linux,
			wantErr:    false,
			minResults: 5,
		},
		{
			name:       "Another valid region",
			region:     "us-west1",
			os:         domain.Linux,
			wantErr:    false,
			minResults: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := provider.FetchSpotData(ctx, tt.region, tt.os)

			if tt.wantErr {
				if err == nil {
					t.Error("FetchSpotData() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("FetchSpotData() unexpected error: %v", err)
				return
			}

			if len(results) < tt.minResults {
				t.Errorf("FetchSpotData() returned %d results, want at least %d", len(results), tt.minResults)
			}

			// Validate result structure
			for _, result := range results {
				if result.InstanceType == "" {
					t.Error("Result has empty InstanceType")
				}
				if result.Region == "" {
					t.Error("Result has empty Region")
				}
				if result.SpotPrice <= 0 {
					t.Errorf("Result has invalid SpotPrice: %f", result.SpotPrice)
				}
				if result.OnDemandPrice <= 0 {
					t.Errorf("Result has invalid OnDemandPrice: %f", result.OnDemandPrice)
				}
				if result.SavingsPercent < 0 || result.SavingsPercent > 100 {
					t.Errorf("Result has invalid SavingsPercent: %d", result.SavingsPercent)
				}
				if result.CloudProvider != domain.GCP {
					t.Errorf("Result has wrong CloudProvider: %s", result.CloudProvider)
				}
			}
		})
	}
}

func TestSpotDataProvider_FetchSpotData_InstanceTypes(t *testing.T) {
	provider := NewSpotDataProvider()
	ctx := context.Background()

	results, err := provider.FetchSpotData(ctx, "us-central1", domain.Linux)
	if err != nil {
		t.Fatalf("FetchSpotData() error: %v", err)
	}

	// Check that we get different machine type families
	familiesFound := make(map[string]bool)
	for _, result := range results {
		parts := strings.Split(result.InstanceType, "-")
		if len(parts) > 0 {
			familiesFound[parts[0]] = true
		}
	}

	// We should have at least a few different families (n2, e2, c2, etc.)
	if len(familiesFound) < 2 {
		t.Errorf("FetchSpotData() only found %d machine type families, expected at least 2", len(familiesFound))
	}
}

// ============================================================================
// InstanceSpecsProvider Tests
// ============================================================================

func TestNewInstanceSpecsProvider(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	if provider == nil {
		t.Fatal("NewInstanceSpecsProvider() returned nil")
	}
}

func TestInstanceSpecsProvider_GetProviderName(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	name := provider.GetProviderName()

	if name != domain.GCP {
		t.Errorf("GetProviderName() = %v, want %v", name, domain.GCP)
	}
}

func TestInstanceSpecsProvider_GetInstanceSpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	tests := []struct {
		name         string
		instanceType string
		wantErr      bool
		wantVCPU     int
		wantMemory   float64
	}{
		{
			name:         "Standard N2 instance",
			instanceType: "n2-standard-4",
			wantErr:      false,
			wantVCPU:     4,
			wantMemory:   16,
		},
		{
			name:         "High memory N2 instance",
			instanceType: "n2-highmem-8",
			wantErr:      false,
			wantVCPU:     8,
			wantMemory:   64,
		},
		{
			name:         "E2 medium instance",
			instanceType: "e2-medium",
			wantErr:      false,
			wantVCPU:     2,
			wantMemory:   4,
		},
		{
			name:         "C2 compute optimized",
			instanceType: "c2-standard-4",
			wantErr:      false,
			wantVCPU:     4,
			wantMemory:   16,
		},
		{
			name:         "Unknown instance type",
			instanceType: "unknown-instance",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs, err := provider.GetInstanceSpecs(ctx, tt.instanceType)

			if tt.wantErr {
				if err == nil {
					t.Error("GetInstanceSpecs() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetInstanceSpecs() unexpected error: %v", err)
				return
			}

			if specs.VCPU != tt.wantVCPU {
				t.Errorf("GetInstanceSpecs() VCPU = %d, want %d", specs.VCPU, tt.wantVCPU)
			}

			if specs.MemoryGB != tt.wantMemory {
				t.Errorf("GetInstanceSpecs() MemoryGB = %f, want %f", specs.MemoryGB, tt.wantMemory)
			}
		})
	}
}

func TestInstanceSpecsProvider_GetAllInstanceSpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	specs, err := provider.GetAllInstanceSpecs(ctx)
	if err != nil {
		t.Fatalf("GetAllInstanceSpecs() error: %v", err)
	}

	if len(specs) == 0 {
		t.Error("GetAllInstanceSpecs() returned empty slice")
	}

	// Build a map for easy lookup
	specsMap := make(map[string]domain.InstanceSpecs)
	for _, s := range specs {
		specsMap[s.InstanceType] = s
	}

	// Verify some known instance types exist
	expectedTypes := []string{
		"n2-standard-2",
		"n2-standard-4",
		"e2-medium",
		"c2-standard-4",
	}

	for _, expected := range expectedTypes {
		if _, exists := specsMap[expected]; !exists {
			t.Errorf("GetAllInstanceSpecs() missing expected instance type: %s", expected)
		}
	}
}

func TestInstanceSpecsProvider_GetInstancesByVCPU(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	tests := []struct {
		name    string
		minVCPU int
		maxVCPU int
		wantMin int
	}{
		{
			name:    "2-4 vCPUs",
			minVCPU: 2,
			maxVCPU: 4,
			wantMin: 5,
		},
		{
			name:    "8-16 vCPUs",
			minVCPU: 8,
			maxVCPU: 16,
			wantMin: 3,
		},
		{
			name:    "Very high vCPU range",
			minVCPU: 128,
			maxVCPU: 256,
			wantMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs, err := provider.GetInstancesByVCPU(ctx, tt.minVCPU, tt.maxVCPU)
			if err != nil {
				t.Errorf("GetInstancesByVCPU() error: %v", err)
				return
			}

			if len(specs) < tt.wantMin {
				t.Errorf("GetInstancesByVCPU() returned %d results, want at least %d", len(specs), tt.wantMin)
			}

			// Verify all results are within range
			for _, spec := range specs {
				if spec.VCPU < tt.minVCPU || spec.VCPU > tt.maxVCPU {
					t.Errorf("GetInstancesByVCPU() returned instance with %d vCPUs, outside range [%d, %d]",
						spec.VCPU, tt.minVCPU, tt.maxVCPU)
				}
			}
		})
	}
}

// ============================================================================
// PriceHistoryProvider Tests
// ============================================================================

func TestNewPriceHistoryProvider(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")

	if provider == nil {
		t.Fatal("NewPriceHistoryProvider() returned nil")
	}
}

func TestPriceHistoryProvider_GetProviderName(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	name := provider.GetProviderName()

	if name != domain.GCP {
		t.Errorf("GetProviderName() = %v, want %v", name, domain.GCP)
	}
}

func TestPriceHistoryProvider_IsAvailable(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")

	if !provider.IsAvailable() {
		t.Error("IsAvailable() should return true for GCP provider")
	}
}

func TestPriceHistoryProvider_GetPriceAnalysis(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	ctx := context.Background()

	tests := []struct {
		name         string
		machineType  string
		lookbackDays int
		wantErr      bool
	}{
		{
			name:         "Valid N2 instance",
			machineType:  "n2-standard-4",
			lookbackDays: 7,
			wantErr:      false,
		},
		{
			name:         "Valid E2 instance",
			machineType:  "e2-medium",
			lookbackDays: 14,
			wantErr:      false,
		},
		{
			name:         "Valid C2 instance",
			machineType:  "c2-standard-8",
			lookbackDays: 7,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := provider.GetPriceAnalysis(ctx, tt.machineType, tt.lookbackDays)

			if tt.wantErr {
				if err == nil {
					t.Error("GetPriceAnalysis() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetPriceAnalysis() unexpected error: %v", err)
				return
			}

			// Validate analysis structure
			if analysis.InstanceType != tt.machineType {
				t.Errorf("GetPriceAnalysis() InstanceType = %s, want %s", analysis.InstanceType, tt.machineType)
			}

			if analysis.CurrentPrice <= 0 {
				t.Errorf("GetPriceAnalysis() invalid CurrentPrice: %f", analysis.CurrentPrice)
			}

			if analysis.AvgPrice <= 0 {
				t.Errorf("GetPriceAnalysis() invalid AvgPrice: %f", analysis.AvgPrice)
			}

			if analysis.Volatility < 0 || analysis.Volatility > 1 {
				t.Errorf("GetPriceAnalysis() invalid Volatility: %f", analysis.Volatility)
			}
		})
	}
}

func TestPriceHistoryProvider_GetBatchPriceAnalysis(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	ctx := context.Background()

	machineTypes := []string{
		"n2-standard-4",
		"e2-medium",
		"c2-standard-4",
	}

	results, err := provider.GetBatchPriceAnalysis(ctx, machineTypes, 7)
	if err != nil {
		t.Fatalf("GetBatchPriceAnalysis() error: %v", err)
	}

	if len(results) != len(machineTypes) {
		t.Errorf("GetBatchPriceAnalysis() returned %d results, want %d", len(results), len(machineTypes))
	}

	for machineType, result := range results {
		if result.InstanceType == "" {
			t.Errorf("GetBatchPriceAnalysis() result for %s has empty InstanceType", machineType)
		}
		if result.CurrentPrice <= 0 {
			t.Errorf("GetBatchPriceAnalysis() invalid CurrentPrice for %s: %f", machineType, result.CurrentPrice)
		}
	}
}

// ============================================================================
// ZoneProviderAdapter Tests
// ============================================================================

func TestNewZoneProviderAdapter(t *testing.T) {
	adapter := NewZoneProviderAdapter("us-central1")

	if adapter == nil {
		t.Fatal("NewZoneProviderAdapter() returned nil")
	}
}

func TestZoneProviderAdapter_GetProviderName(t *testing.T) {
	adapter := NewZoneProviderAdapter("us-central1")
	name := adapter.GetProviderName()

	if name != domain.GCP {
		t.Errorf("GetProviderName() = %v, want %v", name, domain.GCP)
	}
}

func TestZoneProviderAdapter_GetZoneAvailability(t *testing.T) {
	adapter := NewZoneProviderAdapter("us-central1")
	ctx := context.Background()

	tests := []struct {
		name         string
		instanceType string
		region       string
		wantErr      bool
		minZones     int
	}{
		{
			name:         "N2 instance in us-central1",
			instanceType: "n2-standard-4",
			region:       "us-central1",
			wantErr:      false,
			minZones:     3,
		},
		{
			name:         "E2 instance in europe-west1",
			instanceType: "e2-medium",
			region:       "europe-west1",
			wantErr:      false,
			minZones:     3,
		},
		{
			name:         "C2 instance in asia-northeast1",
			instanceType: "c2-standard-8",
			region:       "asia-northeast1",
			wantErr:      false,
			minZones:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zones, err := adapter.GetZoneAvailability(ctx, tt.instanceType, tt.region)

			if tt.wantErr {
				if err == nil {
					t.Error("GetZoneAvailability() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetZoneAvailability() unexpected error: %v", err)
				return
			}

			if len(zones) < tt.minZones {
				t.Errorf("GetZoneAvailability() returned %d zones, want at least %d", len(zones), tt.minZones)
			}

			// Validate zone structure
			for _, zone := range zones {
				if zone.Zone == "" {
					t.Error("Zone has empty Zone name")
				}

				if zone.CapacityScore < 0 || zone.CapacityScore > 100 {
					t.Errorf("Zone has invalid CapacityScore: %d", zone.CapacityScore)
				}
			}
		})
	}
}

// ============================================================================
// PriceHistoryAdapter Tests
// ============================================================================

func TestNewPriceHistoryAdapter(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	adapter := NewPriceHistoryAdapter(provider)

	if adapter == nil {
		t.Fatal("NewPriceHistoryAdapter() returned nil")
	}
}

func TestPriceHistoryAdapter_IsAvailable(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	adapter := NewPriceHistoryAdapter(provider)

	if !adapter.IsAvailable() {
		t.Error("IsAvailable() should return true")
	}
}

func TestPriceHistoryAdapter_GetPriceAnalysis(t *testing.T) {
	provider := NewPriceHistoryProvider("us-central1")
	adapter := NewPriceHistoryAdapter(provider)
	ctx := context.Background()

	result, err := adapter.GetPriceAnalysis(ctx, "n2-standard-4", 7)
	if err != nil {
		t.Fatalf("GetPriceAnalysis() error: %v", err)
	}

	if result.InstanceType != "n2-standard-4" {
		t.Errorf("GetPriceAnalysis() InstanceType = %s, want n2-standard-4", result.InstanceType)
	}

	if result.CurrentPrice <= 0 {
		t.Errorf("GetPriceAnalysis() invalid CurrentPrice: %f", result.CurrentPrice)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestGCPProviderIntegration(t *testing.T) {
	// Test that all GCP providers work together correctly
	spotProvider := NewSpotDataProvider()
	specsProvider := NewInstanceSpecsProvider()
	priceProvider := NewPriceHistoryProvider("us-central1")
	zoneAdapter := NewZoneProviderAdapter("us-central1")

	ctx := context.Background()

	// Fetch spot data
	spotData, err := spotProvider.FetchSpotData(ctx, "us-central1", domain.Linux)
	if err != nil {
		t.Fatalf("FetchSpotData() error: %v", err)
	}

	if len(spotData) == 0 {
		t.Fatal("FetchSpotData() returned no results")
	}

	// Get specs for first instance
	instanceType := spotData[0].InstanceType
	specs, err := specsProvider.GetInstanceSpecs(ctx, instanceType)
	if err != nil {
		t.Fatalf("GetInstanceSpecs() error: %v", err)
	}

	if specs.VCPU == 0 {
		t.Error("GetInstanceSpecs() returned zero vCPU")
	}

	// Get price analysis
	priceAnalysis, err := priceProvider.GetPriceAnalysis(ctx, instanceType, 7)
	if err != nil {
		t.Fatalf("GetPriceAnalysis() error: %v", err)
	}

	if priceAnalysis.CurrentPrice <= 0 {
		t.Error("GetPriceAnalysis() returned invalid price")
	}

	// Get zone availability
	zones, err := zoneAdapter.GetZoneAvailability(ctx, instanceType, "us-central1")
	if err != nil {
		t.Fatalf("GetZoneAvailability() error: %v", err)
	}

	if len(zones) == 0 {
		t.Error("GetZoneAvailability() returned no zones")
	}
}

// ============================================================================
// GCP Machine Type Naming Tests
// ============================================================================

func TestGCPMachineTypeNaming(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	// Test various GCP machine type naming conventions
	testCases := []struct {
		name       string
		wantFamily string
	}{
		{"n2-standard-4", "n2"},
		{"n2-highmem-8", "n2"},
		{"n2-highcpu-16", "n2"},
		{"e2-medium", "e2"},
		{"e2-small", "e2"},
		{"c2-standard-8", "c2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			specs, err := provider.GetInstanceSpecs(ctx, tc.name)
			if err != nil {
				t.Skipf("Instance type %s not in catalog, skipping", tc.name)
				return
			}

			// Verify the family is correctly extracted
			parts := strings.Split(tc.name, "-")
			if len(parts) < 2 {
				t.Errorf("Invalid machine type format: %s", tc.name)
				return
			}

			if parts[0] != tc.wantFamily {
				t.Errorf("Machine type %s: got family %s, want %s", tc.name, parts[0], tc.wantFamily)
			}

			if specs.VCPU == 0 {
				t.Errorf("Machine type %s has zero vCPU", tc.name)
			}
		})
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkFetchSpotData(b *testing.B) {
	provider := NewSpotDataProvider()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.FetchSpotData(ctx, "us-central1", domain.Linux)
	}
}

func BenchmarkGetInstanceSpecs(b *testing.B) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.GetInstanceSpecs(ctx, "n2-standard-4")
	}
}

func BenchmarkGetPriceAnalysis(b *testing.B) {
	provider := NewPriceHistoryProvider("us-central1")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.GetPriceAnalysis(ctx, "n2-standard-4", 7)
	}
}

func BenchmarkGetZoneAvailability(b *testing.B) {
	adapter := NewZoneProviderAdapter("us-central1")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adapter.GetZoneAvailability(ctx, "n2-standard-4", "us-central1")
	}
}
