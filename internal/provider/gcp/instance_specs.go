// Package gcp implements GCP instance specifications catalog.
// This catalog contains metadata about GCP machine types including
// vCPU, memory, GPU info, and series information.
package gcp

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
)

// InstanceSpecsProvider provides instance specifications for GCP machine types
type InstanceSpecsProvider struct {
	mu    sync.RWMutex
	specs map[string]domain.InstanceSpecs
}

// NewInstanceSpecsProvider creates a new GCP instance specs provider
func NewInstanceSpecsProvider() *InstanceSpecsProvider {
	p := &InstanceSpecsProvider{
		specs: make(map[string]domain.InstanceSpecs),
	}
	p.initializeSpecs()
	return p
}

// GetProviderName returns the cloud provider identifier
func (p *InstanceSpecsProvider) GetProviderName() domain.CloudProvider {
	return domain.GCP
}

// GetInstanceSpecs returns specifications for a specific machine type
func (p *InstanceSpecsProvider) GetInstanceSpecs(ctx context.Context, machineType string) (*domain.InstanceSpecs, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Try exact match first
	if specs, exists := p.specs[machineType]; exists {
		return &specs, nil
	}

	// Try to derive specs from machine type name
	derivedSpecs := p.deriveSpecsFromName(machineType)
	if derivedSpecs != nil {
		return derivedSpecs, nil
	}

	return nil, domain.NewInstanceSpecsError(machineType, domain.ErrNotFound)
}

// GetAllInstanceSpecs returns specifications for all known machine types
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

// deriveSpecsFromName attempts to derive instance specifications from the machine type name
func (p *InstanceSpecsProvider) deriveSpecsFromName(machineType string) *domain.InstanceSpecs {
	// GCP format: {family}-{type}-{vcpu} or {family}-{type}
	// Examples: n2-standard-4, e2-medium, c2-standard-8, n2d-standard-2

	parts := strings.Split(machineType, "-")
	if len(parts) < 2 {
		return nil
	}

	family := strings.ToLower(parts[0])
	machineClass := strings.ToLower(parts[1])

	// Extract vCPU count
	vcpu := 0
	if len(parts) >= 3 {
		var err error
		vcpu, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil
		}
	} else {
		// Predefined sizes
		vcpu = getPredefinedVCPU(machineClass)
	}

	if vcpu == 0 {
		return nil
	}

	// Calculate memory based on machine class
	memoryGB := calculateMemoryGB(family, machineClass, vcpu)

	// Determine category
	category := p.getCategory(family)

	// Determine architecture
	architecture := p.getArchitecture(family)

	// Determine generation
	generation := p.getGeneration(family)

	return &domain.InstanceSpecs{
		InstanceType:    machineType,
		VCPU:            vcpu,
		MemoryGB:        memoryGB,
		HasGPU:          p.hasGPU(family),
		GPUCount:        p.getGPUCount(family, machineType),
		GPUType:         p.getGPUType(family, machineType),
		Architecture:    architecture,
		Category:        category,
		Generation:      generation,
		IsDeprecated:    p.isDeprecated(family),
		IsBurstable:     p.isBurstable(family),
		IsBareMetal:     strings.Contains(machineType, "metal"),
		CloudProvider:   domain.GCP,
		ProcessorFamily: p.getProcessorFamily(family),
	}
}

// getPredefinedVCPU returns vCPU for predefined machine sizes
func getPredefinedVCPU(machineClass string) int {
	predefined := map[string]int{
		"micro":   1,
		"small":   1,
		"medium":  1,
		"large":   2,
		"xlarge":  4,
		"2xlarge": 8,
		"4xlarge": 16,
		"8xlarge": 32,
	}
	if v, ok := predefined[machineClass]; ok {
		return v
	}
	return 0
}

// calculateMemoryGB returns memory based on family and machine class
func calculateMemoryGB(family, machineClass string, vcpu int) float64 {
	// Memory ratio depends on machine class
	var ratio float64
	switch machineClass {
	case "standard":
		ratio = 4.0 // 4 GB per vCPU
	case "highmem":
		ratio = 8.0 // 8 GB per vCPU
	case "highcpu":
		ratio = 1.0 // 1 GB per vCPU
	case "ultramem":
		ratio = 24.0 // 24 GB per vCPU
	case "megamem":
		ratio = 14.9 // ~15 GB per vCPU
	case "micro":
		return 0.25
	case "small":
		return 0.5
	case "medium":
		return 2.0
	case "large":
		return 4.0
	default:
		ratio = 4.0
	}

	// Special cases for some families
	switch family {
	case "e2":
		if machineClass == "medium" {
			return 4.0
		}
	case "f1":
		return 0.6 // f1-micro
	case "g1":
		return 1.7 // g1-small
	}

	return float64(vcpu) * ratio
}

// getCategory returns the instance category
func (p *InstanceSpecsProvider) getCategory(family string) domain.InstanceCategory {
	switch family {
	case "c2", "c2d", "c3", "c3d":
		return domain.ComputeOptimized
	case "m1", "m2", "m3":
		return domain.MemoryOptimized
	case "a2", "a3", "g2":
		return domain.AcceleratedComputing
	case "z3":
		return domain.StorageOptimized
	default:
		return domain.GeneralPurpose
	}
}

// getArchitecture returns the CPU architecture
func (p *InstanceSpecsProvider) getArchitecture(family string) string {
	switch family {
	case "t2a": // Tau ARM
		return "arm64"
	default:
		return "x86_64"
	}
}

// getGeneration returns the instance generation
func (p *InstanceSpecsProvider) getGeneration(family string) domain.InstanceGeneration {
	switch family {
	case "n1", "f1", "g1":
		return domain.Legacy
	case "n2", "e2", "c2", "m2":
		return domain.Previous
	case "n2d", "c2d", "t2d", "t2a":
		return domain.Current
	case "c3", "c3d", "m3", "a3":
		return domain.Current
	default:
		return domain.Current
	}
}

// getProcessorFamily returns the processor family
func (p *InstanceSpecsProvider) getProcessorFamily(family string) string {
	switch family {
	case "n2d", "c2d", "t2d":
		return "AMD EPYC"
	case "t2a":
		return "Ampere Altra"
	case "c3":
		return "Intel Sapphire Rapids"
	default:
		return "Intel"
	}
}

// isDeprecated checks if the family is deprecated
func (p *InstanceSpecsProvider) isDeprecated(family string) bool {
	deprecated := map[string]bool{
		"n1": false, // Still available but old
		"f1": false, // Shared-core
		"g1": false, // Shared-core
	}
	return deprecated[family]
}

// isBurstable checks if the family is burstable
func (p *InstanceSpecsProvider) isBurstable(family string) bool {
	burstable := map[string]bool{
		"e2": true, // E2 has CPU bursting
		"f1": true,
		"g1": true,
	}
	return burstable[family]
}

// hasGPU checks if the family has GPU
func (p *InstanceSpecsProvider) hasGPU(family string) bool {
	gpuFamilies := map[string]bool{
		"a2": true,
		"a3": true,
		"g2": true,
	}
	return gpuFamilies[family]
}

// getGPUCount returns GPU count for machine type
func (p *InstanceSpecsProvider) getGPUCount(family, machineType string) int {
	if !p.hasGPU(family) {
		return 0
	}

	// Parse GPU count from machine type
	// a2-highgpu-1g, a2-megagpu-16g, etc.
	re := regexp.MustCompile(`(\d+)g$`)
	matches := re.FindStringSubmatch(machineType)
	if len(matches) >= 2 {
		count, _ := strconv.Atoi(matches[1])
		return count
	}

	// Default GPU counts
	switch family {
	case "a2":
		if strings.Contains(machineType, "megagpu") {
			return 16
		}
		return 1
	case "g2":
		return 1
	default:
		return 1
	}
}

// getGPUType returns GPU type for machine type
func (p *InstanceSpecsProvider) getGPUType(family, machineType string) string {
	switch family {
	case "a2":
		return "NVIDIA A100"
	case "a3":
		return "NVIDIA H100"
	case "g2":
		return "NVIDIA L4"
	default:
		return ""
	}
}

// initializeSpecs populates the specs catalog with known GCP machine types
func (p *InstanceSpecsProvider) initializeSpecs() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// E2 Series (General Purpose - Cost Optimized)
	e2Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"micro", 2, 1},
		{"small", 2, 2},
		{"medium", 2, 4},
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"highmem-2", 2, 16},
		{"highmem-4", 4, 32},
		{"highmem-8", 8, 64},
		{"highmem-16", 16, 128},
		{"highcpu-2", 2, 2},
		{"highcpu-4", 4, 4},
		{"highcpu-8", 8, 8},
		{"highcpu-16", 16, 16},
		{"highcpu-32", 32, 32},
	}
	for _, t := range e2Types {
		name := "e2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Current,
			IsBurstable:     true,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel/AMD",
		}
	}

	// N2 Series (General Purpose)
	n2Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"standard-48", 48, 192},
		{"standard-64", 64, 256},
		{"standard-80", 80, 320},
		{"standard-96", 96, 384},
		{"standard-128", 128, 512},
		{"highmem-2", 2, 16},
		{"highmem-4", 4, 32},
		{"highmem-8", 8, 64},
		{"highmem-16", 16, 128},
		{"highmem-32", 32, 256},
		{"highmem-48", 48, 384},
		{"highmem-64", 64, 512},
		{"highmem-80", 80, 640},
		{"highmem-96", 96, 768},
		{"highmem-128", 128, 864},
		{"highcpu-2", 2, 2},
		{"highcpu-4", 4, 4},
		{"highcpu-8", 8, 8},
		{"highcpu-16", 16, 16},
		{"highcpu-32", 32, 32},
		{"highcpu-48", 48, 48},
		{"highcpu-64", 64, 64},
		{"highcpu-80", 80, 80},
		{"highcpu-96", 96, 96},
	}
	for _, t := range n2Types {
		name := "n2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Cascade Lake",
		}
	}

	// N2D Series (AMD-based General Purpose)
	n2dTypes := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"standard-48", 48, 192},
		{"standard-64", 64, 256},
		{"standard-80", 80, 320},
		{"standard-96", 96, 384},
		{"standard-128", 128, 512},
		{"standard-224", 224, 896},
		{"highmem-2", 2, 16},
		{"highmem-4", 4, 32},
		{"highmem-8", 8, 64},
		{"highmem-16", 16, 128},
		{"highmem-32", 32, 256},
		{"highmem-48", 48, 384},
		{"highmem-64", 64, 512},
		{"highmem-80", 80, 640},
		{"highmem-96", 96, 768},
		{"highcpu-2", 2, 2},
		{"highcpu-4", 4, 4},
		{"highcpu-8", 8, 8},
		{"highcpu-16", 16, 16},
		{"highcpu-32", 32, 32},
		{"highcpu-48", 48, 48},
		{"highcpu-64", 64, 64},
		{"highcpu-80", 80, 80},
		{"highcpu-96", 96, 96},
		{"highcpu-128", 128, 128},
		{"highcpu-224", 224, 224},
	}
	for _, t := range n2dTypes {
		name := "n2d-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "AMD EPYC",
		}
	}

	// C2 Series (Compute Optimized)
	c2Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-30", 30, 120},
		{"standard-60", 60, 240},
	}
	for _, t := range c2Types {
		name := "c2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.ComputeOptimized,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Cascade Lake",
		}
	}

	// C2D Series (AMD Compute Optimized)
	c2dTypes := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"standard-56", 56, 224},
		{"standard-112", 112, 448},
		{"highmem-2", 2, 16},
		{"highmem-4", 4, 32},
		{"highmem-8", 8, 64},
		{"highmem-16", 16, 128},
		{"highmem-32", 32, 256},
		{"highmem-56", 56, 448},
		{"highmem-112", 112, 896},
		{"highcpu-2", 2, 2},
		{"highcpu-4", 4, 4},
		{"highcpu-8", 8, 8},
		{"highcpu-16", 16, 16},
		{"highcpu-32", 32, 32},
		{"highcpu-56", 56, 56},
		{"highcpu-112", 112, 112},
	}
	for _, t := range c2dTypes {
		name := "c2d-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.ComputeOptimized,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "AMD EPYC Milan",
		}
	}

	// C3 Series (Next-gen Compute Optimized)
	c3Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-22", 22, 88},
		{"standard-44", 44, 176},
		{"standard-88", 88, 352},
		{"standard-176", 176, 704},
		{"highmem-4", 4, 32},
		{"highmem-8", 8, 64},
		{"highmem-22", 22, 176},
		{"highmem-44", 44, 352},
		{"highmem-88", 88, 704},
		{"highmem-176", 176, 1408},
		{"highcpu-4", 4, 8},
		{"highcpu-8", 8, 16},
		{"highcpu-22", 22, 44},
		{"highcpu-44", 44, 88},
		{"highcpu-88", 88, 176},
		{"highcpu-176", 176, 352},
	}
	for _, t := range c3Types {
		name := "c3-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.ComputeOptimized,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Sapphire Rapids",
		}
	}

	// M2 Series (Memory Optimized)
	m2Types := []struct {
		suffix    string
		vcpu      int
		memoryTiB float64
	}{
		{"ultramem-208", 208, 5.75},
		{"ultramem-416", 416, 11.5},
		{"megamem-416", 416, 5.75},
	}
	for _, t := range m2Types {
		name := "m2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memoryTiB * 1024,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.MemoryOptimized,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Cascade Lake",
		}
	}

	// M3 Series (Memory Optimized - next gen)
	m3Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"ultramem-32", 32, 976},
		{"ultramem-64", 64, 1952},
		{"ultramem-128", 128, 3904},
		{"megamem-64", 64, 976},
		{"megamem-128", 128, 1952},
	}
	for _, t := range m3Types {
		name := "m3-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.MemoryOptimized,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Ice Lake",
		}
	}

	// T2D Series (Tau - AMD General Purpose)
	t2dTypes := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-1", 1, 4},
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"standard-48", 48, 192},
		{"standard-60", 60, 240},
	}
	for _, t := range t2dTypes {
		name := "t2d-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "AMD EPYC Milan",
		}
	}

	// T2A Series (Tau - ARM)
	t2aTypes := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-1", 1, 4},
		{"standard-2", 2, 8},
		{"standard-4", 4, 16},
		{"standard-8", 8, 32},
		{"standard-16", 16, 64},
		{"standard-32", 32, 128},
		{"standard-48", 48, 192},
	}
	for _, t := range t2aTypes {
		name := "t2a-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "arm64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Ampere Altra",
		}
	}

	// A2 Series (GPU - A100)
	a2Types := []struct {
		suffix   string
		vcpu     int
		memory   float64
		gpuCount int
		gpuMem   float64
	}{
		{"highgpu-1g", 12, 85, 1, 40},
		{"highgpu-2g", 24, 170, 2, 80},
		{"highgpu-4g", 48, 340, 4, 160},
		{"highgpu-8g", 96, 680, 8, 320},
		{"megagpu-16g", 96, 1360, 16, 640},
		{"ultragpu-1g", 12, 170, 1, 80},
		{"ultragpu-2g", 24, 340, 2, 160},
		{"ultragpu-4g", 48, 680, 4, 320},
		{"ultragpu-8g", 96, 1360, 8, 640},
	}
	for _, t := range a2Types {
		name := "a2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          true,
			GPUCount:        t.gpuCount,
			GPUType:         "NVIDIA A100",
			GPUMemoryGB:     t.gpuMem,
			Architecture:    "x86_64",
			Category:        domain.AcceleratedComputing,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Cascade Lake",
		}
	}

	// G2 Series (GPU - L4)
	g2Types := []struct {
		suffix   string
		vcpu     int
		memory   float64
		gpuCount int
	}{
		{"standard-4", 4, 16, 1},
		{"standard-8", 8, 32, 1},
		{"standard-12", 12, 48, 1},
		{"standard-16", 16, 64, 1},
		{"standard-24", 24, 96, 2},
		{"standard-32", 32, 128, 1},
		{"standard-48", 48, 192, 4},
		{"standard-96", 96, 384, 8},
	}
	for _, t := range g2Types {
		name := "g2-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          true,
			GPUCount:        t.gpuCount,
			GPUType:         "NVIDIA L4",
			GPUMemoryGB:     float64(t.gpuCount) * 24, // L4 has 24GB each
			Architecture:    "x86_64",
			Category:        domain.AcceleratedComputing,
			Generation:      domain.Current,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel",
		}
	}

	// N1 Series (Previous Generation)
	n1Types := []struct {
		suffix string
		vcpu   int
		memory float64
	}{
		{"standard-1", 1, 3.75},
		{"standard-2", 2, 7.5},
		{"standard-4", 4, 15},
		{"standard-8", 8, 30},
		{"standard-16", 16, 60},
		{"standard-32", 32, 120},
		{"standard-64", 64, 240},
		{"standard-96", 96, 360},
		{"highmem-2", 2, 13},
		{"highmem-4", 4, 26},
		{"highmem-8", 8, 52},
		{"highmem-16", 16, 104},
		{"highmem-32", 32, 208},
		{"highmem-64", 64, 416},
		{"highmem-96", 96, 624},
		{"highcpu-2", 2, 1.8},
		{"highcpu-4", 4, 3.6},
		{"highcpu-8", 8, 7.2},
		{"highcpu-16", 16, 14.4},
		{"highcpu-32", 32, 28.8},
		{"highcpu-64", 64, 57.6},
		{"highcpu-96", 96, 86.4},
	}
	for _, t := range n1Types {
		name := "n1-" + t.suffix
		p.specs[name] = domain.InstanceSpecs{
			InstanceType:    name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Previous,
			IsBurstable:     false,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel Skylake",
		}
	}

	// Shared-core types
	sharedCore := []struct {
		name   string
		vcpu   int
		memory float64
	}{
		{"f1-micro", 1, 0.6},
		{"g1-small", 1, 1.7},
	}
	for _, t := range sharedCore {
		p.specs[t.name] = domain.InstanceSpecs{
			InstanceType:    t.name,
			VCPU:            t.vcpu,
			MemoryGB:        t.memory,
			HasGPU:          false,
			Architecture:    "x86_64",
			Category:        domain.GeneralPurpose,
			Generation:      domain.Legacy,
			IsBurstable:     true,
			CloudProvider:   domain.GCP,
			ProcessorFamily: "Intel",
		}
	}
}

// init registers the GCP specs provider with the factory
func init() {
	provider.RegisterSpecsProviderCreator(domain.GCP, func() (domain.InstanceSpecsProvider, error) {
		return NewInstanceSpecsProvider(), nil
	})
}
