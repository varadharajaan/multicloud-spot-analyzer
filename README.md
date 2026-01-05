# 🔍 Multi-Cloud Spot Analyzer

> AI-powered CLI and Web UI for analyzing and recommending optimal spot/preemptible instances across AWS, Azure, and GCP.

🚀 **[Try Live Demo](https://qx54q2ue7r76l5x7jiws7g5a2m0orvkm.lambda-url.us-east-1.on.aws/)** — No installation required!

[![Go Version](https://img.shields.io/badge/Go-1.23-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## ✨ Features

- **🌐 Multi-Cloud** - Support for AWS, Azure, and GCP Spot VMs
- **🌐 Web UI** - Modern dashboard interface with dark/light theme support
- **🗣️ Natural Language** - Describe requirements in plain English
- **🎯 Use Case Presets** - Quick configs for Kubernetes, Database, ASG, Batch
- **🧠 AI-Powered Analysis** - Smart scoring algorithm combining savings, stability, and fitness metrics
- **📊 Real Cloud Data** - Fetches live data from AWS Spot Advisor API and Azure Retail Prices API
- **🔮 Price Predictions** - Forecasts spot prices using linear regression on historical data
- **🌐 AZ Recommendations** - Identifies best availability zones (top 2: best + runner-up)
- **⚡ Enhanced Mode** - Uses AWS DescribeSpotPriceHistory for real volatility/trend analysis
- **📦 Instance Families** - Filter by family (t, m, c, r, etc.)
- **🔧 Burstable Support** - Include/exclude T-family instances with `--allow-burstable`
- **🔧 Config File** - Central YAML configuration for all settings
- **📚 Swagger API** - Full OpenAPI 3.0 documentation
- **☁️ AWS Lambda** - Deploy as serverless with SAM
- **📝 Rolling Logs** - Automatic log rotation with compression
- **🏥 Health Monitoring** - `/api/health` endpoint with cache/AWS/uptime checks
- **🚦 Rate Limiting** - Token bucket rate limiting (100 req/min per IP)
- **⚡ Performance** - Parallel AZ fetching, connection pooling

## ☁️ Cloud Provider Support

| Provider | Spot Pricing | Per-AZ Pricing | Historical Data | Auth Required |
|----------|--------------|----------------|-----------------|---------------|
| **AWS** | ✅ Real-time | ✅ Per-AZ prices | ✅ 90 days | Optional (for price history) |
| **Azure** | ✅ Real-time | ❌ Regional only | ❌ Current only | Optional (for AZ availability) |
| **GCP** | ✅ Real-time | ✅ Per-Zone prices | ✅ Simulated | None required |

### AWS Features
- Spot instance pricing and savings from Spot Advisor
- Real-time per-AZ spot prices via `DescribeSpotPriceHistory`
- 90-day price history for trend analysis
- Volatility and interruption frequency data

### Azure Features
- Spot VM pricing via Azure Retail Prices API (no auth required)
- Savings percentage vs pay-as-you-go pricing
- Per-zone VM availability via Compute SKUs API (requires auth)
- **Smart AZ Selection** with multi-factor scoring:
  - **Zone Capacity Score** - Analyzes VM type diversity per zone (higher = more capacity)
  - **Availability Score** - Real-time SKU availability checks
  - **Price Score** - Predicted pricing (uniform across Azure zones)
  - **Stability Score** - Based on zone restrictions and quota limits

**Note**: Azure spot prices are uniform across all zones in a region. Our smart AZ recommendations combine SKU availability data with zone capacity analysis to determine optimal zones.

📖 See [docs/azure-setup.md](docs/azure-setup.md) for Azure configuration.

### GCP Features
- Spot VM pricing (formerly Preemptible VMs) with 60-91% discounts
- 185+ machine types across E2, N2, N2D, C2, C2D, C3, M2, M3, T2D, T2A, A2, G2 series
- Per-zone pricing and availability analysis
- 40+ regions across Americas, Europe, Asia Pacific, and Middle East/Africa
- GPU (A2, G2) and ARM (T2A) instance support
- Smart zone selection with capacity scoring

📖 See [docs/gcp-setup.md](docs/gcp-setup.md) for GCP details.

### 🎯 Smart AZ Selection (Azure)

Our Azure AZ recommendations use a dual-approach analysis:

1. **Approach 1: SKU Availability API** - Queries Azure Compute Resource SKUs to check real-time availability:
   - Detects zone restrictions (which zones support each VM type)
   - Identifies quota limits and capacity constraints
   - Provides definitive availability status per zone

2. **Approach 2: Zone Capacity Score** - Analyzes VM type diversity:
   - Counts how many different VM types are available in each zone
   - Higher diversity = higher capacity and better stability
   - Normalized to 0-100 scale for scoring

**Combined Smart Score** = Availability (25%) + Capacity (25%) + Price (20%) + Stability (15%) + Interruption (15%)

Example output:
```
Zone capacity scores for eastus: map[eastus-1:25 eastus-2:54 eastus-3:100]
```
In this example, `eastus-3` has the highest capacity (100%) with most VM types available.

## 🖥️ Web UI

Start the web interface for a visual experience:

```bash
# Build and run web server
go build -o spot-web ./cmd/web
./spot-web

# Open http://localhost:8000
```

### Two UI Versions

- **Classic UI (v1)** - Clean, functional interface (`http://localhost:8000/`)
- **Modern UI (v2)** - Dashboard with dark/light theme (`http://localhost:8000/index-v2.html`)

### Web UI Features

- **🗣️ Natural Language Input** - Type "I need a small Kubernetes cluster for weekend testing"
- **🎯 Quick Presets** - One-click configs for common use cases
- **⚙️ Visual Configuration** - CPU, RAM, Architecture selectors
- **📦 Family Filtering** - Filter by instance families (m, c, r, t, etc.)
- **📊 Interactive Results** - Sortable table with score breakdown
- **🔢 Configurable Top N** - Choose results count (5, 10, 15, 20, 30, 50, 100)
- **🌐 AZ Details** - Click to see pricing across all availability zones
- **🌙 Dark Mode** - Toggle between light and dark themes (v2)

See [docs/web-ui.md](docs/web-ui.md) for full documentation.

## 🚀 Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/varadharajaan/multicloud-spot-analyzer.git
cd multicloud-spot-analyzer

# Build
go build -o spot-analyzer .

# Or install directly
go install github.com/varadharajaan/multicloud-spot-analyzer@latest
```

### Basic Usage

```bash
# Find best 2 vCPU instances in us-east-1
./spot-analyzer analyze --vcpu 2 --region us-east-1

# Enhanced AI analysis with real price history (requires AWS credentials)
./spot-analyzer analyze --vcpu 4 --enhanced --region us-west-2

# Price predictions for a specific instance
./spot-analyzer predict --instance m5.large --region us-east-1

# Availability zone recommendations
./spot-analyzer az --instance c5.xlarge --region us-east-1
```

## 📖 Commands

### `analyze` - Find optimal spot instances

```bash
spot-analyzer analyze [flags]

Flags:
  --vcpu int              Minimum vCPU cores (default 2)
  --max-vcpu int          Maximum vCPU cores (0 = no limit)
  --memory float          Minimum memory in GB
  --region string         AWS region (default "us-east-1")
  --category string       Instance category (general, compute, memory, storage)
  --arch string           CPU architecture (x86_64, arm64)
  --max-interruption int  Max interruption level 0-4 (default 2)
  --gpu                   Require GPU instances
  --allow-burstable       Include burstable T-family instances (default: from config)
  --families strings      Filter by instance families (t,m,c,r,etc.)
  --enhanced              Use enhanced AI analysis
  --debug                 Show raw API data for verification
  --top int               Number of results (default 10)
```

### `predict` - Price predictions

```bash
spot-analyzer predict --instance <type> --region <region>

Output:
  - Current price
  - Predicted prices (1h, 6h, 24h)
  - Trend direction (rising/falling/stable)
  - Volatility risk level
  - Confidence percentage
  - Optimal launch time
```

### `az` - Availability zone recommendations

```bash
spot-analyzer az --instance <type> --region <region>

Output:
  - Ranked list of AZs by price/stability
  - Price differential between best/worst
  - Volatility analysis per AZ
```

## 🏗️ Architecture

```
multicloud-spot-analyzer/
├── main.go                          # Entry point
├── config.yaml                      # Central configuration file
├── template.yaml                    # SAM template for Lambda deployment (240s timeout)
├── api/
│   └── openapi.json                # OpenAPI 3.0 specification
├── internal/
│   ├── domain/                      # Domain models & interfaces
│   │   ├── models.go               # Core data structures
│   │   ├── interfaces.go           # Provider interfaces
│   │   └── errors.go               # Custom errors
│   ├── config/                      # Configuration management
│   │   └── config.go               # YAML config with env/Secrets Manager support
│   ├── controller/                  # Programmatic API
│   │   └── controller.go           # Controller for library use
│   ├── logging/                     # Structured logging
│   │   ├── logger.go               # JSON logging for Athena/BigQuery
│   │   └── rolling.go              # Rolling log file support
│   ├── provider/
│   │   ├── factory.go              # Provider factory (Singleton)
│   │   ├── cache_manager.go        # In-memory cache with TTL
│   │   ├── aws/
│   │   │   ├── spot_provider.go    # AWS Spot Advisor API client
│   │   │   ├── instance_specs.go   # EC2 instance catalog
│   │   │   ├── price_history.go    # DescribeSpotPriceHistory client
│   │   │   └── real_data_test.go   # Tests proving real data
│   │   ├── azure/
│   │   │   ├── spot_provider.go    # Azure Retail Prices API client
│   │   │   ├── instance_specs.go   # Azure VM size catalog
│   │   │   ├── price_history.go    # Azure price analysis
│   │   │   └── sku_availability.go # Azure Compute SKUs API (per-zone availability)
│   │   └── gcp/
│   │       ├── spot_provider.go    # GCP Spot VM pricing
│   │       ├── instance_specs.go   # GCP machine type catalog (185+ types)
│   │       ├── price_history.go    # GCP price analysis
│   │       └── zone_provider_adapter.go # GCP zone availability
│   ├── analyzer/
│   │   ├── smart_analyzer.go       # Multi-factor scoring algorithm
│   │   ├── enhanced_scoring.go     # AI-powered enhanced analysis
│   │   ├── predictions.go          # Price predictions & AZ recommendations
│   │   ├── filter.go               # Instance filtering logic
│   │   └── recommendation.go       # Recommendation engine
│   ├── web/
│   │   ├── server.go               # HTTP server with API handlers
│   │   └── static/                 # Web UI assets
│   │       ├── index.html          # Classic UI (v1)
│   │       ├── index-v2.html       # Modern UI (v2)
│   │       ├── swagger.html        # API documentation
│   │       ├── styles.css          # Classic styles
│   │       ├── styles-v2.css       # Modern styles with themes
│   │       ├── app.js              # Classic UI JavaScript
│   │       └── app-v2.js           # Modern UI JavaScript
│   └── cli/
│       └── cli.go                  # Cobra CLI implementation
├── cmd/
│   ├── web/                        # Web server entry point
│   └── lambda/                     # AWS Lambda handler
├── docs/
│   ├── azure-setup.md             # Azure configuration guide
│   ├── web-ui.md                  # Web UI documentation
│   └── natural-language.md        # NLP features guide
└── utils/
    ├── azure/                     # Azure setup utilities
    │   └── setup_azure_creds.ps1  # Automated Azure credential setup
    ├── lambda/                    # Lambda deployment utilities
    │   ├── sam_deploy.py          # Build & deploy script (with auto-clean)
    │   ├── sam_cleanup.py         # Full stack cleanup
    │   ├── show_stack_outputs.py  # View stack outputs
    │   ├── stack-outputs.txt      # Saved stack outputs (auto-generated)
    │   └── tail_logs.py           # Tail CloudWatch logs
    └── test/                       # Integration test suite
        ├── integration_test.py    # Local server API tests
        ├── lambda_integration_test.py  # Lambda API tests
        └── logs/                  # Test reports (gitignored)
```

## ⚙️ Configuration

All settings are centralized in `config.yaml`:

```yaml
# Server settings
server:
  port: 8000
  read_timeout: 30s
  write_timeout: 60s

# Cache settings
cache:
  ttl: 2h
  cleanup_interval: 10m
  lambda_path: "/tmp/spot-analyzer-cache"

# AWS settings
aws:
  default_region: "us-east-1"
  price_history_lookback_days: 7

# Azure settings (optional - for smart AZ recommendations)
azure:
  default_region: "eastus"
  # Authentication for Compute SKUs API (optional)
  tenantId: ""       # From: az ad sp create-for-rbac
  clientId: ""       # From: az ad sp create-for-rbac
  clientSecret: ""   # From: az ad sp create-for-rbac
  subscriptionId: "" # From: az account show

# Analysis settings
analysis:
  default_top_n: 10
  az_recommendations: 2  # Show best + next best AZ

# Logging settings
logging:
  level: "info"
  enable_file: true
  max_size_mb: 100
  max_backups: 3
  compress: true

# UI settings
ui:
  version: "v1"  # v1 = classic, v2 = modern
  theme: "light"
```

Environment variables override config file values:
- `SPOT_ANALYZER_PORT` - Server port
- `SPOT_ANALYZER_CACHE_TTL` - Cache duration
- `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SUBSCRIPTION_ID` - Azure auth

## 📡 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check (cache, AWS, uptime) |
| `/api/analyze` | POST | Analyze spot instances |
| `/api/az` | POST | Get AZ recommendations |
| `/api/families` | GET | List available instance families |
| `/api/presets` | GET | Get use case presets |
| `/api/parse-requirements` | POST | Parse natural language |
| `/api/cache/status` | GET | Check cache status |
| `/api/cache/refresh` | POST | Refresh cache |
| `/api/openapi.json` | GET | OpenAPI specification |

**Rate Limiting**: `/api/analyze`, `/api/az`, and `/api/cache/refresh` are rate-limited to 100 requests/minute per IP.

See `/swagger.html` for interactive API documentation.

## ☁️ AWS Lambda Deployment

**🌐 Live Demo**: [https://qx54q2ue7r76l5x7jiws7g5a2m0orvkm.lambda-url.us-east-1.on.aws/](https://qx54q2ue7r76l5x7jiws7g5a2m0orvkm.lambda-url.us-east-1.on.aws/)

Deploy as a serverless function with a **FREE public Function URL**:

```bash
# Quick deploy with Python script (recommended)
python utils/lambda/sam_deploy.py

# Or manually with SAM CLI
sam build
sam deploy --stack-name spot-analyzer-prod --region us-east-1 --capabilities CAPABILITY_IAM --resolve-s3
```

### Deployment Features

- **Free Function URL** - No API Gateway costs, public HTTPS endpoint
- **CloudWatch Logs** - 14-day retention, managed by CloudFormation
- **IAM Permissions** - EC2 spot price access automatically configured
- **Environment Support** - Deploy as `dev` or `prod`

### Lambda Utility Scripts

```bash
# Deploy (cleans build artifacts first, then builds and deploys)
python utils/lambda/sam_deploy.py

# Deploy without cleaning (faster, uses cached build)
python utils/lambda/sam_deploy.py --no-clean

# Deploy to specific environment
python utils/lambda/sam_deploy.py --env dev

# View outputs (get Function URL)
python utils/lambda/show_stack_outputs.py

# Tail logs
python utils/lambda/tail_logs.py

# Full cleanup
python utils/lambda/sam_cleanup.py
```

**sam_deploy.py Features:**
- Automatically cleans `.aws-sam/`, `bootstrap`, and `stack-outputs.txt` before rebuild
- Use `--no-clean` to skip cleanup and use cached build
- Saves stack outputs to `stack-outputs.txt` for integration tests

See [utils/lambda/README.md](utils/lambda/README.md) for full documentation.

## 📦 Instance Family Filtering

Filter results by instance family:

```bash
# CLI
./spot-analyzer analyze --vcpu 4 --families m,c,r

# API
curl -X POST http://localhost:8000/api/analyze \
  -H "Content-Type: application/json" \
  -d '{"minVcpu": 4, "families": ["m", "c", "r"]}'
```

Supported families: t, m, c, r, i, d, g, p, inf, hpc

## 🔧 Controller API (Programmatic Use)

Use the controller package for programmatic access:

```go
import "github.com/spot-analyzer/internal/controller"

ctrl := controller.New()

// Analyze with family filtering
result, err := ctrl.Analyze(ctx, controller.AnalyzeRequest{
    MinVCPU:      4,
    Region:       "us-east-1",
    Families:     []string{"m", "c"},
    RefreshCache: true,
})

// Get AZ recommendations
azResult, err := ctrl.RecommendAZ(ctx, controller.AZRequest{
    InstanceType: "m5.large",
    Region:       "us-east-1",
})
```

## 📊 Scoring Algorithm

### Base Score (60%)
| Factor | Weight | Description |
|--------|--------|-------------|
| Savings | 30% | Discount vs on-demand pricing |
| Stability | 25% | Low interruption rate |
| Fitness | 25% | Match to requirements |
| Value | 20% | Performance per cost |

### Enhanced Score (40%) - With `--enhanced`
| Factor | Weight | Description |
|--------|--------|-------------|
| Volatility | 25% | Price stability over time |
| Trend | 20% | Price direction (rising/falling) |
| Capacity Pool | 20% | Multi-AZ availability |
| Time Pattern | 20% | Consistency across time |
| Popularity | 15% | Hidden gem detection |

## 🔐 AWS Credentials

For enhanced features (real price history, predictions), configure AWS credentials:

```bash
# Option 1: Environment variables
export AWS_ACCESS_KEY_ID=your_key
export AWS_SECRET_ACCESS_KEY=your_secret

# Option 2: AWS CLI profile
aws configure

# Option 3: IAM role (EC2/ECS)
# Automatically detected
```

Required permissions:
- `ec2:DescribeSpotPriceHistory`

## 🧪 Testing

### Unit Tests

```bash
# Run all tests
go test -v ./...

# Run specific package tests
go test -v ./internal/analyzer/...
go test -v ./internal/web/...
go test -v ./internal/controller/...

# Run data validation tests (proves real API data)
go test -v ./internal/provider/aws/ -run "TestRealData|TestDataNotHardcoded"
```

### Integration Tests

Comprehensive API integration tests are available for both local and Lambda deployments:

```bash
# Local Integration Test (tests http://localhost:8000)
# Start the web server first: go run ./cmd/web
python utils/test/integration_test.py

# Lambda Integration Test (auto-detects URL from stack-outputs.txt)
python utils/test/lambda_integration_test.py

# Lambda test with specific URL
python utils/test/lambda_integration_test.py --url https://xyz.lambda-url.us-east-1.on.aws

# Lambda test with custom timeout (for slow Azure SKU fetches)
python utils/test/lambda_integration_test.py --timeout 300
```

**Integration Test Features:**
- Tests all 6 API endpoints: AWS/Azure/GCP × Analyze/AZ
- Validates response structure and data quality
- Measures response times
- Generates JSON and TXT reports in `utils/test/logs/`
- Color-coded terminal output
- Exit code reflects pass/fail status (for CI/CD)

### Test Coverage

| Package | Tests | Description |
|---------|-------|-------------|
| `internal/domain` | 7 | Model validation, interruption levels |
| `internal/config` | 4 | Config loading, defaults, families |
| `internal/analyzer` | 4 | Filter logic, family extraction |
| `internal/controller` | 9 | API analysis, AZ recommendations |
| `internal/provider/aws` | 12 | Mock provider, instance specs, burstable, real data |
| `internal/provider/azure` | 6 | Azure provider, instance specs, SKU availability |
| `internal/provider/gcp` | 22 | GCP provider, machine types, zone availability, price analysis |
| `internal/web` | 8 | Health endpoint, rate limiter, handlers |

| Test | What It Proves |
|------|----------------|
| `TestRealDataValidation` | Provider data matches direct AWS API call |
| `TestDataNotHardcoded` | Different regions return different data |
| `TestAPIEndpointIsReal` | Using real AWS S3 endpoint |
| `TestInstanceCountReasonable` | Fetches 500-2000 instances |
| `TestSavingsRangeValid` | All values in valid ranges (0-100%, 0-4) |
| `TestPriceHistoryRealData` | DescribeSpotPriceHistory returns real prices |
| `TestBurstableFamilySpecs` | T-family instances have correct specs |
| `TestHealthEndpoint` | Health check returns status |
| `TestRateLimiter` | Rate limiting works correctly |

## 📈 Example Output

```
🧠 ENHANCED AI ANALYSIS
✅ Found 3 matching instances (analyzed 1067, filtered 1332)

RANK  INSTANCE    vCPU  MEM   SAVINGS  INTERRUPT  BASE  ENHANCED  FINAL
----  --------    ----  ---   -------  ---------  ----  --------  -----
1     i4i.large   2     8GB   76%      <5%        0.93  0.78      0.87
2     x2gd.large  2     16GB  66%      <5%        0.89  0.78      0.85
3     i3en.large  2     8GB   75%      <5%        0.89  0.78      0.84

🏆 TOP RECOMMENDATION: i4i.large
   💰 Savings: 76% vs On-Demand
   ⚡ Stability: <5% interruption rate
   
   💡 AI INSIGHTS:
      📊 REAL DATA: Using actual AWS price history
      🌟 HIDDEN GEM: Underutilized instance - excellent choice!
      🌐 MULTI-AZ READY: Available across multiple AZs
```

## 🗺️ Roadmap

- [x] AWS Spot Advisor integration
- [x] Smart multi-factor scoring
- [x] Enhanced AI analysis with DescribeSpotPriceHistory
- [x] Price predictions
- [x] AZ recommendations
- [x] Web UI dashboard (v1 + v2)
- [x] Natural language requirements parser
- [x] Use case presets
- [x] Central YAML configuration
- [x] Instance family filtering
- [x] Swagger/OpenAPI documentation
- [x] Controller package for programmatic use
- [x] AWS Lambda deployment with SAM
- [x] Rolling logs with compression
- [x] Dark/Light theme toggle
- [x] Health monitoring endpoint
- [x] API rate limiting
- [x] Burstable instance support
- [x] Parallel AZ price fetching
- [x] AWS connection pooling
- [x] Configurable Top N results
- [x] Comprehensive unit tests
- [x] Azure Spot VM support
- [x] GCP Spot VM support (185+ machine types, 40+ regions)
- [ ] Cost estimation calculator
- [ ] Terraform/Pulumi output generation

## 📚 Documentation

- [Web UI Guide](docs/web-ui.md)
- [Natural Language Parser](docs/natural-language.md)
- [Use Case Presets](docs/presets.md)
- [Azure Setup Guide](docs/azure-setup.md)
- [GCP Setup Guide](docs/gcp-setup.md)
- [Changelog](CHANGELOG.md)
- [API Documentation](api/openapi.json) | [Swagger UI](/swagger.html)

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

## 🤝 Contributing

Contributions welcome! Here's how you can help:

### How to Contribute

- **🐛 Bug Reports** - Open an issue with clear reproduction steps and environment details
- **💡 Feature Requests** - Describe the use case and expected behavior in an issue
- **🔧 Code Contributions** - Fork the repo, create a feature branch, and submit a PR
- **☁️ Cloud Provider Support** - Help add Azure Spot VM or GCP Preemptible VM providers
- **📖 Documentation** - Improve docs, add examples, or fix typos
- **🧪 Testing** - Add test cases, especially for edge cases

### Contribution Guidelines

1. **Fork & Clone** - Fork the repository and clone locally
2. **Branch** - Create a feature branch: `git checkout -b feature/azure-provider`
3. **Code Style** - Follow Go conventions, run `go fmt` and `go vet`
4. **Tests** - Add tests for new functionality, ensure all tests pass
5. **Commit Messages** - Use conventional commits: `feat:`, `fix:`, `docs:`, etc.
6. **Pull Request** - Open a PR with clear description of changes

### Adding a New Cloud Provider

All three major cloud providers (AWS, Azure, GCP) are now supported! To add a new provider:

1. Create provider in `internal/provider/<cloud>/`
2. Implement `SpotDataProvider` and `InstanceSpecsProvider` interfaces
3. Add `init()` function to auto-register with factory
4. The Web UI will automatically support the new provider!

See existing AWS, Azure, or GCP implementations as reference.

---

*Stop guessing, start saving* 💰
