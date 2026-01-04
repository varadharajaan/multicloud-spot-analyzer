// Package analyzer implements filtering logic for spot instances.
package analyzer

import (
	"strings"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
)

// SmartFilter implements domain.InstanceFilter with intelligent filtering
type SmartFilter struct {
	cloudProvider domain.CloudProvider
}

// NewSmartFilter creates a new smart filter
func NewSmartFilter() *SmartFilter {
	return &SmartFilter{
		cloudProvider: domain.AWS, // Default to AWS
	}
}

// NewSmartFilterForProvider creates a filter for a specific cloud provider
func NewSmartFilterForProvider(cloudProvider domain.CloudProvider) *SmartFilter {
	return &SmartFilter{
		cloudProvider: cloudProvider,
	}
}

// Filter removes instances that don't match requirements
func (f *SmartFilter) Filter(
	instances []domain.InstanceSpecs,
	spots map[string]domain.SpotData,
	requirements domain.UsageRequirements,
) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)

	for _, spec := range instances {
		spot, hasSpot := spots[spec.InstanceType]
		if !hasSpot {
			continue // No spot data available
		}

		eligible, _ := f.IsEligible(spec, &spot, requirements)
		if eligible {
			result = append(result, spec)
		}
	}

	return result
}

// IsEligible checks if a single instance matches requirements
func (f *SmartFilter) IsEligible(
	spec domain.InstanceSpecs,
	spot *domain.SpotData,
	requirements domain.UsageRequirements,
) (bool, []string) {
	reasons := make([]string, 0)

	// 1. Check deprecated status - CRITICAL filter
	if spec.IsDeprecated {
		reasons = append(reasons, "instance type is deprecated")
		return false, reasons
	}

	// 2. Check vCPU requirements
	if spec.VCPU < requirements.MinVCPU {
		reasons = append(reasons, "insufficient vCPU")
		return false, reasons
	}
	if requirements.MaxVCPU > 0 && spec.VCPU > requirements.MaxVCPU {
		reasons = append(reasons, "exceeds maximum vCPU")
		return false, reasons
	}

	// 3. Check memory requirements
	if requirements.MinMemoryGB > 0 && spec.MemoryGB < requirements.MinMemoryGB {
		reasons = append(reasons, "insufficient memory")
		return false, reasons
	}
	if requirements.MaxMemoryGB > 0 && spec.MemoryGB > requirements.MaxMemoryGB {
		reasons = append(reasons, "exceeds maximum memory")
		return false, reasons
	}

	// 4. GPU filtering - CRITICAL
	// If user doesn't require GPU, exclude GPU instances (they're expensive and overkill)
	if !requirements.RequiresGPU && spec.HasGPU {
		reasons = append(reasons, "GPU instance not needed for non-GPU workload")
		return false, reasons
	}

	// If user requires GPU, exclude non-GPU instances
	if requirements.RequiresGPU && !spec.HasGPU {
		reasons = append(reasons, "GPU required but instance has no GPU")
		return false, reasons
	}

	// Check minimum GPU count
	if requirements.RequiresGPU && requirements.MinGPUCount > 0 {
		if spec.GPUCount < requirements.MinGPUCount {
			reasons = append(reasons, "insufficient GPU count")
			return false, reasons
		}
	}

	// Check GPU type preference
	if requirements.GPUType != "" && spec.HasGPU {
		if !strings.Contains(strings.ToLower(spec.GPUType), strings.ToLower(requirements.GPUType)) {
			reasons = append(reasons, "GPU type mismatch")
			return false, reasons
		}
	}

	// 5. Burstable instance filtering
	if spec.IsBurstable && !requirements.AllowBurstable {
		reasons = append(reasons, "burstable instances not allowed")
		return false, reasons
	}

	// 6. Bare metal filtering
	if spec.IsBareMetal && !requirements.AllowBareMetal {
		reasons = append(reasons, "bare metal instances not allowed")
		return false, reasons
	}

	// 7. Instance family filtering (uses provider-specific extractor)
	if len(requirements.Families) > 0 {
		family := provider.ExtractFamilyForProvider(spec.InstanceType, f.cloudProvider)
		if !filterContainsFamily(requirements.Families, family) {
			reasons = append(reasons, "instance family not in allowed list")
			return false, reasons
		}
	}

	// 8. Architecture filtering
	if requirements.Architecture != "" && spec.Architecture != requirements.Architecture {
		reasons = append(reasons, "architecture mismatch")
		return false, reasons
	}

	// 9. Storage requirements
	if requirements.MinStorageGB > 0 && spec.StorageGB < requirements.MinStorageGB {
		reasons = append(reasons, "insufficient storage")
		return false, reasons
	}

	// 10. Interruption frequency filtering
	if spot != nil && spot.InterruptionFrequency > requirements.MaxInterruption {
		reasons = append(reasons, "interruption frequency too high")
		return false, reasons
	}

	// 10. Minimum savings filtering
	if spot != nil && requirements.MinSavingsPercent > 0 {
		if spot.SavingsPercent < requirements.MinSavingsPercent {
			reasons = append(reasons, "savings below minimum threshold")
			return false, reasons
		}
	}

	// 11. Zero savings check (likely unavailable)
	if spot != nil && spot.SavingsPercent == 0 {
		reasons = append(reasons, "no savings data available (likely unavailable)")
		return false, reasons
	}

	// 12. Instance category preference (soft filter - just for warnings)
	// We don't filter on category, but it affects scoring

	return true, reasons
}

// FilterByCategory returns instances matching a specific category
func (f *SmartFilter) FilterByCategory(
	instances []domain.InstanceSpecs,
	category domain.InstanceCategory,
) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range instances {
		if spec.Category == category {
			result = append(result, spec)
		}
	}
	return result
}

// FilterByGeneration returns instances of specified generation or newer
func (f *SmartFilter) FilterByGeneration(
	instances []domain.InstanceSpecs,
	minGeneration domain.InstanceGeneration,
) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range instances {
		if spec.Generation <= minGeneration { // Lower is newer
			result = append(result, spec)
		}
	}
	return result
}

// FilterNonDeprecated returns only non-deprecated instances
func (f *SmartFilter) FilterNonDeprecated(instances []domain.InstanceSpecs) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range instances {
		if !spec.IsDeprecated {
			result = append(result, spec)
		}
	}
	return result
}

// FilterNonGPU returns only non-GPU instances
func (f *SmartFilter) FilterNonGPU(instances []domain.InstanceSpecs) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range instances {
		if !spec.HasGPU {
			result = append(result, spec)
		}
	}
	return result
}

// FilterGPU returns only GPU instances
func (f *SmartFilter) FilterGPU(instances []domain.InstanceSpecs) []domain.InstanceSpecs {
	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range instances {
		if spec.HasGPU {
			result = append(result, spec)
		}
	}
	return result
}

// filterContainsFamily checks if a family is in the list of allowed families (case-insensitive)
func filterContainsFamily(families []string, family string) bool {
	for _, f := range families {
		if strings.EqualFold(f, family) {
			return true
		}
	}
	return false
}

// filterExtractFamily extracts the family prefix from an instance type
// AWS: "m5.large" -> "m", "c6i.xlarge" -> "c"
// Azure: "Standard_D4s_v5" -> "D", "Standard_B2s" -> "B"
func filterExtractFamily(instanceType string) string {
	// Handle Azure format: Standard_D4s_v5 -> D
	if strings.HasPrefix(instanceType, "Standard_") {
		// Remove "Standard_" prefix
		remaining := instanceType[9:]
		// Extract letters before the first digit
		for i, c := range remaining {
			if c >= '0' && c <= '9' {
				return strings.ToUpper(remaining[:i])
			}
		}
		return strings.ToUpper(remaining)
	}

	// Handle AWS format: m5.large -> m
	for i, c := range instanceType {
		if c >= '0' && c <= '9' {
			return instanceType[:i]
		}
	}
	return instanceType
}
