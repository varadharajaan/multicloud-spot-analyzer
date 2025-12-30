package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/spot-analyzer/internal/domain"
)

const testSpotAdvisorURL = "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json"

// TestRealDataValidation proves that the spot advisor data is fetched from real AWS API
// and not hardcoded. It compares our provider's data against a direct HTTP call.
func TestRealDataValidation(t *testing.T) {
	// Skip if running in CI without network access
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Fetch data directly from AWS S3 (ground truth)
	directData, err := fetchDirectFromAWS()
	if err != nil {
		t.Fatalf("Failed to fetch directly from AWS: %v", err)
	}

	// Step 2: Fetch data through our provider
	provider := NewSpotDataProvider()
	providerData, err := provider.FetchSpotData(ctx, "us-east-1", domain.Linux)
	if err != nil {
		t.Fatalf("Provider failed to fetch data: %v", err)
	}

	// Convert provider data to map for easier comparison
	providerMap := make(map[string]*domain.SpotData)
	for i := range providerData {
		providerMap[providerData[i].InstanceType] = &providerData[i]
	}

	// Step 3: Compare specific instance data points
	testInstances := []string{"m5.large", "c5.xlarge", "r5.2xlarge", "t3.medium", "i3.large"}
	matchCount := 0

	for _, instanceType := range testInstances {
		directSpot, directOk := directData[instanceType]
		providerSpot, providerOk := providerMap[instanceType]

		if directOk != providerOk {
			t.Errorf("Instance %s: existence mismatch (direct=%v, provider=%v)",
				instanceType, directOk, providerOk)
			continue
		}

		if !directOk {
			continue // Instance not found in either
		}

		// Compare savings percentage
		if directSpot.SavingsPercent != providerSpot.SavingsPercent {
			t.Errorf("Instance %s: SavingsPercent mismatch (direct=%d, provider=%d)",
				instanceType, directSpot.SavingsPercent, providerSpot.SavingsPercent)
		} else {
			matchCount++
		}

		// Compare interruption frequency
		if int(directSpot.InterruptionFrequency) != int(providerSpot.InterruptionFrequency) {
			t.Errorf("Instance %s: InterruptionFrequency mismatch (direct=%d, provider=%d)",
				instanceType, directSpot.InterruptionFrequency, providerSpot.InterruptionFrequency)
		}
	}

	t.Logf("✅ Validated %d/%d instance types match between direct API call and provider", matchCount, len(testInstances))
}

// TestDataNotHardcoded proves data varies by region (not hardcoded)
func TestDataNotHardcoded(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := NewSpotDataProvider()

	// Fetch data from multiple regions
	regions := []string{"us-east-1", "us-west-2", "eu-west-1"}
	regionData := make(map[string][]domain.SpotData)

	for _, region := range regions {
		data, err := provider.FetchSpotData(ctx, region, domain.Linux)
		if err != nil {
			t.Errorf("Failed to fetch data for %s: %v", region, err)
			continue
		}
		regionData[region] = data
		t.Logf("Region %s: fetched %d instance types", region, len(data))
	}

	// Verify regions have different data (proves not hardcoded per-region)
	if len(regionData) >= 2 {
		r1Data := regionData["us-east-1"]
		r2Data := regionData["us-west-2"]

		// Convert to maps
		r1Map := make(map[string]*domain.SpotData)
		for i := range r1Data {
			r1Map[r1Data[i].InstanceType] = &r1Data[i]
		}
		r2Map := make(map[string]*domain.SpotData)
		for i := range r2Data {
			r2Map[r2Data[i].InstanceType] = &r2Data[i]
		}

		differences := 0
		for instType, r1Spot := range r1Map {
			if r2Spot, ok := r2Map[instType]; ok {
				if r1Spot.SavingsPercent != r2Spot.SavingsPercent ||
					r1Spot.InterruptionFrequency != r2Spot.InterruptionFrequency {
					differences++
				}
			}
		}

		if differences == 0 {
			t.Error("All regions have identical data - possible hardcoding!")
		} else {
			t.Logf("✅ Found %d differences between regions - data is region-specific", differences)
		}
	}
}

// TestAPIEndpointIsReal verifies the API endpoint is the real AWS S3 bucket
func TestAPIEndpointIsReal(t *testing.T) {
	expectedURL := "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json"

	// Verify the URL is accessible
	resp, err := http.Head(expectedURL)
	if err != nil {
		t.Fatalf("Failed to reach AWS API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("AWS API returned status %d", resp.StatusCode)
	}

	t.Logf("✅ AWS Spot Advisor API is accessible at %s", expectedURL)
}

// TestInstanceCountReasonable verifies we get a reasonable number of instances
func TestInstanceCountReasonable(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := NewSpotDataProvider()
	data, err := provider.FetchSpotData(ctx, "us-east-1", domain.Linux)
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}

	// AWS has 600+ instance types, us-east-1 should have most of them
	minExpected := 500
	maxExpected := 2000

	count := len(data)
	if count < minExpected {
		t.Errorf("Too few instances: got %d, expected at least %d", count, minExpected)
	}
	if count > maxExpected {
		t.Errorf("Unexpectedly many instances: got %d, expected at most %d", count, maxExpected)
	}

	t.Logf("✅ Fetched %d instance types (reasonable range: %d-%d)", count, minExpected, maxExpected)
}

// TestSavingsRangeValid verifies savings percentages are in valid range
func TestSavingsRangeValid(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := NewSpotDataProvider()
	data, err := provider.FetchSpotData(ctx, "us-east-1", domain.Linux)
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}

	invalidCount := 0
	for _, spot := range data {
		// Savings should be 0-100%
		if spot.SavingsPercent < 0 || spot.SavingsPercent > 100 {
			t.Errorf("Invalid savings for %s: %d%%", spot.InstanceType, spot.SavingsPercent)
			invalidCount++
		}
		// Interruption should be 0-4
		if spot.InterruptionFrequency < 0 || spot.InterruptionFrequency > 4 {
			t.Errorf("Invalid interruption for %s: %d", spot.InstanceType, spot.InterruptionFrequency)
			invalidCount++
		}
	}

	if invalidCount == 0 {
		t.Logf("✅ All %d instances have valid data ranges", len(data))
	}
}

// TestPriceHistoryRealData tests that DescribeSpotPriceHistory returns real data
func TestPriceHistoryRealData(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		t.Skip("Skipping network test")
	}

	// This test requires AWS credentials
	provider, err := NewPriceHistoryProvider("us-east-1")
	if err != nil {
		t.Skipf("Skipping price history test: %v", err)
	}

	if !provider.IsAvailable() {
		t.Skip("AWS credentials not available - skipping price history test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test with a common instance type
	analysis, err := provider.GetPriceAnalysis(ctx, "m5.large", 3)
	if err != nil {
		t.Fatalf("Failed to get price analysis: %v", err)
	}

	if analysis == nil {
		t.Fatal("Price analysis returned nil")
	}

	// Verify we got real data
	if analysis.DataPoints < 10 {
		t.Errorf("Too few data points: %d (expected at least 10)", analysis.DataPoints)
	}

	if analysis.CurrentPrice <= 0 {
		t.Error("Current price is zero or negative")
	}

	if analysis.AvgPrice <= 0 {
		t.Error("Average price is zero or negative")
	}

	// Verify price is reasonable for m5.large (usually $0.01-0.10/hr for spot)
	if analysis.CurrentPrice > 0.5 {
		t.Errorf("Price seems too high for m5.large spot: $%.4f", analysis.CurrentPrice)
	}

	t.Logf("✅ Price history validated: m5.large current=$%.4f avg=$%.4f points=%d",
		analysis.CurrentPrice, analysis.AvgPrice, analysis.DataPoints)
}

// SpotInstanceData mirrors the domain type for testing
type SpotInstanceData struct {
	SavingsPercent        int
	InterruptionFrequency int
}

// fetchDirectFromAWS fetches data directly from AWS S3 for comparison
func fetchDirectFromAWS() (map[string]*SpotInstanceData, error) {
	resp, err := http.Get(testSpotAdvisorURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawData struct {
		SpotAdvisor map[string]map[string]map[string]struct {
			S int `json:"s"` // Savings percentage
			R int `json:"r"` // Interruption rating
		} `json:"spot_advisor"`
	}

	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract us-east-1 Linux data
	regionData, ok := rawData.SpotAdvisor["us-east-1"]
	if !ok {
		return nil, fmt.Errorf("us-east-1 not found in data")
	}

	osData, ok := regionData["Linux"]
	if !ok {
		return nil, fmt.Errorf("Linux data not found for us-east-1")
	}

	result := make(map[string]*SpotInstanceData)
	for instanceType, data := range osData {
		result[instanceType] = &SpotInstanceData{
			SavingsPercent:        data.S,
			InterruptionFrequency: data.R,
		}
	}

	return result, nil
}

// BenchmarkSpotDataFetch benchmarks the spot data fetching
func BenchmarkSpotDataFetch(b *testing.B) {
	if os.Getenv("SKIP_NETWORK_TESTS") == "true" {
		b.Skip("Skipping network test")
	}

	ctx := context.Background()
	provider := NewSpotDataProvider()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := provider.FetchSpotData(ctx, "us-east-1", domain.Linux)
		if err != nil {
			b.Fatal(err)
		}
	}
}
