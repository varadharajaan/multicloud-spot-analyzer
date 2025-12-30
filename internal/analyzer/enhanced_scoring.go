// Package analyzer provides enhanced scoring strategies for spot instance analysis.
// This file implements advanced scoring algorithms that go beyond basic AWS Spot Advisor data.
package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
)

// EnhancedScoringStrategy defines the interface for advanced scoring algorithms
type EnhancedScoringStrategy interface {
	// Name returns the strategy identifier
	Name() string

	// ComputeEnhancedScore calculates additional score factors
	ComputeEnhancedScore(ctx context.Context, instance *domain.InstanceAnalysis, requirements domain.UsageRequirements) (*EnhancedScoreFactors, error)
}

// EnhancedScoreFactors contains additional scoring factors beyond basic analysis
type EnhancedScoreFactors struct {
	// VolatilityScore: 0.0 (very volatile) to 1.0 (very stable pricing)
	VolatilityScore float64 `json:"volatility_score"`

	// TrendScore: 0.0 (getting worse) to 1.0 (getting better/stable)
	TrendScore float64 `json:"trend_score"`

	// CapacityPoolScore: 0.0 (limited pools) to 1.0 (many AZ options)
	CapacityPoolScore float64 `json:"capacity_pool_score"`

	// TimePatternScore: 0.0 (high time variance) to 1.0 (consistent availability)
	TimePatternScore float64 `json:"time_pattern_score"`

	// PopularityScore: 0.0 (over-used) to 1.0 (under-utilized hidden gem)
	PopularityScore float64 `json:"popularity_score"`

	// CombinedEnhancedScore: Weighted combination of all factors
	CombinedEnhancedScore float64 `json:"combined_enhanced_score"`

	// Insights generated from the analysis
	Insights []string `json:"insights"`
}

// HistoricalPriceStrategy analyzes historical spot price data from AWS
// to compute volatility and trend scores using real AWS DescribeSpotPriceHistory API
type HistoricalPriceStrategy struct {
	httpClient    *http.Client
	priceProvider PriceHistoryProvider
	region        string
	useRealData   bool
}

// PriceHistoryProvider interface for AWS price history data
type PriceHistoryProvider interface {
	IsAvailable() bool
	GetPriceAnalysis(ctx context.Context, instanceType string, lookbackDays int) (*PriceAnalysis, error)
	GetBatchPriceAnalysis(ctx context.Context, instanceTypes []string, lookbackDays int) (map[string]*PriceAnalysis, error)
}

// PriceAnalysis contains computed metrics from historical price data
type PriceAnalysis struct {
	InstanceType     string
	AvailabilityZone string
	CurrentPrice     float64
	AvgPrice         float64
	MinPrice         float64
	MaxPrice         float64
	StdDev           float64
	Volatility       float64 // StdDev / AvgPrice (coefficient of variation)
	TrendSlope       float64 // Positive = prices rising, Negative = falling
	TrendScore       float64 // Normalized -1 to 1
	DataPoints       int
	TimeSpanHours    float64
	HourlyPattern    map[int]float64          // Hour -> Avg price
	WeekdayPattern   map[time.Weekday]float64 // Weekday -> Avg price
	LastUpdated      time.Time
	AllAZData        map[string]*AZAnalysis // All availability zone data
}

// AZAnalysis contains per-AZ price analysis
type AZAnalysis struct {
	AvailabilityZone string
	AvgPrice         float64
	MinPrice         float64
	MaxPrice         float64
	Volatility       float64
	DataPoints       int
}

// NewHistoricalPriceStrategy creates a new historical price analyzer
func NewHistoricalPriceStrategy() *HistoricalPriceStrategy {
	return &HistoricalPriceStrategy{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		useRealData: false,
	}
}

// NewHistoricalPriceStrategyWithAWS creates a strategy with real AWS price history
func NewHistoricalPriceStrategyWithAWS(provider PriceHistoryProvider, region string) *HistoricalPriceStrategy {
	return &HistoricalPriceStrategy{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		priceProvider: provider,
		region:        region,
		useRealData:   provider != nil && provider.IsAvailable(),
	}
}

// IsUsingRealData returns true if real AWS price history is being used
func (s *HistoricalPriceStrategy) IsUsingRealData() bool {
	return s.useRealData
}

func (s *HistoricalPriceStrategy) Name() string {
	return "historical_price_analysis"
}

// ComputeEnhancedScore calculates enhanced scoring factors
// Uses real AWS DescribeSpotPriceHistory when credentials are available,
// otherwise falls back to heuristics based on instance characteristics
func (s *HistoricalPriceStrategy) ComputeEnhancedScore(
	ctx context.Context,
	instance *domain.InstanceAnalysis,
	requirements domain.UsageRequirements,
) (*EnhancedScoreFactors, error) {
	factors := &EnhancedScoreFactors{
		Insights: make([]string, 0),
	}

	// Try to get real price history data
	var priceAnalysis *PriceAnalysis
	if s.useRealData && s.priceProvider != nil {
		analysis, err := s.priceProvider.GetPriceAnalysis(ctx, instance.Specs.InstanceType, 7) // 7 days lookback
		if err == nil && analysis != nil {
			priceAnalysis = analysis
		}
	}

	// Compute scores - use real data if available, otherwise fall back to heuristics
	if priceAnalysis != nil {
		factors.VolatilityScore = s.computeVolatilityScoreReal(priceAnalysis)
		factors.TrendScore = s.computeTrendScoreReal(priceAnalysis)
		factors.TimePatternScore = s.computeTimePatternScoreReal(priceAnalysis)
		factors.CapacityPoolScore = s.computeCapacityPoolScoreReal(priceAnalysis, requirements.Region)
		factors.Insights = append(factors.Insights, "📊 REAL DATA: Using actual AWS price history for analysis")
	} else {
		factors.VolatilityScore = s.computeVolatilityScore(instance)
		factors.TrendScore = s.computeTrendScore(instance)
		factors.TimePatternScore = s.computeTimePatternScore(instance)
		factors.CapacityPoolScore = s.computeCapacityPoolScore(instance, requirements.Region)
	}

	// Popularity score uses Spot Advisor data (always available)
	factors.PopularityScore = s.computePopularityScore(instance)

	// Combine all factors with weights
	factors.CombinedEnhancedScore = s.combineFactors(factors)

	// Generate additional insights
	factors.Insights = append(factors.Insights, s.generateInsights(instance, factors)...)

	return factors, nil
}

// computeVolatilityScoreReal calculates volatility from actual price history
func (s *HistoricalPriceStrategy) computeVolatilityScoreReal(analysis *PriceAnalysis) float64 {
	// Volatility (coefficient of variation): lower = better
	// Typical range: 0.01 (very stable) to 0.5+ (very volatile)
	if analysis.Volatility <= 0.05 {
		return 0.95 // Very stable
	} else if analysis.Volatility <= 0.10 {
		return 0.85
	} else if analysis.Volatility <= 0.20 {
		return 0.70
	} else if analysis.Volatility <= 0.35 {
		return 0.50
	}
	return 0.30 // Very volatile
}

// computeTrendScoreReal calculates trend from actual price history
func (s *HistoricalPriceStrategy) computeTrendScoreReal(analysis *PriceAnalysis) float64 {
	// TrendScore from price history: negative slope = prices falling = good
	// Positive slope = prices rising = bad
	// Range: -1 to 1, invert for our score (0 to 1)
	score := 0.5 - (analysis.TrendScore * 0.4) // Invert: falling prices = higher score
	return clamp(score, 0.0, 1.0)
}

// computeTimePatternScoreReal analyzes actual time-based patterns
func (s *HistoricalPriceStrategy) computeTimePatternScoreReal(analysis *PriceAnalysis) float64 {
	if len(analysis.HourlyPattern) == 0 {
		return 0.6 // Default
	}

	// Calculate variance in hourly prices - lower variance = more consistent
	var prices []float64
	for _, price := range analysis.HourlyPattern {
		prices = append(prices, price)
	}

	if len(prices) < 2 {
		return 0.6
	}

	// Calculate coefficient of variation for hourly prices
	avg := 0.0
	for _, p := range prices {
		avg += p
	}
	avg /= float64(len(prices))

	variance := 0.0
	for _, p := range prices {
		diff := p - avg
		variance += diff * diff
	}
	variance /= float64(len(prices))
	stdDev := math.Sqrt(variance)

	cv := 0.0
	if avg > 0 {
		cv = stdDev / avg
	}

	// Lower CV = more consistent across time
	if cv <= 0.02 {
		return 0.95
	} else if cv <= 0.05 {
		return 0.80
	} else if cv <= 0.10 {
		return 0.65
	}
	return 0.50
}

// computeCapacityPoolScoreReal uses real AZ data from price history
func (s *HistoricalPriceStrategy) computeCapacityPoolScoreReal(analysis *PriceAnalysis, region string) float64 {
	score := 0.7 // Base score

	// If we have AZ data, use it
	if analysis.AvailabilityZone != "" {
		score += 0.1 // We have AZ-specific data
	}

	// More data points suggests better availability
	if analysis.DataPoints >= 500 {
		score += 0.15
	} else if analysis.DataPoints >= 100 {
		score += 0.10
	} else if analysis.DataPoints >= 50 {
		score += 0.05
	}

	return clamp(score, 0.0, 1.0)
}

// computeVolatilityScore estimates price volatility based on instance characteristics
// Newer generations and less popular instance families tend to have more stable pricing
func (s *HistoricalPriceStrategy) computeVolatilityScore(instance *domain.InstanceAnalysis) float64 {
	score := 0.7 // Base score

	// Current generation instances have more predictable pricing
	switch instance.Specs.Generation {
	case domain.Current:
		score += 0.2
	case domain.Previous:
		score += 0.1
	case domain.Legacy:
		score -= 0.1
	case domain.Deprecated:
		score -= 0.3
	}

	// Larger instances tend to have more stable pricing (less competition)
	if instance.Specs.VCPU >= 16 {
		score += 0.1
	} else if instance.Specs.VCPU <= 2 {
		score -= 0.1 // Small instances are more competitive
	}

	// Specialized instances (storage, memory optimized) often more stable
	switch instance.Specs.Category {
	case domain.StorageOptimized:
		score += 0.1
	case domain.MemoryOptimized:
		score += 0.05
	}

	// ARM instances tend to be less volatile (less mainstream adoption)
	if instance.Specs.Architecture == "arm64" {
		score += 0.1
	}

	return clamp(score, 0.0, 1.0)
}

// computeTrendScore estimates if an instance type is becoming more or less popular
// Based on instance generation and family patterns
func (s *HistoricalPriceStrategy) computeTrendScore(instance *domain.InstanceAnalysis) float64 {
	score := 0.5 // Neutral base

	// Newer generations trending upward (more people discovering them)
	// but current gen with low interruption = stable good choice
	if instance.Specs.Generation == domain.Current && instance.SpotData.InterruptionFrequency <= 1 {
		score = 0.85 // Stable, good trend
	} else if instance.Specs.Generation == domain.Current {
		score = 0.7 // Good but some competition
	} else if instance.Specs.Generation == domain.Previous {
		score = 0.6 // People migrating away - could be opportunity
	}

	// High interruption rate suggests increasing demand (negative trend for stability)
	if instance.SpotData.InterruptionFrequency >= 3 {
		score -= 0.2
	}

	return clamp(score, 0.0, 1.0)
}

// computeCapacityPoolScore estimates availability across multiple AZs
// More AZ options = better for fault tolerance
func (s *HistoricalPriceStrategy) computeCapacityPoolScore(instance *domain.InstanceAnalysis, region string) float64 {
	score := 0.7 // Base score

	// Mainstream instance families available in more AZs
	family := extractInstanceFamily(instance.Specs.InstanceType)
	mainstreamFamilies := map[string]bool{
		"m5": true, "m6i": true, "m6a": true, "m7i": true,
		"c5": true, "c6i": true, "c6a": true, "c7i": true,
		"r5": true, "r6i": true, "r6a": true, "r7i": true,
	}

	if mainstreamFamilies[family] {
		score += 0.2 // Available in most AZs
	}

	// Current gen ARM instances have growing availability
	if instance.Specs.Architecture == "arm64" && instance.Specs.Generation == domain.Current {
		score += 0.1
	}

	// Large regions have more AZ options
	largeRegions := map[string]bool{
		"us-east-1": true, "us-west-2": true, "eu-west-1": true,
		"eu-central-1": true, "ap-northeast-1": true,
	}
	if largeRegions[region] {
		score += 0.1
	}

	return clamp(score, 0.0, 1.0)
}

// computeTimePatternScore estimates consistency across time periods
// Some instances have predictable weekend/night patterns
func (s *HistoricalPriceStrategy) computeTimePatternScore(instance *domain.InstanceAnalysis) float64 {
	score := 0.6 // Base score

	// Low interruption rate suggests consistent availability
	switch instance.SpotData.InterruptionFrequency {
	case 0:
		score = 0.95 // Very consistent
	case 1:
		score = 0.85
	case 2:
		score = 0.70
	case 3:
		score = 0.50
	case 4:
		score = 0.30 // High variance likely
	}

	return score
}

// computePopularityScore identifies "hidden gems" - underutilized instances
// Lower popularity = less competition = better availability
func (s *HistoricalPriceStrategy) computePopularityScore(instance *domain.InstanceAnalysis) float64 {
	// High savings + low interruption = hidden gem (underutilized)
	savingsScore := float64(instance.SpotData.SavingsPercent) / 100.0
	stabilityScore := 1.0 - float64(instance.SpotData.InterruptionFrequency)/4.0

	// Hidden gem formula: high savings AND high stability = underutilized
	if savingsScore >= 0.7 && stabilityScore >= 0.75 {
		return 0.95 // True hidden gem!
	} else if savingsScore >= 0.6 && stabilityScore >= 0.75 {
		return 0.85
	} else if stabilityScore >= 0.75 {
		return 0.7 // Stable but popular (lower savings)
	}

	return 0.5 // Average
}

// combineFactors weights and combines all enhanced scoring factors
func (s *HistoricalPriceStrategy) combineFactors(factors *EnhancedScoreFactors) float64 {
	// Weights for enhanced factors
	const (
		volatilityWeight   = 0.25
		trendWeight        = 0.20
		capacityPoolWeight = 0.20
		timePatternWeight  = 0.20
		popularityWeight   = 0.15
	)

	combined := factors.VolatilityScore*volatilityWeight +
		factors.TrendScore*trendWeight +
		factors.CapacityPoolScore*capacityPoolWeight +
		factors.TimePatternScore*timePatternWeight +
		factors.PopularityScore*popularityWeight

	return combined
}

// generateInsights creates human-readable insights from the analysis
func (s *HistoricalPriceStrategy) generateInsights(
	instance *domain.InstanceAnalysis,
	factors *EnhancedScoreFactors,
) []string {
	insights := make([]string, 0)

	// Volatility insights
	if factors.VolatilityScore >= 0.85 {
		insights = append(insights, "💎 STABLE PRICING: This instance has very predictable spot pricing with minimal fluctuations")
	} else if factors.VolatilityScore <= 0.5 {
		insights = append(insights, "⚠️ VOLATILE PRICING: Expect price fluctuations; consider setting a max price limit")
	}

	// Hidden gem identification
	if factors.PopularityScore >= 0.9 {
		insights = append(insights, "🌟 HIDDEN GEM: High savings + low interruption suggests this instance is underutilized - excellent choice!")
	}

	// Trend insights
	if factors.TrendScore >= 0.8 {
		insights = append(insights, "📈 POSITIVE TREND: This instance type maintains stable availability over time")
	} else if factors.TrendScore <= 0.4 {
		insights = append(insights, "📉 INCREASING COMPETITION: This instance type is becoming more popular, expect more interruptions")
	}

	// Capacity pool insights
	if factors.CapacityPoolScore >= 0.85 {
		insights = append(insights, "🌐 MULTI-AZ READY: Widely available across availability zones for high availability setups")
	}

	// Time pattern insights
	if factors.TimePatternScore >= 0.9 {
		insights = append(insights, "⏰ TIME-STABLE: Consistent availability regardless of time of day or week")
	} else if factors.TimePatternScore <= 0.5 {
		insights = append(insights, "🕐 TIME-SENSITIVE: Consider scheduling workloads during off-peak hours (nights/weekends)")
	}

	// Combined recommendation
	if factors.CombinedEnhancedScore >= 0.85 {
		insights = append(insights, "🏆 TOP PICK: All factors indicate this is an excellent spot instance choice")
	} else if factors.CombinedEnhancedScore >= 0.7 {
		insights = append(insights, "✅ RECOMMENDED: Good overall profile for spot workloads")
	} else if factors.CombinedEnhancedScore <= 0.5 {
		insights = append(insights, "⚡ USE WITH CAUTION: Consider instance diversification strategy")
	}

	return insights
}

// EnhancedAnalyzer wraps SmartAnalyzer with additional scoring strategies
type EnhancedAnalyzer struct {
	*SmartAnalyzer
	strategies    []EnhancedScoringStrategy
	usingRealData bool
}

// NewEnhancedAnalyzer creates an analyzer with advanced scoring capabilities (heuristics only)
func NewEnhancedAnalyzer(
	spotProvider domain.SpotDataProvider,
	specsProvider domain.InstanceSpecsProvider,
) *EnhancedAnalyzer {
	return &EnhancedAnalyzer{
		SmartAnalyzer: NewSmartAnalyzer(spotProvider, specsProvider),
		strategies: []EnhancedScoringStrategy{
			NewHistoricalPriceStrategy(),
		},
		usingRealData: false,
	}
}

// NewEnhancedAnalyzerWithPriceHistory creates an analyzer with real AWS price history
func NewEnhancedAnalyzerWithPriceHistory(
	spotProvider domain.SpotDataProvider,
	specsProvider domain.InstanceSpecsProvider,
	priceProvider PriceHistoryProvider,
	region string,
) *EnhancedAnalyzer {
	strategy := NewHistoricalPriceStrategyWithAWS(priceProvider, region)
	return &EnhancedAnalyzer{
		SmartAnalyzer: NewSmartAnalyzer(spotProvider, specsProvider),
		strategies: []EnhancedScoringStrategy{
			strategy,
		},
		usingRealData: strategy.IsUsingRealData(),
	}
}

// IsUsingRealData returns true if real AWS price history is being used
func (a *EnhancedAnalyzer) IsUsingRealData() bool {
	return a.usingRealData
}

// AnalyzeEnhanced performs analysis with additional enhanced scoring
func (a *EnhancedAnalyzer) AnalyzeEnhanced(
	ctx context.Context,
	requirements domain.UsageRequirements,
) (*EnhancedAnalysisResult, error) {
	// First, run the basic analysis
	basicResult, err := a.Analyze(ctx, requirements)
	if err != nil {
		return nil, err
	}

	// Enhance each ranked instance with additional scoring - in parallel
	enhancedInstances := make([]*EnhancedRankedInstance, len(basicResult.TopInstances))

	var wg sync.WaitGroup
	// Use a semaphore to limit concurrent AWS API calls
	sem := make(chan struct{}, 10) // Max 10 concurrent price fetches

	for i := range basicResult.TopInstances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			instance := &basicResult.TopInstances[idx]
			enhanced := &EnhancedRankedInstance{
				InstanceAnalysis: instance,
				EnhancedFactors:  make(map[string]*EnhancedScoreFactors),
			}

			// Apply each strategy
			for _, strategy := range a.strategies {
				factors, err := strategy.ComputeEnhancedScore(ctx, instance, requirements)
				if err != nil {
					continue // Skip failed strategies
				}
				enhanced.EnhancedFactors[strategy.Name()] = factors
			}

			// Compute final enhanced score
			enhanced.FinalScore = a.computeFinalScore(instance, enhanced.EnhancedFactors)
			enhanced.AllInsights = a.aggregateInsights(enhanced.EnhancedFactors)

			enhancedInstances[idx] = enhanced
		}(i)
	}

	wg.Wait()

	// Re-sort by final enhanced score
	sort.Slice(enhancedInstances, func(i, j int) bool {
		return enhancedInstances[i].FinalScore > enhancedInstances[j].FinalScore
	})

	// Update ranks
	for i := range enhancedInstances {
		enhancedInstances[i].InstanceAnalysis.Rank = i + 1
	}

	return &EnhancedAnalysisResult{
		AnalysisResult:    basicResult,
		EnhancedInstances: enhancedInstances,
		ScoringStrategies: a.getStrategyNames(),
	}, nil
}

// computeFinalScore combines basic score with enhanced factors
func (a *EnhancedAnalyzer) computeFinalScore(
	instance *domain.InstanceAnalysis,
	enhancedFactors map[string]*EnhancedScoreFactors,
) float64 {
	// Base score gets 60% weight, enhanced factors get 40%
	baseWeight := 0.60
	enhancedWeight := 0.40

	enhancedScore := 0.0
	count := 0
	for _, factors := range enhancedFactors {
		enhancedScore += factors.CombinedEnhancedScore
		count++
	}
	if count > 0 {
		enhancedScore /= float64(count)
	}

	return instance.Score*baseWeight + enhancedScore*enhancedWeight
}

// aggregateInsights combines insights from all strategies
func (a *EnhancedAnalyzer) aggregateInsights(factors map[string]*EnhancedScoreFactors) []string {
	insights := make([]string, 0)
	for _, f := range factors {
		insights = append(insights, f.Insights...)
	}
	return insights
}

func (a *EnhancedAnalyzer) getStrategyNames() []string {
	names := make([]string, len(a.strategies))
	for i, s := range a.strategies {
		names[i] = s.Name()
	}
	return names
}

// EnhancedRankedInstance extends InstanceAnalysis with enhanced scoring
type EnhancedRankedInstance struct {
	*domain.InstanceAnalysis
	EnhancedFactors map[string]*EnhancedScoreFactors `json:"enhanced_factors"`
	FinalScore      float64                          `json:"final_score"`
	AllInsights     []string                         `json:"all_insights"`
}

// EnhancedAnalysisResult extends AnalysisResult with enhanced data
type EnhancedAnalysisResult struct {
	*domain.AnalysisResult
	EnhancedInstances []*EnhancedRankedInstance `json:"enhanced_instances"`
	ScoringStrategies []string                  `json:"scoring_strategies"`
}

// Utility functions
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func extractInstanceFamily(instanceType string) string {
	// Extract family like "m5" from "m5.large"
	for i, c := range instanceType {
		if c == '.' {
			return instanceType[:i]
		}
	}
	return instanceType
}

// SpotPriceHistoryFetcher can fetch real historical data from AWS
// This is a placeholder for production implementation
type SpotPriceHistoryFetcher struct {
	// Would use AWS SDK v2 ec2.DescribeSpotPriceHistory
}

// FetchPriceHistory fetches 90 days of spot price history
// In production, this would call AWS API
func (f *SpotPriceHistoryFetcher) FetchPriceHistory(
	ctx context.Context,
	region string,
	instanceType string,
	startTime time.Time,
	endTime time.Time,
) ([]SpotPricePoint, error) {
	// Placeholder - would use:
	// ec2Client.DescribeSpotPriceHistory(ctx, &ec2.DescribeSpotPriceHistoryInput{
	//     InstanceTypes: []types.InstanceType{types.InstanceType(instanceType)},
	//     StartTime:     aws.Time(startTime),
	//     EndTime:       aws.Time(endTime),
	//     ProductDescriptions: []string{"Linux/UNIX"},
	// })
	return nil, fmt.Errorf("not implemented - requires AWS credentials")
}

// SpotPricePoint represents a single price data point
type SpotPricePoint struct {
	Timestamp    time.Time `json:"timestamp"`
	Price        float64   `json:"price"`
	InstanceType string    `json:"instance_type"`
	AZ           string    `json:"availability_zone"`
}

// PriceStatistics computed from historical data
type PriceStatistics struct {
	Mean            float64   `json:"mean"`
	StdDev          float64   `json:"std_dev"`
	Min             float64   `json:"min"`
	Max             float64   `json:"max"`
	Trend           float64   `json:"trend"` // Positive = increasing, negative = decreasing
	VolatilityIndex float64   `json:"volatility_index"`
	PricePoints     int       `json:"price_points"`
	AnalyzedFrom    time.Time `json:"analyzed_from"`
	AnalyzedTo      time.Time `json:"analyzed_to"`
}

// ComputeStatistics calculates statistics from price history
func ComputeStatistics(prices []SpotPricePoint) *PriceStatistics {
	if len(prices) == 0 {
		return nil
	}

	stats := &PriceStatistics{
		PricePoints:  len(prices),
		AnalyzedFrom: prices[0].Timestamp,
		AnalyzedTo:   prices[len(prices)-1].Timestamp,
	}

	// Calculate mean
	sum := 0.0
	stats.Min = prices[0].Price
	stats.Max = prices[0].Price
	for _, p := range prices {
		sum += p.Price
		if p.Price < stats.Min {
			stats.Min = p.Price
		}
		if p.Price > stats.Max {
			stats.Max = p.Price
		}
	}
	stats.Mean = sum / float64(len(prices))

	// Calculate standard deviation
	varSum := 0.0
	for _, p := range prices {
		diff := p.Price - stats.Mean
		varSum += diff * diff
	}
	stats.StdDev = math.Sqrt(varSum / float64(len(prices)))

	// Calculate volatility index (coefficient of variation)
	if stats.Mean > 0 {
		stats.VolatilityIndex = stats.StdDev / stats.Mean
	}

	// Calculate trend using simple linear regression slope
	if len(prices) >= 2 {
		stats.Trend = calculateTrend(prices)
	}

	return stats
}

// calculateTrend computes a simple linear regression slope
func calculateTrend(prices []SpotPricePoint) float64 {
	n := float64(len(prices))
	if n < 2 {
		return 0
	}

	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i, p := range prices {
		x := float64(i)
		y := p.Price
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Slope = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

// VantageInstanceData represents enriched instance data from instances.vantage.sh
type VantageInstanceData struct {
	InstanceType     string  `json:"instance_type"`
	OnDemandPrice    float64 `json:"on_demand_price"`
	SpotPrice        float64 `json:"spot_price"`
	VCPU             int     `json:"vcpu"`
	MemoryGB         float64 `json:"memory_gb"`
	NetworkBandwidth string  `json:"network_bandwidth"`
	Category         string  `json:"category"`
}

// FetchVantageData fetches enriched instance data from Vantage
// This provides additional pricing context
func FetchVantageData(ctx context.Context, region string) ([]VantageInstanceData, error) {
	// instances.vantage.sh provides a JSON API
	url := fmt.Sprintf("https://instances.vantage.sh/instances.json?region=%s", region)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vantage API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data []VantageInstanceData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return data, nil
}
