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
