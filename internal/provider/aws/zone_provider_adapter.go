// Package aws provides the zone availability adapter for smart AZ selection.
package aws

import (
	"context"
	"strings"

	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
)

// ZoneProviderAdapter adapts AWS data providers to the analyzer interface
type ZoneProviderAdapter struct {
	spotProvider *SpotDataProvider
	region       string
}

// NewZoneProviderAdapter creates a new zone provider adapter for AWS
func NewZoneProviderAdapter(region string) *ZoneProviderAdapter {
	return &ZoneProviderAdapter{
		spotProvider: NewSpotDataProvider(),
		region:       region,
	}
}

// IsAvailable returns true for AWS as Spot Advisor data is publicly available
func (a *ZoneProviderAdapter) IsAvailable() bool {
	return true
}

// GetZoneAvailability returns zone availability for a VM in the configured region
// Note: AWS Spot Advisor doesn't provide zone-specific data, so we infer availability
// from the presence of the instance type in the region data
func (a *ZoneProviderAdapter) GetZoneAvailability(ctx context.Context, instanceType, region string) ([]analyzer.ZoneInfo, error) {
	targetRegion := region
	if targetRegion == "" {
		targetRegion = a.region
	}

	// For AWS, we need to determine available zones from EC2 API or use standard zone suffixes
	// Since we don't have EC2 credentials, we'll use the standard zone patterns
	zones := getAWSZonesForRegion(targetRegion)

	// Check if the instance type is available in this region
	spotData, err := a.spotProvider.FetchSpotData(ctx, targetRegion, domain.Linux)
	if err != nil {
		logging.Debug("AWS: No spot data for %s in %s: %v", instanceType, targetRegion, err)
		// Even without spot data, instance might be available - return zones with low confidence
		result := make([]analyzer.ZoneInfo, len(zones))
		for i, z := range zones {
			result[i] = analyzer.ZoneInfo{
				Zone:          z,
				Available:     false, // Can't confirm
				Restricted:    false,
				CapacityScore: 50, // Medium confidence
			}
		}
		return result, nil
	}

	// Check if this specific instance type has data
	instanceFound := false
	var instanceSpotData domain.SpotData
	for _, sd := range spotData {
		if sd.InstanceType == instanceType {
			instanceFound = true
			instanceSpotData = sd
			break
		}
	}

	result := make([]analyzer.ZoneInfo, len(zones))
	for i, z := range zones {
		capacityScore := 50 // Default medium capacity

		if instanceFound {
			// Instance is available - use interruption frequency to estimate capacity
			// Lower interruption = higher capacity availability
			switch instanceSpotData.InterruptionFrequency {
			case 0: // <5%
				capacityScore = 90
			case 1: // 5-10%
				capacityScore = 75
			case 2: // 10-15%
				capacityScore = 60
			case 3: // 15-20%
				capacityScore = 45
			case 4: // >20%
				capacityScore = 30
			}
		}

		result[i] = analyzer.ZoneInfo{
			Zone:          z,
			Available:     instanceFound,
			Restricted:    false,
			CapacityScore: capacityScore,
		}
	}

	return result, nil
}

// getAWSZonesForRegion returns the standard availability zones for an AWS region
func getAWSZonesForRegion(region string) []string {
	// Most AWS regions have 3 availability zones named a, b, c
	// Some have more or fewer - this is a reasonable default
	zoneMap := map[string][]string{
		"us-east-1":      {"us-east-1a", "us-east-1b", "us-east-1c", "us-east-1d", "us-east-1e", "us-east-1f"},
		"us-east-2":      {"us-east-2a", "us-east-2b", "us-east-2c"},
		"us-west-1":      {"us-west-1a", "us-west-1b"},
		"us-west-2":      {"us-west-2a", "us-west-2b", "us-west-2c", "us-west-2d"},
		"eu-west-1":      {"eu-west-1a", "eu-west-1b", "eu-west-1c"},
		"eu-west-2":      {"eu-west-2a", "eu-west-2b", "eu-west-2c"},
		"eu-west-3":      {"eu-west-3a", "eu-west-3b", "eu-west-3c"},
		"eu-central-1":   {"eu-central-1a", "eu-central-1b", "eu-central-1c"},
		"eu-north-1":     {"eu-north-1a", "eu-north-1b", "eu-north-1c"},
		"ap-south-1":     {"ap-south-1a", "ap-south-1b", "ap-south-1c"},
		"ap-southeast-1": {"ap-southeast-1a", "ap-southeast-1b", "ap-southeast-1c"},
		"ap-southeast-2": {"ap-southeast-2a", "ap-southeast-2b", "ap-southeast-2c"},
		"ap-northeast-1": {"ap-northeast-1a", "ap-northeast-1c", "ap-northeast-1d"},
		"ap-northeast-2": {"ap-northeast-2a", "ap-northeast-2b", "ap-northeast-2c", "ap-northeast-2d"},
		"ap-northeast-3": {"ap-northeast-3a", "ap-northeast-3b", "ap-northeast-3c"},
		"sa-east-1":      {"sa-east-1a", "sa-east-1b", "sa-east-1c"},
		"ca-central-1":   {"ca-central-1a", "ca-central-1b", "ca-central-1d"},
		"me-south-1":     {"me-south-1a", "me-south-1b", "me-south-1c"},
		"af-south-1":     {"af-south-1a", "af-south-1b", "af-south-1c"},
	}

	if zones, ok := zoneMap[region]; ok {
		return zones
	}

	// Default to 3 zones for unknown regions
	return []string{
		region + "a",
		region + "b",
		region + "c",
	}
}

// CapacityProviderAdapter provides capacity estimates for AWS VMs
type CapacityProviderAdapter struct {
	spotProvider *SpotDataProvider
	region       string
}

// NewCapacityProviderAdapter creates a new capacity provider adapter for AWS
func NewCapacityProviderAdapter(region string) *CapacityProviderAdapter {
	return &CapacityProviderAdapter{
		spotProvider: NewSpotDataProvider(),
		region:       region,
	}
}

// GetCapacityScore returns an estimated capacity score (0-100) for a VM in a zone
// For AWS, we estimate based on:
// - Instance type interruption rate (lower = more capacity)
// - Instance family popularity
// - Region popularity
func (c *CapacityProviderAdapter) GetCapacityScore(ctx context.Context, instanceType, zone string) (int, error) {
	// Extract region from zone (e.g., "us-east-1a" -> "us-east-1")
	region := extractRegionFromZone(zone)
	if region == "" {
		region = c.region
	}

	// Get spot data to estimate capacity
	spotData, err := c.spotProvider.FetchSpotData(ctx, region, domain.Linux)
	if err != nil {
		// No data - return medium score
		return 50, nil
	}

	// Find the instance type
	for _, sd := range spotData {
		if sd.InstanceType == instanceType {
			// Use interruption frequency as inverse capacity indicator
			// Lower interruption = higher capacity availability
			switch sd.InterruptionFrequency {
			case 0: // <5%
				return 95, nil
			case 1: // 5-10%
				return 80, nil
			case 2: // 10-15%
				return 65, nil
			case 3: // 15-20%
				return 45, nil
			case 4: // >20%
				return 25, nil
			}
		}
	}

	// Instance not found - estimate based on family
	baseScore := 50
	lowerType := strings.ToLower(instanceType)

	// General purpose (t, m) usually have more capacity
	if strings.HasPrefix(lowerType, "t") || strings.HasPrefix(lowerType, "m") {
		baseScore = 70
	}

	// Compute optimized (c) typically good availability
	if strings.HasPrefix(lowerType, "c") {
		baseScore = 65
	}

	// Memory optimized (r) good availability
	if strings.HasPrefix(lowerType, "r") {
		baseScore = 60
	}

	// GPU instances (p, g) often constrained
	if strings.HasPrefix(lowerType, "p") || strings.HasPrefix(lowerType, "g") {
		baseScore = 35
	}

	// FPGA/Inf instances very constrained
	if strings.HasPrefix(lowerType, "f") || strings.HasPrefix(lowerType, "inf") {
		baseScore = 25
	}

	return baseScore, nil
}

// extractRegionFromZone extracts the region from an AWS zone name
func extractRegionFromZone(zone string) string {
	if len(zone) < 2 {
		return ""
	}
	// AWS zones are region + letter (e.g., "us-east-1a")
	return zone[:len(zone)-1]
}
