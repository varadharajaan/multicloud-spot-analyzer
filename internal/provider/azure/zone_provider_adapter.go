// Package azure provides the zone availability adapter for smart AZ selection.
package azure

import (
	"context"

	"github.com/spot-analyzer/internal/analyzer"
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
func (a *ZoneProviderAdapter) GetZoneAvailability(ctx context.Context, vmSize, region string) ([]analyzer.ZoneInfo, error) {
	// Use the provided region or fall back to configured region
	targetRegion := region
	if targetRegion == "" {
		targetRegion = a.region
	}

	// Get zone availability from SKU API
	zones, err := a.provider.GetZoneAvailability(ctx, vmSize, targetRegion)
	if err != nil {
		return nil, err
	}

	// Convert to analyzer.ZoneInfo
	result := make([]analyzer.ZoneInfo, len(zones))
	for i, z := range zones {
		result[i] = analyzer.ZoneInfo{
			Zone:           z.Zone,
			Available:      z.Available,
			Restricted:     z.Restricted,
			RestrictionMsg: z.RestrictionReason,
			CapacityScore:  z.CapacityScore,
		}
	}

	return result, nil
}

// CapacityProviderAdapter provides capacity estimates for Azure VMs
// Azure doesn't expose real capacity data, so we estimate based on:
// - Number of zones available (more zones = more capacity likely)
// - VM series popularity
// - Region tier (primary regions have more capacity)
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

// GetCapacityScore returns an estimated capacity score (0-100) for a VM in a zone
func (c *CapacityProviderAdapter) GetCapacityScore(ctx context.Context, vmSize, zone string) (int, error) {
	// Get all zones for this VM to estimate relative capacity
	zones, err := c.skuProvider.GetZoneAvailability(ctx, vmSize, c.region)
	if err != nil {
		return 50, nil // Return medium score on error
	}

	// More zones available = more capacity overall
	baseScore := len(zones) * 25 // 1 zone = 25, 2 zones = 50, 3 zones = 75
	if baseScore > 75 {
		baseScore = 75
	}

	// Check if this specific zone is restricted
	for _, z := range zones {
		if z.Zone == zone {
			if z.Restricted {
				return baseScore / 2, nil // Half score if restricted
			}
			// Add zone-specific adjustment based on capacity score from SKU API
			if z.CapacityScore > 0 {
				return (baseScore + z.CapacityScore) / 2, nil
			}
			return baseScore + 25, nil // Available and not restricted = add bonus
		}
	}

	return 25, nil // Zone not found = low score
}

// PrimaryRegions returns true if the region is a primary Azure region (typically has more capacity)
func isPrimaryRegion(region string) bool {
	primaryRegions := map[string]bool{
		"eastus":         true,
		"eastus2":        true,
		"westus2":        true,
		"westeurope":     true,
		"northeurope":    true,
		"southeastasia":  true,
		"australiaeast":  true,
		"uksouth":        true,
		"centralus":      true,
		"canadacentral":  true,
		"japaneast":      true,
		"koreacentral":   true,
		"francecentral":  true,
		"germanywestcentral": true,
	}
	return primaryRegions[region]
}
