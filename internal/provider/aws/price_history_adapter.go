package aws

import (
	"context"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
)

// PriceHistoryAdapter adapts the AWS PriceHistoryProvider to the analyzer.PriceHistoryProvider interface
type PriceHistoryAdapter struct {
	provider *PriceHistoryProvider
}

// NewPriceHistoryAdapter wraps a PriceHistoryProvider to implement analyzer.PriceHistoryProvider
func NewPriceHistoryAdapter(provider *PriceHistoryProvider) *PriceHistoryAdapter {
	return &PriceHistoryAdapter{provider: provider}
}

// IsAvailable returns true if AWS credentials are configured and working
func (a *PriceHistoryAdapter) IsAvailable() bool {
	return a.provider.IsAvailable()
}

// GetPriceAnalysis fetches and analyzes historical prices, converting to analyzer types
func (a *PriceHistoryAdapter) GetPriceAnalysis(ctx context.Context, instanceType string, lookbackDays int) (*analyzer.PriceAnalysis, error) {
	awsAnalysis, err := a.provider.GetPriceAnalysis(ctx, instanceType, lookbackDays)
	if err != nil || awsAnalysis == nil {
		return nil, err
	}
	return convertToAnalyzerPriceAnalysis(awsAnalysis), nil
}

// GetBatchPriceAnalysis fetches analysis for multiple instance types
func (a *PriceHistoryAdapter) GetBatchPriceAnalysis(ctx context.Context, instanceTypes []string, lookbackDays int) (map[string]*analyzer.PriceAnalysis, error) {
	awsResults, err := a.provider.GetBatchPriceAnalysis(ctx, instanceTypes, lookbackDays)
	if err != nil || awsResults == nil {
		return nil, err
	}

	results := make(map[string]*analyzer.PriceAnalysis)
	for k, v := range awsResults {
		results[k] = convertToAnalyzerPriceAnalysis(v)
	}
	return results, nil
}

func convertToAnalyzerPriceAnalysis(aws *PriceAnalysis) *analyzer.PriceAnalysis {
	if aws == nil {
		return nil
	}
	return &analyzer.PriceAnalysis{
		InstanceType:     aws.InstanceType,
		AvailabilityZone: aws.AvailabilityZone,
		CurrentPrice:     aws.CurrentPrice,
		AvgPrice:         aws.AvgPrice,
		MinPrice:         aws.MinPrice,
		MaxPrice:         aws.MaxPrice,
		StdDev:           aws.StdDev,
		Volatility:       aws.Volatility,
		TrendSlope:       aws.TrendSlope,
		TrendScore:       aws.TrendScore,
		DataPoints:       aws.DataPoints,
		TimeSpanHours:    aws.TimeSpanHours,
		HourlyPattern:    copyIntFloatMap(aws.HourlyPattern),
		WeekdayPattern:   copyWeekdayFloatMap(aws.WeekdayPattern),
		LastUpdated:      aws.LastUpdated,
	}
}

func copyIntFloatMap(src map[int]float64) map[int]float64 {
	if src == nil {
		return nil
	}
	dst := make(map[int]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyWeekdayFloatMap(src map[time.Weekday]float64) map[time.Weekday]float64 {
	if src == nil {
		return nil
	}
	dst := make(map[time.Weekday]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
