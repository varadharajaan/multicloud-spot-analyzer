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
			Timeout: 30 * time.Second,
		},
		cacheManager: provider.GetCacheManager(),
	}
}

// IsAvailable checks if Azure credentials are configured
func (p *SKUAvailabilityProvider) IsAvailable() bool {
	cfg := config.Get()
	return cfg.Azure.TenantID != "" &&
		cfg.Azure.ClientID != "" &&
		cfg.Azure.ClientSecret != "" &&
		cfg.Azure.SubscriptionID != ""
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

	// Get access token
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	cfg := config.Get()

	// Build API URL - filter by location and VM size
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/skus?api-version=2021-07-01&$filter=location eq '%s'",
		cfg.Azure.SubscriptionID,
		region,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Azure API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp SKUsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	// Find the specific VM size
	normalizedVMSize := strings.ToLower(vmSize)
	normalizedVMSize = strings.TrimPrefix(normalizedVMSize, "standard_")

	for _, sku := range apiResp.Value {
		if sku.ResourceType != "virtualMachines" {
			continue
		}

		skuName := strings.ToLower(sku.Name)
		skuName = strings.TrimPrefix(skuName, "standard_")

		if skuName == normalizedVMSize || strings.EqualFold(sku.Name, vmSize) {
			return &sku, nil
		}
	}

	return nil, fmt.Errorf("VM size %s not found in region %s", vmSize, region)
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
