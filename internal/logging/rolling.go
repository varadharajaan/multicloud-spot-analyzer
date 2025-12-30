// Package logging provides rolling log file support.
package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RollingConfig configures rolling log behavior
type RollingConfig struct {
	MaxSize     int64  // Max size in bytes before rotation (default: 10MB)
	MaxAge      int    // Max days to keep old logs (default: 7)
	MaxBackups  int    // Max number of old logs to keep (default: 5)
	Compress    bool   // Compress rotated logs (default: true)
	BaseName    string // Base log file name
	LogDir      string // Directory for logs
	TimePattern string // Time pattern for rotation (default: "2006-01-02")
}

// DefaultRollingConfig returns sensible defaults
func DefaultRollingConfig() RollingConfig {
	return RollingConfig{
		MaxSize:     10 * 1024 * 1024, // 10MB
		MaxAge:      7,                // 7 days
		MaxBackups:  5,                // 5 backups
		Compress:    true,
		BaseName:    "spot-analyzer",
		LogDir:      "logs",
		TimePattern: "2006-01-02",
	}
}

// RollingWriter provides rolling log file functionality
type RollingWriter struct {
	mu          sync.Mutex
	config      RollingConfig
	currentFile *os.File
	currentSize int64
	currentDate string
	isJSON      bool
}

// NewRollingWriter creates a new rolling log writer
func NewRollingWriter(cfg RollingConfig, isJSON bool) (*RollingWriter, error) {
	rw := &RollingWriter{
		config: cfg,
		isJSON: isJSON,
	}

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open initial file
	if err := rw.openNewFile(); err != nil {
		return nil, err
	}

	// Clean up old logs on startup
	go rw.cleanOldLogs()

	return rw, nil
}

// Write implements io.Writer
func (rw *RollingWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	// Check if we need to rotate
	if rw.shouldRotate(len(p)) {
		if err := rw.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = rw.currentFile.Write(p)
	rw.currentSize += int64(n)
	return
}

// Close closes the rolling writer
func (rw *RollingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.currentFile != nil {
		return rw.currentFile.Close()
	}
	return nil
}

// shouldRotate checks if rotation is needed
func (rw *RollingWriter) shouldRotate(newBytes int) bool {
	// Rotate if date changed
	currentDate := time.Now().Format(rw.config.TimePattern)
	if currentDate != rw.currentDate {
		return true
	}

	// Rotate if max size exceeded
	if rw.config.MaxSize > 0 && rw.currentSize+int64(newBytes) > rw.config.MaxSize {
		return true
	}

	return false
}

// rotate performs log rotation
func (rw *RollingWriter) rotate() error {
	if rw.currentFile != nil {
		rw.currentFile.Close()

		// Compress old file if configured
		if rw.config.Compress {
			go rw.compressFile(rw.currentPath())
		}
	}

	return rw.openNewFile()
}

// openNewFile opens a new log file
func (rw *RollingWriter) openNewFile() error {
	rw.currentDate = time.Now().Format(rw.config.TimePattern)
	path := rw.currentPath()

	// Check if file exists and get its size
	var currentSize int64
	if info, err := os.Stat(path); err == nil {
		currentSize = info.Size()
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	rw.currentFile = f
	rw.currentSize = currentSize
	return nil
}

// currentPath returns the current log file path
func (rw *RollingWriter) currentPath() string {
	ext := ".log"
	if rw.isJSON {
		ext = ".jsonl"
	}
	return filepath.Join(rw.config.LogDir, fmt.Sprintf("%s-%s%s", rw.config.BaseName, rw.currentDate, ext))
}

// compressFile compresses a log file
func (rw *RollingWriter) compressFile(path string) error {
	// Don't compress if file doesn't exist or is current file
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Open source file
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	// Create gzip file
	gzPath := path + ".gz"
	dst, err := os.Create(gzPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	// Write compressed content
	gzw := gzip.NewWriter(dst)
	gzw.Name = filepath.Base(path)
	gzw.ModTime = time.Now()

	if _, err := io.Copy(gzw, src); err != nil {
		os.Remove(gzPath)
		return err
	}

	if err := gzw.Close(); err != nil {
		os.Remove(gzPath)
		return err
	}

	// Remove original file
	return os.Remove(path)
}

// cleanOldLogs removes logs older than MaxAge and keeps only MaxBackups
func (rw *RollingWriter) cleanOldLogs() {
	ext := ".log"
	if rw.isJSON {
		ext = ".jsonl"
	}

	pattern := filepath.Join(rw.config.LogDir, fmt.Sprintf("%s-*%s*", rw.config.BaseName, ext))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	// Filter out current file
	currentPath := rw.currentPath()
	var oldFiles []string
	for _, f := range files {
		if f != currentPath && f != currentPath+".gz" {
			oldFiles = append(oldFiles, f)
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(oldFiles, func(i, j int) bool {
		infoI, _ := os.Stat(oldFiles[i])
		infoJ, _ := os.Stat(oldFiles[j])
		if infoI == nil || infoJ == nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	cutoff := time.Now().AddDate(0, 0, -rw.config.MaxAge)

	for i, f := range oldFiles {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}

		// Remove if too old
		if info.ModTime().Before(cutoff) {
			os.Remove(f)
			continue
		}

		// Remove if too many backups
		if rw.config.MaxBackups > 0 && i >= rw.config.MaxBackups {
			os.Remove(f)
		}
	}
}

// GetLogFiles returns list of current log files (for status endpoint)
func GetLogFiles(logDir string) []LogFileInfo {
	var files []LogFileInfo

	pattern := filepath.Join(logDir, "spot-analyzer-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return files
	}

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		files = append(files, LogFileInfo{
			Name:     filepath.Base(path),
			Path:     path,
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified.After(files[j].Modified)
	})

	return files
}

// LogFileInfo contains information about a log file
type LogFileInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// FormatSize formats bytes to human-readable size
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// IsLambda returns true if running in AWS Lambda
func IsLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// LambdaWriter writes logs to stdout in a format CloudWatch can capture
type LambdaWriter struct {
	component string
	level     Level
}

// NewLambdaWriter creates a writer for Lambda/CloudWatch
func NewLambdaWriter(component string, level Level) *LambdaWriter {
	return &LambdaWriter{component: component, level: level}
}

// Write implements io.Writer - formats for CloudWatch
func (lw *LambdaWriter) Write(p []byte) (n int, err error) {
	// CloudWatch automatically captures stdout with timestamps
	// We just need to ensure proper formatting
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	// Format: [LEVEL] [Component] Message
	// CloudWatch adds its own timestamp
	fmt.Printf("[%s] [%s] %s\n", lw.level.String(), lw.component, msg)
	return len(p), nil
}
