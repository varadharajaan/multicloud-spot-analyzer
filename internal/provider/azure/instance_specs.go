// Package azure implements Azure instance specifications catalog.
// This catalog contains metadata about Azure VM sizes including
// vCPU, memory, GPU info, and series information.
package azure

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
)

// InstanceSpecsProvider provides instance specifications for Azure VMs
type InstanceSpecsProvider struct {
	mu    sync.RWMutex
	specs map[string]domain.InstanceSpecs
}

// NewInstanceSpecsProvider creates a new Azure instance specs provider
func NewInstanceSpecsProvider() *InstanceSpecsProvider {
	p := &InstanceSpecsProvider{
		specs: make(map[string]domain.InstanceSpecs),
	}
	p.initializeSpecs()
	return p
}

// GetProviderName returns the cloud provider identifier
func (p *InstanceSpecsProvider) GetProviderName() domain.CloudProvider {
	return domain.Azure
}

// GetInstanceSpecs returns specifications for a specific VM size
func (p *InstanceSpecsProvider) GetInstanceSpecs(ctx context.Context, vmSize string) (*domain.InstanceSpecs, error) {
	// Normalize VM size name
	normalizedSize := p.normalizeVMSize(vmSize)

	p.mu.RLock()
	if specs, exists := p.specs[normalizedSize]; exists {
		p.mu.RUnlock()
		return &specs, nil
	}
	p.mu.RUnlock()

	// Try to derive specs from VM size name
	derived := p.deriveSpecsFromName(vmSize)
	if derived != nil {
		return derived, nil
	}

	return nil, domain.NewInstanceSpecsError(vmSize, domain.ErrNotFound)
}

// GetAllInstanceSpecs returns specifications for all known VM sizes
func (p *InstanceSpecsProvider) GetAllInstanceSpecs(ctx context.Context) ([]domain.InstanceSpecs, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]domain.InstanceSpecs, 0, len(p.specs))
	for _, spec := range p.specs {
		result = append(result, spec)
	}
	return result, nil
}

// GetInstancesByVCPU returns instances matching the vCPU requirement
func (p *InstanceSpecsProvider) GetInstancesByVCPU(ctx context.Context, minVCPU, maxVCPU int) ([]domain.InstanceSpecs, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]domain.InstanceSpecs, 0)
	for _, spec := range p.specs {
		if spec.VCPU >= minVCPU && (maxVCPU == 0 || spec.VCPU <= maxVCPU) {
			result = append(result, spec)
		}
	}
	return result, nil
}

// normalizeVMSize normalizes Azure VM size names
func (p *InstanceSpecsProvider) normalizeVMSize(vmSize string) string {
	// Remove "Standard_" prefix if present
	normalized := strings.TrimPrefix(vmSize, "Standard_")

	// Convert to consistent format
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, " ", "_")

	return normalized
}

// deriveSpecsFromName attempts to derive instance specifications from the VM size name
func (p *InstanceSpecsProvider) deriveSpecsFromName(vmSize string) *domain.InstanceSpecs {
	// Remove Standard_ prefix
	cleanSize := strings.TrimPrefix(vmSize, "Standard_")
	cleanSize = strings.TrimPrefix(cleanSize, "standard_")

	// Parse VM size pattern: [Family][vCPU count][version]_[memory suffix]
	// Examples: D2s_v5, E4-2ds_v5, NC24ads_A100_v4

	series, vcpu, memGB := p.parseVMSize(cleanSize)
	if vcpu == 0 {
		return nil
	}

	category := p.parseSeries(series)
	hasGPU := p.isGPUSeries(series)
	generation := p.parseGeneration(cleanSize)
	architecture := p.parseArchitecture(cleanSize)

	return &domain.InstanceSpecs{
		InstanceType:  vmSize,
		VCPU:          vcpu,
		MemoryGB:      memGB,
		HasGPU:        hasGPU,
		GPUCount:      p.getGPUCount(series, vcpu),
		GPUType:       p.getGPUType(series),
		Architecture:  architecture,
		Category:      category,
		Generation:    generation,
		IsDeprecated:  p.isDeprecated(series),
		IsBurstable:   p.isBurstable(series),
		IsBareMetal:   strings.Contains(strings.ToLower(vmSize), "metal"),
		CloudProvider: domain.Azure,
	}
}

// parseVMSize extracts series, vCPU, and memory from VM size string
func (p *InstanceSpecsProvider) parseVMSize(size string) (series string, vcpu int, memGB float64) {
	// Common patterns:
	// D2s_v5 -> D series, 2 vCPU
	// E4-2ds_v5 -> E series, 4 vCPU (constrained to 2)
	// NC24ads_A100_v4 -> NC series, 24 vCPU

	// First, extract the version suffix
	size = strings.Split(size, "_v")[0]
	size = strings.Split(size, " v")[0]

	// Extract series (letter(s) at the start)
	re := regexp.MustCompile(`^([A-Za-z]+)(\d+)`)
	matches := re.FindStringSubmatch(size)
	if len(matches) < 3 {
		return "", 0, 0
	}

	series = strings.ToUpper(matches[1])
	vcpu, _ = strconv.Atoi(matches[2])

	// Estimate memory based on series and vCPU
	memGB = p.estimateMemory(series, vcpu)

	return series, vcpu, memGB
}

// estimateMemory estimates memory based on series type and vCPU count
func (p *InstanceSpecsProvider) estimateMemory(series string, vcpu int) float64 {
	// Memory ratios by series (memory per vCPU in GB)
	ratios := map[string]float64{
		"A":   1.75,
		"B":   1.0,  // Burstable - lower memory
		"D":   4.0,  // General purpose
		"DS":  4.0,
		"E":   8.0,  // Memory optimized
		"ES":  8.0,
		"F":   2.0,  // Compute optimized
		"FS":  2.0,
		"G":   6.5,  // Memory and storage
		"GS":  6.5,
		"H":   7.0,  // High performance
		"HB":  4.0,
		"HC":  4.0,
		"L":   8.0,  // Storage optimized
		"LS":  8.0,
		"M":   14.0, // Very high memory
		"MS":  14.0,
		"NC":  6.0,  // GPU compute
		"NCS": 6.0,
		"ND":  6.0,  // GPU deep learning
		"NV":  7.0,  // GPU visualization
		"NVS": 7.0,
		"DC":  4.0,  // Confidential
		"DCS": 4.0,
		"EC":  8.0,  // Memory-optimized confidential
		"ECS": 8.0,
	}

	ratio := 4.0 // Default
	for prefix, r := range ratios {
		if strings.HasPrefix(strings.ToUpper(series), prefix) {
			ratio = r
			break
		}
	}

	return float64(vcpu) * ratio
}

// parseSeries determines the instance category from the series
func (p *InstanceSpecsProvider) parseSeries(series string) domain.InstanceCategory {
	seriesUpper := strings.ToUpper(series)

	switch {
	case strings.HasPrefix(seriesUpper, "F"):
		return domain.ComputeOptimized
	case strings.HasPrefix(seriesUpper, "E"), strings.HasPrefix(seriesUpper, "M"):
		return domain.MemoryOptimized
	case strings.HasPrefix(seriesUpper, "L"):
		return domain.StorageOptimized
	case strings.HasPrefix(seriesUpper, "NC"), strings.HasPrefix(seriesUpper, "ND"),
		strings.HasPrefix(seriesUpper, "NV"), strings.HasPrefix(seriesUpper, "NG"):
		return domain.AcceleratedComputing
	case strings.HasPrefix(seriesUpper, "H"):
		return domain.HighPerformance
	default:
		return domain.GeneralPurpose
	}
}

// isGPUSeries checks if the series includes GPU
func (p *InstanceSpecsProvider) isGPUSeries(series string) bool {
	gpuSeries := []string{"NC", "ND", "NV", "NG", "NP"}
	seriesUpper := strings.ToUpper(series)

	for _, gpu := range gpuSeries {
		if strings.HasPrefix(seriesUpper, gpu) {
			return true
		}
	}
	return false
}

// getGPUCount returns the number of GPUs for GPU instances
func (p *InstanceSpecsProvider) getGPUCount(series string, vcpu int) int {
	if !p.isGPUSeries(series) {
		return 0
	}

	// GPU count typically scales with vCPU
	switch {
	case vcpu >= 64:
		return 4
	case vcpu >= 48:
		return 4
	case vcpu >= 24:
		return 2
	case vcpu >= 12:
		return 1
	default:
		return 1
	}
}

// getGPUType returns the GPU type for GPU instances
func (p *InstanceSpecsProvider) getGPUType(series string) string {
	seriesUpper := strings.ToUpper(series)

	gpuTypes := map[string]string{
		"NCA100": "NVIDIA A100",
		"NCA10":  "NVIDIA A10",
		"NCV100": "NVIDIA V100",
		"NCT4":   "NVIDIA T4",
		"NDV4":   "NVIDIA A100",
		"NDM":    "NVIDIA A100 80GB",
		"NV":     "AMD Radeon Pro",
		"NG":     "NVIDIA GRID",
	}

	for prefix, gpuType := range gpuTypes {
		if strings.HasPrefix(seriesUpper, prefix) {
			return gpuType
		}
	}

	if strings.HasPrefix(seriesUpper, "NC") {
		return "NVIDIA"
	}
	if strings.HasPrefix(seriesUpper, "ND") {
		return "NVIDIA"
	}

	return ""
}

// parseGeneration determines the generation from the VM size
func (p *InstanceSpecsProvider) parseGeneration(size string) domain.InstanceGeneration {
	// Extract version number (e.g., v5, v4, v3)
	re := regexp.MustCompile(`_v(\d+)$|v(\d+)$| v(\d+)$`)
	matches := re.FindStringSubmatch(strings.ToLower(size))

	version := 0
	for _, m := range matches[1:] {
		if m != "" {
			version, _ = strconv.Atoi(m)
			break
		}
	}

	switch {
	case version >= 5:
		return domain.Current
	case version >= 4:
		return domain.Previous
	case version >= 3:
		return domain.Legacy
	case version >= 1:
		return domain.Deprecated
	default:
		// Check for newer naming patterns
		if strings.Contains(size, "ads") || strings.Contains(size, "pds") {
			return domain.Current
		}
		return domain.Previous
	}
}

// parseArchitecture determines the CPU architecture
func (p *InstanceSpecsProvider) parseArchitecture(size string) string {
	sizeLower := strings.ToLower(size)

	// Azure ARM-based instances typically have "p" in the suffix
	// e.g., Dpsv5, Epsv5 (Ampere Altra)
	if strings.Contains(sizeLower, "ps") || strings.Contains(sizeLower, "pls") ||
		strings.Contains(sizeLower, "pbs") || strings.Contains(sizeLower, "pds") {
		return "arm64"
	}

	return "x86_64"
}

// isBurstable checks if the series is burstable
func (p *InstanceSpecsProvider) isBurstable(series string) bool {
	return strings.ToUpper(series) == "B" || strings.HasPrefix(strings.ToUpper(series), "B")
}

// isDeprecated checks if the series is deprecated
func (p *InstanceSpecsProvider) isDeprecated(series string) bool {
	deprecatedSeries := []string{
		"A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9", "A10", "A11",
		"D1", "DS1", "D11", "DS11",
		"G1", "GS1",
	}
	seriesUpper := strings.ToUpper(series)

	for _, deprecated := range deprecatedSeries {
		if seriesUpper == deprecated {
			return true
		}
	}
	return false
}

// initializeSpecs pre-populates the specs catalog with common Azure VM sizes
func (p *InstanceSpecsProvider) initializeSpecs() {
	// General purpose - D series v5
	p.addVMFamily("D", "x86_64", domain.GeneralPurpose, false, false, domain.Current, "_v5")
	p.addVMFamily("Ds", "x86_64", domain.GeneralPurpose, false, false, domain.Current, "_v5")
	p.addVMFamily("Das", "x86_64", domain.GeneralPurpose, false, false, domain.Current, "_v5")
	p.addVMFamily("Dads", "x86_64", domain.GeneralPurpose, false, false, domain.Current, "_v5")
	p.addVMFamily("Dps", "arm64", domain.GeneralPurpose, false, false, domain.Current, "_v5")
	p.addVMFamily("Dpds", "arm64", domain.GeneralPurpose, false, false, domain.Current, "_v5")

	// General purpose - D series v4
	p.addVMFamily("D", "x86_64", domain.GeneralPurpose, false, false, domain.Previous, "_v4")
	p.addVMFamily("Ds", "x86_64", domain.GeneralPurpose, false, false, domain.Previous, "_v4")
	p.addVMFamily("Das", "x86_64", domain.GeneralPurpose, false, false, domain.Previous, "_v4")
	p.addVMFamily("Dads", "x86_64", domain.GeneralPurpose, false, false, domain.Previous, "_v4")

	// Compute optimized - F series
	p.addVMFamily("F", "x86_64", domain.ComputeOptimized, false, false, domain.Current, "_v2")
	p.addVMFamily("Fs", "x86_64", domain.ComputeOptimized, false, false, domain.Current, "_v2")
	p.addVMFamily("Fas", "x86_64", domain.ComputeOptimized, false, false, domain.Current, "_v2")
	p.addVMFamily("Fx", "x86_64", domain.ComputeOptimized, false, false, domain.Current, "")

	// Memory optimized - E series v5
	p.addVMFamily("E", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v5")
	p.addVMFamily("Es", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v5")
	p.addVMFamily("Eas", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v5")
	p.addVMFamily("Eads", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v5")
	p.addVMFamily("Eps", "arm64", domain.MemoryOptimized, false, false, domain.Current, "_v5")
	p.addVMFamily("Epds", "arm64", domain.MemoryOptimized, false, false, domain.Current, "_v5")

	// Memory optimized - E series v4
	p.addVMFamily("E", "x86_64", domain.MemoryOptimized, false, false, domain.Previous, "_v4")
	p.addVMFamily("Es", "x86_64", domain.MemoryOptimized, false, false, domain.Previous, "_v4")

	// Memory optimized - M series
	p.addVMFamily("M", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "")
	p.addVMFamily("Ms", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v2")
	p.addVMFamily("Mds", "x86_64", domain.MemoryOptimized, false, false, domain.Current, "_v2")

	// Storage optimized - L series v3
	p.addVMFamily("L", "x86_64", domain.StorageOptimized, false, false, domain.Current, "_v3")
	p.addVMFamily("Ls", "x86_64", domain.StorageOptimized, false, false, domain.Current, "_v3")
	p.addVMFamily("Las", "x86_64", domain.StorageOptimized, false, false, domain.Current, "_v3")
	p.addVMFamily("Lads", "x86_64", domain.StorageOptimized, false, false, domain.Current, "_v3")

	// Storage optimized - L series v2
	p.addVMFamily("L", "x86_64", domain.StorageOptimized, false, false, domain.Previous, "_v2")
	p.addVMFamily("Ls", "x86_64", domain.StorageOptimized, false, false, domain.Previous, "_v2")

	// Burstable - B series v2
	p.addBurstableFamily("B", "x86_64", domain.Current, "_v2")
	p.addBurstableFamily("Bs", "x86_64", domain.Current, "_v2")
	p.addBurstableFamily("Bps", "arm64", domain.Current, "_v2")

	// GPU instances - NC series
	p.addGPUFamily("NC", "x86_64", "NVIDIA T4", domain.Current, "_v3")
	p.addGPUFamily("NCA100", "x86_64", "NVIDIA A100", domain.Current, "_v4")
	p.addGPUFamily("NCads_A10", "x86_64", "NVIDIA A10", domain.Current, "_v4")

	// GPU instances - ND series
	p.addGPUFamily("ND", "x86_64", "NVIDIA A100", domain.Current, "_v4")
	p.addGPUFamily("NDm_A100", "x86_64", "NVIDIA A100 80GB", domain.Current, "_v4")

	// GPU instances - NV series
	p.addGPUFamily("NV", "x86_64", "AMD Radeon", domain.Current, "_v4")
	p.addGPUFamily("NVads_A10", "x86_64", "NVIDIA A10", domain.Current, "_v5")

	// HPC instances
	p.addVMFamily("HB", "x86_64", domain.HighPerformance, false, false, domain.Current, "_v4")
	p.addVMFamily("HC", "x86_64", domain.HighPerformance, false, false, domain.Current, "")
	p.addVMFamily("HX", "x86_64", domain.HighPerformance, false, false, domain.Current, "")
}

// addVMFamily adds all standard sizes for a VM family
func (p *InstanceSpecsProvider) addVMFamily(family, arch string, category domain.InstanceCategory, hasGPU, isDeprecated bool, generation domain.InstanceGeneration, versionSuffix string) {
	sizes := []struct {
		vcpu int
		mem  float64
	}{
		{2, 8},
		{4, 16},
		{8, 32},
		{16, 64},
		{32, 128},
		{48, 192},
		{64, 256},
		{96, 384},
	}

	// Adjust memory based on category
	memMultiplier := 1.0
	switch category {
	case domain.MemoryOptimized:
		memMultiplier = 2.0
	case domain.ComputeOptimized:
		memMultiplier = 0.5
	}

	for _, size := range sizes {
		vmSize := fmt.Sprintf("Standard_%s%d%s", family, size.vcpu, versionSuffix)
		p.specs[p.normalizeVMSize(vmSize)] = domain.InstanceSpecs{
			InstanceType:  vmSize,
			VCPU:          size.vcpu,
			MemoryGB:      size.mem * memMultiplier,
			HasGPU:        hasGPU,
			Architecture:  arch,
			Category:      category,
			Generation:    generation,
			IsDeprecated:  isDeprecated,
			IsBurstable:   false,
			IsBareMetal:   false,
			CloudProvider: domain.Azure,
		}
	}
}

// addBurstableFamily adds burstable VM sizes
func (p *InstanceSpecsProvider) addBurstableFamily(family, arch string, generation domain.InstanceGeneration, versionSuffix string) {
	sizes := []struct {
		vcpu int
		mem  float64
	}{
		{1, 0.5},
		{1, 1},
		{1, 2},
		{2, 4},
		{2, 8},
		{4, 16},
		{8, 32},
		{16, 64},
	}

	for _, size := range sizes {
		vmSize := fmt.Sprintf("Standard_%s%d%s", family, size.vcpu, versionSuffix)
		p.specs[p.normalizeVMSize(vmSize)] = domain.InstanceSpecs{
			InstanceType:  vmSize,
			VCPU:          size.vcpu,
			MemoryGB:      size.mem,
			HasGPU:        false,
			Architecture:  arch,
			Category:      domain.GeneralPurpose,
			Generation:    generation,
			IsDeprecated:  false,
			IsBurstable:   true,
			IsBareMetal:   false,
			CloudProvider: domain.Azure,
		}
	}
}

// addGPUFamily adds GPU VM sizes
func (p *InstanceSpecsProvider) addGPUFamily(family, arch, gpuType string, generation domain.InstanceGeneration, versionSuffix string) {
	sizes := []struct {
		vcpu     int
		mem      float64
		gpuCount int
	}{
		{6, 56, 1},
		{12, 112, 2},
		{24, 224, 4},
		{48, 448, 4},
		{96, 896, 8},
	}

	for _, size := range sizes {
		vmSize := fmt.Sprintf("Standard_%s%d%s", family, size.vcpu, versionSuffix)
		p.specs[p.normalizeVMSize(vmSize)] = domain.InstanceSpecs{
			InstanceType:  vmSize,
			VCPU:          size.vcpu,
			MemoryGB:      size.mem,
			HasGPU:        true,
			GPUCount:      size.gpuCount,
			GPUType:       gpuType,
			Architecture:  arch,
			Category:      domain.AcceleratedComputing,
			Generation:    generation,
			IsDeprecated:  false,
			IsBurstable:   false,
			IsBareMetal:   false,
			CloudProvider: domain.Azure,
		}
	}
}

// init registers the Azure instance specs provider with the factory
func init() {
	provider.RegisterSpecsProviderCreator(domain.Azure, func() (domain.InstanceSpecsProvider, error) {
		return NewInstanceSpecsProvider(), nil
	})
}
