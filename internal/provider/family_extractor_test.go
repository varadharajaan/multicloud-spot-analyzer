package provider

import (
	"testing"

	"github.com/spot-analyzer/internal/domain"
)

func TestAWSFamilyExtractor(t *testing.T) {
	extractor := NewAWSFamilyExtractor()

	tests := []struct {
		instanceType string
		wantFamily   string
	}{
		{"m5.large", "m"},
		{"c6i.xlarge", "c"},
		{"r5a.2xlarge", "r"},
		{"t3.nano", "t"},
		{"p4d.24xlarge", "p"},
		{"im4gn.large", "im"},
		{"x2idn.xlarge", "x"},
		{"g4dn.xlarge", "g"},
		{"hpc6a.48xlarge", "hpc"},
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := extractor.ExtractFamily(tt.instanceType)
			if got != tt.wantFamily {
				t.Errorf("ExtractFamily(%s) = %s, want %s", tt.instanceType, got, tt.wantFamily)
			}
		})
	}

	if extractor.GetProviderName() != domain.AWS {
		t.Errorf("GetProviderName() = %s, want AWS", extractor.GetProviderName())
	}
}

func TestAzureFamilyExtractor(t *testing.T) {
	extractor := NewAzureFamilyExtractor()

	tests := []struct {
		instanceType string
		wantFamily   string
	}{
		{"Standard_D4s_v5", "D"},
		{"Standard_B2s", "B"},
		{"Standard_E8s_v5", "E"},
		{"Standard_F4s_v2", "F"},
		{"Standard_NC24ads_A100_v4", "NC"},
		{"Standard_M128s", "M"},
		{"Standard_DC2as_v5", "DC"},
		{"Standard_HB120rs_v3", "HB"},
		{"D4s_v5", "D"}, // Without Standard_ prefix
		{"B2s", "B"},    // Without Standard_ prefix
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := extractor.ExtractFamily(tt.instanceType)
			if got != tt.wantFamily {
				t.Errorf("ExtractFamily(%s) = %s, want %s", tt.instanceType, got, tt.wantFamily)
			}
		})
	}

	if extractor.GetProviderName() != domain.Azure {
		t.Errorf("GetProviderName() = %s, want Azure", extractor.GetProviderName())
	}
}

func TestGCPFamilyExtractor(t *testing.T) {
	extractor := NewGCPFamilyExtractor()

	tests := []struct {
		instanceType string
		wantFamily   string
	}{
		{"n2-standard-4", "n2"},
		{"e2-medium", "e2"},
		{"c2-standard-8", "c2"},
		{"n2d-standard-2", "n2d"},
		{"m2-megamem-416", "m2"},
		{"a2-highgpu-1g", "a2"},
		{"t2d-standard-1", "t2d"},
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := extractor.ExtractFamily(tt.instanceType)
			if got != tt.wantFamily {
				t.Errorf("ExtractFamily(%s) = %s, want %s", tt.instanceType, got, tt.wantFamily)
			}
		})
	}

	if extractor.GetProviderName() != domain.GCP {
		t.Errorf("GetProviderName() = %s, want GCP", extractor.GetProviderName())
	}
}

func TestGetFamilyExtractor(t *testing.T) {
	tests := []struct {
		provider    domain.CloudProvider
		instanceType string
		wantFamily  string
	}{
		{domain.AWS, "m5.large", "m"},
		{domain.Azure, "Standard_D4s_v5", "D"},
		{domain.GCP, "n2-standard-4", "n2"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			got := ExtractFamilyForProvider(tt.instanceType, tt.provider)
			if got != tt.wantFamily {
				t.Errorf("ExtractFamilyForProvider(%s, %s) = %s, want %s", 
					tt.instanceType, tt.provider, got, tt.wantFamily)
			}
		})
	}
}

func TestAzureNormalizeName(t *testing.T) {
	extractor := NewAzureFamilyExtractor()

	tests := []struct {
		input string
		want  string
	}{
		{"D4s_v5", "Standard_D4s_v5"},
		{"Standard_D4s_v5", "Standard_D4s_v5"},
		{" B2s ", "Standard_B2s"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractor.NormalizeName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeName(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}
