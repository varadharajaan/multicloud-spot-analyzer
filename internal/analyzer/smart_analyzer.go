// Package analyzer implements intelligent spot instance analysis.
// It uses a multi-factor scoring algorithm to recommend optimal instances
// based on user requirements, spot pricing, interruption rates, and instance specs.
package analyzer

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spot-analyzer/internal/domain"
)

// Weights for the scoring algorithm - these can be tuned based on analysis
const (
	// Primary factors
	SavingsWeight   = 0.30 // How much savings vs on-demand
	StabilityWeight = 0.25 // Low interruption rate
	FitnessWeight   = 0.25 // How well it matches requirements
	ValueWeight     = 0.20 // Performance per cost ratio

	// Penalty factors
	GenerationPenaltyMax = 0.15 // Max penalty for older generations
	BurstablePenalty     = 0.10 // Penalty for burstable instances (if not preferred)

	// Score thresholds
	MinimumViableScore = 0.3 // Minimum score to consider an instance
)

// SmartAnalyzer implements domain.InstanceAnalyzer with intelligent scoring
type SmartAnalyzer struct {
	spotProvider  domain.SpotDataProvider
	specsProvider domain.InstanceSpecsProvider
	filter        domain.InstanceFilter
	recommender   domain.RecommendationEngine
	mu            sync.RWMutex
}

// NewSmartAnalyzer creates a new intelligent analyzer
func NewSmartAnalyzer(
	spotProvider domain.SpotDataProvider,
	specsProvider domain.InstanceSpecsProvider,
) *SmartAnalyzer {
	analyzer := &SmartAnalyzer{
		spotProvider:  spotProvider,
		specsProvider: specsProvider,
	}
	analyzer.filter = NewSmartFilter()
	analyzer.recommender = NewRecommendationEngine()
	return analyzer
}

// Analyze performs comprehensive analysis on spot instances
func (a *SmartAnalyzer) Analyze(ctx context.Context, requirements domain.UsageRequirements) (*domain.AnalysisResult, error) {
	// Validate requirements
	if err := a.validateRequirements(requirements); err != nil {
		return nil, domain.NewAnalysisError("validation", err)
	}

	// Fetch spot data for the region
	spotDataList, err := a.spotProvider.FetchSpotData(ctx, requirements.Region, requirements.OS)
	if err != nil {
		return nil, domain.NewAnalysisError("fetch_spot_data", err)
	}

	// Create spot data lookup map
	spotDataMap := make(map[string]domain.SpotData)
	for _, spot := range spotDataList {
		spotDataMap[spot.InstanceType] = spot
	}

	// Get all instance specs
	allSpecs, err := a.specsProvider.GetAllInstanceSpecs(ctx)
	if err != nil {
		return nil, domain.NewAnalysisError("fetch_specs", err)
	}

	// Enrich specs with derived instances from spot data
	allSpecs = a.enrichSpecsFromSpotData(ctx, allSpecs, spotDataMap)

	// Filter instances based on requirements
	filteredSpecs := a.filter.Filter(allSpecs, spotDataMap, requirements)
	filteredOut := len(allSpecs) - len(filteredSpecs)

	// Score and rank instances
	analyses := make([]domain.InstanceAnalysis, 0, len(filteredSpecs))
	for _, spec := range filteredSpecs {
		spot, exists := spotDataMap[spec.InstanceType]
		if !exists {
			continue
		}

		score, breakdown := a.ScoreInstance(spec, spot, requirements)
		if score < MinimumViableScore {
			continue
		}

		analysis := domain.InstanceAnalysis{
			Specs:          spec,
			SpotData:       spot,
			Score:          score,
			ScoreBreakdown: breakdown,
		}

		// Generate recommendations and warnings
		analysis.Recommendation = a.recommender.GenerateRecommendation(analysis, requirements)
		analysis.Warnings = a.recommender.GenerateWarnings(analysis, requirements)

		analyses = append(analyses, analysis)
	}

	// Sort by score descending
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].Score > analyses[j].Score
	})

	// Apply ranking
	for i := range analyses {
		analyses[i].Rank = i + 1
	}

	// Limit to top N
	topN := requirements.TopN
	if topN <= 0 {
		topN = 10
	}
	if len(analyses) > topN {
		analyses = analyses[:topN]
	}

	return &domain.AnalysisResult{
		Requirements:  requirements,
		TopInstances:  analyses,
		TotalAnalyzed: len(spotDataList),
		FilteredOut:   filteredOut,
		AnalyzedAt:    time.Now(),
		Region:        requirements.Region,
		CloudProvider: domain.AWS,
	}, nil
}

// ScoreInstance calculates a composite score for an instance
func (a *SmartAnalyzer) ScoreInstance(
	specs domain.InstanceSpecs,
	spot domain.SpotData,
	requirements domain.UsageRequirements,
) (float64, domain.ScoreBreakdown) {
	breakdown := domain.ScoreBreakdown{}

	// 1. Savings Score (0-1): Higher savings = better
	// Normalize savings to 0-1 range (assuming max realistic savings is ~90%)
	breakdown.SavingsScore = math.Min(float64(spot.SavingsPercent)/90.0, 1.0)

	// 2. Stability Score (0-1): Lower interruption = better
	// Inverse mapping: 0 (VeryLow) = 1.0, 4 (VeryHigh) = 0.2
	stabilityMap := map[domain.InterruptionFrequency]float64{
		domain.VeryLow:  1.0,
		domain.Low:      0.8,
		domain.Medium:   0.6,
		domain.High:     0.4,
		domain.VeryHigh: 0.2,
	}
	breakdown.StabilityScore = stabilityMap[spot.InterruptionFrequency]

	// 3. Fitness Score (0-1): How well does it match requirements
	breakdown.FitnessScore = a.calculateFitnessScore(specs, requirements)

	// 4. Value Score (0-1): Performance efficiency
	breakdown.ValueScore = a.calculateValueScore(specs, spot, requirements)

	// 5. Generation Penalty: Older generations get penalized
	breakdown.GenerationPenalty = a.calculateGenerationPenalty(specs)

	// Calculate weighted total score
	totalScore := (breakdown.SavingsScore * SavingsWeight) +
		(breakdown.StabilityScore * StabilityWeight) +
		(breakdown.FitnessScore * FitnessWeight) +
		(breakdown.ValueScore * ValueWeight) -
		breakdown.GenerationPenalty

	// Apply burstable penalty if not explicitly allowed
	if specs.IsBurstable && !requirements.AllowBurstable {
		totalScore -= BurstablePenalty
	}

	// Ensure score is within bounds
	totalScore = math.Max(0, math.Min(1, totalScore))

	return totalScore, breakdown
}

// calculateFitnessScore evaluates how well an instance matches requirements
func (a *SmartAnalyzer) calculateFitnessScore(specs domain.InstanceSpecs, requirements domain.UsageRequirements) float64 {
	score := 1.0

	// vCPU fitness: Exact match is best, slight over-provisioning is okay
	if requirements.MinVCPU > 0 {
		vcpuRatio := float64(specs.VCPU) / float64(requirements.MinVCPU)
		if vcpuRatio < 1.0 {
			// Under-provisioned: severe penalty
			score *= vcpuRatio * 0.5
		} else if vcpuRatio <= 1.5 {
			// Slight over-provisioning: ideal
			score *= 1.0
		} else if vcpuRatio <= 2.0 {
			// Moderate over-provisioning: slight penalty
			score *= 0.9
		} else {
			// Significant over-provisioning: larger penalty (but not too harsh)
			score *= 0.8 / math.Log2(vcpuRatio)
		}
	}

	// MaxVCPU check
	if requirements.MaxVCPU > 0 && specs.VCPU > requirements.MaxVCPU {
		score *= 0.5 // Penalty for exceeding max
	}

	// Memory fitness (if specified)
	if requirements.MinMemoryGB > 0 {
		memRatio := specs.MemoryGB / requirements.MinMemoryGB
		if memRatio < 1.0 {
			score *= memRatio * 0.5
		} else if memRatio <= 2.0 {
			score *= 1.0
		} else {
			score *= 0.9
		}
	}

	// Category preference
	if requirements.PreferredCategory != "" && specs.Category == requirements.PreferredCategory {
		score *= 1.1 // Bonus for matching category
	}

	// Architecture preference
	if requirements.Architecture != "" && specs.Architecture != requirements.Architecture {
		score *= 0.7 // Penalty for architecture mismatch
	}

	// GPU matching
	if requirements.RequiresGPU {
		if !specs.HasGPU {
			return 0 // Disqualify non-GPU instances when GPU is required
		}
		if requirements.MinGPUCount > 0 && specs.GPUCount < requirements.MinGPUCount {
			score *= float64(specs.GPUCount) / float64(requirements.MinGPUCount)
		}
	}

	return math.Max(0, math.Min(1, score))
}

// calculateValueScore evaluates the performance-to-cost ratio
func (a *SmartAnalyzer) calculateValueScore(specs domain.InstanceSpecs, spot domain.SpotData, requirements domain.UsageRequirements) float64 {
	// Higher vCPU count with higher savings = better value
	// Use log scale to avoid heavily favoring huge instances
	vcpuFactor := math.Log2(float64(specs.VCPU)+1) / math.Log2(float64(requirements.MinVCPU)+1)

	// Combine with savings
	savingsFactor := float64(spot.SavingsPercent) / 100.0

	// Memory efficiency
	memFactor := 1.0
	if requirements.MinMemoryGB > 0 {
		memFactor = math.Min(specs.MemoryGB/requirements.MinMemoryGB, 2.0) / 2.0
	}

	// Value = normalized combination of factors
	value := (vcpuFactor*0.4 + savingsFactor*0.4 + memFactor*0.2)

	return math.Max(0, math.Min(1, value))
}

// calculateGenerationPenalty returns a penalty for older instance generations
func (a *SmartAnalyzer) calculateGenerationPenalty(specs domain.InstanceSpecs) float64 {
	switch specs.Generation {
	case domain.Current:
		return 0
	case domain.Previous:
		return GenerationPenaltyMax * 0.3
	case domain.Legacy:
		return GenerationPenaltyMax * 0.7
	case domain.Deprecated:
		return GenerationPenaltyMax
	default:
		return GenerationPenaltyMax * 0.5
	}
}

// validateRequirements validates the usage requirements
func (a *SmartAnalyzer) validateRequirements(req domain.UsageRequirements) error {
	if req.MinVCPU <= 0 {
		return domain.NewValidationError("min_vcpu", "must be greater than 0")
	}
	if req.Region == "" {
		return domain.NewValidationError("region", "must be specified")
	}
	if req.MaxVCPU > 0 && req.MaxVCPU < req.MinVCPU {
		return domain.NewValidationError("max_vcpu", "must be >= min_vcpu")
	}
	if req.TopN <= 0 {
		req.TopN = 10
	}
	return nil
}

// enrichSpecsFromSpotData adds specs for instances found in spot data but not in catalog
func (a *SmartAnalyzer) enrichSpecsFromSpotData(
	ctx context.Context,
	specs []domain.InstanceSpecs,
	spotData map[string]domain.SpotData,
) []domain.InstanceSpecs {
	specsMap := make(map[string]bool)
	for _, s := range specs {
		specsMap[s.InstanceType] = true
	}

	for instanceType := range spotData {
		if !specsMap[instanceType] {
			// Try to derive specs from instance type name
			derivedSpec, err := a.specsProvider.GetInstanceSpecs(ctx, instanceType)
			if err == nil && derivedSpec != nil {
				specs = append(specs, *derivedSpec)
				specsMap[instanceType] = true
			}
		}
	}

	return specs
}
