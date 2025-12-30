// Package logging provides structured logging with file and console output.
package logging

import (
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

// Logger provides structured logging
type Logger struct {
	mu          sync.Mutex
	level       Level
	output      io.Writer
	fileOutput  *os.File
	enableColor bool
	logDir      string
}

// Config holds logger configuration
type Config struct {
	Level       Level
	LogDir      string // Directory for log files
	EnableFile  bool   // Write to file
	EnableColor bool   // Color console output
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
		EnableColor: true,
	}
}

// New creates a new logger with the given config
func New(cfg Config) (*Logger, error) {
	l := &Logger{
		level:       cfg.Level,
		output:      os.Stdout,
		enableColor: cfg.EnableColor,
		logDir:      cfg.LogDir,
	}

	if cfg.EnableFile {
		if err := l.setupFileLogging(cfg.LogDir); err != nil {
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
			}
		}
	})
	return defaultLogger
}

// SetDefault sets the default logger
func SetDefault(l *Logger) {
	defaultLogger = l
}

func (l *Logger) setupFileLogging(logDir string) error {
	// Create log directory
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02")
	logFile := filepath.Join(logDir, fmt.Sprintf("spot-analyzer-%s.log", timestamp))

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.fileOutput = f
	l.logDir = logDir
	return nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.fileOutput != nil {
		return l.fileOutput.Close()
	}
	return nil
}

// SetLevel changes the log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log writes a log entry
func (l *Logger) log(level Level, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Format("2006-01-02 15:04:05.000")

	// Get caller info
	_, file, line, ok := runtime.Caller(2)
	caller := "unknown"
	if ok {
		// Get just the filename
		parts := strings.Split(file, "/")
		if len(parts) > 0 {
			caller = fmt.Sprintf("%s:%d", parts[len(parts)-1], line)
		}
	}

	// Format message
	formattedMsg := msg
	if len(args) > 0 {
		formattedMsg = fmt.Sprintf(msg, args...)
	}

	// Plain log entry for file
	plainEntry := fmt.Sprintf("[%s] [%s] [%s] %s\n", now, level.String(), caller, formattedMsg)

	// Write to file
	if l.fileOutput != nil {
		l.fileOutput.WriteString(plainEntry)
	}

	// Colored entry for console
	if l.enableColor {
		colorEntry := fmt.Sprintf("%s[%s]%s [%s] [%s] %s\n",
			level.Color(), level.String(), "\033[0m", now, caller, formattedMsg)
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

// WithFields returns a field logger for structured logging
type Fields map[string]interface{}

// WithFields logs with additional context fields
func (l *Logger) WithFields(fields Fields) *FieldLogger {
	return &FieldLogger{logger: l, fields: fields}
}

// FieldLogger provides structured field logging
type FieldLogger struct {
	logger *Logger
	fields Fields
}

func (fl *FieldLogger) formatFields() string {
	if len(fl.fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fl.fields))
	for k, v := range fl.fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return " " + strings.Join(parts, " ")
}

func (fl *FieldLogger) Debug(msg string, args ...interface{}) {
	fl.logger.log(DEBUG, msg+fl.formatFields(), args...)
}

func (fl *FieldLogger) Info(msg string, args ...interface{}) {
	fl.logger.log(INFO, msg+fl.formatFields(), args...)
}

func (fl *FieldLogger) Warn(msg string, args ...interface{}) {
	fl.logger.log(WARN, msg+fl.formatFields(), args...)
}

func (fl *FieldLogger) Error(msg string, args ...interface{}) {
	fl.logger.log(ERROR, msg+fl.formatFields(), args...)
}
