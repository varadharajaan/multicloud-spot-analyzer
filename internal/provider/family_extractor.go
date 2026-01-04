// Package provider contains shared provider utilities and factory functions.
package provider

import (
	"regexp"
	"strings"

	"github.com/spot-analyzer/internal/domain"
)

// ===============================================
// AWS Family Extractor
// ===============================================

// AWSFamilyExtractor extracts family from AWS instance types
// Examples: m5.large -> "m", c6i.xlarge -> "c", im4gn.large -> "im"
type AWSFamilyExtractor struct{}

// NewAWSFamilyExtractor creates a new AWS family extractor
func NewAWSFamilyExtractor() *AWSFamilyExtractor {
	return &AWSFamilyExtractor{}
}

// ExtractFamily extracts the family from an AWS instance type
func (e *AWSFamilyExtractor) ExtractFamily(instanceType string) string {
	// AWS format: {family}{generation}{modifiers}.{size}
	// Examples: m5.large, c6i.xlarge, r5a.2xlarge, im4gn.large
	for i, c := range instanceType {
		if c >= '0' && c <= '9' {
			return instanceType[:i]
		}
	}
	return instanceType
}

// NormalizeName normalizes AWS instance type for comparison
func (e *AWSFamilyExtractor) NormalizeName(instanceType string) string {
	return strings.ToLower(strings.TrimSpace(instanceType))
}

// GetProviderName returns the cloud provider
func (e *AWSFamilyExtractor) GetProviderName() domain.CloudProvider {
	return domain.AWS
}

// ===============================================
// Azure Family Extractor
// ===============================================

// AzureFamilyExtractor extracts family from Azure VM sizes
// Examples: Standard_D4s_v5 -> "D", Standard_B2s -> "B", Standard_NC24ads_A100_v4 -> "NC"
type AzureFamilyExtractor struct {
	// Regex to extract series from Azure VM names
	seriesRegex *regexp.Regexp
}

// NewAzureFamilyExtractor creates a new Azure family extractor
func NewAzureFamilyExtractor() *AzureFamilyExtractor {
	return &AzureFamilyExtractor{
		seriesRegex: regexp.MustCompile(`^([A-Za-z]+)\d`),
	}
}

// ExtractFamily extracts the family/series from an Azure VM size
func (e *AzureFamilyExtractor) ExtractFamily(instanceType string) string {
	// Azure format: Standard_{Series}{vCPU}{modifiers}_v{version}
	// Examples: Standard_D4s_v5, Standard_B2s, Standard_NC24ads_A100_v4
	
	// Remove "Standard_" prefix
	name := instanceType
	if strings.HasPrefix(name, "Standard_") {
		name = name[9:]
	}
	
	// Extract letters before the first digit
	matches := e.seriesRegex.FindStringSubmatch(name)
	if len(matches) >= 2 {
		return strings.ToUpper(matches[1])
	}
	
	// Fallback: extract until first digit
	for i, c := range name {
		if c >= '0' && c <= '9' {
			return strings.ToUpper(name[:i])
		}
	}
	
	return strings.ToUpper(name)
}

// NormalizeName normalizes Azure VM size for comparison
func (e *AzureFamilyExtractor) NormalizeName(instanceType string) string {
	name := strings.TrimSpace(instanceType)
	// Ensure Standard_ prefix for consistency
	if !strings.HasPrefix(name, "Standard_") {
		name = "Standard_" + name
	}
	return name
}

// GetProviderName returns the cloud provider
func (e *AzureFamilyExtractor) GetProviderName() domain.CloudProvider {
	return domain.Azure
}

// ===============================================
// GCP Family Extractor (Prepared for future)
// ===============================================

// GCPFamilyExtractor extracts family from GCP machine types
// Examples: n2-standard-4 -> "n2", e2-medium -> "e2", c2-standard-8 -> "c2"
type GCPFamilyExtractor struct{}

// NewGCPFamilyExtractor creates a new GCP family extractor
func NewGCPFamilyExtractor() *GCPFamilyExtractor {
	return &GCPFamilyExtractor{}
}

// ExtractFamily extracts the family from a GCP machine type
func (e *GCPFamilyExtractor) ExtractFamily(instanceType string) string {
	// GCP format: {family}-{type}-{vcpu} or {family}-{type}
	// Examples: n2-standard-4, e2-medium, c2-standard-8, n2d-standard-2
	parts := strings.Split(instanceType, "-")
	if len(parts) >= 1 {
		return strings.ToLower(parts[0])
	}
	return instanceType
}

// NormalizeName normalizes GCP machine type for comparison
func (e *GCPFamilyExtractor) NormalizeName(instanceType string) string {
	return strings.ToLower(strings.TrimSpace(instanceType))
}

// GetProviderName returns the cloud provider
func (e *GCPFamilyExtractor) GetProviderName() domain.CloudProvider {
	return domain.GCP
}

// ===============================================
// Factory Function
// ===============================================

// GetFamilyExtractor returns the appropriate family extractor for a cloud provider
func GetFamilyExtractor(provider domain.CloudProvider) domain.FamilyExtractor {
	switch provider {
	case domain.AWS:
		return NewAWSFamilyExtractor()
	case domain.Azure:
		return NewAzureFamilyExtractor()
	case domain.GCP:
		return NewGCPFamilyExtractor()
	default:
		return NewAWSFamilyExtractor() // Default to AWS
	}
}

// ExtractFamilyForProvider extracts family using the appropriate extractor
func ExtractFamilyForProvider(instanceType string, provider domain.CloudProvider) string {
	extractor := GetFamilyExtractor(provider)
	return extractor.ExtractFamily(instanceType)
}
