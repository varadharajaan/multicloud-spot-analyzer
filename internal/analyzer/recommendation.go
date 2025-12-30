// Package analyzer implements recommendation generation logic.
package analyzer

import (
	"fmt"
	"strings"

	"github.com/spot-analyzer/internal/domain"
)

// RecommendationEngine implements domain.RecommendationEngine
type RecommendationEngine struct{}

// NewRecommendationEngine creates a new recommendation engine
func NewRecommendationEngine() *RecommendationEngine {
	return &RecommendationEngine{}
}

// GenerateRecommendation creates a recommendation string for an instance
func (e *RecommendationEngine) GenerateRecommendation(
	analysis domain.InstanceAnalysis,
	requirements domain.UsageRequirements,
) string {
	var parts []string

	// Score-based recommendation level
	switch {
	case analysis.Score >= 0.85:
		parts = append(parts, "Excellent choice")
	case analysis.Score >= 0.70:
		parts = append(parts, "Good choice")
	case analysis.Score >= 0.55:
		parts = append(parts, "Reasonable choice")
	case analysis.Score >= 0.40:
		parts = append(parts, "Acceptable")
	default:
		parts = append(parts, "Consider alternatives")
	}

	// Add specific insights
	insights := e.generateInsights(analysis, requirements)
	if len(insights) > 0 {
		parts = append(parts, strings.Join(insights, "; "))
	}

	return strings.Join(parts, " - ")
}

// generateInsights creates specific insights about the instance
func (e *RecommendationEngine) generateInsights(
	analysis domain.InstanceAnalysis,
	requirements domain.UsageRequirements,
) []string {
	insights := make([]string, 0)
	breakdown := analysis.ScoreBreakdown

	// Savings insight
	if analysis.SpotData.SavingsPercent >= 80 {
		insights = append(insights, fmt.Sprintf("exceptional savings of %d%%", analysis.SpotData.SavingsPercent))
	} else if analysis.SpotData.SavingsPercent >= 60 {
		insights = append(insights, fmt.Sprintf("good savings of %d%%", analysis.SpotData.SavingsPercent))
	}

	// Stability insight
	switch analysis.SpotData.InterruptionFrequency {
	case domain.VeryLow:
		insights = append(insights, "very stable (<5% interruption)")
	case domain.Low:
		insights = append(insights, "stable (5-10% interruption)")
	case domain.Medium:
		insights = append(insights, "moderate stability (10-15% interruption)")
	}

	// Generation insight
	if analysis.Specs.Generation == domain.Current {
		insights = append(insights, "current generation hardware")
	}

	// Architecture insight
	if analysis.Specs.Architecture == "arm64" {
		insights = append(insights, "ARM-based (Graviton) - excellent price/performance")
	}

	// Category alignment
	if requirements.PreferredCategory != "" && analysis.Specs.Category == requirements.PreferredCategory {
		insights = append(insights, "matches preferred category")
	}

	// Value score insight
	if breakdown.ValueScore >= 0.8 {
		insights = append(insights, "excellent value proposition")
	}

	// Capacity fit
	vcpuRatio := float64(analysis.Specs.VCPU) / float64(requirements.MinVCPU)
	if vcpuRatio >= 1.0 && vcpuRatio <= 1.25 {
		insights = append(insights, "optimal sizing")
	} else if vcpuRatio > 2.0 {
		insights = append(insights, "over-provisioned (consider smaller)")
	}

	return insights
}

// GenerateWarnings identifies potential issues with an instance choice
func (e *RecommendationEngine) GenerateWarnings(
	analysis domain.InstanceAnalysis,
	requirements domain.UsageRequirements,
) []string {
	warnings := make([]string, 0)

	// High interruption warning
	if analysis.SpotData.InterruptionFrequency >= domain.High {
		warnings = append(warnings, fmt.Sprintf(
			"High interruption frequency (%s) - ensure fault-tolerant workload design",
			analysis.SpotData.InterruptionFrequency.String(),
		))
	}

	// Low savings warning
	if analysis.SpotData.SavingsPercent < 30 {
		warnings = append(warnings, fmt.Sprintf(
			"Low savings (%d%%) - consider on-demand for more predictable workloads",
			analysis.SpotData.SavingsPercent,
		))
	}

	// Generation warnings
	if analysis.Specs.Generation == domain.Previous {
		warnings = append(warnings, "Previous generation instance - newer options may offer better performance")
	} else if analysis.Specs.Generation == domain.Legacy {
		warnings = append(warnings, "Legacy generation instance - consider upgrading to current generation")
	}

	// Burstable warning
	if analysis.Specs.IsBurstable {
		warnings = append(warnings, "Burstable instance - performance may be throttled under sustained load")
	}

	// Over-provisioning warning
	vcpuRatio := float64(analysis.Specs.VCPU) / float64(requirements.MinVCPU)
	if vcpuRatio > 3.0 {
		warnings = append(warnings, fmt.Sprintf(
			"Significantly over-provisioned (%.1fx required vCPU) - consider smaller instance",
			vcpuRatio,
		))
	}

	// Memory imbalance warning
	if requirements.MinMemoryGB > 0 {
		memRatio := analysis.Specs.MemoryGB / requirements.MinMemoryGB
		if memRatio > 4.0 {
			warnings = append(warnings, "Significant memory over-provisioning")
		} else if memRatio < 1.5 && memRatio >= 1.0 {
			warnings = append(warnings, "Memory headroom is tight - consider next size up for safety margin")
		}
	}

	// ARM architecture warning
	if analysis.Specs.Architecture == "arm64" && requirements.Architecture == "" {
		warnings = append(warnings, "ARM64 architecture - verify application compatibility before deployment")
	}

	// Bare metal warning
	if analysis.Specs.IsBareMetal {
		warnings = append(warnings, "Bare metal instance - longer provisioning time, verify use case requires it")
	}

	return warnings
}

// GenerateSummary creates a summary of the analysis results
func (e *RecommendationEngine) GenerateSummary(result *domain.AnalysisResult) string {
	if len(result.TopInstances) == 0 {
		return "No suitable instances found matching your requirements. Try relaxing constraints."
	}

	top := result.TopInstances[0]
	summary := fmt.Sprintf(
		"Top recommendation: %s (%d vCPU, %.0f GB RAM) with %d%% savings and %s interruption rate. Score: %.2f",
		top.Specs.InstanceType,
		top.Specs.VCPU,
		top.Specs.MemoryGB,
		top.SpotData.SavingsPercent,
		top.SpotData.InterruptionFrequency.String(),
		top.Score,
	)

	return summary
}

// CompareInstances generates a comparison between two instances
func (e *RecommendationEngine) CompareInstances(a, b domain.InstanceAnalysis) string {
	comparison := fmt.Sprintf(
		"%s vs %s:\n"+
			"  vCPU: %d vs %d\n"+
			"  Memory: %.0f GB vs %.0f GB\n"+
			"  Savings: %d%% vs %d%%\n"+
			"  Interruption: %s vs %s\n"+
			"  Score: %.2f vs %.2f\n",
		a.Specs.InstanceType, b.Specs.InstanceType,
		a.Specs.VCPU, b.Specs.VCPU,
		a.Specs.MemoryGB, b.Specs.MemoryGB,
		a.SpotData.SavingsPercent, b.SpotData.SavingsPercent,
		a.SpotData.InterruptionFrequency.String(), b.SpotData.InterruptionFrequency.String(),
		a.Score, b.Score,
	)

	if a.Score > b.Score {
		comparison += fmt.Sprintf("  Recommendation: Choose %s", a.Specs.InstanceType)
	} else {
		comparison += fmt.Sprintf("  Recommendation: Choose %s", b.Specs.InstanceType)
	}

	return comparison
}
