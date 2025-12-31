// Package azure implements the price history adapter for Azure.
// This adapter translates Azure price data to the analyzer interface.
package azure

import (
	"context"
	"time"

	"github.com/spot-analyzer/internal/analyzer"
)

// PriceHistoryAdapter adapts the Azure PriceHistoryProvider to the analyzer.PriceHistoryProvider interface
type PriceHistoryAdapter struct {
	provider *PriceHistoryProvider
}

// NewPriceHistoryAdapter wraps a PriceHistoryProvider to implement analyzer.PriceHistoryProvider
func NewPriceHistoryAdapter(provider *PriceHistoryProvider) *PriceHistoryAdapter {
	return &PriceHistoryAdapter{provider: provider}
}

// IsAvailable returns true if Azure pricing is available
func (a *PriceHistoryAdapter) IsAvailable() bool {
	return a.provider.IsAvailable()
}

// GetPriceAnalysis fetches and analyzes prices, converting to analyzer types
func (a *PriceHistoryAdapter) GetPriceAnalysis(ctx context.Context, vmSize string, lookbackDays int) (*analyzer.PriceAnalysis, error) {
	azureAnalysis, err := a.provider.GetPriceAnalysis(ctx, vmSize, lookbackDays)
	if err != nil || azureAnalysis == nil {
		return nil, err
	}
	return convertToAnalyzerPriceAnalysis(azureAnalysis), nil
}

// GetBatchPriceAnalysis fetches analysis for multiple VM sizes
func (a *PriceHistoryAdapter) GetBatchPriceAnalysis(ctx context.Context, vmSizes []string, lookbackDays int) (map[string]*analyzer.PriceAnalysis, error) {
	azureResults, err := a.provider.GetBatchPriceAnalysis(ctx, vmSizes, lookbackDays)
	if err != nil || azureResults == nil {
		return nil, err
	}

	results := make(map[string]*analyzer.PriceAnalysis)
	for k, v := range azureResults {
		results[k] = convertToAnalyzerPriceAnalysis(v)
	}
	return results, nil
}

func convertToAnalyzerPriceAnalysis(azure *PriceAnalysis) *analyzer.PriceAnalysis {
	if azure == nil {
		return nil
	}
	return &analyzer.PriceAnalysis{
		InstanceType:     azure.InstanceType,
		AvailabilityZone: azure.AvailabilityZone,
		CurrentPrice:     azure.CurrentPrice,
		AvgPrice:         azure.AvgPrice,
		MinPrice:         azure.MinPrice,
		MaxPrice:         azure.MaxPrice,
		StdDev:           azure.StdDev,
		Volatility:       azure.Volatility,
		TrendSlope:       azure.TrendSlope,
		TrendScore:       azure.TrendScore,
		DataPoints:       azure.DataPoints,
		TimeSpanHours:    azure.TimeSpanHours,
		HourlyPattern:    copyIntFloatMap(azure.HourlyPattern),
		WeekdayPattern:   copyWeekdayFloatMap(azure.WeekdayPattern),
		LastUpdated:      azure.LastUpdated,
		AllAZData:        convertAZData(azure.AllAZData),
	}
}

func convertAZData(src map[string]*AZAnalysis) map[string]*analyzer.AZAnalysis {
	if src == nil {
		return nil
	}
	dst := make(map[string]*analyzer.AZAnalysis, len(src))
	for k, v := range src {
		dst[k] = &analyzer.AZAnalysis{
			AvailabilityZone: v.AvailabilityZone,
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
