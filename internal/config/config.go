// Package config provides centralized configuration management
// for the Spot Analyzer application. It supports loading from
// YAML files, environment variables, and AWS Secrets Manager (for Lambda).
package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	Server           ServerConfig           `yaml:"server"`
	Cache            CacheConfig            `yaml:"cache"`
	AWS              AWSConfig              `yaml:"aws"`
	Azure            AzureConfig            `yaml:"azure"`
	AzureCredentials AzureCredentialsConfig `yaml:"azure_credentials"`
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

// AzureConfig holds Azure-related settings
type AzureConfig struct {
	RetailPricesURL string        `yaml:"retail_prices_url"`
	DefaultRegion   string        `yaml:"default_region"`
	HTTPTimeout     time.Duration `yaml:"http_timeout"`
	// Azure authentication credentials (for Compute SKUs API)
	TenantID       string `yaml:"tenantId"`
	ClientID       string `yaml:"clientId"`
	ClientSecret   string `yaml:"clientSecret"`
	SubscriptionID string `yaml:"subscriptionId"`
}

// AzureCredentialsConfig holds Azure credentials from azure_credentials section
// This is a separate section to avoid conflicts with azure: API settings
type AzureCredentialsConfig struct {
	TenantID       string `yaml:"tenant_id"`
	ClientID       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	SubscriptionID string `yaml:"subscription_id"`
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
		Azure: AzureConfig{
			RetailPricesURL: "https://prices.azure.com/api/retail/prices",
			DefaultRegion:   "eastus",
			HTTPTimeout:     60 * time.Second,
		},
		Analysis: AnalysisConfig{
			DefaultTopN:            10,
			DefaultMaxInterruption: 2,
			ContextTimeout:         60 * time.Second,
			AZRecommendations:      3,    // Show best, 2nd best, and 3rd best AZ
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
		break
	}

	// Load Azure credentials from separate file (azure-config.yaml)
	loadAzureConfigFile()

	// Merge azure_credentials into Azure config (if present)
	mergeAzureCredentials()
}

// loadAzureConfigFile loads Azure credentials from azure-config.yaml
func loadAzureConfigFile() {
	paths := []string{
		"azure-config.yaml",
		"azure-config.yml",
		filepath.Join(getExecutableDir(), "azure-config.yaml"),
		filepath.Join(getExecutableDir(), "azure-config.yml"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Parse into a temporary struct to extract just azure_credentials
		var azureOnly struct {
			AzureCredentials AzureCredentialsConfig `yaml:"azure_credentials"`
		}
		if err := yaml.Unmarshal(data, &azureOnly); err != nil {
			continue
		}

		// Merge into global config
		if azureOnly.AzureCredentials.TenantID != "" {
			globalConfig.AzureCredentials = azureOnly.AzureCredentials
		}
		return
	}
}

// mergeAzureCredentials copies credentials from azure_credentials section to Azure config
func mergeAzureCredentials() {
	creds := globalConfig.AzureCredentials
	if creds.TenantID != "" {
		globalConfig.Azure.TenantID = creds.TenantID
	}
	if creds.ClientID != "" {
		globalConfig.Azure.ClientID = creds.ClientID
	}
	if creds.ClientSecret != "" {
		globalConfig.Azure.ClientSecret = creds.ClientSecret
	}
	if creds.SubscriptionID != "" {
		globalConfig.Azure.SubscriptionID = creds.SubscriptionID
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

		// Load Azure credentials from AWS Secrets Manager in Lambda
		loadAzureCredsFromSecretsManager()
	}

	// Environment variables override (for both local and Lambda)
	// These take precedence over Secrets Manager
	if tenantID := os.Getenv("AZURE_TENANT_ID"); tenantID != "" {
		globalConfig.Azure.TenantID = tenantID
	}
	if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
		globalConfig.Azure.ClientID = clientID
	}
	if clientSecret := os.Getenv("AZURE_CLIENT_SECRET"); clientSecret != "" {
		globalConfig.Azure.ClientSecret = clientSecret
	}
	if subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subscriptionID != "" {
		globalConfig.Azure.SubscriptionID = subscriptionID
	}

	// UI Version
	if uiVersion := os.Getenv("SPOT_ANALYZER_UI_VERSION"); uiVersion != "" {
		globalConfig.UI.Version = uiVersion
	}
}

// AzureSecretsManagerPayload represents the secret structure in AWS Secrets Manager
type AzureSecretsManagerPayload struct {
	TenantID       string `json:"AZURE_TENANT_ID"`
	ClientID       string `json:"AZURE_CLIENT_ID"`
	ClientSecret   string `json:"AZURE_CLIENT_SECRET"`
	SubscriptionID string `json:"AZURE_SUBSCRIPTION_ID"`
}

// loadAzureCredsFromSecretsManager loads Azure credentials from AWS Secrets Manager
// This is only called when running in Lambda
func loadAzureCredsFromSecretsManager() {
	secretName := os.Getenv("AZURE_SECRET_NAME")
	if secretName == "" {
		secretName = "spot-analyzer/azure-credentials" // Default secret name
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load AWS config (uses Lambda's IAM role automatically)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		// Silently fail - Azure features will be disabled
		return
	}

	client := secretsmanager.NewFromConfig(cfg)

	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})
	if err != nil {
		// Silently fail - Azure features will be disabled
		return
	}

	if result.SecretString == nil {
		return
	}

	var payload AzureSecretsManagerPayload
	if err := json.Unmarshal([]byte(*result.SecretString), &payload); err != nil {
		return
	}

	// Apply credentials to config
	if payload.TenantID != "" {
		globalConfig.Azure.TenantID = payload.TenantID
	}
	if payload.ClientID != "" {
		globalConfig.Azure.ClientID = payload.ClientID
	}
	if payload.ClientSecret != "" {
		globalConfig.Azure.ClientSecret = payload.ClientSecret
	}
	if payload.SubscriptionID != "" {
		globalConfig.Azure.SubscriptionID = payload.SubscriptionID
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
