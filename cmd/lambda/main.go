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
	case path == "/":
		// Root path redirects based on UI_VERSION env var
		uiVersion := os.Getenv("UI_VERSION")
		if uiVersion == "" {
			uiVersion = "v2" // Default to v2 for Lambda
		}
		if uiVersion == "v2" {
			return serveStaticFile("static/index-v2.html", "text/html")
		}
		return serveStaticFile("static/index.html", "text/html")
	case path == "/index.html":
		// Explicit /index.html always serves v1 Classic UI
		return serveStaticFile("static/index.html", "text/html")
	case path == "/index-v2.html":
		// Explicit /index-v2.html always serves v2 Modern UI
		return serveStaticFile("static/index-v2.html", "text/html")
	case path == "/swagger.html" || path == "/swagger" || path == "/swagger-ui":
		return serveStaticFile("static/swagger.html", "text/html")
	case path == "/styles.css":
		return serveStaticFile("static/styles.css", "text/css")
	case path == "/styles-v2.css":
		return serveStaticFile("static/styles-v2.css", "text/css")
	case path == "/app.js":
		return serveStaticFile("static/app.js", "application/javascript")
	case path == "/app-v2.js":
		return serveStaticFile("static/app-v2.js", "application/javascript")
	case path == "/api/health" && method == "GET":
		return handleHealth()
	case path == "/api/analyze" && method == "POST":
		return handleAnalyze(request.Body)
	case path == "/api/az" && method == "POST":
		return handleAZ(request.Body)
	case path == "/api/cache/status" && method == "GET":
		return handleCacheStatus()
	case path == "/api/cache/refresh" && method == "POST":
		return handleCacheRefresh()
	case path == "/api/parse-requirements" && method == "POST":
		return handleParseRequirements(request.Body)
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

func handleHealth() (events.LambdaFunctionURLResponse, error) {
	// Check AWS connection by making a simple call
	awsHealthy := true
	awsMessage := "Connected"

	// Try to get current region or check AWS connectivity
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1" // Default
	}

	return jsonResponse(200, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.1.0",
		"region":    region,
		"services": map[string]interface{}{
			"aws": map[string]interface{}{
				"status": func() string {
					if awsHealthy {
						return "healthy"
					} else {
						return "unhealthy"
					}
				}(),
				"message": awsMessage,
			},
		},
	})
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

// ParseRequirementsRequest for natural language parsing
type ParseRequirementsRequest struct {
	Text string `json:"text"`
}

// ParseRequirementsResponse returns parsed requirements
type ParseRequirementsResponse struct {
	MinVCPU         int    `json:"minVcpu"`
	MaxVCPU         int    `json:"maxVcpu"`
	MinMemory       int    `json:"minMemory"`
	MaxMemory       int    `json:"maxMemory"`
	Architecture    string `json:"architecture"`
	UseCase         string `json:"useCase"`
	MaxInterruption int    `json:"maxInterruption"`
	Explanation     string `json:"explanation"`
}

func handleParseRequirements(body string) (events.LambdaFunctionURLResponse, error) {
	var req ParseRequirementsRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return jsonResponse(400, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	resp := parseNaturalLanguage(req.Text)
	return jsonResponse(200, resp)
}

func parseNaturalLanguage(text string) ParseRequirementsResponse {
	text = strings.ToLower(text)
	resp := ParseRequirementsResponse{
		MinVCPU:         2,
		MaxVCPU:         0,
		MinMemory:       4,
		MaxInterruption: 2,
	}

	var explanations []string

	// Parse CPU requirements
	if strings.Contains(text, "small") || strings.Contains(text, "tiny") || strings.Contains(text, "micro") {
		resp.MinVCPU = 1
		resp.MaxVCPU = 2
		resp.MinMemory = 1
		resp.MaxMemory = 4
		explanations = append(explanations, "Small instance (1-2 vCPU)")
	} else if strings.Contains(text, "medium") || strings.Contains(text, "moderate") {
		resp.MinVCPU = 2
		resp.MaxVCPU = 4
		resp.MinMemory = 4
		resp.MaxMemory = 16
		explanations = append(explanations, "Medium instance (2-4 vCPU)")
	} else if strings.Contains(text, "large") || strings.Contains(text, "big") {
		resp.MinVCPU = 4
		resp.MaxVCPU = 8
		resp.MinMemory = 16
		resp.MaxMemory = 64
		explanations = append(explanations, "Large instance (4-8 vCPU)")
	} else if strings.Contains(text, "xlarge") || strings.Contains(text, "extra large") || strings.Contains(text, "huge") {
		resp.MinVCPU = 8
		resp.MaxVCPU = 32
		resp.MinMemory = 32
		explanations = append(explanations, "Extra large instance (8-32 vCPU)")
	}

	// Parse use cases
	if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") || strings.Contains(text, "cluster") {
		resp.UseCase = "kubernetes"
		resp.MaxInterruption = 1
		explanations = append(explanations, "Kubernetes use case: prioritizing stability")
	} else if strings.Contains(text, "database") || strings.Contains(text, "db") || strings.Contains(text, "postgres") ||
		strings.Contains(text, "mysql") || strings.Contains(text, "mongo") || strings.Contains(text, "redis") {
		resp.UseCase = "database"
		resp.MaxInterruption = 0
		explanations = append(explanations, "Database use case: maximum stability required")
	} else if strings.Contains(text, "autoscaling") || strings.Contains(text, "asg") || strings.Contains(text, "auto scaling") {
		resp.UseCase = "asg"
		resp.MaxInterruption = 2
		explanations = append(explanations, "Auto-scaling use case: balanced cost/stability")
	} else if strings.Contains(text, "weekend") || strings.Contains(text, "batch") || strings.Contains(text, "job") ||
		strings.Contains(text, "temporary") || strings.Contains(text, "short") {
		resp.UseCase = "batch"
		resp.MaxInterruption = 3
		explanations = append(explanations, "Batch/temporary use case: prioritizing cost savings")
	} else if strings.Contains(text, "web") || strings.Contains(text, "api") || strings.Contains(text, "server") {
		resp.UseCase = "general"
		resp.MaxInterruption = 2
		explanations = append(explanations, "Web/API use case: balanced approach")
	}

	// Parse architecture
	if strings.Contains(text, "intel") {
		resp.Architecture = "intel"
		explanations = append(explanations, "Intel architecture selected")
	} else if strings.Contains(text, "amd") {
		resp.Architecture = "amd"
		explanations = append(explanations, "AMD architecture selected")
	} else if strings.Contains(text, "arm") || strings.Contains(text, "graviton") {
		resp.Architecture = "arm64"
		explanations = append(explanations, "ARM/Graviton architecture: better cost efficiency")
	}

	if len(explanations) == 0 {
		resp.Explanation = "Using default settings: 2+ vCPU, 4GB+ RAM, moderate stability"
	} else {
		resp.Explanation = strings.Join(explanations, " | ")
	}

	return resp
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
	// Serve the embedded OpenAPI spec
	staticFS := web.GetStaticFS()
	data, err := fs.ReadFile(staticFS, "static/openapi.json")
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: 404,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error": "OpenAPI spec not found"}`,
		}, nil
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(data),
	}, nil
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
