// Package gcp provides credential management for GCP APIs.
package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/spot-analyzer/internal/logging"
)

// CredentialManager handles GCP authentication for API access
type CredentialManager struct {
	projectID   string
	credentials *google.Credentials
	mu          sync.RWMutex
	initialized bool
}

var (
	globalCredManager *CredentialManager
	credManagerOnce   sync.Once
)

// GetCredentialManager returns the singleton credential manager
func GetCredentialManager() *CredentialManager {
	credManagerOnce.Do(func() {
		globalCredManager = &CredentialManager{}
	})
	return globalCredManager
}

// Initialize sets up GCP credentials from various sources
// Priority:
// 1. GOOGLE_APPLICATION_CREDENTIALS_JSON (inline JSON for Lambda)
// 2. GOOGLE_APPLICATION_CREDENTIALS (file path)
// 3. Application Default Credentials (gcloud auth)
// 4. gcp-config.yaml in project root
func (cm *CredentialManager) Initialize(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.initialized {
		return nil
	}

	var err error
	var credJSON []byte

	// 1. Check for inline JSON credentials (Lambda/serverless)
	if jsonCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS_JSON"); jsonCreds != "" {
		logging.Info("Using GCP credentials from GOOGLE_APPLICATION_CREDENTIALS_JSON env var")
		credJSON = []byte(jsonCreds)
	}

	// 2. Check for credentials file path
	if credJSON == nil {
		if credPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credPath != "" {
			logging.Info("Using GCP credentials from file: %s", credPath)
			credJSON, err = os.ReadFile(credPath)
			if err != nil {
				return fmt.Errorf("failed to read credentials file: %w", err)
			}
		}
	}

	// 3. Check for gcp-config.yaml in project root
	if credJSON == nil {
		credJSON = cm.loadFromGCPConfig()
	}

	// Create credentials
	if credJSON != nil {
		cm.credentials, err = google.CredentialsFromJSON(ctx, credJSON,
			"https://www.googleapis.com/auth/cloud-billing.readonly",
			"https://www.googleapis.com/auth/compute.readonly",
		)
		if err != nil {
			return fmt.Errorf("failed to create credentials from JSON: %w", err)
		}
		cm.projectID = cm.extractProjectID(credJSON)
		cm.initialized = true
		logging.Info("GCP credentials initialized for project: %s", cm.projectID)
		return nil
	}

	// 4. Fall back to Application Default Credentials
	logging.Info("Attempting to use Application Default Credentials (ADC)")
	cm.credentials, err = google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/cloud-billing.readonly",
		"https://www.googleapis.com/auth/compute.readonly",
	)
	if err != nil {
		logging.Warn("No GCP credentials found - real API features disabled: %v", err)
		return nil // Not an error - we can fall back to public data
	}

	cm.projectID = cm.credentials.ProjectID
	cm.initialized = true
	logging.Info("GCP ADC initialized for project: %s", cm.projectID)
	return nil
}

// loadFromGCPConfig loads credentials from gcp-config.yaml
func (cm *CredentialManager) loadFromGCPConfig() []byte {
	// Check common locations for gcp-config.yaml
	paths := []string{
		"gcp-config.yaml",
		"../gcp-config.yaml",
		"../../gcp-config.yaml",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Parse YAML to extract service account JSON
		// Look for service_account_json field

		// Simple YAML parsing (avoid external dependency)
		// Look for service_account_json field
		if json := extractYAMLField(string(data), "service_account_json"); json != "" {
			logging.Info("Loaded GCP credentials from %s", path)
			return []byte(json)
		}
	}

	return nil
}

// extractYAMLField extracts a simple YAML field value
func extractYAMLField(yamlContent, fieldName string) string {
	// Simple extraction - look for field: value pattern
	// This handles the case where service_account_json is base64 or inline JSON
	lines := splitLines(yamlContent)
	for i, line := range lines {
		if contains(line, fieldName+":") {
			// Check if value is on same line
			parts := splitColon(line)
			if len(parts) > 1 && trimSpace(parts[1]) != "" && trimSpace(parts[1]) != "|" {
				return trimQuotes(trimSpace(parts[1]))
			}
			// Multi-line value (indented block)
			if i+1 < len(lines) {
				return extractMultilineValue(lines[i+1:])
			}
		}
	}
	return ""
}

// extractMultilineValue extracts indented multiline YAML value
func extractMultilineValue(lines []string) string {
	var result string
	for _, line := range lines {
		if len(line) == 0 || line[0] != ' ' {
			break
		}
		result += trimSpace(line)
	}
	return result
}

// Helper functions to avoid importing strings package
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitColon(s string) []string {
	idx := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// extractProjectID extracts project ID from credentials JSON
func (cm *CredentialManager) extractProjectID(credJSON []byte) string {
	var creds struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(credJSON, &creds); err != nil {
		return ""
	}
	return creds.ProjectID
}

// IsAvailable returns true if GCP credentials are configured
func (cm *CredentialManager) IsAvailable() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.initialized && cm.credentials != nil
}

// GetProjectID returns the configured project ID
func (cm *CredentialManager) GetProjectID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.projectID
}

// GetClientOption returns the option for API clients
func (cm *CredentialManager) GetClientOption() option.ClientOption {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.credentials != nil {
		return option.WithCredentials(cm.credentials)
	}
	return nil
}

// GetTokenSource returns a token source for API authentication
func (cm *CredentialManager) GetTokenSource() oauth2.TokenSource {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.credentials != nil {
		return cm.credentials.TokenSource
	}
	return nil
}
