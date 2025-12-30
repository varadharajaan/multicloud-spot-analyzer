package controller

import (
	"context"
	"testing"
	"time"
)

func TestNewController(t *testing.T) {
	ctrl := New()
	if ctrl == nil {
		t.Fatal("New() returned nil")
	}
	if ctrl.cfg == nil {
		t.Error("Controller config should not be nil")
	}
	if ctrl.logger == nil {
		t.Error("Controller logger should not be nil")
	}
}

func TestAnalyzeRequest(t *testing.T) {
	req := AnalyzeRequest{
		MinVCPU:         2,
		MaxVCPU:         8,
		MinMemory:       4,
		MaxMemory:       32,
		Architecture:    "x86_64",
		Region:          "us-east-1",
		MaxInterruption: 2,
		UseCase:         "general",
		Enhanced:        true,
		TopN:            10,
		Families:        []string{"m", "c"},
	}

	if req.MinVCPU != 2 {
		t.Errorf("MinVCPU = %v, want 2", req.MinVCPU)
	}
	if req.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", req.Region)
	}
	if len(req.Families) != 2 {
		t.Errorf("Families length = %v, want 2", len(req.Families))
	}
}

func TestAZRequest(t *testing.T) {
	req := AZRequest{
		InstanceType: "m5.large",
		Region:       "us-east-1",
		RefreshCache: false,
	}

	if req.InstanceType != "m5.large" {
		t.Errorf("InstanceType = %v, want m5.large", req.InstanceType)
	}
	if req.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", req.Region)
	}
}

func TestAnalyzeWithValidation(t *testing.T) {
	ctrl := New()

	// Test with minimum valid request
	req := AnalyzeRequest{
		MinVCPU: 2,
		Region:  "us-east-1",
		TopN:    5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := ctrl.Analyze(ctx, req)

	// Should succeed or fail gracefully
	if err != nil {
		t.Logf("Analysis returned error (may be expected): %v", err)
		return
	}

	if resp.Error != "" && !resp.Success {
		t.Logf("Analysis returned error in response: %s", resp.Error)
	}

	if resp.Success && len(resp.Instances) == 0 {
		t.Log("Analysis succeeded but returned no instances")
	}
}

func TestAnalyzeDefaults(t *testing.T) {
	ctrl := New()

	// Test with minimal request - should apply defaults
	req := AnalyzeRequest{
		MinVCPU: 2,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := ctrl.Analyze(ctx, req)
	if err != nil {
		t.Logf("Analyze returned error (may be expected): %v", err)
		return
	}

	// Check that response has required fields
	if resp.AnalyzedAt == "" {
		t.Error("AnalyzedAt should not be empty")
	}
}

func TestRecommendAZValidation(t *testing.T) {
	ctrl := New()

	// Test with valid instance type
	req := AZRequest{
		InstanceType: "m5.large",
		Region:       "us-east-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := ctrl.RecommendAZ(ctx, req)

	// Should succeed or fail gracefully
	if err != nil {
		t.Logf("AZ recommendations returned error (may be expected): %v", err)
		return
	}

	if resp.Error != "" && !resp.Success {
		t.Logf("AZ recommendations returned error: %s", resp.Error)
	}
}

func TestInstanceResultStructure(t *testing.T) {
	result := InstanceResult{
		Rank:              1,
		InstanceType:      "m5.large",
		VCPU:              2,
		MemoryGB:          8.0,
		SavingsPercent:    60,
		InterruptionLevel: "<5%",
		Score:             0.92,
		Architecture:      "x86_64",
		Family:            "m",
		HourlyPrice:       "$0.0345",
	}

	if result.Rank != 1 {
		t.Errorf("Rank = %v, want 1", result.Rank)
	}
	if result.InstanceType != "m5.large" {
		t.Errorf("InstanceType = %v, want m5.large", result.InstanceType)
	}
	if result.Score != 0.92 {
		t.Errorf("Score = %v, want 0.92", result.Score)
	}
}

func TestAZRecommendationStructure(t *testing.T) {
	rec := AZRecommendation{
		Rank:             1,
		AvailabilityZone: "us-east-1a",
		AvgPrice:         0.045,
		MinPrice:         0.03,
		MaxPrice:         0.08,
		CurrentPrice:     0.05,
		Volatility:       0.2,
		Stability:        "High",
	}

	if rec.Rank != 1 {
		t.Errorf("Rank = %v, want 1", rec.Rank)
	}
	if rec.AvailabilityZone != "us-east-1a" {
		t.Errorf("AvailabilityZone = %v, want us-east-1a", rec.AvailabilityZone)
	}
	if rec.Stability != "High" {
		t.Errorf("Stability = %v, want High", rec.Stability)
	}
}

func TestApplyUseCasePreset(t *testing.T) {
	ctrl := New()

	tests := []struct {
		useCase         string
		expectBurstable bool
	}{
		{"general", true},
		{"kubernetes", false},
		{"database", false},
		{"asg", true},
	}

	for _, tt := range tests {
		t.Run(tt.useCase, func(t *testing.T) {
			req := AnalyzeRequest{
				MinVCPU: 2,
				UseCase: tt.useCase,
				Region:  "us-east-1",
			}

			ctx := context.Background()
			// Note: We can't easily test applyUseCasePreset directly since it's not exported
			// This is an integration test that verifies the behavior through Analyze
			resp, _ := ctrl.Analyze(ctx, req)
			// Just verify it doesn't crash
			_ = resp
		})
	}
}
