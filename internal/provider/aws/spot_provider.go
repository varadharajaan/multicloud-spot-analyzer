// Package aws implements AWS-specific spot instance data providers.
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
)

const (
	// SpotAdvisorDataURL is the public endpoint for AWS Spot Advisor data
	SpotAdvisorDataURL = "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json"

	// CacheTTL is the cache duration in seconds (1 hour)
	CacheTTL = 3600

	// CacheKey for spot advisor data
	spotAdvisorCacheKey = "aws_spot_advisor_data"
)

// SpotAdvisorResponse represents the response from AWS Spot Advisor API
type SpotAdvisorResponse struct {
	Ranges      []InterruptionRange   `json:"ranges"`
	SpotAdvisor map[string]RegionData `json:"spot_advisor"`
}

// InterruptionRange defines the interruption frequency ranges
type InterruptionRange struct {
	Index int    `json:"index"`
	Label string `json:"label"`
	Dots  int    `json:"dots"`
	Max   int    `json:"max"`
}

// RegionData contains spot data organized by OS
type RegionData map[string]OSData

// OSData contains instance type data
// Key is instance type, value is spot data
type OSData map[string]InstanceSpotData

// InstanceSpotData contains savings and interruption rating
type InstanceSpotData struct {
	Savings      int `json:"s"` // Savings percentage
	Interruption int `json:"r"` // Interruption frequency index (0-4)
}

// SpotDataProvider implements domain.SpotDataProvider for AWS
type SpotDataProvider struct {
	httpClient  *http.Client
	cache       *provider.InMemoryCache
	mu          sync.RWMutex
	lastRefresh time.Time
	rawData     *SpotAdvisorResponse
}

// NewSpotDataProvider creates a new AWS spot data provider
func NewSpotDataProvider() *SpotDataProvider {
	return &SpotDataProvider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: provider.NewInMemoryCache(),
	}
}

// GetProviderName returns the cloud provider identifier
func (p *SpotDataProvider) GetProviderName() domain.CloudProvider {
	return domain.AWS
}

// FetchSpotData retrieves spot instance data for a specific region and OS
func (p *SpotDataProvider) FetchSpotData(ctx context.Context, region string, os domain.OperatingSystem) ([]domain.SpotData, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s_%s_%s", spotAdvisorCacheKey, region, os)
	if cached, exists := p.cache.Get(cacheKey); exists {
		return cached.([]domain.SpotData), nil
	}

	// Ensure we have the raw data
	if err := p.ensureDataLoaded(ctx); err != nil {
		return nil, err
	}

	// Parse data for the specific region and OS
	spotDataList, err := p.parseSpotData(region, os)
	if err != nil {
		return nil, err
	}

	// Cache the result
	p.cache.Set(cacheKey, spotDataList, CacheTTL)

	return spotDataList, nil
}

// ensureDataLoaded loads the spot advisor data if not already loaded
func (p *SpotDataProvider) ensureDataLoaded(ctx context.Context) error {
	p.mu.RLock()
	if p.rawData != nil && time.Since(p.lastRefresh) < time.Duration(CacheTTL)*time.Second {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	return p.fetchRawData(ctx)
}

// fetchRawData fetches the raw spot advisor data from AWS
func (p *SpotDataProvider) fetchRawData(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if p.rawData != nil && time.Since(p.lastRefresh) < time.Duration(CacheTTL)*time.Second {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SpotAdvisorDataURL, nil)
	if err != nil {
		logging.Error("Failed to create request: %v", err)
		return domain.NewSpotDataError(domain.AWS, "", "create_request", err)
	}

	logging.Debug("Fetching spot advisor data from %s", SpotAdvisorDataURL)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		logging.Error("Failed to fetch spot data: %v", err)
		return domain.NewSpotDataError(domain.AWS, "", "fetch", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.Error("Unexpected status code: %d", resp.StatusCode)
		return domain.NewSpotDataError(domain.AWS, "", "fetch",
			fmt.Errorf("unexpected status code: %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewSpotDataError(domain.AWS, "", "read_body", err)
	}

	var data SpotAdvisorResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return domain.NewSpotDataError(domain.AWS, "", "parse", err)
	}

	p.rawData = &data
	p.lastRefresh = time.Now()

	logging.Info("Loaded spot advisor data: %d regions", len(p.rawData.SpotAdvisor))

	return nil
}

// parseSpotData converts raw spot advisor data to domain models
func (p *SpotDataProvider) parseSpotData(region string, os domain.OperatingSystem) ([]domain.SpotData, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.rawData == nil {
		return nil, domain.NewSpotDataError(domain.AWS, region, "parse", domain.ErrNotFound)
	}

	regionData, exists := p.rawData.SpotAdvisor[region]
	if !exists {
		return nil, domain.NewSpotDataError(domain.AWS, region, "parse", domain.ErrUnsupportedRegion)
	}

	osKey := string(os)
	osData, exists := regionData[osKey]
	if !exists {
		return nil, domain.NewSpotDataError(domain.AWS, region, "parse",
			fmt.Errorf("OS %s not found for region %s", os, region))
	}

	result := make([]domain.SpotData, 0, len(osData))
	for instanceType, spotData := range osData {
		result = append(result, domain.SpotData{
			InstanceType:          instanceType,
			Region:                region,
			OS:                    os,
			SavingsPercent:        spotData.Savings,
			InterruptionFrequency: domain.InterruptionFrequency(spotData.Interruption),
			CloudProvider:         domain.AWS,
			LastUpdated:           p.lastRefresh,
		})
	}

	return result, nil
}

// GetSupportedRegions returns all regions supported by AWS Spot Advisor
func (p *SpotDataProvider) GetSupportedRegions(ctx context.Context) ([]string, error) {
	if err := p.ensureDataLoaded(ctx); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	regions := make([]string, 0, len(p.rawData.SpotAdvisor))
	for region := range p.rawData.SpotAdvisor {
		regions = append(regions, region)
	}

	return regions, nil
}

// RefreshData forces a refresh of the cached data
func (p *SpotDataProvider) RefreshData(ctx context.Context) error {
	p.cache.Clear()
	p.mu.Lock()
	p.rawData = nil
	p.mu.Unlock()

	return p.fetchRawData(ctx)
}

// GetAllSpotData returns spot data for all regions and OS types
func (p *SpotDataProvider) GetAllSpotData(ctx context.Context) (*SpotAdvisorResponse, error) {
	if err := p.ensureDataLoaded(ctx); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.rawData, nil
}

// init registers the AWS spot data provider with the factory
func init() {
	provider.RegisterSpotProviderCreator(domain.AWS, func() (domain.SpotDataProvider, error) {
		return NewSpotDataProvider(), nil
	})
}
