package analyzer

import (
	"testing"

	"github.com/spot-analyzer/internal/domain"
)

func TestSmartFilterIsEligible(t *testing.T) {
	filter := NewSmartFilter()

	baseSpec := domain.InstanceSpecs{
		InstanceType: "m5.large",
		VCPU:         2,
		MemoryGB:     8.0,
		HasGPU:       false,
		Architecture: "x86_64",
		Category:     domain.GeneralPurpose,
		Generation:   domain.Current,
		IsBurstable:  false,
		IsDeprecated: false,
	}

	spotData := &domain.SpotData{
		InstanceType:          "m5.large",
		SavingsPercent:        60,
		InterruptionFrequency: domain.VeryLow,
	}

	tests := []struct {
		name         string
		spec         domain.InstanceSpecs
		requirements domain.UsageRequirements
		wantEligible bool
	}{
		{
			name: "Basic eligible instance",
			spec: baseSpec,
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				MaxInterruption: domain.Medium,
				AllowBurstable:  true,
			},
			wantEligible: true,
		},
		{
			name: "Deprecated instance should be filtered",
			spec: func() domain.InstanceSpecs {
				s := baseSpec
				s.IsDeprecated = true
				return s
			}(),
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "Insufficient vCPU",
			spec: baseSpec, // 2 vCPU
			requirements: domain.UsageRequirements{
				MinVCPU:         4, // Requires 4
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "Exceeds max vCPU",
			spec: func() domain.InstanceSpecs {
				s := baseSpec
				s.VCPU = 16
				return s
			}(),
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				MaxVCPU:         8,
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "Insufficient memory",
			spec: baseSpec, // 8 GB
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				MinMemoryGB:     16, // Requires 16 GB
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "GPU not needed but present",
			spec: func() domain.InstanceSpecs {
				s := baseSpec
				s.HasGPU = true
				return s
			}(),
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				RequiresGPU:     false,
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "Burstable not allowed",
			spec: func() domain.InstanceSpecs {
				s := baseSpec
				s.IsBurstable = true
				return s
			}(),
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				AllowBurstable:  false,
				MaxInterruption: domain.Medium,
			},
			wantEligible: false,
		},
		{
			name: "Burstable allowed",
			spec: func() domain.InstanceSpecs {
				s := baseSpec
				s.IsBurstable = true
				return s
			}(),
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				AllowBurstable:  true,
				MaxInterruption: domain.Medium,
			},
			wantEligible: true,
		},
		{
			name: "Architecture mismatch",
			spec: baseSpec, // x86_64
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				Architecture:    "arm64",
				MaxInterruption: domain.Medium,
				AllowBurstable:  true,
			},
			wantEligible: false,
		},
		{
			name: "Family filter - not in list",
			spec: baseSpec, // m5 family
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				Families:        []string{"c", "r"}, // Only c and r families
				MaxInterruption: domain.Medium,
				AllowBurstable:  true,
			},
			wantEligible: false,
		},
		{
			name: "Family filter - in list",
			spec: baseSpec, // m5 family
			requirements: domain.UsageRequirements{
				MinVCPU:         2,
				Families:        []string{"m", "c"}, // m family included
				MaxInterruption: domain.Medium,
				AllowBurstable:  true,
			},
			wantEligible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligible, reasons := filter.IsEligible(tt.spec, spotData, tt.requirements)
			if eligible != tt.wantEligible {
				t.Errorf("IsEligible() = %v, want %v. Reasons: %v", eligible, tt.wantEligible, reasons)
			}
		})
	}
}

func TestSmartFilterFilter(t *testing.T) {
	filter := NewSmartFilter()

	instances := []domain.InstanceSpecs{
		{InstanceType: "m5.large", VCPU: 2, MemoryGB: 8, Architecture: "x86_64"},
		{InstanceType: "m5.xlarge", VCPU: 4, MemoryGB: 16, Architecture: "x86_64"},
		{InstanceType: "c5.large", VCPU: 2, MemoryGB: 4, Architecture: "x86_64"},
		{InstanceType: "t3.large", VCPU: 2, MemoryGB: 8, Architecture: "x86_64", IsBurstable: true},
	}

	spots := map[string]domain.SpotData{
		"m5.large":  {InstanceType: "m5.large", SavingsPercent: 60, InterruptionFrequency: domain.VeryLow},
		"m5.xlarge": {InstanceType: "m5.xlarge", SavingsPercent: 55, InterruptionFrequency: domain.Low},
		"c5.large":  {InstanceType: "c5.large", SavingsPercent: 65, InterruptionFrequency: domain.VeryLow},
		"t3.large":  {InstanceType: "t3.large", SavingsPercent: 70, InterruptionFrequency: domain.VeryLow},
	}

	requirements := domain.UsageRequirements{
		MinVCPU:         2,
		MaxVCPU:         4,
		MinMemoryGB:     4,
		AllowBurstable:  false,
		MaxInterruption: domain.Medium,
	}

	filtered := filter.Filter(instances, spots, requirements)

	// Should include m5.large, m5.xlarge, c5.large but not t3.large (burstable)
	if len(filtered) != 3 {
		t.Errorf("Filter() returned %d instances, want 3", len(filtered))
	}

	for _, inst := range filtered {
		if inst.IsBurstable {
			t.Errorf("Filter() included burstable instance %s", inst.InstanceType)
		}
	}
}

func TestExtractInstanceFamily(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := filterExtractFamily(tt.instanceType)
			if got != tt.wantFamily {
				t.Errorf("filterExtractFamily(%s) = %s, want %s", tt.instanceType, got, tt.wantFamily)
			}
		})
	}
}

func TestContainsFamily(t *testing.T) {
	families := []string{"m", "c", "r"}

	tests := []struct {
		family   string
		expected bool
	}{
		{"m", true},
		{"c", true},
		{"r", true},
		{"t", false},
		{"p", false},
		{"M", true}, // Case insensitive
		{"C", true},
	}

	for _, tt := range tests {
		t.Run(tt.family, func(t *testing.T) {
			got := filterContainsFamily(families, tt.family)
			if got != tt.expected {
				t.Errorf("filterContainsFamily(%v, %s) = %v, want %v", families, tt.family, got, tt.expected)
			}
		})
	}
}
