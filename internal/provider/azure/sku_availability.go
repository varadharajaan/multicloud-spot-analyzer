// Package azure implements Azure SKU availability checking.
// This provides per-zone VM availability data for smart AZ recommendations.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/config"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

// Global mutex and in-flight tracking to prevent duplicate API calls
var (
	skuFetchMu       sync.Mutex
	skuFetchInFlight = make(map[string]chan struct{})
)

// SKUAvailabilityProvider checks VM SKU availability per zone
type SKUAvailabilityProvider struct {
	httpClient   *http.Client
	cacheManager *provider.CacheManager
	accessToken  string
	tokenExpiry  time.Time
	mu           sync.RWMutex
}

// SKUInfo contains availability information for a VM SKU
type SKUInfo struct {
	Name         string         `json:"name"`
	ResourceType string         `json:"resourceType"`
	Tier         string         `json:"tier"`
	Size         string         `json:"size"`
	Family       string         `json:"family"`
	Locations    []string       `json:"locations"`
	LocationInfo []LocationInfo `json:"locationInfo"`
	Capabilities []Capability   `json:"capabilities"`
	Restrictions []Restriction  `json:"restrictions"`
}

// LocationInfo contains zone-specific information
type LocationInfo struct {
	Location    string       `json:"location"`
	Zones       []string     `json:"zones"`
	ZoneDetails []ZoneDetail `json:"zoneDetails,omitempty"`
}

// ZoneDetail contains per-zone capacity info
type ZoneDetail struct {
	Name         []string     `json:"name"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// Capability represents a VM capability
type Capability struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Restriction represents zone or location restrictions
type Restriction struct {
	Type            string   `json:"type"`
	Values          []string `json:"values"`
	ReasonCode      string   `json:"reasonCode"`
	RestrictionInfo struct {
		Locations []string `json:"locations"`
		Zones     []string `json:"zones"`
	} `json:"restrictionInfo"`
}

// ZoneAvailability represents VM availability in a zone
type ZoneAvailability struct {
	Zone              string
	Available         bool
	Restricted        bool
	RestrictionReason string
	CapacityScore     int // Higher = more likely to have capacity
}

// AZRecommendation represents a recommended availability zone
type AZRecommendationResult struct {
	Zone      string
	Score     float64
	Rank      int
	Available bool
	Reason    string
}

// SKUsAPIResponse represents the Azure SKUs API response
type SKUsAPIResponse struct {
	Value    []SKUInfo `json:"value"`
	NextLink string    `json:"nextLink,omitempty"`
}

// TokenResponse represents the Azure AD token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// NewSKUAvailabilityProvider creates a new SKU availability provider
func NewSKUAvailabilityProvider() *SKUAvailabilityProvider {
	return &SKUAvailabilityProvider{
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // SKU API returns large response, needs more time
		},
		cacheManager: provider.GetCacheManager(),
	}
}

// IsAvailable checks if Azure credentials are configured
func (p *SKUAvailabilityProvider) IsAvailable() bool {
	cfg := config.Get()
	available := cfg.Azure.TenantID != "" &&
		cfg.Azure.ClientID != "" &&
		cfg.Azure.ClientSecret != "" &&
		cfg.Azure.SubscriptionID != ""

	return available
}

// IsVMAvailableInRegion checks if a VM size is available in a specific region
// Returns true if the VM is available (found in SKU cache), false otherwise
func (p *SKUAvailabilityProvider) IsVMAvailableInRegion(ctx context.Context, vmSize, region string) bool {
	_, err := p.getSKUInfo(ctx, vmSize, region)
	return err == nil
}

// FilterAvailableVMs filters a list of VM sizes to only those available in the region
func (p *SKUAvailabilityProvider) FilterAvailableVMs(ctx context.Context, vmSizes []string, region string) []string {
	available := make([]string, 0, len(vmSizes))
	for _, vm := range vmSizes {
		if p.IsVMAvailableInRegion(ctx, vm, region) {
			available = append(available, vm)
		}
	}
	return available
}

// GetZoneAvailability returns zone availability for a VM size in a region
func (p *SKUAvailabilityProvider) GetZoneAvailability(ctx context.Context, vmSize, region string) ([]ZoneAvailability, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("azure:sku:%s:%s", region, vmSize)
	if cached, exists := p.cacheManager.Get(cacheKey); exists {
		return cached.([]ZoneAvailability), nil
	}

	// Get SKU info from Azure
	skuInfo, err := p.getSKUInfo(ctx, vmSize, region)
	if err != nil {
		return nil, err
	}

	// Parse zone availability
	zones := p.parseZoneAvailability(skuInfo, region)

	// Cache for 1 hour
	if len(zones) > 0 {
		p.cacheManager.Set(cacheKey, zones, 1*time.Hour)
	}

	return zones, nil
}

// RecommendAZ returns recommended availability zones for a VM size
func (p *SKUAvailabilityProvider) RecommendAZ(ctx context.Context, vmSize, region string) ([]AZRecommendationResult, error) {
	zones, err := p.GetZoneAvailability(ctx, vmSize, region)
	if err != nil {
		return nil, err
	}

	// Score and rank zones
	results := make([]AZRecommendationResult, 0)
	for _, z := range zones {
		score := 0.0
		reason := ""

		if z.Available && !z.Restricted {
			score = 1.0 + float64(z.CapacityScore)/100.0
			reason = "Available with no restrictions"
		} else if z.Available && z.Restricted {
			score = 0.5
			reason = fmt.Sprintf("Restricted: %s", z.RestrictionReason)
		} else {
			score = 0.0
			reason = "Not available in this zone"
		}

		results = append(results, AZRecommendationResult{
			Zone:      z.Zone,
			Score:     score,
			Available: z.Available,
			Reason:    reason,
		})
	}

	// Sort by score (descending), then by zone name for stability
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Zone < results[j].Zone
	})

	// Assign ranks
	for i := range results {
		results[i].Rank = i + 1
	}
	return results, nil
}

// getSKUInfo fetches SKU information from Azure API
func (p *SKUAvailabilityProvider) getSKUInfo(ctx context.Context, vmSize, region string) (*SKUInfo, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Azure credentials not configured")
	}

	// Use global cache key since we fetch all regions at once
	globalCacheKey := "azure:skus:all"

	// Check cache first (fast path)
	if cached, exists := p.cacheManager.Get(globalCacheKey); exists {
		return p.findSKUInCache(cached.(map[string]*SKUInfo), vmSize, region)
	}

	// Synchronize SKU fetching to prevent duplicate API calls
	skuFetchMu.Lock()

	// Double-check cache after acquiring lock
	if cached, exists := p.cacheManager.Get(globalCacheKey); exists {
		skuFetchMu.Unlock()
		return p.findSKUInCache(cached.(map[string]*SKUInfo), vmSize, region)
	}

	// Check if another goroutine is already fetching
	if waitCh, inFlight := skuFetchInFlight["all"]; inFlight {
		skuFetchMu.Unlock()
		// Wait for the in-flight request to complete
		<-waitCh
		// Now cache should be populated
		if cached, exists := p.cacheManager.Get(globalCacheKey); exists {
			return p.findSKUInCache(cached.(map[string]*SKUInfo), vmSize, region)
		}
		return nil, fmt.Errorf("SKU fetch failed")
	}

	// Mark as being fetched
	waitCh := make(chan struct{})
	skuFetchInFlight["all"] = waitCh
	skuFetchMu.Unlock()

	// Ensure we clean up and notify waiters when done
	defer func() {
		skuFetchMu.Lock()
		delete(skuFetchInFlight, "all")
		close(waitCh)
		skuFetchMu.Unlock()
	}()

	// Actually fetch the SKUs
	skuMap, err := p.fetchAllSKUs(ctx, region)
	if err != nil {
		return nil, err
	}

	// Cache the result globally for 2 hours
	p.cacheManager.Set(globalCacheKey, skuMap, 2*time.Hour)
	logging.Info("Cached %d VM SKUs globally", len(skuMap))

	return p.findSKUInCache(skuMap, vmSize, region)
}

// findSKUInCache looks up a VM size in the cached SKU map for a specific region
func (p *SKUAvailabilityProvider) findSKUInCache(skuMap map[string]*SKUInfo, vmSize, region string) (*SKUInfo, error) {
	normalizedVMSize := strings.ToLower(vmSize)
	normalizedVMSize = strings.TrimPrefix(normalizedVMSize, "standard_")
	normalizedRegion := strings.ToLower(region)

	// Use region:vmname as lookup key
	cacheKey := fmt.Sprintf("%s:%s", normalizedRegion, normalizedVMSize)
	if sku, found := skuMap[cacheKey]; found {
		return sku, nil
	}

	return nil, fmt.Errorf("VM size %s not available in region %s", vmSize, region)
}

// fetchAllSKUs fetches all VM SKUs for a region from Azure API
func (p *SKUAvailabilityProvider) fetchAllSKUs(ctx context.Context, region string) (map[string]*SKUInfo, error) {
	// Get access token
	token, err := p.getAccessToken(ctx)
	if err != nil {
		logging.Warn("Failed to get Azure access token: %v", err)
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	cfg := config.Get()

	// Build API URL - DON'T filter by location, as the filtered response
	// doesn't include proper LocationInfo for the filtered region.
	// We'll filter in code after parsing the full response.
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/skus?api-version=2021-07-01",
		cfg.Azure.SubscriptionID,
	)

	logging.Info("Fetching all Azure SKUs (no location filter)")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		logging.Warn("Azure SKU API request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logging.Warn("Azure SKU API returned status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("Azure API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp SKUsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	logging.Info("Azure SKU API returned %d total SKUs", len(apiResp.Value))

	// Build a map of VM SKUs, keyed by "region:vmname" to avoid overwrites
	// since each VM appears once per region in the API response
	skuMap := make(map[string]*SKUInfo)
	for i := range apiResp.Value {
		sku := &apiResp.Value[i]
		if sku.ResourceType != "virtualMachines" {
			continue
		}

		// Get the region from the Locations array (each entry has exactly one)
		if len(sku.Locations) == 0 {
			continue
		}
		skuRegion := strings.ToLower(sku.Locations[0])

		skuName := strings.ToLower(sku.Name)
		skuName = strings.TrimPrefix(skuName, "standard_")

		// Use region:vmname as key
		cacheKey := fmt.Sprintf("%s:%s", skuRegion, skuName)
		skuMap[cacheKey] = sku
	}

	return skuMap, nil
}

// parseZoneAvailability extracts zone availability from SKU info
func (p *SKUAvailabilityProvider) parseZoneAvailability(sku *SKUInfo, region string) []ZoneAvailability {
	zones := make([]ZoneAvailability, 0)

	if sku == nil {
		return zones
	}

	// Find location info for the region
	var locInfo *LocationInfo
	for i := range sku.LocationInfo {
		if strings.EqualFold(sku.LocationInfo[i].Location, region) {
			locInfo = &sku.LocationInfo[i]
			break
		}
	}

	if locInfo == nil || len(locInfo.Zones) == 0 {
		// VM doesn't support zones in this region
		logging.Debug("VM %s does not support availability zones in %s", sku.Name, region)
		return zones
	}

	// Build zone availability list
	for _, zone := range locInfo.Zones {
		za := ZoneAvailability{
			Zone:          fmt.Sprintf("%s-%s", region, zone),
			Available:     true,
			Restricted:    false,
			CapacityScore: 100, // Default high score
		}

		// Check for restrictions
		for _, r := range sku.Restrictions {
			if r.Type == "Zone" {
				for _, rz := range r.RestrictionInfo.Zones {
					if rz == zone {
						za.Restricted = true
						za.RestrictionReason = r.ReasonCode
						za.CapacityScore = 25 // Lower score for restricted zones
						break
					}
				}
			}
		}

		zones = append(zones, za)
	}

	// Sort by zone name for consistency
	sort.Slice(zones, func(i, j int) bool {
		return zones[i].Zone < zones[j].Zone
	})

	return zones
}

// GetZoneCapacityScores returns capacity scores for all zones in a region
// based on how many VM types are available in each zone (Approach 2)
// More VM types available = higher infrastructure capacity = lower eviction risk
func (p *SKUAvailabilityProvider) GetZoneCapacityScores(ctx context.Context, region string) (map[string]int, error) {
	globalCacheKey := "azure:skus:all"
	capacityCacheKey := fmt.Sprintf("azure:zone_capacity:%s", region)

	// Check capacity cache first
	if cached, exists := p.cacheManager.Get(capacityCacheKey); exists {
		return cached.(map[string]int), nil
	}

	// Get all SKUs from cache or fetch
	var skuMap map[string]*SKUInfo
	if cached, exists := p.cacheManager.Get(globalCacheKey); exists {
		skuMap = cached.(map[string]*SKUInfo)
	} else {
		var err error
		skuMap, err = p.fetchAllSKUs(ctx, region)
		if err != nil {
			return nil, err
		}
		p.cacheManager.Set(globalCacheKey, skuMap, 2*time.Hour)
	}

	// Count VM types available per zone
	zoneCounts := make(map[string]int)
	normalizedRegion := strings.ToLower(region)

	for cacheKey, sku := range skuMap {
		// Check if this SKU is for our region (cache key format: region:vmname)
		if !strings.HasPrefix(cacheKey, normalizedRegion+":") {
			continue
		}

		// Count zones for this SKU
		for _, locInfo := range sku.LocationInfo {
			if strings.ToLower(locInfo.Location) == normalizedRegion {
				for _, zone := range locInfo.Zones {
					zoneName := fmt.Sprintf("%s-%s", region, zone)
					zoneCounts[zoneName]++
				}
			}
		}
	}

	if len(zoneCounts) == 0 {
		// Default zones if no data
		return map[string]int{
			fmt.Sprintf("%s-1", region): 50,
			fmt.Sprintf("%s-2", region): 50,
			fmt.Sprintf("%s-3", region): 50,
		}, nil
	}

	// Find min and max for normalization
	minCount, maxCount := 999999, 0
	for _, count := range zoneCounts {
		if count < minCount {
			minCount = count
		}
		if count > maxCount {
			maxCount = count
		}
	}

	// Normalize to 0-100 score
	// Zone with most VM types = 100, zone with least = proportionally lower
	scores := make(map[string]int)
	for zone, count := range zoneCounts {
		if maxCount == minCount {
			scores[zone] = 75 // All zones equal
		} else {
			// Linear normalization: 25 (minimum) to 100 (maximum)
			scores[zone] = 25 + int(float64(count-minCount)/float64(maxCount-minCount)*75)
		}
	}

	logging.Info("Zone capacity scores for %s: %v (based on %d VM types)", region, scores, len(skuMap))
	p.cacheManager.Set(capacityCacheKey, scores, 2*time.Hour)

	return scores, nil
}

// getAccessToken gets or refreshes Azure AD access token
func (p *SKUAvailabilityProvider) getAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Return cached token if still valid
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry.Add(-5*time.Minute)) {
		return p.accessToken, nil
	}

	cfg := config.Get()

	// Request new token
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.Azure.TenantID)

	data := fmt.Sprintf(
		"client_id=%s&client_secret=%s&scope=https://management.azure.com/.default&grant_type=client_credentials",
		cfg.Azure.ClientID,
		cfg.Azure.ClientSecret,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	p.accessToken = tokenResp.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	logging.Debug("Azure access token obtained, expires in %d seconds", tokenResp.ExpiresIn)
	return p.accessToken, nil
}
