// Package logging provides structured JSON logging with file and console output.
// Designed for analysis with Athena, BigQuery, or other query engines.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func (l Level) Color() string {
	switch l {
	case DEBUG:
		return "\033[36m" // Cyan
	case INFO:
		return "\033[32m" // Green
	case WARN:
		return "\033[33m" // Yellow
	case ERROR:
		return "\033[31m" // Red
	default:
		return "\033[0m"
	}
}

// LogEntry represents a structured log entry for JSON output
// Schema designed for Athena/BigQuery analysis
type LogEntry struct {
	Timestamp    string                 `json:"timestamp"`               // ISO 8601 format
	Level        string                 `json:"level"`                   // DEBUG, INFO, WARN, ERROR
	Message      string                 `json:"message"`                 // Log message
	Caller       string                 `json:"caller"`                  // file:line
	Function     string                 `json:"function"`                // Function name
	Component    string                 `json:"component"`               // Component name (web, cli, analyzer, provider)
	RequestID    string                 `json:"request_id,omitempty"`    // For request tracing
	DurationMs   float64                `json:"duration_ms,omitempty"`   // Operation duration in ms
	Region       string                 `json:"region,omitempty"`        // AWS/Azure/GCP region
	Provider     string                 `json:"provider,omitempty"`      // Cloud provider
	InstanceType string                 `json:"instance_type,omitempty"` // Instance type being analyzed
	ErrorMsg     string                 `json:"error,omitempty"`         // Error message if any
	Count        int                    `json:"count,omitempty"`         // Count of items (instances, etc.)
	Fields       map[string]interface{} `json:"fields,omitempty"`        // Additional structured fields
	Version      string                 `json:"version"`                 // Application version
	Hostname     string                 `json:"hostname"`                // Machine hostname
}

// Logger provides structured JSON logging
type Logger struct {
	mu          sync.Mutex
	level       Level
	output      io.Writer
	fileOutput  *os.File
	jsonOutput  *os.File // Separate JSON log file
	enableColor bool
	enableJSON  bool
	logDir      string
	component   string
	hostname    string
	version     string
}

// Config holds logger configuration
type Config struct {
	Level       Level
	LogDir      string // Directory for log files
	EnableFile  bool   // Write to file (human-readable)
	EnableJSON  bool   // Write JSON logs (for Athena/BigQuery)
	EnableColor bool   // Color console output
	Component   string // Component name for log entries
	Version     string // Application version
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Level:       INFO,
		LogDir:      "logs",
		EnableFile:  true,
		EnableJSON:  true, // Enable JSON by default
		EnableColor: true,
		Component:   "spot-analyzer",
		Version:     "1.0.0",
	}
}

// New creates a new logger with the given config
func New(cfg Config) (*Logger, error) {
	hostname, _ := os.Hostname()

	l := &Logger{
		level:       cfg.Level,
		output:      os.Stdout,
		enableColor: cfg.EnableColor,
		enableJSON:  cfg.EnableJSON,
		logDir:      cfg.LogDir,
		component:   cfg.Component,
		hostname:    hostname,
		version:     cfg.Version,
	}

	if cfg.EnableFile || cfg.EnableJSON {
		if err := l.setupFileLogging(cfg.LogDir, cfg.EnableFile, cfg.EnableJSON); err != nil {
			return nil, err
		}
	}

	return l, nil
}

// GetDefault returns the default logger, initializing it if needed
func GetDefault() *Logger {
	once.Do(func() {
		var err error
		defaultLogger, err = New(DefaultConfig())
		if err != nil {
			// Fallback to console-only logging
			defaultLogger = &Logger{
				level:       INFO,
				output:      os.Stdout,
				enableColor: true,
				component:   "spot-analyzer",
				version:     "1.0.0",
			}
		}
	})
	return defaultLogger
}

// SetDefault sets the default logger
func SetDefault(l *Logger) {
	defaultLogger = l
}

func (l *Logger) setupFileLogging(logDir string, enableFile, enableJSON bool) error {
	// Create log directory
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02")

	// Human-readable log file
	if enableFile {
		logFile := filepath.Join(logDir, fmt.Sprintf("spot-analyzer-%s.log", timestamp))
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		l.fileOutput = f
	}

	// JSON log file (JSONL format - one JSON object per line)
	if enableJSON {
		jsonFile := filepath.Join(logDir, fmt.Sprintf("spot-analyzer-%s.jsonl", timestamp))
		f, err := os.OpenFile(jsonFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open JSON log file: %w", err)
		}
		l.jsonOutput = f
	}

	l.logDir = logDir
	return nil
}

// Close closes the log files
func (l *Logger) Close() error {
	var errs []error
	if l.fileOutput != nil {
		if err := l.fileOutput.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if l.jsonOutput != nil {
		if err := l.jsonOutput.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// SetLevel changes the log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// WithComponent returns a new logger with the specified component
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		level:       l.level,
		output:      l.output,
		fileOutput:  l.fileOutput,
		jsonOutput:  l.jsonOutput,
		enableColor: l.enableColor,
		enableJSON:  l.enableJSON,
		logDir:      l.logDir,
		component:   component,
		hostname:    l.hostname,
		version:     l.version,
	}
}

// log writes a log entry
func (l *Logger) log(level Level, msg string, args ...interface{}) {
	l.logWithFields(level, nil, msg, args...)
}

// logWithFields writes a log entry with additional structured fields
func (l *Logger) logWithFields(level Level, fields map[string]interface{}, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	timestamp := now.Format(time.RFC3339Nano)
	humanTime := now.Format("2006-01-02 15:04:05.000")

	// Get caller info
	pc, file, line, ok := runtime.Caller(2)
	caller := "unknown"
	funcName := "unknown"
	if ok {
		// Get just the filename
		parts := strings.Split(file, "/")
		if len(parts) > 0 {
			caller = fmt.Sprintf("%s:%d", parts[len(parts)-1], line)
		}
		// Get function name
		if fn := runtime.FuncForPC(pc); fn != nil {
			fnParts := strings.Split(fn.Name(), ".")
			if len(fnParts) > 0 {
				funcName = fnParts[len(fnParts)-1]
			}
		}
	}

	// Format message
	formattedMsg := msg
	if len(args) > 0 {
		formattedMsg = fmt.Sprintf(msg, args...)
	}

	// Write JSON log entry
	if l.jsonOutput != nil {
		entry := LogEntry{
			Timestamp: timestamp,
			Level:     level.String(),
			Message:   formattedMsg,
			Caller:    caller,
			Function:  funcName,
			Component: l.component,
			Version:   l.version,
			Hostname:  l.hostname,
			Fields:    fields,
		}

		// Extract known fields
		if fields != nil {
			if v, ok := fields["request_id"].(string); ok {
				entry.RequestID = v
			}
			if v, ok := fields["duration_ms"].(float64); ok {
				entry.DurationMs = v
			}
			if v, ok := fields["region"].(string); ok {
				entry.Region = v
			}
			if v, ok := fields["provider"].(string); ok {
				entry.Provider = v
			}
			if v, ok := fields["instance_type"].(string); ok {
				entry.InstanceType = v
			}
			if v, ok := fields["error"].(string); ok {
				entry.ErrorMsg = v
			}
			if v, ok := fields["count"].(int); ok {
				entry.Count = v
			}
		}

		jsonBytes, err := json.Marshal(entry)
		if err == nil {
			l.jsonOutput.Write(jsonBytes)
			l.jsonOutput.WriteString("\n")
		}
	}

	// Plain log entry for file
	plainEntry := fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n", humanTime, level.String(), l.component, caller, formattedMsg)

	// Write to human-readable file
	if l.fileOutput != nil {
		l.fileOutput.WriteString(plainEntry)
	}

	// Colored entry for console
	if l.enableColor {
		colorEntry := fmt.Sprintf("%s[%s]%s [%s] [%s] [%s] %s\n",
			level.Color(), level.String(), "\033[0m", humanTime, l.component, caller, formattedMsg)
		l.output.Write([]byte(colorEntry))
	} else {
		l.output.Write([]byte(plainEntry))
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(DEBUG, msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(INFO, msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(WARN, msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(ERROR, msg, args...)
}

// Package-level convenience functions using default logger

// Debug logs a debug message using the default logger
func Debug(msg string, args ...interface{}) {
	GetDefault().Debug(msg, args...)
}

// Info logs an info message using the default logger
func Info(msg string, args ...interface{}) {
	GetDefault().Info(msg, args...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, args ...interface{}) {
	GetDefault().Warn(msg, args...)
}

// Error logs an error message using the default logger
func Error(msg string, args ...interface{}) {
	GetDefault().Error(msg, args...)
}

// Fields is a map of structured fields
type Fields map[string]interface{}

// WithFields returns a field logger for structured logging
func (l *Logger) WithFields(fields Fields) *FieldLogger {
	return &FieldLogger{logger: l, fields: fields}
}

// WithFields returns a field logger using the default logger
func WithFields(fields Fields) *FieldLogger {
	return GetDefault().WithFields(fields)
}

// FieldLogger provides structured field logging
type FieldLogger struct {
	logger *Logger
	fields Fields
}

func (fl *FieldLogger) Debug(msg string, args ...interface{}) {
	fl.logger.logWithFields(DEBUG, fl.fields, msg, args...)
}

func (fl *FieldLogger) Info(msg string, args ...interface{}) {
	fl.logger.logWithFields(INFO, fl.fields, msg, args...)
}

func (fl *FieldLogger) Warn(msg string, args ...interface{}) {
	fl.logger.logWithFields(WARN, fl.fields, msg, args...)
}

func (fl *FieldLogger) Error(msg string, args ...interface{}) {
	fl.logger.logWithFields(ERROR, fl.fields, msg, args...)
}

// ===== Athena/BigQuery Helper Functions =====

// LogAnalysis logs an analysis operation with full context
func LogAnalysis(region, provider string, instanceCount int, duration time.Duration) {
	WithFields(Fields{
		"region":      region,
		"provider":    provider,
		"count":       instanceCount,
		"duration_ms": float64(duration.Milliseconds()),
	}).Info("Analysis completed")
}

// LogRequest logs an HTTP request with context
func LogRequest(method, path, requestID string, duration time.Duration, statusCode int) {
	WithFields(Fields{
		"request_id":  requestID,
		"method":      method,
		"path":        path,
		"status_code": statusCode,
		"duration_ms": float64(duration.Milliseconds()),
	}).Info("HTTP request")
}

// LogInstanceRecommendation logs a specific instance recommendation
func LogInstanceRecommendation(instanceType, region string, score float64, savings int) {
	WithFields(Fields{
		"instance_type": instanceType,
		"region":        region,
		"score":         score,
		"savings":       savings,
	}).Info("Instance recommended")
}

// LogError logs an error with context
func LogError(operation string, err error, fields Fields) {
	if fields == nil {
		fields = Fields{}
	}
	fields["error"] = err.Error()
	fields["operation"] = operation
	WithFields(fields).Error("Operation failed: %s", operation)
}
