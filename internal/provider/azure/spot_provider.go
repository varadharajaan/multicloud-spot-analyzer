// Package azure implements Azure-specific spot instance data providers.
// It fetches real pricing data from Azure Retail Prices API.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

const (
	// AzureRetailPricesAPI is the public endpoint for Azure pricing data
	AzureRetailPricesAPI = "https://prices.azure.com/api/retail/prices"

	// CacheTTL is the cache duration in seconds (1 hour)
	CacheTTL = 3600

	// Cache key prefixes
	cacheKeySpotData = "azure:spot:"
	cacheKeyRawData  = "azure:raw_prices"
)

// RetailPricesResponse represents the response from Azure Retail Prices API
type RetailPricesResponse struct {
	BillingCurrency    string      `json:"BillingCurrency"`
	CustomerEntityID   string      `json:"CustomerEntityId"`
	CustomerEntityType string      `json:"CustomerEntityType"`
	Items              []PriceItem `json:"Items"`
	NextPageLink       string      `json:"NextPageLink"`
	Count              int         `json:"Count"`
}

// PriceItem represents a single pricing item from Azure
type PriceItem struct {
	CurrencyCode         string  `json:"currencyCode"`
	TierMinimumUnits     float64 `json:"tierMinimumUnits"`
	RetailPrice          float64 `json:"retailPrice"`
	UnitPrice            float64 `json:"unitPrice"`
	ArmRegionName        string  `json:"armRegionName"`
	Location             string  `json:"location"`
	EffectiveStartDate   string  `json:"effectiveStartDate"`
	MeterID              string  `json:"meterId"`
	MeterName            string  `json:"meterName"`
	ProductID            string  `json:"productId"`
	SkuID                string  `json:"skuId"`
	AvailabilityID       string  `json:"availabilityId,omitempty"`
	ProductName          string  `json:"productName"`
	SkuName              string  `json:"skuName"`
	ServiceName          string  `json:"serviceName"`
	ServiceID            string  `json:"serviceId"`
	ServiceFamily        string  `json:"serviceFamily"`
	UnitOfMeasure        string  `json:"unitOfMeasure"`
	Type                 string  `json:"type"`
	IsPrimaryMeterRegion bool    `json:"isPrimaryMeterRegion"`
	ArmSkuName           string  `json:"armSkuName"`
}

// SpotDataProvider implements domain.SpotDataProvider for Azure
type SpotDataProvider struct {
	httpClient   *http.Client
	cacheManager *provider.CacheManager
	mu           sync.RWMutex
	lastRefresh  time.Time
	rawData      map[string][]PriceItem // region -> prices
}

// NewSpotDataProvider creates a new Azure spot data provider
func NewSpotDataProvider() *SpotDataProvider {
	return &SpotDataProvider{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		cacheManager: provider.GetCacheManager(),
		rawData:      make(map[string][]PriceItem),
	}
}

// GetProviderName returns the cloud provider identifier
func (p *SpotDataProvider) GetProviderName() domain.CloudProvider {
	return domain.Azure
}

// FetchSpotData retrieves spot instance data for a specific region and OS
func (p *SpotDataProvider) FetchSpotData(ctx context.Context, region string, os domain.OperatingSystem) ([]domain.SpotData, error) {
	// Check global cache first
	cacheKey := fmt.Sprintf("%s%s_%s", cacheKeySpotData, region, os)
	if cached, exists := p.cacheManager.Get(cacheKey); exists {
		logging.Debug("Cache HIT for Azure %s", cacheKey)
		return cached.([]domain.SpotData), nil
	}
	logging.Debug("Cache MISS for Azure %s", cacheKey)

	// Fetch spot prices for the region
	spotPrices, onDemandPrices, err := p.fetchPricesForRegion(ctx, region, os)
	if err != nil {
		return nil, err
	}

	// Convert to domain.SpotData
	spotDataList := p.convertToSpotData(spotPrices, onDemandPrices, region, os)

	// Cache the result with 2-hour TTL
	if len(spotDataList) > 0 {
		p.cacheManager.Set(cacheKey, spotDataList, 2*time.Hour)
	}

	return spotDataList, nil
}

// fetchPricesForRegion fetches both spot and on-demand prices for a region
func (p *SpotDataProvider) fetchPricesForRegion(ctx context.Context, region string, os domain.OperatingSystem) ([]PriceItem, []PriceItem, error) {
	// Build filter for spot VMs
	osFilter := "Windows"
	if os == domain.Linux {
		osFilter = "Linux"
	}

	// Fetch spot prices
	spotFilter := fmt.Sprintf("armRegionName eq '%s' and serviceName eq 'Virtual Machines' and priceType eq 'Consumption' and contains(skuName, 'Spot')", region)
	if os == domain.Linux {
		spotFilter += " and contains(productName, 'Linux')"
	} else {
		spotFilter += " and contains(productName, 'Windows')"
	}

	spotPrices, err := p.fetchPricesWithFilter(ctx, spotFilter)
	if err != nil {
		logging.Warn("Failed to fetch Azure spot prices for %s: %v", region, err)
		return nil, nil, domain.NewSpotDataError(domain.Azure, region, "fetch_spot", err)
	}

	// Fetch on-demand prices for comparison
	onDemandFilter := fmt.Sprintf("armRegionName eq '%s' and serviceName eq 'Virtual Machines' and priceType eq 'Consumption' and not contains(skuName, 'Spot') and not contains(skuName, 'Low Priority')", region)
	if os == domain.Linux {
		onDemandFilter += " and contains(productName, 'Linux')"
	} else {
		onDemandFilter += " and contains(productName, 'Windows')"
	}

	onDemandPrices, err := p.fetchPricesWithFilter(ctx, onDemandFilter)
	if err != nil {
		logging.Warn("Failed to fetch Azure on-demand prices for %s: %v", region, err)
		// Continue without on-demand prices - we'll estimate savings
	}

	logging.Info("Fetched %d spot and %d on-demand prices for Azure %s (%s)", len(spotPrices), len(onDemandPrices), region, osFilter)

	return spotPrices, onDemandPrices, nil
}

// fetchPricesWithFilter fetches prices using OData filter
func (p *SpotDataProvider) fetchPricesWithFilter(ctx context.Context, filter string) ([]PriceItem, error) {
	var allItems []PriceItem
	nextURL := fmt.Sprintf("%s?$filter=%s", AzureRetailPricesAPI, url.QueryEscape(filter))

	for nextURL != "" {
		items, next, err := p.fetchPricesPage(ctx, nextURL)
		if err != nil {
			return allItems, err
		}
		allItems = append(allItems, items...)
		nextURL = next

		// Limit pages to prevent excessive API calls
		if len(allItems) >= 5000 {
			logging.Debug("Reached 5000 item limit for Azure prices")
			break
		}
	}

	return allItems, nil
}

// fetchPricesPage fetches a single page of prices
func (p *SpotDataProvider) fetchPricesPage(ctx context.Context, pageURL string) ([]PriceItem, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch Azure prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("Azure API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var response RetailPricesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Items, response.NextPageLink, nil
}

// convertToSpotData converts Azure price items to domain.SpotData
func (p *SpotDataProvider) convertToSpotData(spotPrices, onDemandPrices []PriceItem, region string, os domain.OperatingSystem) []domain.SpotData {
	// Build on-demand price lookup map
	onDemandMap := make(map[string]float64)
	for _, item := range onDemandPrices {
		vmSize := p.extractVMSize(item)
		if vmSize != "" && item.UnitPrice > 0 {
			// Keep the lowest on-demand price for each VM size
			if existing, exists := onDemandMap[vmSize]; !exists || item.UnitPrice < existing {
				onDemandMap[vmSize] = item.UnitPrice
			}
		}
	}

	// Convert spot prices to SpotData
	spotDataMap := make(map[string]domain.SpotData)
	for _, item := range spotPrices {
		vmSize := p.extractVMSize(item)
		if vmSize == "" || item.UnitPrice <= 0 {
			continue
		}

		// Calculate savings percentage
		savingsPercent := 0
		onDemandPrice := onDemandMap[vmSize]
		if onDemandPrice > 0 && item.UnitPrice < onDemandPrice {
			savingsPercent = int(((onDemandPrice - item.UnitPrice) / onDemandPrice) * 100)
		} else {
			// Estimate savings based on typical Azure spot discounts (60-90%)
			savingsPercent = 70 // Default estimate
		}

		// Estimate interruption frequency based on VM type and price
		interruptionFreq := p.estimateInterruptionFrequency(vmSize, savingsPercent)

		// Keep the best price for each VM size
		existing, exists := spotDataMap[vmSize]
		if !exists || item.UnitPrice < existing.SpotPrice {
			spotDataMap[vmSize] = domain.SpotData{
				InstanceType:          vmSize,
				Region:                region,
				OS:                    os,
				SavingsPercent:        savingsPercent,
				InterruptionFrequency: interruptionFreq,
				SpotPrice:             item.UnitPrice,
				OnDemandPrice:         onDemandPrice,
				CloudProvider:         domain.Azure,
				LastUpdated:           time.Now(),
			}
		}
	}

	// Convert map to slice
	result := make([]domain.SpotData, 0, len(spotDataMap))
	for _, spotData := range spotDataMap {
		result = append(result, spotData)
	}

	return result
}

// extractVMSize extracts the VM size from the price item
func (p *SpotDataProvider) extractVMSize(item PriceItem) string {
	// armSkuName is the most reliable field for VM size
	if item.ArmSkuName != "" {
		return item.ArmSkuName
	}

	// Try to extract from skuName (e.g., "D2s v3 Spot")
	skuName := item.SkuName
	skuName = strings.TrimSuffix(skuName, " Spot")
	skuName = strings.TrimSuffix(skuName, " Low Priority")
	skuName = strings.ReplaceAll(skuName, " ", "_")

	return skuName
}

// estimateInterruptionFrequency estimates interruption based on VM characteristics
func (p *SpotDataProvider) estimateInterruptionFrequency(vmSize string, savingsPercent int) domain.InterruptionFrequency {
	// Higher savings often correlates with higher interruption risk
	// This is a heuristic - Azure doesn't publish interruption rates like AWS

	vmLower := strings.ToLower(vmSize)

	// Burstable VMs (B-series) tend to have lower interruption
	if strings.HasPrefix(vmLower, "b") || strings.HasPrefix(vmLower, "standard_b") {
		return domain.VeryLow
	}

	// GPU VMs tend to have higher demand/interruption
	if strings.Contains(vmLower, "nc") || strings.Contains(vmLower, "nd") ||
		strings.Contains(vmLower, "nv") || strings.Contains(vmLower, "ng") {
		return domain.High
	}

	// High-memory VMs (M-series) are specialized with variable availability
	if strings.HasPrefix(vmLower, "m") || strings.HasPrefix(vmLower, "standard_m") {
		return domain.Medium
	}

	// Base interruption on savings percentage
	switch {
	case savingsPercent >= 85:
		return domain.High
	case savingsPercent >= 75:
		return domain.Medium
	case savingsPercent >= 60:
		return domain.Low
	default:
		return domain.VeryLow
	}
}

// GetSupportedRegions returns all Azure regions
func (p *SpotDataProvider) GetSupportedRegions(ctx context.Context) ([]string, error) {
	// Return commonly used Azure regions
	// In production, this could be fetched from Azure Resource Manager API
	regions := []string{
		"eastus",
		"eastus2",
		"westus",
		"westus2",
		"westus3",
		"centralus",
		"northcentralus",
		"southcentralus",
		"westcentralus",
		"canadacentral",
		"canadaeast",
		"brazilsouth",
		"northeurope",
		"westeurope",
		"uksouth",
		"ukwest",
		"francecentral",
		"germanywestcentral",
		"norwayeast",
		"switzerlandnorth",
		"swedencentral",
		"uaenorth",
		"southafricanorth",
		"australiaeast",
		"australiasoutheast",
		"australiacentral",
		"eastasia",
		"southeastasia",
		"japaneast",
		"japanwest",
		"koreacentral",
		"koreasouth",
		"centralindia",
		"southindia",
		"westindia",
	}
	return regions, nil
}

// RefreshData forces a refresh of the cached data
func (p *SpotDataProvider) RefreshData(ctx context.Context) error {
	// Clear Azure-specific cache entries
	p.cacheManager.DeletePrefix("azure:")

	p.mu.Lock()
	p.rawData = make(map[string][]PriceItem)
	p.mu.Unlock()

	logging.Info("Azure cache refreshed")
	return nil
}

// init registers the Azure spot data provider with the factory
func init() {
	provider.RegisterSpotProviderCreator(domain.Azure, func() (domain.SpotDataProvider, error) {
		return NewSpotDataProvider(), nil
	})
}
