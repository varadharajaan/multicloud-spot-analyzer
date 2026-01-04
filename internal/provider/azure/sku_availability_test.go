// Package azure tests for SKU availability provider
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spot-analyzer/internal/config"
)

func init() {
	// Change to project root for config loading
	os.Chdir("../../..")
}

// TestSKUAPIConnection tests that we can connect to Azure SKU API
func TestSKUAPIConnection(t *testing.T) {
	// Skip if no Azure credentials configured
	if os.Getenv("AZURE_TENANT_ID") == "" && os.Getenv("SKIP_AZURE_TESTS") != "" {
		t.Skip("Skipping Azure SKU API test - no credentials configured")
	}

	// Load config via Get()
	cfg := config.Get()

	if cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "" || cfg.Azure.ClientSecret == "" {
		t.Skip("Skipping Azure SKU API test - no Azure credentials in config")
	}

	provider := NewSKUAvailabilityProvider()
	if !provider.IsAvailable() {
		t.Fatal("SKU provider reports not available despite credentials being set")
	}

	t.Log("Azure credentials loaded successfully")
	t.Logf("TenantID set: %v", cfg.Azure.TenantID != "")
	t.Logf("ClientID set: %v", cfg.Azure.ClientID != "")
	t.Logf("SubscriptionID set: %v", cfg.Azure.SubscriptionID != "")
}

// TestSKUAPIFetchAllSKUs tests fetching all SKUs for a region
func TestSKUAPIFetchAllSKUs(t *testing.T) {
	cfg := config.Get()

	if cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "" || cfg.Azure.ClientSecret == "" {
		t.Skip("Skipping Azure SKU API test - no Azure credentials in config")
	}

	provider := NewSKUAvailabilityProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Test fetching all SKUs
	skuMap, err := provider.fetchAllSKUs(ctx, "eastus")
	if err != nil {
		t.Fatalf("fetchAllSKUs failed: %v", err)
	}

	t.Logf("Fetched %d VM SKUs globally", len(skuMap))

	if len(skuMap) == 0 {
		t.Fatal("No SKUs returned from API")
	}

	// Print first 20 SKU names to understand the format (now region:vmname)
	count := 0
	t.Log("First 20 SKU keys in cache:")
	for key := range skuMap {
		if count < 20 {
			t.Logf("  %d: %s", count+1, key)
			count++
		}
	}

	// Check for eastus VMs (using new key format region:vmname)
	commonVMs := []string{
		"eastus:d2s_v3", "eastus:d4s_v3", "eastus:d2_v3", "eastus:d4_v3",
		"eastus:ds2_v2", "eastus:ds3_v2",
		"eastus:b2s", "eastus:b2ms",
		"eastus:e2s_v3", "eastus:e4s_v3",
	}

	t.Log("\nChecking eastus VM types:")
	for _, key := range commonVMs {
		if _, found := skuMap[key]; found {
			t.Logf("  ✓ %s found", key)
		} else {
			t.Logf("  ✗ %s NOT found", key)
		}
	}

	// Count eastus VMs
	t.Log("\nEastus VM count:")
	eastusCount := 0
	for key := range skuMap {
		if strings.HasPrefix(key, "eastus:") {
			eastusCount++
		}
	}
	t.Logf("  Total eastus VMs: %d", eastusCount)
}

// TestSKUAPINormalization tests VM name normalization
func TestSKUAPINormalization(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Standard_D2s_v3", "d2s_v3"},
		{"Standard_D2nls_v6", "d2nls_v6"},
		{"Standard_E2nds_v6", "e2nds_v6"},
		{"Standard_DS2_v2", "ds2_v2"},
		{"D2s_v3", "d2s_v3"},
		{"d2s_v3", "d2s_v3"},
	}

	for _, tc := range testCases {
		normalized := strings.ToLower(tc.input)
		normalized = strings.TrimPrefix(normalized, "standard_")
		if normalized != tc.expected {
			t.Errorf("Normalization failed: %s -> %s (expected %s)", tc.input, normalized, tc.expected)
		} else {
			t.Logf("✓ %s -> %s", tc.input, normalized)
		}
	}
}

// TestSKUAPIRawZoneData tests getting raw zone info from SKU API
func TestSKUAPIRawZoneData(t *testing.T) {
	cfg := config.Get()

	if cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "" || cfg.Azure.ClientSecret == "" {
		t.Skip("Skipping Azure SKU API test - no Azure credentials in config")
	}

	provider := NewSKUAvailabilityProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get access token directly
	token, err := provider.getAccessToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get token: %v", err)
	}

	// Fetch SKUs WITHOUT location filter to see full response
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/skus?api-version=2021-07-01",
		cfg.Azure.SubscriptionID,
	)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := provider.httpClient.Do(req)
	if err != nil {
		t.Fatalf("API call failed: %v", err)
	}
	defer resp.Body.Close()

	var apiResp SKUsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	t.Logf("Total SKUs (no filter): %d", len(apiResp.Value))

	// Count how many times D2s_v3 appears
	d2sCount := 0
	for _, sku := range apiResp.Value {
		if strings.EqualFold(sku.Name, "Standard_D2s_v3") {
			d2sCount++
			if d2sCount <= 5 {
				t.Logf("  D2s_v3 entry %d: Locations=%v, LocationInfo[0].Location=%s",
					d2sCount, sku.Locations, sku.LocationInfo[0].Location)
			}
		}
	}
	t.Logf("Total D2s_v3 entries: %d", d2sCount)

	// Find the D2s_v3 entry for eastus specifically
	t.Logf("\n=== Looking for D2s_v3 in eastus ===")
	for _, sku := range apiResp.Value {
		if strings.EqualFold(sku.Name, "Standard_D2s_v3") {
			if len(sku.Locations) > 0 && strings.EqualFold(sku.Locations[0], "eastus") {
				t.Logf("Found! Locations=%v, zones=%v", sku.Locations, sku.LocationInfo[0].Zones)
			}
		}
	}
}

// TestSKUAPIGetZoneAvailability tests getting zone availability for a specific VM
func TestSKUAPIGetZoneAvailability(t *testing.T) {
	cfg := config.Get()

	if cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "" || cfg.Azure.ClientSecret == "" {
		t.Skip("Skipping Azure SKU API test - no Azure credentials in config")
	}

	provider := NewSKUAvailabilityProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Test with a VM that should exist and support zones (v5 series)
	testVMs := []string{
		"Standard_D2s_v5",
		"Standard_D4s_v5",
		"Standard_E2s_v5",
		"Standard_D2ds_v5",
		"Standard_D2s_v3", // older - may not have zones
		"Standard_DS2_v2", // older - may not have zones
	}

	for _, vmSize := range testVMs {
		zones, err := provider.GetZoneAvailability(ctx, vmSize, "eastus")
		if err != nil {
			t.Logf("✗ %s: %v", vmSize, err)
		} else if len(zones) == 0 {
			t.Logf("○ %s: No zones returned (VM may not support zones)", vmSize)
		} else {
			zoneNames := make([]string, len(zones))
			for i, z := range zones {
				zoneNames[i] = z.Zone
			}
			t.Logf("✓ %s: %v", vmSize, zoneNames)
		}
	}
}

// TestSKUAPITokenAcquisition tests Azure AD token acquisition
func TestSKUAPITokenAcquisition(t *testing.T) {
	cfg := config.Get()

	if cfg.Azure.TenantID == "" || cfg.Azure.ClientID == "" || cfg.Azure.ClientSecret == "" {
		t.Skip("Skipping Azure SKU API test - no Azure credentials in config")
	}

	provider := NewSKUAvailabilityProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := provider.getAccessToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get access token: %v", err)
	}

	if token == "" {
		t.Fatal("Token is empty")
	}

	t.Logf("Access token obtained (length: %d)", len(token))
	t.Logf("Token prefix: %s...", token[:20])
}
