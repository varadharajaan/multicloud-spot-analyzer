# Web UI Guide

The Spot Analyzer includes an elegant web-based interface for analyzing spot instances.

## Starting the Web UI

### Option 1: Using the web binary

```bash
# Build the web server
go build -o spot-web ./cmd/web

# Start on default port (8000)
./spot-web

# Start on custom port
./spot-web --port 3000
```

### Option 2: Using the CLI command

```bash
./spot-analyzer web --port 8080
```

## Features

### 🗣️ Natural Language Input

Describe your needs in plain English:

- "I need a small Kubernetes cluster for weekend testing"
- "Database server with high stability"
- "Large scale auto-scaling group with ARM for cost savings"
- "Batch processing job that can tolerate interruptions"

The analyzer will automatically parse your requirements and configure:
- CPU/Memory requirements
- Architecture preference
- Stability level
- Use case optimizations

### 🎯 Quick Presets

Pre-configured profiles for common use cases:

| Preset | Description | Interruption Tolerance |
|--------|-------------|------------------------|
| ☸️ Kubernetes | Stable nodes for K8s workloads | Low (<10%) |
| 🗄️ Database | Maximum stability for data workloads | Minimal (<5%) |
| 📈 Auto Scaling | Balanced cost/stability for ASG | Moderate (10-15%) |
| ⏰ Batch Jobs | Maximum savings for temporary workloads | High (15-20%) |
| 🌐 Web Server | General purpose web workloads | Moderate (10-15%) |
| 🤖 ML Training | Compute-optimized for ML | Moderate (10-15%) |

### ⚙️ Configuration Options

- **vCPU Range**: Minimum and maximum CPU cores
- **Memory**: Minimum and maximum RAM in GB
- **Architecture**: Any, Intel, AMD, or ARM/Graviton
- **Region**: Select from major AWS regions
- **Stability Slider**: Visual control for interruption tolerance
- **Enhanced Mode**: Toggle AI-powered scoring with real price history
- **Results to Show**: Select Top N results (5, 10, 15, 20, 30, 50, 100)

### 📊 Results

The results include:
- Top N recommended instances (configurable)
- Score breakdown
- Savings percentage
- Interruption rate
- Architecture details
- Memory displayed with proper formatting (e.g., 0.5 GB for t3.nano)

## Health Monitoring

The web server provides a health endpoint for monitoring:

```bash
GET /api/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0",
  "checks": {
    "cache": "ok",
    "aws_credentials": "ok",
    "uptime": "2h30m15s"
  }
}
```

## Rate Limiting

API endpoints are rate-limited to prevent abuse:

- **Rate**: 100 requests per minute per IP
- **Affected Endpoints**: `/api/analyze`, `/api/az`, `/api/cache/refresh`
- **Response on Limit**: HTTP 429 Too Many Requests

## API Endpoints

The web server exposes these REST APIs:

### POST /api/analyze

Analyze instances based on requirements.

```json
{
  "minVcpu": 2,
  "maxVcpu": 8,
  "minMemory": 4,
  "maxMemory": 32,
  "architecture": "x86_64",
  "region": "us-east-1",
  "maxInterruption": 2,
  "useCase": "kubernetes",
  "enhanced": true,
  "topN": 10
}
```

### POST /api/parse-requirements

Parse natural language requirements.

```json
{
  "text": "I need a medium Kubernetes cluster with ARM architecture"
}
```

### GET /api/presets

Get available use case presets.

## Screenshots

### Main Interface
![Main Interface](images/web-main.png)

### Natural Language Parsing
![NL Parsing](images/web-nl.png)

### Results View
![Results](images/web-results.png)
