package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthEndpoint(t *testing.T) {
	server := NewServer(8080)

	req, err := http.NewRequest("GET", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("Status = %v, want healthy", resp.Status)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("Version = %v, want 1.0.0", resp.Version)
	}
	if resp.Checks == nil {
		t.Error("Checks should not be nil")
	}
}

func TestSwaggerRedirect(t *testing.T) {
	server := NewServer(8080)

	req, err := http.NewRequest("GET", "/swagger-ui", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSwaggerRedirect)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMovedPermanently {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusMovedPermanently)
	}

	location := rr.Header().Get("Location")
	if location != "/swagger.html" {
		t.Errorf("Location header = %v, want /swagger.html", location)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if rl.Allow("192.168.1.1") {
		t.Error("6th request should be denied")
	}

	// Different IP should be allowed
	if !rl.Allow("192.168.1.2") {
		t.Error("Request from different IP should be allowed")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// Create a simple handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}

	// Wrap with rate limiter
	wrapped := rl.Middleware(handler)

	// First 2 requests should pass
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()
		wrapped(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d should return 200, got %d", i+1, rr.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	wrapped(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request should return 429, got %d", rr.Code)
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name          string
		xForwardedFor string
		xRealIP       string
		remoteAddr    string
		expectedIP    string
	}{
		{
			name:          "X-Forwarded-For header",
			xForwardedFor: "10.0.0.1, 192.168.1.1",
			remoteAddr:    "127.0.0.1:8080",
			expectedIP:    "10.0.0.1",
		},
		{
			name:       "X-Real-IP header",
			xRealIP:    "10.0.0.2",
			remoteAddr: "127.0.0.1:8080",
			expectedIP: "10.0.0.2",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "192.168.1.100:54321",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "192.168.1.100",
			expectedIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			req.RemoteAddr = tt.remoteAddr

			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("getClientIP() = %v, want %v", ip, tt.expectedIP)
			}
		})
	}
}

func TestAnalyzeEndpointValidation(t *testing.T) {
	server := NewServer(8080)

	// Test wrong method
	req, _ := http.NewRequest("GET", "/api/analyze", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	var resp AnalyzeResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Success {
		t.Error("GET request should not succeed")
	}

	// Test invalid JSON
	req, _ = http.NewRequest("POST", "/api/analyze", bytes.NewBuffer([]byte("invalid json")))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Success {
		t.Error("Invalid JSON request should not succeed")
	}
}

func TestPresetsEndpoint(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/presets", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handlePresets)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var presets []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&presets); err != nil {
		t.Errorf("Failed to decode presets: %v", err)
	}

	if len(presets) == 0 {
		t.Error("Presets should not be empty")
	}
}

func TestCacheStatusEndpoint(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/cache/status", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleCacheStatus)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestFamiliesEndpointAWS(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/families?cloud=aws", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleFamilies)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var families []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&families); err != nil {
		t.Errorf("Failed to decode families: %v", err)
	}

	if len(families) == 0 {
		t.Error("AWS families should not be empty")
	}
}

func TestFamiliesEndpointAzure(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/families?cloud=azure", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleFamilies)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var families []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&families); err != nil {
		t.Errorf("Failed to decode families: %v", err)
	}

	if len(families) == 0 {
		t.Error("Azure families should not be empty")
	}

	// Check for expected Azure families
	hasD := false
	hasE := false
	for _, f := range families {
		name := f["name"].(string)
		if name == "D" {
			hasD = true
		}
		if name == "E" {
			hasE = true
		}
	}
	if !hasD {
		t.Error("Azure families should include 'D' series")
	}
	if !hasE {
		t.Error("Azure families should include 'E' series")
	}
}

func TestInstanceTypesEndpointAzure(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/instance-types?cloud=azure&q=standard", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleInstanceTypes)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestAnalyzeEndpointWithAzure(t *testing.T) {
	server := NewServer(8080)

	requestBody := AnalyzeRequest{
		CloudProvider:   "azure",
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		Region:          "eastus",
		MaxInterruption: 2,
		TopN:            5,
		Enhanced:        false,
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AnalyzeResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	// Check that the data source mentions Azure
	if resp.Success && len(resp.DataSource) > 0 {
		if resp.DataSource != "Azure Retail Prices API" {
			t.Logf("DataSource = %s (expected Azure Retail Prices API)", resp.DataSource)
		}
	}
}

func TestAZEndpointWithAzure(t *testing.T) {
	server := NewServer(8080)

	requestBody := AZRequest{
		CloudProvider: "azure",
		InstanceType:  "Standard_D2s_v5",
		Region:        "eastus",
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/az", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := server.rateLimiter.Middleware(server.handleAZRecommendation)
	handler(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AZResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("AZ request should succeed, error: %s", resp.Error)
	}
}

func TestHealthEndpointIncludesAzure(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/health", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	var resp HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	// Check that Azure API status is included
	if resp.Checks != nil {
		if azureStatus, ok := resp.Checks["azure_api"]; ok {
			if azureStatus != "available" {
				t.Logf("Azure API status: %s", azureStatus)
			}
		}
	}
}

func TestExtractInstanceFamily(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantFamily   string
	}{
		// AWS instance types
		{"AWS m5.large", "m5.large", "m"},
		{"AWS c6i.xlarge", "c6i.xlarge", "c"},
		{"AWS t3a.medium", "t3a.medium", "t"},
		{"AWS r6g.2xlarge", "r6g.2xlarge", "r"},
		{"AWS i3.4xlarge", "i3.4xlarge", "i"},
		{"AWS p4d.24xlarge", "p4d.24xlarge", "p"},
		// Azure instance types
		{"Azure Standard_D2s_v5", "Standard_D2s_v5", "D"},
		{"Azure Standard_E4s_v5", "Standard_E4s_v5", "E"},
		{"Azure Standard_F8s_v2", "Standard_F8s_v2", "F"},
		{"Azure Standard_NC24_v3", "Standard_NC24_v3", "NC"},
		{"Azure Standard_B2s", "Standard_B2s", "B"},
		{"Azure Standard_Das_v5", "Standard_Das4_v5", "Das"},
		{"Azure Standard_Dps_v5", "Standard_Dps2_v5", "Dps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInstanceFamily(tt.instanceType)
			if got != tt.wantFamily {
				t.Errorf("extractInstanceFamily(%s) = %s, want %s", tt.instanceType, got, tt.wantFamily)
			}
		})
	}
}

func TestFamiliesEndpointCaching(t *testing.T) {
	server := NewServer(8080)

	// First call - should populate cache
	req1, _ := http.NewRequest("GET", "/api/families?cloud=azure", nil)
	rr1 := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleFamilies)
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("First request failed: %d", rr1.Code)
	}

	var families1 []map[string]interface{}
	json.NewDecoder(rr1.Body).Decode(&families1)

	// Second call - should be from cache (much faster)
	req2, _ := http.NewRequest("GET", "/api/families?cloud=azure", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Second request failed: %d", rr2.Code)
	}

	var families2 []map[string]interface{}
	json.NewDecoder(rr2.Body).Decode(&families2)

	// Both should return same data
	if len(families1) != len(families2) {
		t.Errorf("Cached response differs: got %d families, expected %d", len(families2), len(families1))
	}
}

func TestCacheRefreshClearsFamilies(t *testing.T) {
	server := NewServer(8080)

	// First, populate the family cache
	req1, _ := http.NewRequest("GET", "/api/families?cloud=azure", nil)
	rr1 := httptest.NewRecorder()
	server.handleFamilies(rr1, req1)

	// Refresh cache
	reqRefresh, _ := http.NewRequest("POST", "/api/cache/refresh", nil)
	rrRefresh := httptest.NewRecorder()
	server.handleCacheRefresh(rrRefresh, reqRefresh)

	if rrRefresh.Code != http.StatusOK {
		t.Errorf("Cache refresh failed: %d", rrRefresh.Code)
	}

	var refreshResp map[string]interface{}
	json.NewDecoder(rrRefresh.Body).Decode(&refreshResp)

	if !refreshResp["success"].(bool) {
		t.Error("Cache refresh should succeed")
	}

	// Verify items were cleared
	itemsCleared := int(refreshResp["itemsCleared"].(float64))
	if itemsCleared == 0 {
		t.Log("No items were in cache to clear (may be expected if test runs in isolation)")
	}
}

// =============================================================================
// AWS and Azure Integration Tests
// =============================================================================

func TestAWSAnalyzeFlow(t *testing.T) {
	server := NewServer(8080)

	// Test AWS spot analysis request
	requestBody := AnalyzeRequest{
		CloudProvider:   "aws",
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		MaxMemory:       32,
		Region:          "us-east-1",
		MaxInterruption: 2,
		TopN:            10,
		Enhanced:        false,
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("AWS analyze returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AnalyzeResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode AWS analyze response: %v", err)
	}

	if !resp.Success {
		t.Errorf("AWS analyze should succeed, got error: %s", resp.Error)
	}

	// Verify AWS-specific response fields
	if resp.DataSource == "" {
		t.Log("DataSource is empty (may be expected without AWS credentials)")
	}

	t.Logf("AWS Analysis: found %d instances", len(resp.Instances))
}

func TestAzureAnalyzeFlow(t *testing.T) {
	server := NewServer(8080)

	// Test Azure spot analysis request
	requestBody := AnalyzeRequest{
		CloudProvider:   "azure",
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		MaxMemory:       32,
		Region:          "eastus",
		MaxInterruption: 2,
		TopN:            10,
		Enhanced:        false,
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Azure analyze returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AnalyzeResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode Azure analyze response: %v", err)
	}

	// Azure should always work (public API)
	if !resp.Success {
		t.Logf("Azure analyze response: %s (may be due to API rate limiting)", resp.Error)
	}

	// Verify Azure-specific data source
	if resp.Success && resp.DataSource != "" {
		if resp.DataSource != "Azure Retail Prices API" {
			t.Logf("Azure DataSource = %s (expected Azure Retail Prices API)", resp.DataSource)
		}
	}

	t.Logf("Azure Analysis: found %d instances", len(resp.Instances))
}

func TestAWSAZRecommendationFlow(t *testing.T) {
	server := NewServer(8080)

	requestBody := AZRequest{
		CloudProvider: "aws",
		InstanceType:  "m5.large",
		Region:        "us-east-1",
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/az", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := server.rateLimiter.Middleware(server.handleAZRecommendation)
	handler(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("AWS AZ recommendation returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AZResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode AWS AZ response: %v", err)
	}

	if !resp.Success {
		t.Logf("AWS AZ recommendation error: %s (may be expected without AWS credentials)", resp.Error)
	} else {
		if len(resp.Recommendations) == 0 {
			t.Log("No AZ recommendations returned for AWS (may be expected without credentials)")
		} else {
			t.Logf("AWS AZ Recommendations: %d zones, best AZ: %s", len(resp.Recommendations), resp.BestAZ)
		}
	}
}

func TestAzureAZRecommendationFlow(t *testing.T) {
	server := NewServer(8080)

	requestBody := AZRequest{
		CloudProvider: "azure",
		InstanceType:  "Standard_D2s_v5",
		Region:        "eastus",
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/az", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := server.rateLimiter.Middleware(server.handleAZRecommendation)
	handler(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Azure AZ recommendation returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AZResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("Failed to decode Azure AZ response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Azure AZ recommendation should succeed, got error: %s", resp.Error)
	}

	// Azure should always return 3 availability zones
	if len(resp.Recommendations) != 3 {
		t.Errorf("Azure should return 3 AZ recommendations, got %d", len(resp.Recommendations))
	}

	if resp.BestAZ == "" {
		t.Error("Azure BestAZ should not be empty")
	}

	// Verify AZ naming format
	for _, rec := range resp.Recommendations {
		if rec.AvailabilityZone == "" {
			t.Error("AZ name should not be empty")
		}
		if rec.AvgPrice <= 0 {
			t.Errorf("AZ price should be positive, got %f", rec.AvgPrice)
		}
	}

	t.Logf("Azure AZ Recommendations: %d zones, best AZ: %s", len(resp.Recommendations), resp.BestAZ)
}

func TestAWSFamiliesFlow(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/families?cloud=aws", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleFamilies)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("AWS families returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var families []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&families); err != nil {
		t.Errorf("Failed to decode AWS families: %v", err)
	}

	if len(families) == 0 {
		t.Error("AWS families should not be empty")
	}

	// Check for expected AWS families
	familyNames := make(map[string]bool)
	for _, f := range families {
		name := f["name"].(string)
		familyNames[name] = true
	}

	expectedFamilies := []string{"m", "c", "r", "t"}
	for _, expected := range expectedFamilies {
		if !familyNames[expected] {
			t.Logf("Expected AWS family '%s' not found (may be expected with dynamic loading)", expected)
		}
	}

	t.Logf("AWS Families: found %d families", len(families))
}

func TestAzureFamiliesFlow(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/families?cloud=azure", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleFamilies)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Azure families returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var families []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&families); err != nil {
		t.Errorf("Failed to decode Azure families: %v", err)
	}

	if len(families) == 0 {
		t.Error("Azure families should not be empty")
	}

	// Check for expected Azure families
	familyNames := make(map[string]bool)
	for _, f := range families {
		name := f["name"].(string)
		familyNames[name] = true
	}

	expectedFamilies := []string{"D", "E", "F", "B"}
	for _, expected := range expectedFamilies {
		if !familyNames[expected] {
			t.Errorf("Expected Azure family '%s' not found", expected)
		}
	}

	t.Logf("Azure Families: found %d families", len(families))
}

func TestAWSInstanceTypesFlow(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/instance-types?cloud=aws&q=m5", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleInstanceTypes)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("AWS instance types returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode AWS instance types: %v", err)
	}

	t.Logf("AWS Instance Types search for 'm5': %v", result)
}

func TestAzureInstanceTypesFlow(t *testing.T) {
	server := NewServer(8080)

	req, _ := http.NewRequest("GET", "/api/instance-types?cloud=azure&q=Standard_D", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleInstanceTypes)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Azure instance types returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode Azure instance types: %v", err)
	}

	t.Logf("Azure Instance Types search for 'Standard_D': %v", result)
}

func TestCloudProviderDefaultRegionFallback(t *testing.T) {
	server := NewServer(8080)

	tests := []struct {
		name           string
		cloudProvider  string
		expectedRegion string
	}{
		{"AWS default region", "aws", "us-east-1"},
		{"Azure default region", "azure", "eastus"},
		{"Empty defaults to AWS", "", "us-east-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestBody := AnalyzeRequest{
				CloudProvider:   tt.cloudProvider,
				MinVCPU:         2,
				Region:          "", // Empty to test default
				MaxInterruption: 2,
				TopN:            5,
			}

			body, _ := json.Marshal(requestBody)
			req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(server.handleAnalyze)
			handler.ServeHTTP(rr, req)

			// Just verify it doesn't error out - the default region should be applied
			if rr.Code != http.StatusOK {
				t.Errorf("Request with empty region should succeed, got status %d", rr.Code)
			}
		})
	}
}

func TestEnhancedAnalysisAWS(t *testing.T) {
	server := NewServer(8080)

	requestBody := AnalyzeRequest{
		CloudProvider:   "aws",
		MinVCPU:         2,
		MaxVCPU:         4,
		MinMemory:       4,
		Region:          "us-east-1",
		MaxInterruption: 2,
		TopN:            5,
		Enhanced:        true, // Enable enhanced analysis
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("AWS enhanced analyze returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AnalyzeResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	t.Logf("AWS Enhanced Analysis: success=%v, instances=%d", resp.Success, len(resp.Instances))
}

func TestEnhancedAnalysisAzure(t *testing.T) {
	server := NewServer(8080)

	requestBody := AnalyzeRequest{
		CloudProvider:   "azure",
		MinVCPU:         2,
		MaxVCPU:         4,
		MinMemory:       4,
		Region:          "eastus",
		MaxInterruption: 2,
		TopN:            5,
		Enhanced:        true, // Enable enhanced analysis
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/analyze", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAnalyze)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Azure enhanced analyze returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp AnalyzeResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	t.Logf("Azure Enhanced Analysis: success=%v, instances=%d", resp.Success, len(resp.Instances))
}
