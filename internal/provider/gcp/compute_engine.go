// Package gcp provides Compute Engine API client for zone availability.
package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

const (
	// ComputeAPIBase is the base URL for Compute Engine API
	ComputeAPIBase = "https://compute.googleapis.com/compute/v1"

	// Cache TTL for zone availability (1 hours - reduce API calls)
	ZoneAvailabilityCacheTTL = 1 * time.Hour

	// Cache TTL for zones list (24 hours - zones rarely change)
	ZonesListCacheTTL = 24 * time.Hour

	// Cache key prefix for zone availability
	cacheKeyZoneAvail = "gcp:zone_avail:"

	// Cache key prefix for zones list
	cacheKeyZonesList = "gcp:zones_list:"
)

// ComputeEngineClient provides access to GCP Compute Engine API for zone availability
type ComputeEngineClient struct {
	httpClient   *http.Client
	credManager  *CredentialManager
	cacheManager *provider.CacheManager
	mu           sync.RWMutex
}

// ZoneAvailabilityInfo contains zone availability information
type ZoneAvailabilityInfo struct {
	Zone           string
	MachineType    string
	Available      bool
	Restriction    string
	CapacityStatus string // AVAILABLE, LIMITED, UNAVAILABLE
	LastUpdated    time.Time
}

// MachineTypeInfo represents a machine type from the API
type MachineTypeInfo struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Zone               string `json:"zone"`
	GuestCPUs          int    `json:"guestCpus"`
	MemoryMB           int    `json:"memoryMb"`
	MaximumPersistDisk int    `json:"maximumPersistentDisks"`
	IsSharedCPU        bool   `json:"isSharedCpu"`
	Deprecated         *struct {
		State       string `json:"state"`
		Replacement string `json:"replacement"`
	} `json:"deprecated,omitempty"`
}

// MachineTypeListResponse is the API response for listing machine types
type MachineTypeListResponse struct {
	Items         []MachineTypeInfo `json:"items"`
	NextPageToken string            `json:"nextPageToken"`
}

// ZoneInfo represents a zone from the API
type ZoneInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // UP, DOWN
	Region      string `json:"region"`
}

// ZoneListResponse is the API response for listing zones
type ZoneListResponse struct {
	Items         []ZoneInfo `json:"items"`
	NextPageToken string     `json:"nextPageToken"`
}

// NewComputeEngineClient creates a new Compute Engine API client
func NewComputeEngineClient() *ComputeEngineClient {
	return &ComputeEngineClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		credManager:  GetCredentialManager(),
		cacheManager: provider.GetCacheManager(),
	}
}

// GetZoneAvailability checks machine type availability in zones for a region
// Uses aggregated API to minimize API calls
func (c *ComputeEngineClient) GetZoneAvailability(ctx context.Context, machineType, region string) ([]ZoneAvailabilityInfo, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s%s:%s", cacheKeyZoneAvail, region, machineType)
	if cached, exists := c.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache HIT for GCP zone availability %s:%s", region, machineType)
		return cached.([]ZoneAvailabilityInfo), nil
	}
	logging.Debug("Cache MISS for GCP zone availability %s:%s", region, machineType)

	// Check if credentials are available
	if !c.credManager.IsAvailable() {
		logging.Debug("GCP credentials not available, using estimated availability")
		return nil, nil // Return nil to fall back to estimates
	}

	// Initialize credentials if needed
	if err := c.credManager.Initialize(ctx); err != nil {
		logging.Warn("Failed to initialize GCP credentials: %v", err)
		return nil, nil
	}

	// Get zones for the region (cached for 24h)
	zones, err := c.getZonesForRegion(ctx, region)
	if err != nil {
		logging.Warn("Failed to get zones for region %s: %v", region, err)
		return nil, nil
	}

	// Use aggregated machineTypes API to get availability in single call
	results, err := c.getMachineTypeAvailabilityBatch(ctx, machineType, region, zones)
	if err != nil {
		logging.Warn("Failed to batch check machine type: %v", err)
		// Fall back to simple zone list with assumed availability
		results = c.createDefaultAvailability(zones, machineType)
	}

	// Cache results
	if len(results) > 0 {
		c.cacheManager.Set(cacheKey, results, ZoneAvailabilityCacheTTL)
	}

	return results, nil
}

// getZonesForRegion retrieves zones for a region from the API (with caching)
func (c *ComputeEngineClient) getZonesForRegion(ctx context.Context, region string) ([]ZoneInfo, error) {
	// Check cache first - zones rarely change
	cacheKey := cacheKeyZonesList + region
	if cached, exists := c.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache HIT for GCP zones list %s", region)
		return cached.([]ZoneInfo), nil
	}
	logging.Debug("Cache MISS for GCP zones list %s - fetching from API", region)

	projectID := c.credManager.GetProjectID()
	if projectID == "" {
		return nil, fmt.Errorf("no project ID configured")
	}

	// List all zones and filter by region prefix (simpler and more reliable than filter param)
	url := fmt.Sprintf("%s/projects/%s/zones",
		ComputeAPIBase, projectID)

	req, err := c.createAuthenticatedRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zones: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body for more details
		var errBody map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&errBody) == nil {
			if errInfo, ok := errBody["error"].(map[string]interface{}); ok {
				if msg, ok := errInfo["message"].(string); ok {
					return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, msg)
				}
			}
		}
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var zoneResp ZoneListResponse
	if err := json.NewDecoder(resp.Body).Decode(&zoneResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter zones that belong to this region
	var regionZones []ZoneInfo
	for _, z := range zoneResp.Items {
		// Zone names are like "us-central1-a", region is "us-central1"
		if strings.HasPrefix(z.Name, region+"-") {
			regionZones = append(regionZones, z)
		}
	}

	// Cache the zones list for 24 hours
	if len(regionZones) > 0 {
		c.cacheManager.Set(cacheKey, regionZones, ZonesListCacheTTL)
		logging.Debug("Cached %d zones for region %s", len(regionZones), region)
	}

	return regionZones, nil
}

// AggregatedMachineTypesResponse is the response from aggregated machineTypes API
type AggregatedMachineTypesResponse struct {
	Items map[string]struct {
		MachineTypes []MachineTypeInfo `json:"machineTypes"`
	} `json:"items"`
}

// getMachineTypeAvailabilityBatch uses aggregated API to check availability in one call
func (c *ComputeEngineClient) getMachineTypeAvailabilityBatch(ctx context.Context, machineType, region string, zones []ZoneInfo) ([]ZoneAvailabilityInfo, error) {
	projectID := c.credManager.GetProjectID()
	if projectID == "" {
		return nil, fmt.Errorf("no project ID configured")
	}

	// Use aggregated machineTypes API with filter - single call for all zones
	// Filter by machine type name to reduce response size
	url := fmt.Sprintf("%s/projects/%s/aggregated/machineTypes?filter=name=%s",
		ComputeAPIBase, projectID, machineType)

	req, err := c.createAuthenticatedRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch aggregated machine types: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aggregated API returned status %d", resp.StatusCode)
	}

	var aggResp AggregatedMachineTypesResponse
	if err := json.NewDecoder(resp.Body).Decode(&aggResp); err != nil {
		return nil, fmt.Errorf("failed to decode aggregated response: %w", err)
	}

	// Build results for zones in this region
	var results []ZoneAvailabilityInfo
	zoneSet := make(map[string]bool)
	for _, z := range zones {
		zoneSet[z.Name] = true
	}

	// Check which zones have this machine type
	zonesWithType := make(map[string]bool)
	for scopeKey, scope := range aggResp.Items {
		// scopeKey is like "zones/us-central1-a"
		for _, mt := range scope.MachineTypes {
			if mt.Name == machineType {
				// Extract zone name from scope key
				if strings.HasPrefix(scopeKey, "zones/") {
					zoneName := strings.TrimPrefix(scopeKey, "zones/")
					zonesWithType[zoneName] = true
				}
			}
		}
	}

	// Create availability info for each zone in the region
	for _, zone := range zones {
		avail := ZoneAvailabilityInfo{
			Zone:        zone.Name,
			MachineType: machineType,
			LastUpdated: time.Now(),
		}
		
		if zonesWithType[zone.Name] {
			avail.Available = true
			avail.CapacityStatus = "AVAILABLE"
		} else {
			avail.Available = false
			avail.CapacityStatus = "UNAVAILABLE"
			avail.Restriction = "Machine type not available in this zone"
		}
		
		results = append(results, avail)
	}

	logging.Debug("Batch checked %s in %d zones (single API call)", machineType, len(results))
	return results, nil
}

// createDefaultAvailability creates default availability assuming all zones are available
func (c *ComputeEngineClient) createDefaultAvailability(zones []ZoneInfo, machineType string) []ZoneAvailabilityInfo {
	var results []ZoneAvailabilityInfo
	for _, zone := range zones {
		results = append(results, ZoneAvailabilityInfo{
			Zone:           zone.Name,
			MachineType:    machineType,
			Available:      true,
			CapacityStatus: "AVAILABLE",
			LastUpdated:    time.Now(),
		})
	}
	return results
}

// checkMachineTypeInZone checks if a machine type is available in a zone (legacy, kept for specific lookups)
func (c *ComputeEngineClient) checkMachineTypeInZone(ctx context.Context, machineType, zone string) ZoneAvailabilityInfo {
	result := ZoneAvailabilityInfo{
		Zone:           zone,
		MachineType:    machineType,
		Available:      false,
		CapacityStatus: "UNKNOWN",
		LastUpdated:    time.Now(),
	}

	projectID := c.credManager.GetProjectID()
	if projectID == "" {
		result.Restriction = "No project ID configured"
		return result
	}

	// Query machine type in this zone
	url := fmt.Sprintf("%s/projects/%s/zones/%s/machineTypes/%s",
		ComputeAPIBase, projectID, zone, machineType)

	req, err := c.createAuthenticatedRequest(ctx, "GET", url)
	if err != nil {
		result.Restriction = fmt.Sprintf("Request error: %v", err)
		return result
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		result.Restriction = fmt.Sprintf("API error: %v", err)
		return result
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Machine type exists in this zone
		var mt MachineTypeInfo
		if err := json.NewDecoder(resp.Body).Decode(&mt); err == nil {
			result.Available = true
			result.CapacityStatus = "AVAILABLE"

			// Check if deprecated
			if mt.Deprecated != nil {
				result.Restriction = fmt.Sprintf("Deprecated: %s", mt.Deprecated.State)
				if mt.Deprecated.State == "DELETED" {
					result.Available = false
					result.CapacityStatus = "UNAVAILABLE"
				}
			}
		}

	case http.StatusNotFound:
		result.Available = false
		result.CapacityStatus = "UNAVAILABLE"
		result.Restriction = "Machine type not available in this zone"

	case http.StatusForbidden:
		// Likely quota or permission issue
		result.Restriction = "Access denied - check permissions or quota"
		result.CapacityStatus = "LIMITED"

	default:
		result.Restriction = fmt.Sprintf("API returned status %d", resp.StatusCode)
	}

	return result
}

// GetAllMachineTypesInZone retrieves all machine types available in a zone
func (c *ComputeEngineClient) GetAllMachineTypesInZone(ctx context.Context, zone string) ([]MachineTypeInfo, error) {
	if !c.credManager.IsAvailable() {
		return nil, nil
	}

	projectID := c.credManager.GetProjectID()
	if projectID == "" {
		return nil, fmt.Errorf("no project ID configured")
	}

	var allTypes []MachineTypeInfo
	pageToken := ""

	for {
		url := fmt.Sprintf("%s/projects/%s/zones/%s/machineTypes",
			ComputeAPIBase, projectID, zone)
		if pageToken != "" {
			url += "?pageToken=" + pageToken
		}

		req, err := c.createAuthenticatedRequest(ctx, "GET", url)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch machine types: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var mtResp MachineTypeListResponse
		if err := json.NewDecoder(resp.Body).Decode(&mtResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		allTypes = append(allTypes, mtResp.Items...)

		if mtResp.NextPageToken == "" {
			break
		}
		pageToken = mtResp.NextPageToken
	}

	return allTypes, nil
}

// createAuthenticatedRequest creates an HTTP request with authentication
func (c *ComputeEngineClient) createAuthenticatedRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	// Get token from credential manager
	tokenSource := c.credManager.GetTokenSource()
	if tokenSource == nil {
		return nil, fmt.Errorf("no token source available")
	}

	// Get OAuth2 token
	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	return req, nil
}

// IsAvailable returns true if Compute Engine API can be accessed
func (c *ComputeEngineClient) IsAvailable() bool {
	return c.credManager.IsAvailable()
}

// GetCapacityScore estimates capacity score for a zone based on availability
func (c *ComputeEngineClient) GetCapacityScore(ctx context.Context, machineType, zone string) int {
	avail, err := c.GetZoneAvailability(ctx, machineType, getRegionFromZone(zone))
	if err != nil || avail == nil {
		return -1 // Unknown, use fallback
	}

	for _, z := range avail {
		if z.Zone == zone {
			switch z.CapacityStatus {
			case "AVAILABLE":
				if z.Restriction == "" {
					return 90
				}
				return 70 // Available but with restrictions
			case "LIMITED":
				return 50
			case "UNAVAILABLE":
				return 0
			}
		}
	}

	return -1 // Not found
}

// getRegionFromZone extracts region from zone name (e.g., "us-central1-a" -> "us-central1")
func getRegionFromZone(zone string) string {
	lastDash := strings.LastIndex(zone, "-")
	if lastDash > 0 {
		return zone[:lastDash]
	}
	return zone
}
