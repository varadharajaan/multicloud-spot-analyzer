// Package main provides the Lambda handler for Spot Analyzer.
// This is the entry point for AWS Lambda Function URL deployment.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/config"
	"github.com/spot-analyzer/internal/controller"
	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/provider"
	awsprovider "github.com/spot-analyzer/internal/provider/aws"
	"github.com/spot-analyzer/internal/web"
)

// Handler processes Lambda Function URL requests
func Handler(ctx context.Context, request events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	path := request.RawPath
	method := request.RequestContext.HTTP.Method

	// Log request (goes to CloudWatch)
	fmt.Printf("[%s] %s %s\n", time.Now().Format(time.RFC3339), method, path)

	// CORS headers
	headers := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Content-Type":                 "application/json",
	}

	// Handle OPTIONS (CORS preflight)
	if method == "OPTIONS" {
		return events.LambdaFunctionURLResponse{
			StatusCode: 200,
			Headers:    headers,
			Body:       "",
		}, nil
	}

	// Route request
	switch {
	case path == "/" || path == "/index.html":
		return serveStaticFile("static/index.html", "text/html")
	case path == "/swagger.html":
		return serveStaticFile("static/swagger.html", "text/html")
	case path == "/styles.css":
		return serveStaticFile("static/styles.css", "text/css")
	case path == "/app.js":
		return serveStaticFile("static/app.js", "application/javascript")
	case path == "/api/analyze" && method == "POST":
		return handleAnalyze(request.Body)
	case path == "/api/az" && method == "POST":
		return handleAZ(request.Body)
	case path == "/api/cache/status" && method == "GET":
		return handleCacheStatus()
	case path == "/api/cache/refresh" && method == "POST":
		return handleCacheRefresh()
	case path == "/api/presets" && method == "GET":
		return handlePresets()
	case path == "/api/families" && method == "GET":
		return handleFamilies()
	case path == "/api/openapi.json" && method == "GET":
		return handleOpenAPI()
	default:
		// Try static files
		if strings.HasPrefix(path, "/") {
			filePath := "static" + path
			contentType := getContentType(path)
			return serveStaticFile(filePath, contentType)
		}
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Headers:    headers,
			Body:       `{"error": "Not found"}`,
		}, nil
	}
}

func serveStaticFile(path string, contentType string) (events.LambdaFunctionURLResponse, error) {
	// Use web package's embedded static files
	staticFS := web.GetStaticFS()
	data, err := fs.ReadFile(staticFS, path)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       fmt.Sprintf(`{"error": "File not found: %s"}`, path),
		}, nil
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": contentType},
		Body:       string(data),
	}, nil
}

func getContentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	default:
		return "text/plain"
	}
}

func handleAnalyze(body string) (events.LambdaFunctionURLResponse, error) {
	var req controller.AnalyzeRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return jsonResponse(400, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	ctrl := controller.New()
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	resp, err := ctrl.Analyze(ctx, req)
	if err != nil {
		return jsonResponse(500, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return jsonResponse(200, resp)
}

func handleAZ(body string) (events.LambdaFunctionURLResponse, error) {
	var req controller.AZRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return jsonResponse(400, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	ctrl := controller.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := ctrl.RecommendAZ(ctx, req)
	if err != nil {
		return jsonResponse(500, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return jsonResponse(200, resp)
}

func handleCacheStatus() (events.LambdaFunctionURLResponse, error) {
	ctrl := controller.New()
	status := ctrl.GetCacheStatus()
	return jsonResponse(200, status)
}

func handleCacheRefresh() (events.LambdaFunctionURLResponse, error) {
	ctrl := controller.New()
	itemsCleared, _ := ctrl.RefreshCache()
	return jsonResponse(200, map[string]interface{}{
		"success":      true,
		"itemsCleared": itemsCleared,
		"message":      fmt.Sprintf("Cache cleared: %d items removed", itemsCleared),
		"refreshTime":  time.Now().Format(time.RFC3339),
	})
}

func handlePresets() (events.LambdaFunctionURLResponse, error) {
	presets := []map[string]interface{}{
		{"id": "kubernetes", "name": "Kubernetes", "description": "Stable K8s nodes", "icon": "☸️", "minVcpu": 2, "minMemory": 4, "interruption": 1},
		{"id": "database", "name": "Database", "description": "Max stability", "icon": "🗄️", "minVcpu": 2, "minMemory": 8, "interruption": 0},
		{"id": "asg", "name": "Auto Scaling", "description": "Balanced ASG", "icon": "📈", "minVcpu": 2, "minMemory": 4, "interruption": 2},
		{"id": "batch", "name": "Batch Jobs", "description": "Cost savings", "icon": "⏰", "minVcpu": 2, "minMemory": 4, "interruption": 3},
		{"id": "web", "name": "Web Server", "description": "General purpose", "icon": "🌐", "minVcpu": 2, "minMemory": 4, "interruption": 2},
		{"id": "ml", "name": "ML Training", "description": "Compute-optimized", "icon": "🤖", "minVcpu": 8, "minMemory": 32, "interruption": 2},
	}
	return jsonResponse(200, presets)
}

func handleFamilies() (events.LambdaFunctionURLResponse, error) {
	cfg := config.Get()
	return jsonResponse(200, cfg.InstanceFamilies.Available)
}

func handleOpenAPI() (events.LambdaFunctionURLResponse, error) {
	// Return embedded OpenAPI spec or error
	return jsonResponse(200, map[string]string{
		"message": "OpenAPI spec available at /api/openapi.json",
	})
}

func jsonResponse(statusCode int, body interface{}) (events.LambdaFunctionURLResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "Failed to serialize response"}`,
		}, nil
	}

	return events.LambdaFunctionURLResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		},
		Body: string(jsonBody),
	}, nil
}

func main() {
	// Initialize config
	_ = config.Get()

	// Start Lambda handler
	lambda.Start(Handler)
}

// Ensure imports are used
var _ = domain.AWS
var _ = provider.GetCacheManager
var _ = awsprovider.NewPriceHistoryProvider
var _ = analyzer.NewEnhancedAnalyzer
var _ = fs.FS(nil)
var _ = os.Getenv
var _ = http.MethodGet
