// Package azure contains tests for Azure spot data providers.
package azure

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spot-analyzer/internal/domain"
)

func TestNewSpotDataProvider(t *testing.T) {
	provider := NewSpotDataProvider()
	if provider == nil {
		t.Fatal("NewSpotDataProvider() returned nil")
	}
	if provider.GetProviderName() != domain.Azure {
		t.Errorf("GetProviderName() = %v, want %v", provider.GetProviderName(), domain.Azure)
	}
}

func TestGetSupportedRegions(t *testing.T) {
	provider := NewSpotDataProvider()
	ctx := context.Background()

	regions, err := provider.GetSupportedRegions(ctx)
	if err != nil {
		t.Fatalf("GetSupportedRegions() error = %v", err)
	}

	if len(regions) == 0 {
		t.Error("GetSupportedRegions() returned empty list")
	}

	// Check for some expected regions
	expectedRegions := map[string]bool{
		"eastus":        false,
		"westus2":       false,
		"westeurope":    false,
		"australiaeast": false,
	}

	for _, region := range regions {
		if _, ok := expectedRegions[region]; ok {
			expectedRegions[region] = true
		}
	}

	for region, found := range expectedRegions {
		if !found {
			t.Errorf("Expected region %s not found in supported regions", region)
		}
	}
}

func TestNewInstanceSpecsProvider(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	if provider == nil {
		t.Fatal("NewInstanceSpecsProvider() returned nil")
	}
	if provider.GetProviderName() != domain.Azure {
		t.Errorf("GetProviderName() = %v, want %v", provider.GetProviderName(), domain.Azure)
	}
}

func TestGetAllInstanceSpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	specs, err := provider.GetAllInstanceSpecs(ctx)
	if err != nil {
		t.Fatalf("GetAllInstanceSpecs() error = %v", err)
	}

	if len(specs) == 0 {
		t.Error("GetAllInstanceSpecs() returned empty list")
	}

	// Verify specs have required fields
	for _, spec := range specs {
		if spec.InstanceType == "" {
			t.Error("Instance spec has empty InstanceType")
		}
		if spec.VCPU <= 0 {
			t.Errorf("Instance %s has invalid VCPU: %d", spec.InstanceType, spec.VCPU)
		}
		if spec.MemoryGB <= 0 {
			t.Errorf("Instance %s has invalid MemoryGB: %f", spec.InstanceType, spec.MemoryGB)
		}
		if spec.CloudProvider != domain.Azure {
			t.Errorf("Instance %s has wrong CloudProvider: %v", spec.InstanceType, spec.CloudProvider)
		}
	}
}

func TestGetInstanceSpecs(t *testing.T) {
	provider := NewInstanceSpecsProvider()
	ctx := context.Background()

	tests := []struct {
		vmSize   string
		wantVCPU int
		wantGPU  bool
		wantArch string
	}{
		{"Standard_D2_v5", 2, false, "x86_64"},
		{"Standard_D4_v5", 4, false, "x86_64"},
		{"Standard_E2_v5", 2, false, "x86_64"},
		{"Standard_F2s_v2", 2, false, "x86_64"},
	}

	for _, tt := range tests {
		t.Run(tt.vmSize, func(t *testing.T) {
			spec, err := provider.GetInstanceSpecs(ctx, tt.vmSize)
			if err != nil {
				t.Fatalf("GetInstanceSpecs(%s) error = %v", tt.vmSize, err)
			}

			if spec.VCPU != tt.wantVCPU {
				t.Errorf("VCPU = %d, want %d", spec.VCPU, tt.wantVCPU)
			}
			if spec.HasGPU != tt.wantGPU {
				t.Errorf("HasGPU = %v, want %v", spec.HasGPU, tt.wantGPU)
			}
			if spec.Architecture != tt.wantArch {
				t.Errorf("Architecture = %s, want %s", spec.Architecture, tt.wantArch)
			}
		})
	}
}

func TestDeriveSpecsFromName(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		vmSize       string
		wantVCPU     int
		wantCategory domain.InstanceCategory
		wantHasGPU   bool
	}{
		{"Standard_D2s_v5", 2, domain.GeneralPurpose, false},
		{"Standard_E4s_v5", 4, domain.MemoryOptimized, false},
		{"Standard_F8s_v2", 8, domain.ComputeOptimized, false},
		{"Standard_NC6_v3", 6, domain.AcceleratedComputing, true},
		{"Standard_L8s_v3", 8, domain.StorageOptimized, false},
		{"Standard_B2s_v2", 2, domain.GeneralPurpose, false},
	}

	for _, tt := range tests {
		t.Run(tt.vmSize, func(t *testing.T) {
			spec := provider.deriveSpecsFromName(tt.vmSize)
			if spec == nil {
				t.Fatalf("deriveSpecsFromName(%s) returned nil", tt.vmSize)
			}

			if spec.VCPU != tt.wantVCPU {
				t.Errorf("VCPU = %d, want %d", spec.VCPU, tt.wantVCPU)
			}
			if spec.Category != tt.wantCategory {
				t.Errorf("Category = %v, want %v", spec.Category, tt.wantCategory)
			}
			if spec.HasGPU != tt.wantHasGPU {
				t.Errorf("HasGPU = %v, want %v", spec.HasGPU, tt.wantHasGPU)
			}
		})
	}
}

func TestEstimateInterruptionFrequency(t *testing.T) {
	provider := NewSpotDataProvider()

	tests := []struct {
		name             string
		vmSize           string
		savingsPercent   int
		wantInterruption domain.InterruptionFrequency
	}{
		{"burstable", "Standard_B2s", 70, domain.VeryLow},
		{"gpu", "Standard_NC24", 70, domain.High},
		{"high_savings", "Standard_D2s_v5", 90, domain.High},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.estimateInterruptionFrequency(tt.vmSize, tt.savingsPercent)
			if got != tt.wantInterruption {
				t.Errorf("estimateInterruptionFrequency(%s, %d) = %v, want %v",
					tt.vmSize, tt.savingsPercent, got, tt.wantInterruption)
			}
		})
	}
}

func TestPriceHistoryProvider(t *testing.T) {
	provider := NewPriceHistoryProvider("eastus")

	if !provider.IsAvailable() {
		t.Error("PriceHistoryProvider should always be available")
	}
}

func TestPriceHistoryAdapter(t *testing.T) {
	provider := NewPriceHistoryProvider("eastus")
	adapter := NewPriceHistoryAdapter(provider)

	if !adapter.IsAvailable() {
		t.Error("PriceHistoryAdapter should be available when provider is available")
	}
}

func TestExtractVMSize(t *testing.T) {
	provider := NewSpotDataProvider()

	tests := []struct {
		item     PriceItem
		expected string
	}{
		{
			item:     PriceItem{ArmSkuName: "Standard_D2s_v5"},
			expected: "Standard_D2s_v5",
		},
		{
			item:     PriceItem{SkuName: "D2s v5 Spot"},
			expected: "D2s_v5",
		},
		{
			item:     PriceItem{SkuName: "E4s v5 Low Priority"},
			expected: "E4s_v5",
		},
	}

	for i, tt := range tests {
		t.Run("case_"+strconv.Itoa(i), func(t *testing.T) {
			got := provider.extractVMSize(tt.item)
			if got != tt.expected {
				t.Errorf("extractVMSize() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestConvertToSpotData(t *testing.T) {
	provider := NewSpotDataProvider()

	spotPrices := []PriceItem{
		{
			ArmSkuName:    "Standard_D2s_v5",
			UnitPrice:     0.10,
			ArmRegionName: "eastus",
		},
		{
			ArmSkuName:    "Standard_D4s_v5",
			UnitPrice:     0.20,
			ArmRegionName: "eastus",
		},
	}

	onDemandPrices := []PriceItem{
		{
			ArmSkuName:    "Standard_D2s_v5",
			UnitPrice:     0.20, // 50% savings
			ArmRegionName: "eastus",
		},
		{
			ArmSkuName:    "Standard_D4s_v5",
			UnitPrice:     0.40, // 50% savings
			ArmRegionName: "eastus",
		},
	}

	result := provider.convertToSpotData(spotPrices, onDemandPrices, "eastus", domain.Linux)

	if len(result) != 2 {
		t.Fatalf("convertToSpotData() returned %d items, want 2", len(result))
	}

	for _, spotData := range result {
		if spotData.CloudProvider != domain.Azure {
			t.Errorf("CloudProvider = %v, want Azure", spotData.CloudProvider)
		}
		if spotData.SavingsPercent != 50 {
			t.Errorf("SavingsPercent = %d, want 50", spotData.SavingsPercent)
		}
		if spotData.Region != "eastus" {
			t.Errorf("Region = %s, want eastus", spotData.Region)
		}
	}
}

func TestGenerateAZData(t *testing.T) {
	provider := NewPriceHistoryProvider("eastus")

	basePrice := 0.10
	volatility := 0.1

	azData := provider.generateAZData("eastus", basePrice, volatility)

	if len(azData) != 3 {
		t.Errorf("generateAZData() returned %d AZs, want 3", len(azData))
	}

	for azName, analysis := range azData {
		if analysis.AvailabilityZone != azName {
			t.Errorf("AZ name mismatch: %s vs %s", analysis.AvailabilityZone, azName)
		}
		if analysis.AvgPrice <= 0 {
			t.Errorf("Invalid AvgPrice for %s: %f", azName, analysis.AvgPrice)
		}
		if analysis.Volatility <= 0 {
			t.Errorf("Invalid Volatility for %s: %f", azName, analysis.Volatility)
		}
	}
}

func TestGetAZRecommendations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := NewPriceHistoryProvider("eastus")

	// This test may fail due to network issues, but should not panic
	recs, err := provider.GetAZRecommendations(ctx, "Standard_D2s_v5")

	// If we get an error due to network, that's acceptable in tests
	if err != nil {
		t.Skipf("Skipping due to network error: %v", err)
		return
	}

	// Azure should always return 3 availability zones
	if len(recs) != 3 {
		t.Errorf("Expected 3 AZ recommendations, got %d", len(recs))
	}

	for _, rec := range recs {
		if rec.AvailabilityZone == "" {
			t.Error("Empty AvailabilityZone in recommendation")
		}
		if rec.Rank <= 0 {
			t.Errorf("Invalid Rank: %d", rec.Rank)
		}
		if rec.AvgPrice <= 0 {
			t.Errorf("Invalid AvgPrice: %f", rec.AvgPrice)
		}
		// Verify AZ naming format (e.g., "eastus-zone1")
		if !strings.Contains(rec.AvailabilityZone, "zone") {
			t.Errorf("Invalid AZ name format: %s", rec.AvailabilityZone)
		}
	}

	// Verify ranks are sequential
	for i, rec := range recs {
		if rec.Rank != i+1 {
			t.Errorf("Expected rank %d, got %d", i+1, rec.Rank)
		}
	}
}

func TestParseVMSize(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		size       string
		wantSeries string
		wantVCPU   int
	}{
		{"D2s_v5", "D", 2},
		{"E4s_v5", "E", 4},
		{"F8s_v2", "F", 8},
		{"NC24_v3", "NC", 24},
		{"M128s_v2", "M", 128},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			series, vcpu, _ := provider.parseVMSize(tt.size)
			if series != tt.wantSeries {
				t.Errorf("series = %s, want %s", series, tt.wantSeries)
			}
			if vcpu != tt.wantVCPU {
				t.Errorf("vcpu = %d, want %d", vcpu, tt.wantVCPU)
			}
		})
	}
}

func TestParseGeneration(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		size    string
		wantGen domain.InstanceGeneration
	}{
		{"D2s_v5", domain.Current},
		{"E4s_v4", domain.Previous},
		{"F8s_v3", domain.Legacy},
		{"NC24_v2", domain.Deprecated}, // v2 is deprecated per the code
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			got := provider.parseGeneration(tt.size)
			if got != tt.wantGen {
				t.Errorf("parseGeneration(%s) = %v, want %v", tt.size, got, tt.wantGen)
			}
		})
	}
}

func TestParseArchitecture(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		size     string
		wantArch string
	}{
		{"D2s_v5", "x86_64"},
		{"Dps_v5", "arm64"},
		{"E4ps_v5", "arm64"},
		{"NC24_v3", "x86_64"},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			got := provider.parseArchitecture(tt.size)
			if got != tt.wantArch {
				t.Errorf("parseArchitecture(%s) = %s, want %s", tt.size, got, tt.wantArch)
			}
		})
	}
}

func TestIsBurstable(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		series string
		want   bool
	}{
		{"B", true},
		{"B1", true},
		{"D", false},
		{"E", false},
		{"NC", false},
	}

	for _, tt := range tests {
		t.Run(tt.series, func(t *testing.T) {
			got := provider.isBurstable(tt.series)
			if got != tt.want {
				t.Errorf("isBurstable(%s) = %v, want %v", tt.series, got, tt.want)
			}
		})
	}
}

func TestIsGPUSeries(t *testing.T) {
	provider := NewInstanceSpecsProvider()

	tests := []struct {
		series string
		want   bool
	}{
		{"NC", true},
		{"ND", true},
		{"NV", true},
		{"NG", true},
		{"D", false},
		{"E", false},
		{"F", false},
	}

	for _, tt := range tests {
		t.Run(tt.series, func(t *testing.T) {
			got := provider.isGPUSeries(tt.series)
			if got != tt.want {
				t.Errorf("isGPUSeries(%s) = %v, want %v", tt.series, got, tt.want)
			}
		})
	}
}
