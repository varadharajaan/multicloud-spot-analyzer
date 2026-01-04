// Package aws implements AWS instance specifications catalog.
// This catalog contains metadata about EC2 instance types including
// vCPU, memory, GPU info, and deprecation status.
package aws

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
)

// InstanceSpecsProvider provides instance specifications for AWS EC2 instances
type InstanceSpecsProvider struct {
	mu    sync.RWMutex
	specs map[string]domain.InstanceSpecs
}

// NewInstanceSpecsProvider creates a new AWS instance specs provider
func NewInstanceSpecsProvider() *InstanceSpecsProvider {
	p := &InstanceSpecsProvider{
		specs: make(map[string]domain.InstanceSpecs),
	}
	p.initializeSpecs()
	return p
}

// GetProviderName returns the cloud provider identifier
func (p *InstanceSpecsProvider) GetProviderName() domain.CloudProvider {
	return domain.AWS
}

// GetInstanceSpecs returns specifications for a specific instance type
func (p *InstanceSpecsProvider) GetInstanceSpecs(ctx context.Context, instanceType string) (*domain.InstanceSpecs, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if specs, exists := p.specs[instanceType]; exists {
		return &specs, nil
	}

	// No estimation or derivation - only return known specs
	return nil, domain.NewInstanceSpecsError(instanceType, domain.ErrNotFound)
}

// GetAllInstanceSpecs returns specifications for all known instance types
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

// deriveSpecsFromName attempts to derive instance specifications from the instance type name
func (p *InstanceSpecsProvider) deriveSpecsFromName(instanceType string) *domain.InstanceSpecs {
	parts := strings.Split(instanceType, ".")
	if len(parts) != 2 {
		return nil
	}

	family := parts[0]
	size := parts[1]

	// Determine vCPU and memory based on size
	vcpu, memGB := p.parseSize(size)
	if vcpu == 0 {
		return nil
	}

	// Determine category and other properties from family
	category := p.parseCategory(family)
	hasGPU := p.isGPUFamily(family)
	isDeprecated := p.isDeprecatedFamily(family)
	generation := p.parseGeneration(family)
	architecture := p.parseArchitecture(family)

	return &domain.InstanceSpecs{
		InstanceType:  instanceType,
		VCPU:          vcpu,
		MemoryGB:      memGB,
		HasGPU:        hasGPU,
		GPUCount:      p.getGPUCount(family, size),
		Architecture:  architecture,
		Category:      category,
		Generation:    generation,
		IsDeprecated:  isDeprecated,
		IsBurstable:   p.isBurstableFamily(family),
		IsBareMetal:   strings.HasSuffix(size, "metal"),
		CloudProvider: domain.AWS,
	}
}

// parseSize extracts vCPU and memory from size string
func (p *InstanceSpecsProvider) parseSize(size string) (vcpu int, memGB float64) {
	// Handle metal instances
	if strings.Contains(size, "metal") {
		// Metal instances vary greatly, use regex to extract multiplier if present
		re := regexp.MustCompile(`metal-(\d+)xl`)
		if matches := re.FindStringSubmatch(size); len(matches) > 1 {
			// Parse the multiplier
			var mult int
			fmt.Sscanf(matches[1], "%d", &mult)
			return mult * 4, float64(mult * 4 * 2) // Rough estimate
		}
		return 96, 192 // Default large metal
	}

	sizeMap := map[string]struct {
		vcpu int
		mem  float64
	}{
		"nano":     {1, 0.5},
		"micro":    {1, 1},
		"small":    {1, 2},
		"medium":   {1, 4},
		"large":    {2, 8},
		"xlarge":   {4, 16},
		"2xlarge":  {8, 32},
		"3xlarge":  {12, 48},
		"4xlarge":  {16, 64},
		"6xlarge":  {24, 96},
		"8xlarge":  {32, 128},
		"9xlarge":  {36, 144},
		"10xlarge": {40, 160},
		"12xlarge": {48, 192},
		"16xlarge": {64, 256},
		"18xlarge": {72, 288},
		"24xlarge": {96, 384},
		"32xlarge": {128, 512},
		"48xlarge": {192, 768},
		"56xlarge": {224, 896},
	}

	if s, exists := sizeMap[size]; exists {
		return s.vcpu, s.mem
	}

	return 0, 0
}

// parseCategory determines the instance category from the family prefix
func (p *InstanceSpecsProvider) parseCategory(family string) domain.InstanceCategory {
	prefix := strings.ToLower(family)

	switch {
	case strings.HasPrefix(prefix, "c"):
		return domain.ComputeOptimized
	case strings.HasPrefix(prefix, "r"), strings.HasPrefix(prefix, "x"), strings.HasPrefix(prefix, "u"):
		return domain.MemoryOptimized
	case strings.HasPrefix(prefix, "i"), strings.HasPrefix(prefix, "d"), strings.HasPrefix(prefix, "h"):
		return domain.StorageOptimized
	case strings.HasPrefix(prefix, "p"), strings.HasPrefix(prefix, "g"), strings.HasPrefix(prefix, "inf"), strings.HasPrefix(prefix, "trn"):
		return domain.AcceleratedComputing
	case strings.HasPrefix(prefix, "hpc"):
		return domain.HighPerformance
	default:
		return domain.GeneralPurpose
	}
}

// isGPUFamily checks if the instance family includes GPU
func (p *InstanceSpecsProvider) isGPUFamily(family string) bool {
	gpuFamilies := []string{"p2", "p3", "p4", "p5", "g3", "g4", "g5", "g6", "inf", "trn", "dl", "gr", "vt"}
	prefix := strings.ToLower(family)

	for _, gpuFamily := range gpuFamilies {
		if strings.HasPrefix(prefix, gpuFamily) {
			return true
		}
	}
	return false
}

// isDeprecatedFamily checks if the instance family is deprecated
func (p *InstanceSpecsProvider) isDeprecatedFamily(family string) bool {
	deprecatedFamilies := []string{
		"t1", "m1", "m2", "m3", "c1", "c3", "cc1", "cc2", "cg1", "cr1",
		"hi1", "hs1", "g2", "r3", "i2", "d2",
	}
	prefix := strings.ToLower(family)

	for _, deprecated := range deprecatedFamilies {
		if strings.HasPrefix(prefix, deprecated) {
			return true
		}
	}
	return false
}

// parseGeneration determines the generation from the family name
func (p *InstanceSpecsProvider) parseGeneration(family string) domain.InstanceGeneration {
	// Extract generation number from family (e.g., m5 -> 5, c7g -> 7)
	re := regexp.MustCompile(`[a-z]+(\d+)`)
	matches := re.FindStringSubmatch(strings.ToLower(family))

	if len(matches) < 2 {
		return domain.Legacy
	}

	var gen int
	fmt.Sscanf(matches[1], "%d", &gen)

	switch {
	case gen >= 7:
		return domain.Current
	case gen >= 5:
		return domain.Previous
	case gen >= 3:
		return domain.Legacy
	default:
		return domain.Deprecated
	}
}

// parseArchitecture determines the CPU architecture from the family name
func (p *InstanceSpecsProvider) parseArchitecture(family string) string {
	// Graviton instances end with 'g' (e.g., m6g, c7g, r6g)
	lower := strings.ToLower(family)

	if strings.HasSuffix(lower, "g") || strings.HasSuffix(lower, "gd") || strings.HasSuffix(lower, "gn") {
		// But not 'hpc6a', 'c6i' etc - check for Graviton indicators
		if strings.Contains(lower, "6g") || strings.Contains(lower, "7g") || strings.Contains(lower, "8g") {
			return "arm64"
		}
	}

	if strings.HasPrefix(lower, "a1") {
		return "arm64"
	}

	return "x86_64"
}

// isBurstableFamily checks if the instance family is burstable
func (p *InstanceSpecsProvider) isBurstableFamily(family string) bool {
	burstableFamilies := []string{"t2", "t3", "t3a", "t4g"}
	prefix := strings.ToLower(family)

	for _, burstable := range burstableFamilies {
		if prefix == burstable {
			return true
		}
	}
	return false
}

// getGPUCount returns the number of GPUs for GPU instances
func (p *InstanceSpecsProvider) getGPUCount(family, size string) int {
	if !p.isGPUFamily(family) {
		return 0
	}

	// GPU count typically scales with instance size
	sizeToGPU := map[string]int{
		"xlarge":   1,
		"2xlarge":  1,
		"4xlarge":  1,
		"8xlarge":  1,
		"12xlarge": 4,
		"16xlarge": 4,
		"24xlarge": 4,
		"48xlarge": 8,
		"metal":    8,
	}

	if count, exists := sizeToGPU[size]; exists {
		return count
	}
	return 1
}

// initializeSpecs pre-populates the specs catalog with known instance types
func (p *InstanceSpecsProvider) initializeSpecs() {
	// Current generation general purpose instances
	p.addInstanceFamily("m5", "x86_64", domain.GeneralPurpose, false, false, domain.Previous)
	p.addInstanceFamily("m5a", "x86_64", domain.GeneralPurpose, false, false, domain.Previous)
	p.addInstanceFamily("m5d", "x86_64", domain.GeneralPurpose, false, false, domain.Previous)
	p.addInstanceFamily("m5n", "x86_64", domain.GeneralPurpose, false, false, domain.Previous)
	p.addInstanceFamily("m5zn", "x86_64", domain.GeneralPurpose, false, false, domain.Previous)
	p.addInstanceFamily("m6i", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m6a", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m6id", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m6in", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m6g", "arm64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m6gd", "arm64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m7i", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m7i-flex", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m7g", "arm64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m7gd", "arm64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m7a", "x86_64", domain.GeneralPurpose, false, false, domain.Current)
	p.addInstanceFamily("m8g", "arm64", domain.GeneralPurpose, false, false, domain.Current)

	// Compute optimized instances
	p.addInstanceFamily("c5", "x86_64", domain.ComputeOptimized, false, false, domain.Previous)
	p.addInstanceFamily("c5a", "x86_64", domain.ComputeOptimized, false, false, domain.Previous)
	p.addInstanceFamily("c5d", "x86_64", domain.ComputeOptimized, false, false, domain.Previous)
	p.addInstanceFamily("c5n", "x86_64", domain.ComputeOptimized, false, false, domain.Previous)
	p.addInstanceFamily("c6i", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6a", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6id", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6in", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6g", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6gd", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c6gn", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7i", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7i-flex", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7g", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7gd", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7gn", "arm64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c7a", "x86_64", domain.ComputeOptimized, false, false, domain.Current)
	p.addInstanceFamily("c8g", "arm64", domain.ComputeOptimized, false, false, domain.Current)

	// Memory optimized instances
	p.addInstanceFamily("r5", "x86_64", domain.MemoryOptimized, false, false, domain.Previous)
	p.addInstanceFamily("r5a", "x86_64", domain.MemoryOptimized, false, false, domain.Previous)
	p.addInstanceFamily("r5b", "x86_64", domain.MemoryOptimized, false, false, domain.Previous)
	p.addInstanceFamily("r5d", "x86_64", domain.MemoryOptimized, false, false, domain.Previous)
	p.addInstanceFamily("r5n", "x86_64", domain.MemoryOptimized, false, false, domain.Previous)
	p.addInstanceFamily("r6i", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r6a", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r6id", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r6in", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r6g", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r6gd", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r7i", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r7iz", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r7g", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r7gd", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r7a", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("r8g", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("x2idn", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("x2iedn", "x86_64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("x2gd", "arm64", domain.MemoryOptimized, false, false, domain.Current)
	p.addInstanceFamily("x8g", "arm64", domain.MemoryOptimized, false, false, domain.Current)

	// Storage optimized instances
	p.addInstanceFamily("i3", "x86_64", domain.StorageOptimized, false, false, domain.Previous)
	p.addInstanceFamily("i3en", "x86_64", domain.StorageOptimized, false, false, domain.Previous)
	p.addInstanceFamily("i4i", "x86_64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("i4g", "arm64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("im4gn", "arm64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("is4gen", "arm64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("i7i", "x86_64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("i7ie", "x86_64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("i8g", "arm64", domain.StorageOptimized, false, false, domain.Current)
	p.addInstanceFamily("d2", "x86_64", domain.StorageOptimized, false, true, domain.Deprecated)
	p.addInstanceFamily("d3", "x86_64", domain.StorageOptimized, false, false, domain.Previous)
	p.addInstanceFamily("d3en", "x86_64", domain.StorageOptimized, false, false, domain.Previous)

	// Burstable instances
	p.addBurstableFamily("t2", "x86_64", domain.Previous)
	p.addBurstableFamily("t3", "x86_64", domain.Current)
	p.addBurstableFamily("t3a", "x86_64", domain.Current)
	p.addBurstableFamily("t4g", "arm64", domain.Current)

	// GPU instances
	p.addGPUFamily("g4dn", "x86_64", "NVIDIA T4", domain.Current)
	p.addGPUFamily("g4ad", "x86_64", "AMD Radeon Pro V520", domain.Current)
	p.addGPUFamily("g5", "x86_64", "NVIDIA A10G", domain.Current)
	p.addGPUFamily("g6", "x86_64", "NVIDIA L4", domain.Current)
	p.addGPUFamily("g6e", "x86_64", "NVIDIA L40S", domain.Current)
	p.addGPUFamily("gr6", "x86_64", "NVIDIA L4", domain.Current)
	p.addGPUFamily("p3", "x86_64", "NVIDIA V100", domain.Previous)
	p.addGPUFamily("p4d", "x86_64", "NVIDIA A100", domain.Current)
	p.addGPUFamily("p4de", "x86_64", "NVIDIA A100", domain.Current)
	p.addGPUFamily("p5", "x86_64", "NVIDIA H100", domain.Current)

	// Inference instances
	p.addGPUFamily("inf1", "x86_64", "AWS Inferentia", domain.Previous)
	p.addGPUFamily("inf2", "x86_64", "AWS Inferentia2", domain.Current)
	p.addGPUFamily("trn1", "x86_64", "AWS Trainium", domain.Current)
	p.addGPUFamily("trn1n", "x86_64", "AWS Trainium", domain.Current)

	// Deprecated families
	p.addDeprecatedFamily("m1")
	p.addDeprecatedFamily("m2")
	p.addDeprecatedFamily("m3")
	p.addDeprecatedFamily("c1")
	p.addDeprecatedFamily("c3")
	p.addDeprecatedFamily("r3")
	p.addDeprecatedFamily("i2")
	p.addDeprecatedFamily("t1")
	p.addDeprecatedFamily("g2")
}

// addInstanceFamily adds all standard sizes for an instance family
func (p *InstanceSpecsProvider) addInstanceFamily(family, arch string, category domain.InstanceCategory, hasGPU, isDeprecated bool, generation domain.InstanceGeneration) {
	sizes := []struct {
		suffix string
		vcpu   int
		mem    float64
	}{
		{"large", 2, 8},
		{"xlarge", 4, 16},
		{"2xlarge", 8, 32},
		{"4xlarge", 16, 64},
		{"8xlarge", 32, 128},
		{"12xlarge", 48, 192},
		{"16xlarge", 64, 256},
		{"24xlarge", 96, 384},
		{"32xlarge", 128, 512},
		{"48xlarge", 192, 768},
		{"metal", 96, 384},
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
		instanceType := fmt.Sprintf("%s.%s", family, size.suffix)
		p.specs[instanceType] = domain.InstanceSpecs{
			InstanceType:  instanceType,
			VCPU:          size.vcpu,
			MemoryGB:      size.mem * memMultiplier,
			HasGPU:        hasGPU,
			Architecture:  arch,
			Category:      category,
			Generation:    generation,
			IsDeprecated:  isDeprecated,
			IsBurstable:   false,
			IsBareMetal:   size.suffix == "metal",
			CloudProvider: domain.AWS,
		}
	}
}

// addBurstableFamily adds burstable instance types
func (p *InstanceSpecsProvider) addBurstableFamily(family, arch string, generation domain.InstanceGeneration) {
	sizes := []struct {
		suffix string
		vcpu   int
		mem    float64
	}{
		{"nano", 2, 0.5},
		{"micro", 2, 1},
		{"small", 2, 2},
		{"medium", 2, 4},
		{"large", 2, 8},
		{"xlarge", 4, 16},
		{"2xlarge", 8, 32},
	}

	for _, size := range sizes {
		instanceType := fmt.Sprintf("%s.%s", family, size.suffix)
		p.specs[instanceType] = domain.InstanceSpecs{
			InstanceType:  instanceType,
			VCPU:          size.vcpu,
			MemoryGB:      size.mem,
			HasGPU:        false,
			Architecture:  arch,
			Category:      domain.GeneralPurpose,
			Generation:    generation,
			IsDeprecated:  false,
			IsBurstable:   true,
			IsBareMetal:   false,
			CloudProvider: domain.AWS,
		}
	}
}

// addGPUFamily adds GPU instance types
func (p *InstanceSpecsProvider) addGPUFamily(family, arch, gpuType string, generation domain.InstanceGeneration) {
	sizes := []struct {
		suffix   string
		vcpu     int
		mem      float64
		gpuCount int
	}{
		{"xlarge", 4, 16, 1},
		{"2xlarge", 8, 32, 1},
		{"4xlarge", 16, 64, 1},
		{"8xlarge", 32, 128, 1},
		{"12xlarge", 48, 192, 4},
		{"16xlarge", 64, 256, 4},
		{"24xlarge", 96, 384, 4},
		{"48xlarge", 192, 768, 8},
		{"metal", 96, 384, 8},
	}

	for _, size := range sizes {
		instanceType := fmt.Sprintf("%s.%s", family, size.suffix)
		p.specs[instanceType] = domain.InstanceSpecs{
			InstanceType:  instanceType,
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
			IsBareMetal:   size.suffix == "metal",
			CloudProvider: domain.AWS,
		}
	}
}

// addDeprecatedFamily marks a family as deprecated
func (p *InstanceSpecsProvider) addDeprecatedFamily(family string) {
	sizes := []string{"small", "medium", "large", "xlarge", "2xlarge", "4xlarge", "8xlarge"}

	for _, size := range sizes {
		instanceType := fmt.Sprintf("%s.%s", family, size)
		vcpu, mem := p.parseSize(size)
		p.specs[instanceType] = domain.InstanceSpecs{
			InstanceType:  instanceType,
			VCPU:          vcpu,
			MemoryGB:      mem,
			HasGPU:        false,
			Architecture:  "x86_64",
			Category:      domain.GeneralPurpose,
			Generation:    domain.Deprecated,
			IsDeprecated:  true,
			IsBurstable:   false,
			IsBareMetal:   false,
			CloudProvider: domain.AWS,
		}
	}
}

// init registers the AWS instance specs provider with the factory
func init() {
	provider.RegisterSpecsProviderCreator(domain.AWS, func() (domain.InstanceSpecsProvider, error) {
		return NewInstanceSpecsProvider(), nil
	})
}
