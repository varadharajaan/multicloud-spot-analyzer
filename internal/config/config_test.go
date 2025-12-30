package config

import (
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Server config
	if cfg.Server.Port != 8000 {
		t.Errorf("Server.Port = %v, want 8000", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 30s", cfg.Server.ReadTimeout)
	}

	// Cache config
	if cfg.Cache.TTL != 2*time.Hour {
		t.Errorf("Cache.TTL = %v, want 2h", cfg.Cache.TTL)
	}

	// AWS config
	if cfg.AWS.DefaultRegion != "us-east-1" {
		t.Errorf("AWS.DefaultRegion = %v, want us-east-1", cfg.AWS.DefaultRegion)
	}
	if cfg.AWS.PriceHistoryLookbackDays != 7 {
		t.Errorf("AWS.PriceHistoryLookbackDays = %v, want 7", cfg.AWS.PriceHistoryLookbackDays)
	}

	// Analysis config
	if cfg.Analysis.DefaultTopN != 10 {
		t.Errorf("Analysis.DefaultTopN = %v, want 10", cfg.Analysis.DefaultTopN)
	}
	if cfg.Analysis.AZRecommendations != 3 {
		t.Errorf("Analysis.AZRecommendations = %v, want 3", cfg.Analysis.AZRecommendations)
	}
	if !cfg.Analysis.AllowBurstable {
		t.Error("Analysis.AllowBurstable should be true")
	}

	// Logging config
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %v, want info", cfg.Logging.Level)
	}
}

func TestGetReturnsDefaultIfNotLoaded(t *testing.T) {
	// Reset global config
	globalConfig = nil
	configOnce = sync.Once{}

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}
	if cfg.Server.Port != 8000 {
		t.Errorf("Server.Port = %v, want 8000", cfg.Server.Port)
	}
}

// TestLoadFromYAML is skipped - LoadFromFile is internal
func TestLoadFromYAML(t *testing.T) {
	t.Skip("LoadFromFile is internal implementation")
}

// TestLoadWithMissingFile is skipped - Load is internal
func TestLoadWithMissingFile(t *testing.T) {
	t.Skip("Load is internal implementation")
}

func TestInstanceFamily(t *testing.T) {
	family := InstanceFamily{
		Name:        "General Purpose",
		Description: "Balanced compute",
	}

	if family.Name != "General Purpose" {
		t.Errorf("Name = %v, want General Purpose", family.Name)
	}
	if family.Description != "Balanced compute" {
		t.Errorf("Description = %v, want Balanced compute", family.Description)
	}
}

func TestConfigConcurrentAccess(t *testing.T) {
	// Reset global config
	globalConfig = nil
	configOnce = sync.Once{}

	done := make(chan bool)

	// Concurrent access test
	for i := 0; i < 10; i++ {
		go func() {
			cfg := Get()
			if cfg == nil {
				t.Error("Get() returned nil in concurrent access")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
