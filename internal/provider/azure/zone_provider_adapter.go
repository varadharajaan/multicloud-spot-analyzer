// Package azure provides the zone availability adapter for smart AZ selection.
package azure

import (
	"context"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/logging"
)

// ZoneProviderAdapter adapts the SKU availability provider to the analyzer interface
type ZoneProviderAdapter struct {
	provider *SKUAvailabilityProvider
	region   string
}

// NewZoneProviderAdapter creates a new zone provider adapter
func NewZoneProviderAdapter(region string) *ZoneProviderAdapter {
	return &ZoneProviderAdapter{
		provider: NewSKUAvailabilityProvider(),
		region:   region,
	}
}

// IsAvailable returns true if Azure credentials are configured
func (a *ZoneProviderAdapter) IsAvailable() bool {
	return a.provider.IsAvailable()
}

// GetZoneAvailability returns zone availability for a VM in the configured region
// For Azure, most VMs are available in zones 1, 2, 3 - we return these with varying capacity scores
func (a *ZoneProviderAdapter) GetZoneAvailability(ctx context.Context, vmSize, region string) ([]analyzer.ZoneInfo, error) {
	// Use the provided region or fall back to configured region
	targetRegion := region
	if targetRegion == "" {
		targetRegion = a.region
	}

	// Azure SKU API bulk fetch takes 30-60 seconds first time, but is cached after
	// Use 90 second timeout to allow initial fetch to complete
	fetchCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	
	zones, err := a.provider.GetZoneAvailability(fetchCtx, vmSize, targetRegion)
	if err == nil && len(zones) > 0 {
		// Got real data
		result := make([]analyzer.ZoneInfo, len(zones))
		for i, z := range zones {
			result[i] = analyzer.ZoneInfo{
				Zone:           z.Zone,
				Available:      z.Available,
				Restricted:     z.Restricted,
				RestrictionMsg: z.RestrictionReason,
				CapacityScore:  z.CapacityScore,
				UsingRealData:  true,
			}
		}
		return result, nil
	}

	// Log the error for debugging
	if err != nil {
		logging.Warn("Azure SKU fetch failed: %v (will use fallback)", err)
	}

	// Fallback: Return standard Azure zones with default capacity scores
	// We don't have real data - return equal scores and mark as not using real data
	return []analyzer.ZoneInfo{
		{Zone: targetRegion + "-1", Available: true, CapacityScore: 75, UsingRealData: false},
		{Zone: targetRegion + "-2", Available: true, CapacityScore: 75, UsingRealData: false},
		{Zone: targetRegion + "-3", Available: true, CapacityScore: 75, UsingRealData: false},
	}, nil
}

// CapacityProviderAdapter provides capacity estimates for Azure VMs
// Uses per-VM SKU data for zone availability
type CapacityProviderAdapter struct {
	skuProvider *SKUAvailabilityProvider
	region      string
}

// NewCapacityProviderAdapter creates a new capacity provider adapter
func NewCapacityProviderAdapter(region string) *CapacityProviderAdapter {
	return &CapacityProviderAdapter{
		skuProvider: NewSKUAvailabilityProvider(),
		region:      region,
	}
}

// GetCapacityScore returns the capacity score (0-100) for a VM in a zone
// Uses per-VM SKU data only - avoids bulk SKU fetch that takes 30+ seconds
func (c *CapacityProviderAdapter) GetCapacityScore(ctx context.Context, vmSize, zone string) (int, error) {
	// Get zone availability for this specific VM (fast, ~100ms via fetchSingleSKU)
	zones, err := c.skuProvider.GetZoneAvailability(ctx, vmSize, c.region)
	if err != nil {
		return 50, nil // Default medium score on error
	}

	// Base score depends on how many zones the VM is available in
	// More zones available = more capacity overall
	baseScore := len(zones) * 25 // 1 zone = 25, 2 zones = 50, 3 zones = 75
	if baseScore > 75 {
		baseScore = 75
	}

	// Check if this specific zone is available/restricted
	for _, z := range zones {
		if z.Zone == zone {
			if !z.Available {
				return 10, nil // Not available = very low score
			}
			if z.Restricted {
				return baseScore / 2, nil // Restricted = half score
			}
			if z.CapacityScore > 0 {
				return (baseScore + z.CapacityScore) / 2, nil
			}
			return baseScore + 25, nil // Available and not restricted = add bonus
		}
	}

	return 25, nil // Zone not found = low score
}

// isPrimaryRegion returns true if the region is a primary Azure region (typically has more capacity)
func isPrimaryRegion(region string) bool {
	primaryRegions := map[string]bool{
		"eastus":             true,
		"eastus2":            true,
		"westus2":            true,
		"westeurope":         true,
		"northeurope":        true,
		"southeastasia":      true,
		"australiaeast":      true,
		"uksouth":            true,
		"centralus":          true,
		"canadacentral":      true,
		"japaneast":          true,
		"koreacentral":       true,
		"francecentral":      true,
		"germanywestcentral": true,
	}
	return primaryRegions[region]
}
