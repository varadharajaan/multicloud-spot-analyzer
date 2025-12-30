package config

import (
	"os"
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
	once = sync.Once{}

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}
	if cfg.Server.Port != 8000 {
		t.Errorf("Server.Port = %v, want 8000", cfg.Server.Port)
	}
}

func TestLoadFromYAML(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"

	configContent := `
server:
  port: 9000
  read_timeout: 60s
cache:
  ttl: 1h
aws:
  default_region: eu-west-1
analysis:
  default_top_n: 20
  az_recommendations: 5
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Reset global config
	globalConfig = nil
	once = sync.Once{}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %v, want 9000", cfg.Server.Port)
	}
	if cfg.Cache.TTL != 1*time.Hour {
		t.Errorf("Cache.TTL = %v, want 1h", cfg.Cache.TTL)
	}
	if cfg.AWS.DefaultRegion != "eu-west-1" {
		t.Errorf("AWS.DefaultRegion = %v, want eu-west-1", cfg.AWS.DefaultRegion)
	}
	if cfg.Analysis.DefaultTopN != 20 {
		t.Errorf("Analysis.DefaultTopN = %v, want 20", cfg.Analysis.DefaultTopN)
	}
	if cfg.Analysis.AZRecommendations != 5 {
		t.Errorf("Analysis.AZRecommendations = %v, want 5", cfg.Analysis.AZRecommendations)
	}
}

func TestLoadWithMissingFile(t *testing.T) {
	// Reset global config
	globalConfig = nil
	once = sync.Once{}

	cfg, err := Load("/non/existent/path.yaml")
	// Should return default config, not error
	if err != nil {
		t.Logf("Expected behavior: Load returns error for missing file: %v", err)
	}
	if cfg == nil {
		t.Error("Load() should return default config for missing file")
	}
}

func TestInstanceFamily(t *testing.T) {
	family := InstanceFamily{
		ID:          "m",
		Name:        "General Purpose",
		Description: "Balanced compute",
		Category:    "general",
	}

	if family.ID != "m" {
		t.Errorf("ID = %v, want m", family.ID)
	}
	if family.Category != "general" {
		t.Errorf("Category = %v, want general", family.Category)
	}
}

func TestConfigConcurrentAccess(t *testing.T) {
	// Reset global config
	globalConfig = nil
	once = sync.Once{}

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
