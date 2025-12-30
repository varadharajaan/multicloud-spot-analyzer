// Package domain contains custom error types for the application.
package domain

import (
	"errors"
	"fmt"
)

// Base errors
var (
	ErrNotFound           = errors.New("resource not found")
	ErrInvalidInput       = errors.New("invalid input")
	ErrProviderUnavailable = errors.New("cloud provider unavailable")
	ErrRateLimited        = errors.New("rate limited")
	ErrNetworkError       = errors.New("network error")
	ErrParseError         = errors.New("parse error")
	ErrUnsupportedProvider = errors.New("unsupported cloud provider")
	ErrUnsupportedRegion  = errors.New("unsupported region")
)

// SpotDataError represents errors when fetching spot data
type SpotDataError struct {
	Provider  CloudProvider
	Region    string
	Operation string
	Err       error
}

func (e *SpotDataError) Error() string {
	return fmt.Sprintf("spot data error [provider=%s, region=%s, operation=%s]: %v",
		e.Provider, e.Region, e.Operation, e.Err)
}

func (e *SpotDataError) Unwrap() error {
	return e.Err
}

// NewSpotDataError creates a new SpotDataError
func NewSpotDataError(provider CloudProvider, region, operation string, err error) *SpotDataError {
	return &SpotDataError{
		Provider:  provider,
		Region:    region,
		Operation: operation,
		Err:       err,
	}
}

// InstanceSpecsError represents errors when fetching instance specifications
type InstanceSpecsError struct {
	InstanceType string
	Err          error
}

func (e *InstanceSpecsError) Error() string {
	return fmt.Sprintf("instance specs error [type=%s]: %v", e.InstanceType, e.Err)
}

func (e *InstanceSpecsError) Unwrap() error {
	return e.Err
}

// NewInstanceSpecsError creates a new InstanceSpecsError
func NewInstanceSpecsError(instanceType string, err error) *InstanceSpecsError {
	return &InstanceSpecsError{
		InstanceType: instanceType,
		Err:          err,
	}
}

// ValidationError represents input validation errors
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error [field=%s]: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// AnalysisError represents errors during instance analysis
type AnalysisError struct {
	Phase string
	Err   error
}

func (e *AnalysisError) Error() string {
	return fmt.Sprintf("analysis error [phase=%s]: %v", e.Phase, e.Err)
}

func (e *AnalysisError) Unwrap() error {
	return e.Err
}

// NewAnalysisError creates a new AnalysisError
func NewAnalysisError(phase string, err error) *AnalysisError {
	return &AnalysisError{
		Phase: phase,
		Err:   err,
	}
}
