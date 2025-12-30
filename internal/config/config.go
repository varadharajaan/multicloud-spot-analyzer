// Package config provides centralized configuration management
// for the Spot Analyzer application. It supports loading from
// YAML files and environment variables.
package config

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	Server           ServerConfig           `yaml:"server"`
	Cache            CacheConfig            `yaml:"cache"`
	AWS              AWSConfig              `yaml:"aws"`
	Analysis         AnalysisConfig         `yaml:"analysis"`
	Logging          LoggingConfig          `yaml:"logging"`
	UI               UIConfig               `yaml:"ui"`
	InstanceFamilies InstanceFamiliesConfig `yaml:"instance_families"`
}

// ServerConfig holds server-related settings
type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// CacheConfig holds cache-related settings
type CacheConfig struct {
	TTL             time.Duration `yaml:"ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	LambdaPath      string        `yaml:"lambda_path"`
}

// AWSConfig holds AWS-related settings
type AWSConfig struct {
	SpotAdvisorURL           string        `yaml:"spot_advisor_url"`
	PriceHistoryLookbackDays int           `yaml:"price_history_lookback_days"`
	DefaultRegion            string        `yaml:"default_region"`
	HTTPTimeout              time.Duration `yaml:"http_timeout"`
}

// AnalysisConfig holds analysis-related settings
type AnalysisConfig struct {
	DefaultTopN            int           `yaml:"default_top_n"`
	DefaultMaxInterruption int           `yaml:"default_max_interruption"`
	ContextTimeout         time.Duration `yaml:"context_timeout"`
	AZRecommendations      int           `yaml:"az_recommendations"`
	AllowBurstable         bool          `yaml:"allow_burstable"`
	AllowBareMetal         bool          `yaml:"allow_bare_metal"`
}

// LoggingConfig holds logging-related settings
type LoggingConfig struct {
	Level       string `yaml:"level"`
	EnableFile  bool   `yaml:"enable_file"`
	EnableJSON  bool   `yaml:"enable_json"`
	EnableColor bool   `yaml:"enable_color"`
	LogDir      string `yaml:"log_dir"`
	MaxSizeMB   int    `yaml:"max_size_mb"`
	MaxBackups  int    `yaml:"max_backups"`
	MaxAgeDays  int    `yaml:"max_age_days"`
	Compress    bool   `yaml:"compress"`
}

// UIConfig holds UI-related settings
type UIConfig struct {
	Version string `yaml:"version"`
	Theme   string `yaml:"theme"`
}

// InstanceFamiliesConfig holds instance family settings
type InstanceFamiliesConfig struct {
	Available []InstanceFamily `yaml:"available"`
}

// InstanceFamily represents an EC2 instance family
type InstanceFamily struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMu     sync.RWMutex
)

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8000,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
		},
		Cache: CacheConfig{
			TTL:             2 * time.Hour,
			CleanupInterval: 10 * time.Minute,
			LambdaPath:      "/tmp/spot-analyzer-cache",
		},
		AWS: AWSConfig{
			SpotAdvisorURL:           "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json",
			PriceHistoryLookbackDays: 7,
			DefaultRegion:            "us-east-1",
			HTTPTimeout:              30 * time.Second,
		},
		Analysis: AnalysisConfig{
			DefaultTopN:            10,
			DefaultMaxInterruption: 2,
			ContextTimeout:         60 * time.Second,
			AZRecommendations:      2,
			AllowBurstable:         true, // Include t-family by default
			AllowBareMetal:         false,
		},
		Logging: LoggingConfig{
			Level:       "info",
			EnableFile:  true,
			EnableJSON:  true,
			EnableColor: true,
			LogDir:      "logs",
			MaxSizeMB:   100,
			MaxBackups:  3,
			MaxAgeDays:  7,
			Compress:    true,
		},
		UI: UIConfig{
			Version: "v1",
			Theme:   "light",
		},
		InstanceFamilies: InstanceFamiliesConfig{
			Available: []InstanceFamily{
				{Name: "t", Description: "Burstable (T2, T3, T3a, T4g)"},
				{Name: "m", Description: "General Purpose (M5, M6i, M7i)"},
				{Name: "c", Description: "Compute Optimized (C5, C6i, C7i)"},
				{Name: "r", Description: "Memory Optimized (R5, R6i, R7i)"},
				{Name: "i", Description: "Storage Optimized (I3, I4i)"},
				{Name: "d", Description: "Dense Storage (D2, D3)"},
				{Name: "g", Description: "GPU Instances (G4dn, G5)"},
				{Name: "p", Description: "GPU Compute (P3, P4d)"},
				{Name: "inf", Description: "Inference (Inf1, Inf2)"},
				{Name: "hpc", Description: "High Performance Computing"},
			},
		},
	}
}

// Get returns the global configuration (singleton)
func Get() *Config {
	configOnce.Do(func() {
		globalConfig = DefaultConfig()
		loadConfigFile()
		loadEnvOverrides()
	})
	return globalConfig
}

// Reload reloads the configuration from file
func Reload() error {
	configMu.Lock()
	defer configMu.Unlock()

	globalConfig = DefaultConfig()
	loadConfigFile()
	loadEnvOverrides()
	return nil
}

// loadConfigFile loads configuration from config.yaml
func loadConfigFile() {
	// Try multiple paths for config file
	paths := []string{
		"config.yaml",
		"config.yml",
		filepath.Join(getExecutableDir(), "config.yaml"),
		filepath.Join(getExecutableDir(), "config.yml"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		if err := yaml.Unmarshal(data, globalConfig); err != nil {
			continue
		}
		return
	}
}

// loadEnvOverrides applies environment variable overrides
func loadEnvOverrides() {
	// Server port
	if port := os.Getenv("SPOT_ANALYZER_PORT"); port != "" {
		if p, err := time.ParseDuration(port); err == nil {
			globalConfig.Server.Port = int(p.Seconds())
		}
	}

	// Cache TTL
	if ttl := os.Getenv("SPOT_ANALYZER_CACHE_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			globalConfig.Cache.TTL = d
		}
	}

	// AWS Region
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		globalConfig.AWS.DefaultRegion = region
	}

	// Lambda detection - adjust settings for Lambda environment
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		globalConfig.Logging.EnableFile = false
		globalConfig.Logging.EnableColor = false
		globalConfig.Cache.LambdaPath = "/tmp/spot-analyzer-cache"
	}

	// UI Version
	if uiVersion := os.Getenv("SPOT_ANALYZER_UI_VERSION"); uiVersion != "" {
		globalConfig.UI.Version = uiVersion
	}
}

// getExecutableDir returns the directory containing the executable
func getExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// IsLambda returns true if running in AWS Lambda
func IsLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// GetCachePath returns the appropriate cache path
func GetCachePath() string {
	if IsLambda() {
		return Get().Cache.LambdaPath
	}
	return ""
}
