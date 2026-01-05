// Package gcp provides the Cloud Billing Catalog API client for real-time Spot VM pricing.
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
	// BillingCatalogAPIBase is the base URL for Cloud Billing Catalog API
	// This is a PUBLIC API - no authentication required, just an API key
	BillingCatalogAPIBase = "https://cloudbilling.googleapis.com/v1"

	// ComputeEngineServiceID is the service ID for Compute Engine in the Catalog API
	ComputeEngineServiceID = "6F81-5844-456A" // Compute Engine service ID

	// Cache TTL for billing data (24 hours - prices update at most once per day)
	BillingCacheTTL = 24 * time.Hour

	// Cache key for billing data
	cacheKeyBilling = "gcp:billing:"
)

// BillingCatalogClient provides access to GCP Cloud Billing Catalog API
type BillingCatalogClient struct {
	httpClient   *http.Client
	apiKey       string
	cacheManager *provider.CacheManager
	mu           sync.RWMutex
	skuCache     map[string]*SKUPricing // region:machineType -> pricing
	lastRefresh  time.Time
}

// SKUPricing contains pricing information from the Catalog API
type SKUPricing struct {
	SKUID          string
	Description    string
	MachineFamily  string
	Region         string
	UsageType      string // OnDemand, Preemptible, Commit1Mo, Commit1Yr
	VCPUPriceHour  float64
	MemPriceGBHour float64
	TotalPriceHour float64 // For predefined machine types
	EffectiveTime  time.Time
	Available      bool
}

// CatalogResponse represents the SKU list response
type CatalogResponse struct {
	SKUs          []SKU  `json:"skus"`
	NextPageToken string `json:"nextPageToken"`
}

// SKU represents a single SKU from the Catalog API
type SKU struct {
	Name           string      `json:"name"`
	SKUID          string      `json:"skuId"`
	Description    string      `json:"description"`
	Category       SKUCategory `json:"category"`
	ServiceRegions []string    `json:"serviceRegions"`
	PricingInfo    []Pricing   `json:"pricingInfo"`
	GeoTaxonomy    GeoTaxonomy `json:"geoTaxonomy"`
}

// SKUCategory contains categorization data
type SKUCategory struct {
	ServiceDisplayName string `json:"serviceDisplayName"`
	ResourceFamily     string `json:"resourceFamily"`
	ResourceGroup      string `json:"resourceGroup"`
	UsageType          string `json:"usageType"` // OnDemand, Preemptible
}

// Pricing contains pricing information
type Pricing struct {
	EffectiveTime     string            `json:"effectiveTime"`
	Summary           string            `json:"summary"`
	PricingExpression PricingExpression `json:"pricingExpression"`
}

// PricingExpression contains the actual price
type PricingExpression struct {
	UsageUnit            string       `json:"usageUnit"`
	UsageUnitDescription string       `json:"usageUnitDescription"`
	DisplayQuantity      float64      `json:"displayQuantity"`
	TieredRates          []TieredRate `json:"tieredRates"`
}

// TieredRate contains the price for a tier
type TieredRate struct {
	StartUsageAmount float64   `json:"startUsageAmount"`
	UnitPrice        UnitPrice `json:"unitPrice"`
}

// UnitPrice represents a monetary amount
type UnitPrice struct {
	CurrencyCode string `json:"currencyCode"`
	Units        string `json:"units"`
	Nanos        int64  `json:"nanos"`
}

// GeoTaxonomy contains geographic info
type GeoTaxonomy struct {
	Type    string   `json:"type"` // GLOBAL, REGIONAL, MULTI_REGIONAL
	Regions []string `json:"regions"`
}

// NewBillingCatalogClient creates a new billing catalog client
func NewBillingCatalogClient() *BillingCatalogClient {
	return &BillingCatalogClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		cacheManager: provider.GetCacheManager(),
		skuCache:     make(map[string]*SKUPricing),
	}
}

// SetAPIKey sets the API key for the Catalog API (optional but recommended)
func (c *BillingCatalogClient) SetAPIKey(apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = apiKey
}

// GetSpotPricing retrieves Spot VM pricing for a region
func (c *BillingCatalogClient) GetSpotPricing(ctx context.Context, region string) (map[string]*SKUPricing, error) {
	// Check cache first
	cacheKey := cacheKeyBilling + region
	if cached, exists := c.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache HIT for GCP billing %s", region)
		return cached.(map[string]*SKUPricing), nil
	}
	logging.Debug("Cache MISS for GCP billing %s", region)

	// Fetch from API
	pricing, err := c.fetchSpotPricingFromAPI(ctx, region)
	if err != nil {
		return nil, err
	}

	// Cache results
	if len(pricing) > 0 {
		c.cacheManager.Set(cacheKey, pricing, BillingCacheTTL)
	}

	return pricing, nil
}

// fetchSpotPricingFromAPI fetches Spot/Preemptible pricing from the Catalog API
func (c *BillingCatalogClient) fetchSpotPricingFromAPI(ctx context.Context, region string) (map[string]*SKUPricing, error) {
	pricing := make(map[string]*SKUPricing)

	// Build URL for fetching Compute Engine SKUs
	url := fmt.Sprintf("%s/services/%s/skus", BillingCatalogAPIBase, ComputeEngineServiceID)
	if c.apiKey != "" {
		url += "?key=" + c.apiKey
	}

	pageToken := ""
	totalSKUs := 0
	spotSKUs := 0

	for {
		pageURL := url
		if pageToken != "" {
			if c.apiKey != "" {
				pageURL += "&pageToken=" + pageToken
			} else {
				pageURL += "?pageToken=" + pageToken
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch SKUs: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// If we get a 403, the API might require authentication
			if resp.StatusCode == http.StatusForbidden {
				logging.Warn("GCP Billing Catalog API returned 403 - falling back to estimated pricing")
				return nil, nil // Return nil to fall back to estimates
			}
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var catalogResp CatalogResponse
		if err := json.NewDecoder(resp.Body).Decode(&catalogResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		// Process SKUs
		for _, sku := range catalogResp.SKUs {
			totalSKUs++

			// Only process Preemptible (Spot) SKUs for Compute (CPU/RAM)
			if sku.Category.UsageType != "Preemptible" {
				continue
			}

			// Filter by resource family (Compute)
			if sku.Category.ResourceFamily != "Compute" {
				continue
			}

			// Check if this SKU applies to the target region
			appliesToRegion := false
			for _, r := range sku.ServiceRegions {
				if r == region || sku.GeoTaxonomy.Type == "GLOBAL" {
					appliesToRegion = true
					break
				}
			}
			if !appliesToRegion {
				continue
			}

			// Parse the pricing
			skuPricing := c.parseSKUPricing(sku, region)
			if skuPricing != nil {
				spotSKUs++
				key := c.getSKUCacheKey(skuPricing.MachineFamily, skuPricing.Region)
				pricing[key] = skuPricing
			}
		}

		// Check for more pages
		if catalogResp.NextPageToken == "" {
			break
		}
		pageToken = catalogResp.NextPageToken
	}

	logging.Info("Fetched %d Spot SKUs from %d total Compute Engine SKUs for region %s",
		spotSKUs, totalSKUs, region)

	return pricing, nil
}

// parseSKUPricing extracts pricing from a SKU
func (c *BillingCatalogClient) parseSKUPricing(sku SKU, region string) *SKUPricing {
	if len(sku.PricingInfo) == 0 {
		return nil
	}

	pricingInfo := sku.PricingInfo[0]
	if len(pricingInfo.PricingExpression.TieredRates) == 0 {
		return nil
	}

	// Get the first tier rate (usually the only one for compute)
	rate := pricingInfo.PricingExpression.TieredRates[0]

	// Calculate hourly price
	units := 0.0
	if rate.UnitPrice.Units != "" {
		fmt.Sscanf(rate.UnitPrice.Units, "%f", &units)
	}
	nanos := float64(rate.UnitPrice.Nanos) / 1e9
	hourlyPrice := units + nanos

	// Extract machine family from description
	family := c.extractFamilyFromDescription(sku.Description)

	// Determine if this is CPU or RAM pricing
	resourceGroup := strings.ToLower(sku.Category.ResourceGroup)

	pricing := &SKUPricing{
		SKUID:         sku.SKUID,
		Description:   sku.Description,
		MachineFamily: family,
		Region:        region,
		UsageType:     sku.Category.UsageType,
		Available:     true,
	}

	// Set CPU or memory price based on resource group
	if strings.Contains(resourceGroup, "cpu") || strings.Contains(resourceGroup, "core") {
		pricing.VCPUPriceHour = hourlyPrice
	} else if strings.Contains(resourceGroup, "ram") || strings.Contains(resourceGroup, "memory") {
		pricing.MemPriceGBHour = hourlyPrice
	}

	// Parse effective time
	if pricingInfo.EffectiveTime != "" {
		t, err := time.Parse(time.RFC3339, pricingInfo.EffectiveTime)
		if err == nil {
			pricing.EffectiveTime = t
		}
	}

	return pricing
}

// extractFamilyFromDescription extracts machine family from SKU description
func (c *BillingCatalogClient) extractFamilyFromDescription(desc string) string {
	desc = strings.ToLower(desc)

	// Common family patterns
	families := []string{"n2d", "n2", "n1", "n4", "e2", "c2d", "c2", "c3d", "c3", "c4",
		"m2", "m3", "t2d", "t2a", "a2", "a3", "g2", "h3", "z3"}

	for _, f := range families {
		if strings.Contains(desc, f+" ") || strings.Contains(desc, f+"-") {
			return f
		}
	}

	// Check for predefined types
	if strings.Contains(desc, "predefined") {
		return "predefined"
	}

	return "unknown"
}

// getSKUCacheKey generates a cache key for a SKU
func (c *BillingCatalogClient) getSKUCacheKey(family, region string) string {
	return family + ":" + region
}

// GetPriceForMachineType calculates the hourly spot price for a machine type
func (c *BillingCatalogClient) GetPriceForMachineType(ctx context.Context, machineType, region string, vcpus int, memoryGB float64) (float64, error) {
	pricing, err := c.GetSpotPricing(ctx, region)
	if err != nil {
		return 0, err
	}

	if pricing == nil || len(pricing) == 0 {
		return 0, nil // No real pricing available
	}

	family := extractFamily(machineType)
	key := c.getSKUCacheKey(family, region)

	skuPricing, exists := pricing[key]
	if !exists {
		// Try to find a similar family or use general pricing
		return 0, nil
	}

	// Calculate total price
	cpuCost := float64(vcpus) * skuPricing.VCPUPriceHour
	memCost := memoryGB * skuPricing.MemPriceGBHour

	return cpuCost + memCost, nil
}

// IsAvailable returns true if the API can be accessed
func (c *BillingCatalogClient) IsAvailable() bool {
	// Try a quick API check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/services/%s", BillingCatalogAPIBase, ComputeEngineServiceID)
	if c.apiKey != "" {
		url += "?key=" + c.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
