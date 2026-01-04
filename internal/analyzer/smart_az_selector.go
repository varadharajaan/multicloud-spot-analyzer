// Package analyzer provides smart AZ selection combining multiple factors.
// This module implements intelligent availability zone recommendations based on:
// - Zone availability from cloud SKU APIs
// - Estimated capacity scores
// - Spot price predictions for missing data
// - Interruption rate estimates
package analyzer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spot-analyzer/internal/logging"
)

// SmartAZSelector provides intelligent AZ recommendations by combining
// multiple data sources and prediction algorithms.
type SmartAZSelector struct {
	region        string
	cloudProvider string // "aws" or "azure"

	// Data providers (set via options)
	zoneProvider     ZoneAvailabilityProvider
	priceProvider    PriceHistoryProvider
	capacityProvider CapacityProvider
}

// ZoneAvailabilityProvider interface for getting zone availability data
type ZoneAvailabilityProvider interface {
	// GetZoneAvailability returns which zones a VM is available in
	GetZoneAvailability(ctx context.Context, vmSize, region string) ([]ZoneInfo, error)
	// IsAvailable returns true if the provider has credentials
	IsAvailable() bool
}

// ZoneInfo contains availability info for a zone
type ZoneInfo struct {
	Zone           string
	Available      bool
	Restricted     bool
	RestrictionMsg string
	CapacityScore  int // 0-100, higher = more capacity likely
}

// CapacityProvider interface for getting capacity estimates
type CapacityProvider interface {
	// GetCapacityScore returns estimated capacity (0-100) for a VM in a zone
	GetCapacityScore(ctx context.Context, vmSize, zone string) (int, error)
}

// SmartAZResult contains the smart AZ recommendation result
type SmartAZResult struct {
	InstanceType string        `json:"instance_type"`
	Region       string        `json:"region"`
	Rankings     []SmartAZRank `json:"rankings"`
	BestAZ       string        `json:"best_az"`
	NextBestAZ   string        `json:"next_best_az"`
	Insights     []string      `json:"insights"`
	DataSources  []string      `json:"data_sources"` // Which APIs were used
	GeneratedAt  time.Time     `json:"generated_at"`
	Confidence   float64       `json:"confidence"` // 0-1, how confident we are
}

// SmartAZRank contains ranking for a single AZ with combined scoring
type SmartAZRank struct {
	Zone              string  `json:"zone"`
	Rank              int     `json:"rank"`
	CombinedScore     float64 `json:"combined_score"`     // 0-100, higher is better
	AvailabilityScore float64 `json:"availability_score"` // Based on zone availability
	CapacityScore     float64 `json:"capacity_score"`     // Estimated capacity
	PriceScore        float64 `json:"price_score"`        // Lower price = higher score
	StabilityScore    float64 `json:"stability_score"`    // Lower volatility = higher score
	InterruptionScore float64 `json:"interruption_score"` // Lower interruption = higher score

	// Raw data
	SpotPrice        float64 `json:"spot_price"`
	OnDemandPrice    float64 `json:"on_demand_price"`
	PricePredicted   bool    `json:"price_predicted"` // True if price was predicted
	Volatility       float64 `json:"volatility"`
	InterruptionRate float64 `json:"interruption_rate"` // Estimated %
	Available        bool    `json:"available"`
	Restricted       bool    `json:"restricted"`

	Explanation string `json:"explanation"`
}

// ScoreWeights defines the weights for combining different scores
type ScoreWeights struct {
	Availability float64 // Weight for zone availability (0-1)
	Capacity     float64 // Weight for capacity score (0-1)
	Price        float64 // Weight for price score (0-1)
	Stability    float64 // Weight for stability score (0-1)
	Interruption float64 // Weight for interruption rate (0-1)
}

// DefaultWeights returns balanced default weights
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.25, // 25% - is the VM available?
		Capacity:     0.25, // 25% - how much capacity?
		Price:        0.20, // 20% - price matters but not most
		Stability:    0.15, // 15% - price stability
		Interruption: 0.15, // 15% - interruption rate
	}
}

// HighCapacityWeights prioritizes capacity over price
func HighCapacityWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.20,
		Capacity:     0.40, // Heavy weight on capacity
		Price:        0.15,
		Stability:    0.10,
		Interruption: 0.15,
	}
}

// LowCostWeights prioritizes price over capacity
func LowCostWeights() ScoreWeights {
	return ScoreWeights{
		Availability: 0.20,
		Capacity:     0.15,
		Price:        0.35, // Heavy weight on price
		Stability:    0.15,
		Interruption: 0.15,
	}
}

// NewSmartAZSelector creates a new smart AZ selector
func NewSmartAZSelector(region, cloudProvider string) *SmartAZSelector {
	return &SmartAZSelector{
		region:        region,
		cloudProvider: strings.ToLower(cloudProvider),
	}
}

// WithZoneProvider sets the zone availability provider
func (s *SmartAZSelector) WithZoneProvider(p ZoneAvailabilityProvider) *SmartAZSelector {
	s.zoneProvider = p
	return s
}

// WithPriceProvider sets the price history provider
func (s *SmartAZSelector) WithPriceProvider(p PriceHistoryProvider) *SmartAZSelector {
	s.priceProvider = p
	return s
}

// WithCapacityProvider sets the capacity provider
func (s *SmartAZSelector) WithCapacityProvider(p CapacityProvider) *SmartAZSelector {
	s.capacityProvider = p
	return s
}

// RecommendAZ provides smart AZ recommendations
func (s *SmartAZSelector) RecommendAZ(ctx context.Context, vmSize string, weights ScoreWeights) (*SmartAZResult, error) {
	result := &SmartAZResult{
		InstanceType: vmSize,
		Region:       s.region,
		Rankings:     make([]SmartAZRank, 0),
		Insights:     make([]string, 0),
		DataSources:  make([]string, 0),
		GeneratedAt:  time.Now(),
		Confidence:   0.5, // Default medium confidence
	}

	// Step 1: Get zone availability
	zones, err := s.getZoneAvailability(ctx, vmSize)
	if err != nil {
		logging.Debug("Zone availability check for %s: %v (will use defaults)", vmSize, err)
		// Fall back to default zones
		zones = s.getDefaultZones()
	}

	if len(zones) == 0 {
		result.Insights = append(result.Insights, "⚠️ No availability zones found for this VM type")
		return result, nil
	}

	// Step 2: Get price data for each zone
	priceData := s.getPriceData(ctx, vmSize, zones)

	// Step 3: Calculate scores for each zone
	for _, zone := range zones {
		rank := s.calculateZoneScore(ctx, vmSize, zone, priceData, weights)
		result.Rankings = append(result.Rankings, rank)
	}

	// Step 4: Sort by combined score (descending)
	sort.Slice(result.Rankings, func(i, j int) bool {
		return result.Rankings[i].CombinedScore > result.Rankings[j].CombinedScore
	})

	// Step 5: Assign ranks
	for i := range result.Rankings {
		result.Rankings[i].Rank = i + 1
	}

	// Step 6: Set best AZ
	if len(result.Rankings) > 0 {
		result.BestAZ = result.Rankings[0].Zone
		if len(result.Rankings) > 1 {
			result.NextBestAZ = result.Rankings[1].Zone
		}
	}

	// Step 7: Calculate confidence based on data sources used
	result.Confidence = s.calculateConfidence(result)

	// Step 8: Generate insights
	result.Insights = s.generateInsights(result, weights)

	return result, nil
}

// getZoneAvailability gets zone availability from provider or defaults
func (s *SmartAZSelector) getZoneAvailability(ctx context.Context, vmSize string) ([]ZoneInfo, error) {
	if s.zoneProvider != nil && s.zoneProvider.IsAvailable() {
		zones, err := s.zoneProvider.GetZoneAvailability(ctx, vmSize, s.region)
		if err == nil && len(zones) > 0 {
			return zones, nil
		}
		logging.Debug("Zone provider returned no data for %s, using defaults", vmSize)
	}
	return nil, fmt.Errorf("no zone provider available")
}

// getDefaultZones returns default zones for a region
func (s *SmartAZSelector) getDefaultZones() []ZoneInfo {
	var zoneNames []string

	if s.cloudProvider == "azure" {
		zoneNames = []string{
			fmt.Sprintf("%s-1", s.region),
			fmt.Sprintf("%s-2", s.region),
			fmt.Sprintf("%s-3", s.region),
		}
	} else { // AWS
		zoneNames = []string{
			fmt.Sprintf("%sa", s.region),
			fmt.Sprintf("%sb", s.region),
			fmt.Sprintf("%sc", s.region),
		}
	}

	zones := make([]ZoneInfo, len(zoneNames))
	for i, name := range zoneNames {
		zones[i] = ZoneInfo{
			Zone:          name,
			Available:     true,
			CapacityScore: 50, // Unknown = medium
		}
	}
	return zones
}

// ZonePriceData contains price data for a zone
type ZonePriceData struct {
	SpotPrice     float64
	OnDemandPrice float64
	Volatility    float64
	Predicted     bool // True if price was predicted
	DataPoints    int
}

// getPriceData gets price data for all zones
func (s *SmartAZSelector) getPriceData(ctx context.Context, vmSize string, zones []ZoneInfo) map[string]*ZonePriceData {
	data := make(map[string]*ZonePriceData)

	if s.priceProvider == nil || !s.priceProvider.IsAvailable() {
		logging.Debug("No price provider available, will predict prices")
		for _, z := range zones {
			data[z.Zone] = s.predictPrice(vmSize, z.Zone)
		}
		return data
	}

	// Get price analysis from provider
	analysis, err := s.priceProvider.GetPriceAnalysis(ctx, vmSize, 7) // 7 days
	if err != nil || analysis == nil {
		logging.Debug("Price analysis failed for %s: %v", vmSize, err)
		for _, z := range zones {
			data[z.Zone] = s.predictPrice(vmSize, z.Zone)
		}
		return data
	}

	// If we have per-AZ data, use it
	if analysis.AllAZData != nil && len(analysis.AllAZData) > 0 {
		for zone, azData := range analysis.AllAZData {
			data[zone] = &ZonePriceData{
				SpotPrice:     azData.AvgPrice,
				OnDemandPrice: azData.AvgPrice * 3, // Estimate on-demand as 3x spot
				Volatility:    azData.Volatility,
				DataPoints:    azData.DataPoints,
				Predicted:     false,
			}
		}
	} else {
		// Use overall price for all zones
		for _, z := range zones {
			data[z.Zone] = &ZonePriceData{
				SpotPrice:     analysis.AvgPrice,
				OnDemandPrice: analysis.AvgPrice * 3,
				Volatility:    analysis.Volatility,
				DataPoints:    analysis.DataPoints,
				Predicted:     false,
			}
		}
	}

	// Fill in missing zones with predictions
	for _, z := range zones {
		if _, exists := data[z.Zone]; !exists {
			data[z.Zone] = s.predictPrice(vmSize, z.Zone)
		}
	}

	return data
}

// predictPrice predicts spot price when no real data available
func (s *SmartAZSelector) predictPrice(vmSize, zone string) *ZonePriceData {
	// Price prediction based on VM size analysis
	basePrice := s.estimateBasePriceFromSize(vmSize)

	// Zone-based adjustment (zone 1 usually slightly higher demand)
	zoneMultiplier := 1.0
	if strings.HasSuffix(zone, "a") || strings.HasSuffix(zone, "-1") {
		zoneMultiplier = 1.05 // 5% higher for zone a/1
	} else if strings.HasSuffix(zone, "c") || strings.HasSuffix(zone, "-3") {
		zoneMultiplier = 0.95 // 5% lower for zone c/3
	}

	predicted := &ZonePriceData{
		SpotPrice:     basePrice * zoneMultiplier,
		OnDemandPrice: basePrice * 3 * zoneMultiplier,
		Volatility:    0.15, // Assume moderate volatility
		Predicted:     true,
		DataPoints:    0,
	}

	logging.Debug("Predicted price for %s in %s: $%.4f", vmSize, zone, predicted.SpotPrice)
	return predicted
}

// estimateBasePriceFromSize estimates base spot price from VM size name
func (s *SmartAZSelector) estimateBasePriceFromSize(vmSize string) float64 {
	// Extract vCPU count from VM name (heuristic)
	size := strings.ToLower(vmSize)

	// Base prices per vCPU (rough estimates)
	var basePricePerVCPU float64

	if s.cloudProvider == "azure" {
		// Azure pricing heuristics
		basePricePerVCPU = 0.01 // $0.01 per vCPU/hour base

		// Adjust for series
		if strings.Contains(size, "nc") || strings.Contains(size, "nd") || strings.Contains(size, "nv") {
			basePricePerVCPU = 0.15 // GPU instances
		} else if strings.Contains(size, "m") && !strings.Contains(size, "ms") {
			basePricePerVCPU = 0.03 // Memory-optimized
		} else if strings.Contains(size, "f") {
			basePricePerVCPU = 0.015 // Compute-optimized
		} else if strings.Contains(size, "l") {
			basePricePerVCPU = 0.025 // Storage-optimized
		}
	} else {
		// AWS pricing heuristics
		basePricePerVCPU = 0.008 // $0.008 per vCPU/hour base

		if strings.Contains(size, "p") || strings.Contains(size, "g") || strings.Contains(size, "inf") {
			basePricePerVCPU = 0.12 // GPU/Inference instances
		} else if strings.Contains(size, "r") || strings.Contains(size, "x") {
			basePricePerVCPU = 0.025 // Memory-optimized
		} else if strings.Contains(size, "c") {
			basePricePerVCPU = 0.012 // Compute-optimized
		} else if strings.Contains(size, "i") || strings.Contains(size, "d") {
			basePricePerVCPU = 0.02 // Storage-optimized
		}
	}

	// Extract vCPU count from size
	vcpus := s.extractVCPUCount(vmSize)

	return basePricePerVCPU * float64(vcpus)
}

// extractVCPUCount extracts vCPU count from VM size name
func (s *SmartAZSelector) extractVCPUCount(vmSize string) int {
	size := strings.ToLower(vmSize)

	// Common patterns: d2s_v5, m5.xlarge, Standard_D2s_v5
	// Look for numbers
	vcpus := 2 // Default

	// Azure pattern: D2s_v5 -> 2 vCPUs
	for i, c := range size {
		if c >= '0' && c <= '9' {
			// Found a digit, extract the number
			num := 0
			for j := i; j < len(size) && size[j] >= '0' && size[j] <= '9'; j++ {
				num = num*10 + int(size[j]-'0')
			}
			if num > 0 && num <= 512 { // Reasonable vCPU range
				vcpus = num
				break
			}
		}
	}

	// AWS xlarge pattern
	if strings.Contains(size, "xlarge") {
		count := strings.Count(size, "x")
		if strings.Contains(size, "metal") {
			vcpus = 96 // Metal instances
		} else {
			vcpus = 4 * count // Each x = 4 vCPUs roughly
			if vcpus < 4 {
				vcpus = 4
			}
		}
	} else if strings.Contains(size, "large") {
		vcpus = 2
	} else if strings.Contains(size, "medium") {
		vcpus = 1
	} else if strings.Contains(size, "small") {
		vcpus = 1
	}

	return vcpus
}

// calculateZoneScore calculates the combined score for a zone
func (s *SmartAZSelector) calculateZoneScore(ctx context.Context, vmSize string, zone ZoneInfo, priceData map[string]*ZonePriceData, weights ScoreWeights) SmartAZRank {
	rank := SmartAZRank{
		Zone:       zone.Zone,
		Available:  zone.Available,
		Restricted: zone.Restricted,
	}

	// 1. Availability Score (0-100)
	if zone.Available && !zone.Restricted {
		rank.AvailabilityScore = 100
	} else if zone.Available && zone.Restricted {
		rank.AvailabilityScore = 50
	} else {
		rank.AvailabilityScore = 0
	}

	// 2. Capacity Score (0-100)
	rank.CapacityScore = float64(zone.CapacityScore)
	if s.capacityProvider != nil {
		if score, err := s.capacityProvider.GetCapacityScore(ctx, vmSize, zone.Zone); err == nil {
			rank.CapacityScore = float64(score)
		}
	}

	// 3. Price Score (0-100, lower price = higher score)
	if pd, ok := priceData[zone.Zone]; ok {
		rank.SpotPrice = pd.SpotPrice
		rank.OnDemandPrice = pd.OnDemandPrice
		rank.Volatility = pd.Volatility
		rank.PricePredicted = pd.Predicted

		// Normalize price score (assume $0.01 = 100, $1.00 = 0)
		if pd.SpotPrice > 0 {
			rank.PriceScore = math.Max(0, 100-(pd.SpotPrice*100))
			if rank.PriceScore > 100 {
				rank.PriceScore = 100
			}
		} else {
			rank.PriceScore = 50 // Unknown
		}

		// 4. Stability Score (0-100, lower volatility = higher score)
		rank.StabilityScore = math.Max(0, 100-(pd.Volatility*200))
		if rank.StabilityScore > 100 {
			rank.StabilityScore = 100
		}
	} else {
		rank.PriceScore = 50
		rank.StabilityScore = 50
	}

	// 5. Interruption Score (0-100, lower interruption = higher score)
	// Estimate interruption from capacity and price
	rank.InterruptionRate = s.estimateInterruptionRate(zone, priceData[zone.Zone])
	rank.InterruptionScore = math.Max(0, 100-(rank.InterruptionRate*5)) // 20% interruption = 0 score
	if rank.InterruptionScore > 100 {
		rank.InterruptionScore = 100
	}

	// Calculate combined score
	rank.CombinedScore =
		rank.AvailabilityScore*weights.Availability +
			rank.CapacityScore*weights.Capacity +
			rank.PriceScore*weights.Price +
			rank.StabilityScore*weights.Stability +
			rank.InterruptionScore*weights.Interruption

	// Generate explanation
	rank.Explanation = s.generateZoneExplanation(rank)

	return rank
}

// estimateInterruptionRate estimates interruption rate from available data
func (s *SmartAZSelector) estimateInterruptionRate(zone ZoneInfo, priceData *ZonePriceData) float64 {
	// Base interruption rate from capacity score
	// High capacity (100) = low interruption (5%)
	// Low capacity (0) = high interruption (20%)
	baseRate := 5.0 + (100.0-float64(zone.CapacityScore))*0.15

	// Adjust based on price volatility (high volatility = high interruption)
	if priceData != nil && priceData.Volatility > 0 {
		baseRate += priceData.Volatility * 20
	}

	// Cap at reasonable range
	if baseRate < 2 {
		baseRate = 2
	} else if baseRate > 25 {
		baseRate = 25
	}

	return baseRate
}

// generateZoneExplanation generates human-readable explanation for a zone
func (s *SmartAZSelector) generateZoneExplanation(rank SmartAZRank) string {
	parts := make([]string, 0)

	if !rank.Available {
		return "❌ VM not available in this zone"
	}

	if rank.Restricted {
		parts = append(parts, "⚠️ Restricted access")
	}

	if rank.CapacityScore >= 80 {
		parts = append(parts, "✓ High capacity")
	} else if rank.CapacityScore >= 50 {
		parts = append(parts, "○ Moderate capacity")
	} else {
		parts = append(parts, "⚠️ Limited capacity")
	}

	if rank.PricePredicted {
		parts = append(parts, fmt.Sprintf("💡 Predicted price: $%.4f", rank.SpotPrice))
	} else if rank.SpotPrice > 0 {
		parts = append(parts, fmt.Sprintf("💰 Spot: $%.4f", rank.SpotPrice))
	}

	if rank.InterruptionRate < 8 {
		parts = append(parts, "✓ Low interruption risk")
	} else if rank.InterruptionRate < 15 {
		parts = append(parts, "○ Moderate interruption risk")
	} else {
		parts = append(parts, "⚠️ High interruption risk")
	}

	return strings.Join(parts, " | ")
}

// calculateConfidence calculates confidence score based on data sources
func (s *SmartAZSelector) calculateConfidence(result *SmartAZResult) float64 {
	confidence := 0.3 // Base confidence

	// Add confidence for each data source
	if s.zoneProvider != nil && s.zoneProvider.IsAvailable() {
		confidence += 0.25
		result.DataSources = append(result.DataSources, "zone_availability_api")
	}

	if s.priceProvider != nil && s.priceProvider.IsAvailable() {
		confidence += 0.25
		result.DataSources = append(result.DataSources, "price_history_api")
	}

	if s.capacityProvider != nil {
		confidence += 0.2
		result.DataSources = append(result.DataSources, "capacity_api")
	}

	// Check if any prices were predicted
	predictedCount := 0
	for _, r := range result.Rankings {
		if r.PricePredicted {
			predictedCount++
		}
	}
	if predictedCount > 0 {
		// Reduce confidence if many prices predicted
		confidence -= float64(predictedCount) / float64(len(result.Rankings)) * 0.2
	}

	if confidence > 1.0 {
		confidence = 1.0
	} else if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// generateInsights generates actionable insights from the analysis
func (s *SmartAZSelector) generateInsights(result *SmartAZResult, weights ScoreWeights) []string {
	insights := make([]string, 0)

	if len(result.Rankings) == 0 {
		insights = append(insights, "⚠️ No zones available for this VM type")
		return insights
	}

	best := result.Rankings[0]

	// Primary recommendation
	insights = append(insights, fmt.Sprintf("🏆 Best zone: %s (score: %.1f/100)", best.Zone, best.CombinedScore))

	// Capacity insight
	if best.CapacityScore >= 80 {
		insights = append(insights, "✓ High capacity available in recommended zone")
	} else if best.CapacityScore < 50 {
		insights = append(insights, "⚠️ Limited capacity - consider alternative VM sizes")
	}

	// Price insight
	if best.PricePredicted {
		insights = append(insights, fmt.Sprintf("💡 Spot price predicted at $%.4f/hour (no historical data available)", best.SpotPrice))
	} else if best.SpotPrice > 0 {
		savings := (best.OnDemandPrice - best.SpotPrice) / best.OnDemandPrice * 100
		insights = append(insights, fmt.Sprintf("💰 Estimated savings: %.0f%% vs on-demand", savings))
	}

	// Interruption insight
	if best.InterruptionRate < 8 {
		insights = append(insights, fmt.Sprintf("✓ Low interruption risk (~%.0f%%)", best.InterruptionRate))
	} else if best.InterruptionRate >= 15 {
		insights = append(insights, fmt.Sprintf("⚠️ High interruption risk (~%.0f%%) - use Spot Blocks if available", best.InterruptionRate))
	}

	// Alternative zone comparison
	if len(result.Rankings) > 1 {
		second := result.Rankings[1]
		scoreDiff := best.CombinedScore - second.CombinedScore
		if scoreDiff < 5 {
			insights = append(insights, fmt.Sprintf("ℹ️ %s is a close alternative (score: %.1f)", second.Zone, second.CombinedScore))
		}
	}

	// Confidence insight
	if result.Confidence < 0.5 {
		insights = append(insights, "⚠️ Low confidence - limited data available for this VM type")
	} else if result.Confidence >= 0.8 {
		insights = append(insights, "✓ High confidence recommendation based on multiple data sources")
	}

	return insights
}
