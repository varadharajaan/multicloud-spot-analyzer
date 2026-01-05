// Package gcp provides the zone availability adapter for smart AZ selection.
package gcp

import (
	"context"
	"strings"

	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/domain"
)

// ZoneProviderAdapter adapts GCP zone availability to the analyzer interface
type ZoneProviderAdapter struct {
	region string
}

// NewZoneProviderAdapter creates a new zone provider adapter
func NewZoneProviderAdapter(region string) *ZoneProviderAdapter {
	return &ZoneProviderAdapter{
		region: region,
	}
}

// IsAvailable returns true - GCP zone data is available via static configuration
func (a *ZoneProviderAdapter) IsAvailable() bool {
	return true
}

// GetZoneAvailability returns zone availability for a machine type in the configured region
func (a *ZoneProviderAdapter) GetZoneAvailability(ctx context.Context, machineType, region string) ([]analyzer.ZoneInfo, error) {
	// Use the provided region or fall back to configured region
	targetRegion := region
	if targetRegion == "" {
		targetRegion = a.region
	}

	zones := getZonesForRegion(targetRegion)
	family := extractFamily(machineType)

	result := make([]analyzer.ZoneInfo, 0, len(zones))
	for _, zone := range zones {
		// Check machine type availability in zone
		available, restricted, reason := a.checkZoneAvailability(family, machineType, zone)

		// Calculate capacity score based on zone characteristics
		capacityScore := a.calculateCapacityScore(family, zone)

		result = append(result, analyzer.ZoneInfo{
			Zone:           zone,
			Available:      available,
			Restricted:     restricted,
			RestrictionMsg: reason,
			CapacityScore:  capacityScore,
		})
	}

	return result, nil
}

// checkZoneAvailability checks if a machine type is available in a zone
func (a *ZoneProviderAdapter) checkZoneAvailability(family, machineType, zone string) (available, restricted bool, reason string) {
	// Most machine types are available in most zones
	// GPU instances have zone restrictions
	if strings.HasPrefix(family, "a2") || strings.HasPrefix(family, "a3") || strings.HasPrefix(family, "g2") {
		// GPU instances have zone restrictions
		if !isGPUAvailableInZone(zone) {
			return false, true, "GPU instances not available in this zone"
		}
	}

	// Memory-optimized instances may have restrictions
	if strings.HasPrefix(family, "m2") || strings.HasPrefix(family, "m3") {
		if !isMemoryOptimizedAvailableInZone(zone) {
			return true, true, "Limited capacity for memory-optimized instances"
		}
	}

	// ARM instances (T2A) have zone restrictions
	if family == "t2a" {
		if !isARMAvailableInZone(zone) {
			return false, true, "ARM instances not available in this zone"
		}
	}

	return true, false, ""
}

// isGPUAvailableInZone checks GPU availability in a zone
func isGPUAvailableInZone(zone string) bool {
	// GPU instances are available in select zones
	gpuZones := map[string]bool{
		"us-central1-a":          true,
		"us-central1-b":          true,
		"us-central1-c":          true,
		"us-central1-f":          true,
		"us-east1-b":             true,
		"us-east1-c":             true,
		"us-east1-d":             true,
		"us-east4-a":             true,
		"us-east4-b":             true,
		"us-east4-c":             true,
		"us-west1-a":             true,
		"us-west1-b":             true,
		"us-west2-b":             true,
		"us-west2-c":             true,
		"us-west4-a":             true,
		"us-west4-b":             true,
		"europe-west1-b":         true,
		"europe-west1-c":         true,
		"europe-west1-d":         true,
		"europe-west2-a":         true,
		"europe-west2-b":         true,
		"europe-west4-a":         true,
		"europe-west4-b":         true,
		"europe-west4-c":         true,
		"asia-east1-a":           true,
		"asia-east1-b":           true,
		"asia-east1-c":           true,
		"asia-northeast1-a":      true,
		"asia-northeast1-c":      true,
		"asia-northeast3-b":      true,
		"asia-south1-a":          true,
		"asia-south1-b":          true,
		"asia-southeast1-a":      true,
		"asia-southeast1-b":      true,
		"asia-southeast1-c":      true,
		"australia-southeast1-a": true,
		"australia-southeast1-c": true,
	}
	return gpuZones[zone]
}

// isMemoryOptimizedAvailableInZone checks memory-optimized availability
func isMemoryOptimizedAvailableInZone(zone string) bool {
	// Memory-optimized instances are available in most zones but with capacity limits
	// For simplicity, return true for major zones
	region := getRegionFromZone(zone)
	return isHighDemandRegion(region)
}

// isARMAvailableInZone checks ARM (T2A) availability
func isARMAvailableInZone(zone string) bool {
	// T2A is available in limited regions/zones
	armZones := map[string]bool{
		"us-central1-a":     true,
		"us-central1-b":     true,
		"us-central1-f":     true,
		"europe-west4-a":    true,
		"europe-west4-b":    true,
		"asia-southeast1-b": true,
		"asia-southeast1-c": true,
	}
	return armZones[zone]
}

// getRegionFromZone extracts region from zone name
func getRegionFromZone(zone string) string {
	// Zone format: region-zone (e.g., us-central1-a)
	lastDash := strings.LastIndex(zone, "-")
	if lastDash > 0 {
		return zone[:lastDash]
	}
	return zone
}

// calculateCapacityScore estimates capacity score for a zone
func (a *ZoneProviderAdapter) calculateCapacityScore(family, zone string) int {
	baseScore := 70 // Default base score

	// Adjust based on zone position (a is usually primary)
	if strings.HasSuffix(zone, "-a") {
		baseScore += 10
	} else if strings.HasSuffix(zone, "-b") {
		baseScore += 5
	}

	// Adjust based on region
	region := getRegionFromZone(zone)
	if isHighDemandRegion(region) {
		baseScore -= 10 // Higher demand = lower capacity
	}

	// Adjust based on machine family
	switch family {
	case "e2", "n2", "n2d":
		baseScore += 10 // Common instances have good capacity
	case "c2", "c2d", "c3":
		baseScore -= 5 // Compute instances slightly lower
	case "a2", "a3", "g2":
		baseScore -= 20 // GPU instances have limited capacity
	case "t2a":
		baseScore -= 15 // ARM instances limited availability
	}

	// Clamp to valid range
	if baseScore > 100 {
		baseScore = 100
	}
	if baseScore < 10 {
		baseScore = 10
	}

	return baseScore
}

// CapacityProviderAdapter provides capacity estimates for GCP VMs
type CapacityProviderAdapter struct {
	region string
}

// NewCapacityProviderAdapter creates a new capacity provider adapter
func NewCapacityProviderAdapter(region string) *CapacityProviderAdapter {
	return &CapacityProviderAdapter{
		region: region,
	}
}

// GetCapacityScore returns the capacity score (0-100) for a VM in a zone
func (c *CapacityProviderAdapter) GetCapacityScore(ctx context.Context, machineType, zone string) (int, error) {
	family := extractFamily(machineType)
	adapter := &ZoneProviderAdapter{region: c.region}
	return adapter.calculateCapacityScore(family, zone), nil
}

// GetProviderName returns the cloud provider
func (a *ZoneProviderAdapter) GetProviderName() domain.CloudProvider {
	return domain.GCP
}

// GetAllZones returns all zones in a region
func (a *ZoneProviderAdapter) GetAllZones(ctx context.Context, region string) ([]string, error) {
	return getZonesForRegion(region), nil
}
