// Package gcp provides the price history adapter for the analyzer interface.
package gcp

import (
	"context"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
)

// PriceHistoryAdapter adapts GCP price history provider to the analyzer interface
type PriceHistoryAdapter struct {
	provider *PriceHistoryProvider
}

// NewPriceHistoryAdapter creates a new price history adapter
func NewPriceHistoryAdapter(provider *PriceHistoryProvider) *PriceHistoryAdapter {
	return &PriceHistoryAdapter{
		provider: provider,
	}
}

// IsAvailable returns true if the provider is available
func (a *PriceHistoryAdapter) IsAvailable() bool {
	return a.provider.IsAvailable()
}

// GetPriceAnalysis fetches and analyzes prices, converting to analyzer types
func (a *PriceHistoryAdapter) GetPriceAnalysis(ctx context.Context, machineType string, lookbackDays int) (*analyzer.PriceAnalysis, error) {
	gcpAnalysis, err := a.provider.GetPriceAnalysis(ctx, machineType, lookbackDays)
	if err != nil || gcpAnalysis == nil {
		return nil, err
	}
	return convertToAnalyzerPriceAnalysis(gcpAnalysis), nil
}

// GetBatchPriceAnalysis fetches analysis for multiple machine types
func (a *PriceHistoryAdapter) GetBatchPriceAnalysis(ctx context.Context, machineTypes []string, lookbackDays int) (map[string]*analyzer.PriceAnalysis, error) {
	gcpResults, err := a.provider.GetBatchPriceAnalysis(ctx, machineTypes, lookbackDays)
	if err != nil || gcpResults == nil {
		return nil, err
	}

	results := make(map[string]*analyzer.PriceAnalysis)
	for k, v := range gcpResults {
		results[k] = convertToAnalyzerPriceAnalysis(v)
	}
	return results, nil
}

func convertToAnalyzerPriceAnalysis(gcp *PriceAnalysis) *analyzer.PriceAnalysis {
	if gcp == nil {
		return nil
	}
	return &analyzer.PriceAnalysis{
		InstanceType:     gcp.InstanceType,
		AvailabilityZone: gcp.Zone,
		CurrentPrice:     gcp.CurrentPrice,
		AvgPrice:         gcp.AvgPrice,
		MinPrice:         gcp.MinPrice,
		MaxPrice:         gcp.MaxPrice,
		StdDev:           gcp.StdDev,
		Volatility:       gcp.Volatility,
		TrendSlope:       gcp.TrendSlope,
		TrendScore:       gcp.TrendScore,
		DataPoints:       gcp.DataPoints,
		TimeSpanHours:    gcp.TimeSpanHours,
		HourlyPattern:    copyIntFloatMap(gcp.HourlyPattern),
		WeekdayPattern:   copyWeekdayFloatMap(gcp.WeekdayPattern),
		LastUpdated:      gcp.LastUpdated,
		AllAZData:        convertAZData(gcp.AllZoneData),
		UsingRealSKUData: gcp.UsingRealSKUData,
	}
}

func convertAZData(src map[string]*ZoneAnalysis) map[string]*analyzer.AZAnalysis {
	if src == nil {
		return nil
	}
	dst := make(map[string]*analyzer.AZAnalysis, len(src))
	for k, v := range src {
		dst[k] = &analyzer.AZAnalysis{
			AvailabilityZone: v.Zone,
			AvgPrice:         v.AvgPrice,
			MinPrice:         v.MinPrice,
			MaxPrice:         v.MaxPrice,
			Volatility:       v.Volatility,
			DataPoints:       v.DataPoints,
		}
	}
	return dst
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
