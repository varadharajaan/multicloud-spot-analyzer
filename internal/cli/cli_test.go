package cli

import (
	"context"
	"testing"
	"time"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
)

func TestCLINew(t *testing.T) {
	cli := New()
	if cli == nil {
		t.Error("New() should return a non-nil CLI")
	}
	if cli.rootCmd == nil {
		t.Error("CLI rootCmd should not be nil")
	}
}

func TestCLIRootCommand(t *testing.T) {
	cli := New()

	// Root command should have subcommands
	if len(cli.rootCmd.Commands()) == 0 {
		t.Error("Root command should have subcommands")
	}

	// Check for expected subcommands
	expectedCommands := []string{"analyze", "regions", "refresh", "predict", "az", "web"}
	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range cli.rootCmd.Commands() {
			if cmd.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found", expected)
		}
	}
}

func TestParseCloudProviderFromDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected domain.CloudProvider
	}{
		{"aws lowercase", "aws", domain.AWS},
		{"AWS uppercase", "AWS", domain.AWS},
		{"azure lowercase", "azure", domain.Azure},
		{"Azure mixed", "Azure", domain.Azure},
		{"gcp lowercase", "gcp", domain.GCP},
		{"empty defaults to AWS", "", domain.AWS},
		{"unknown defaults to AWS", "unknown", domain.AWS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := domain.ParseCloudProvider(tt.input)
			if result != tt.expected {
				t.Errorf("ParseCloudProvider(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAWSProviderCreation(t *testing.T) {
	factory := provider.GetFactory()

	// Test AWS spot data provider creation
	spotProvider, err := factory.CreateSpotDataProvider(domain.AWS)
	if err != nil {
		t.Errorf("Failed to create AWS spot provider: %v", err)
	}
	if spotProvider == nil {
		t.Error("AWS spot provider should not be nil")
	}

	// Test AWS specs provider creation
	specsProvider, err := factory.CreateInstanceSpecsProvider(domain.AWS)
	if err != nil {
		t.Errorf("Failed to create AWS specs provider: %v", err)
	}
	if specsProvider == nil {
		t.Error("AWS specs provider should not be nil")
	}
}

func TestAzureProviderCreation(t *testing.T) {
	factory := provider.GetFactory()

	// Test Azure spot data provider creation
	spotProvider, err := factory.CreateSpotDataProvider(domain.Azure)
	if err != nil {
		t.Errorf("Failed to create Azure spot provider: %v", err)
	}
	if spotProvider == nil {
		t.Error("Azure spot provider should not be nil")
	}

	// Test Azure specs provider creation
	specsProvider, err := factory.CreateInstanceSpecsProvider(domain.Azure)
	if err != nil {
		t.Errorf("Failed to create Azure specs provider: %v", err)
	}
	if specsProvider == nil {
		t.Error("Azure specs provider should not be nil")
	}
}

func TestAWSRegions(t *testing.T) {
	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(domain.AWS)
	if err != nil {
		t.Fatalf("Failed to create AWS spot provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	regions, err := spotProvider.GetSupportedRegions(ctx)
	if err != nil {
		t.Errorf("Failed to get AWS regions: %v", err)
	}

	if len(regions) == 0 {
		t.Error("AWS should return at least one region")
	}

	// Check for expected regions
	expectedRegions := []string{"us-east-1", "us-west-2", "eu-west-1"}
	for _, expected := range expectedRegions {
		found := false
		for _, region := range regions {
			if region == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected AWS region '%s' not found", expected)
		}
	}

	t.Logf("AWS Regions: %d total", len(regions))
}

func TestAzureRegions(t *testing.T) {
	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(domain.Azure)
	if err != nil {
		t.Fatalf("Failed to create Azure spot provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	regions, err := spotProvider.GetSupportedRegions(ctx)
	if err != nil {
		t.Errorf("Failed to get Azure regions: %v", err)
	}

	if len(regions) == 0 {
		t.Error("Azure should return at least one region")
	}

	// Check for expected regions
	expectedRegions := []string{"eastus", "westus2", "westeurope"}
	for _, expected := range expectedRegions {
		found := false
		for _, region := range regions {
			if region == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected Azure region '%s' not found", expected)
		}
	}

	t.Logf("Azure Regions: %d total", len(regions))
}

func TestAWSSpotDataFetch(t *testing.T) {
	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(domain.AWS)
	if err != nil {
		t.Fatalf("Failed to create AWS spot provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	spotData, err := spotProvider.FetchSpotData(ctx, "us-east-1", domain.Linux)
	if err != nil {
		t.Errorf("Failed to fetch AWS spot data: %v", err)
	}

	if len(spotData) == 0 {
		t.Error("AWS should return spot data")
	}

	// Verify data quality
	for i, data := range spotData {
		if i > 10 {
			break // Just check first 10
		}
		if data.InstanceType == "" {
			t.Error("InstanceType should not be empty")
		}
		if data.CloudProvider != domain.AWS {
			t.Errorf("CloudProvider = %v, want AWS", data.CloudProvider)
		}
		if data.Region != "us-east-1" {
			t.Errorf("Region = %s, want us-east-1", data.Region)
		}
	}

	t.Logf("AWS Spot Data: %d instances for us-east-1", len(spotData))
}

func TestAzureSpotDataFetch(t *testing.T) {
	factory := provider.GetFactory()
	spotProvider, err := factory.CreateSpotDataProvider(domain.Azure)
	if err != nil {
		t.Fatalf("Failed to create Azure spot provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	spotData, err := spotProvider.FetchSpotData(ctx, "eastus", domain.Linux)
	if err != nil {
		t.Errorf("Failed to fetch Azure spot data: %v", err)
	}

	if len(spotData) == 0 {
		t.Log("Azure returned no spot data (may be due to API issues)")
	}

	// Verify data quality
	for i, data := range spotData {
		if i > 10 {
			break // Just check first 10
		}
		if data.InstanceType == "" {
			t.Error("InstanceType should not be empty")
		}
		if data.CloudProvider != domain.Azure {
			t.Errorf("CloudProvider = %v, want Azure", data.CloudProvider)
		}
		if data.Region != "eastus" {
			t.Errorf("Region = %s, want eastus", data.Region)
		}
	}

	t.Logf("Azure Spot Data: %d instances for eastus", len(spotData))
}

func TestAWSInstanceSpecs(t *testing.T) {
	factory := provider.GetFactory()
	specsProvider, err := factory.CreateInstanceSpecsProvider(domain.AWS)
	if err != nil {
		t.Fatalf("Failed to create AWS specs provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allSpecs, err := specsProvider.GetAllInstanceSpecs(ctx)
	if err != nil {
		t.Errorf("Failed to get AWS instance specs: %v", err)
	}

	if len(allSpecs) == 0 {
		t.Error("AWS should return instance specs")
	}

	// Test specific instance lookup
	spec, err := specsProvider.GetInstanceSpecs(ctx, "m5.large")
	if err != nil {
		t.Errorf("Failed to get m5.large specs: %v", err)
	}
	if spec == nil {
		t.Error("m5.large specs should not be nil")
	} else {
		if spec.InstanceType != "m5.large" {
			t.Errorf("InstanceType = %s, want m5.large", spec.InstanceType)
		}
		if spec.VCPU < 1 {
			t.Error("VCPU should be at least 1")
		}
		if spec.MemoryGB < 1 {
			t.Error("MemoryGB should be at least 1")
		}
	}

	t.Logf("AWS Instance Specs: %d instance types", len(allSpecs))
}

func TestAzureInstanceSpecs(t *testing.T) {
	factory := provider.GetFactory()
	specsProvider, err := factory.CreateInstanceSpecsProvider(domain.Azure)
	if err != nil {
		t.Fatalf("Failed to create Azure specs provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allSpecs, err := specsProvider.GetAllInstanceSpecs(ctx)
	if err != nil {
		t.Errorf("Failed to get Azure instance specs: %v", err)
	}

	if len(allSpecs) == 0 {
		t.Error("Azure should return instance specs")
	}

	// Test specific instance lookup
	spec, err := specsProvider.GetInstanceSpecs(ctx, "Standard_D2s_v5")
	if err != nil {
		t.Errorf("Failed to get Standard_D2s_v5 specs: %v", err)
	}
	if spec == nil {
		t.Error("Standard_D2s_v5 specs should not be nil")
	} else {
		if spec.InstanceType != "Standard_D2s_v5" {
			t.Errorf("InstanceType = %s, want Standard_D2s_v5", spec.InstanceType)
		}
		if spec.VCPU < 1 {
			t.Error("VCPU should be at least 1")
		}
		if spec.MemoryGB < 1 {
			t.Error("MemoryGB should be at least 1")
		}
		if spec.CloudProvider != domain.Azure {
			t.Errorf("CloudProvider = %v, want Azure", spec.CloudProvider)
		}
	}

	t.Logf("Azure Instance Specs: %d instance types", len(allSpecs))
}

func TestCloudProviderDefaultRegions(t *testing.T) {
	tests := []struct {
		provider       domain.CloudProvider
		expectedRegion string
	}{
		{domain.AWS, "us-east-1"},
		{domain.Azure, "eastus"},
		{domain.GCP, "us-central1"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			region := tt.provider.DefaultRegion()
			if region != tt.expectedRegion {
				t.Errorf("DefaultRegion() = %s, want %s", region, tt.expectedRegion)
			}
		})
	}
}

func TestCloudProviderCacheKeyPrefix(t *testing.T) {
	tests := []struct {
		provider       domain.CloudProvider
		expectedPrefix string
	}{
		{domain.AWS, "aws:"},
		{domain.Azure, "azure:"},
		{domain.GCP, "gcp:"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			prefix := tt.provider.CacheKeyPrefix()
			if prefix != tt.expectedPrefix {
				t.Errorf("CacheKeyPrefix() = %s, want %s", prefix, tt.expectedPrefix)
			}
		})
	}
}

func TestFactorySupportedProviders(t *testing.T) {
	factory := provider.GetFactory()

	providers := factory.GetSupportedProviders()
	if len(providers) < 2 {
		t.Errorf("Expected at least 2 supported providers, got %d", len(providers))
	}

	// Check AWS is supported
	if !factory.IsProviderSupported(domain.AWS) {
		t.Error("AWS should be supported")
	}

	// Check Azure is supported
	if !factory.IsProviderSupported(domain.Azure) {
		t.Error("Azure should be supported")
	}

	t.Logf("Supported providers: %v", providers)
}
