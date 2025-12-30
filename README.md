# 🔍 Multi-Cloud Spot Analyzer

> AI-powered CLI and Web UI for analyzing and recommending optimal spot/preemptible instances across AWS, Azure, and GCP.

[![Go Version](https://img.shields.io/badge/Go-1.23-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## ✨ Features

- **🌐 Web UI** - Elegant browser interface with natural language support
- **🗣️ Natural Language** - Describe requirements in plain English
- **🎯 Use Case Presets** - Quick configs for Kubernetes, Database, ASG, Batch
- **🧠 AI-Powered Analysis** - Smart scoring algorithm combining savings, stability, and fitness metrics
- **📊 Real AWS Data** - Fetches live data from AWS Spot Advisor API
- **🔮 Price Predictions** - Forecasts spot prices using linear regression on historical data
- **🌐 AZ Recommendations** - Identifies best availability zones for cost optimization
- **⚡ Enhanced Mode** - Uses AWS DescribeSpotPriceHistory for real volatility/trend analysis
- **🔬 Debug Mode** - Verify data sources with raw API output

## 🖥️ Web UI (New!)

Start the web interface for a visual experience:

```bash
# Build and run web server
go build -o spot-web ./cmd/web
./spot-web --port 8080

# Open http://localhost:8080
```

### Web UI Features

- **🗣️ Natural Language Input** - Type "I need a small Kubernetes cluster for weekend testing"
- **🎯 Quick Presets** - One-click configs for common use cases
- **⚙️ Visual Configuration** - CPU, RAM, Architecture selectors
- **📊 Interactive Results** - Sortable table with score breakdown

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
├── internal/
│   ├── domain/                      # Domain models & interfaces
│   │   ├── models.go               # Core data structures
│   │   ├── interfaces.go           # Provider interfaces
│   │   └── errors.go               # Custom errors
│   ├── provider/
│   │   ├── factory.go              # Provider factory (Singleton)
│   │   ├── cache.go                # In-memory cache with TTL
│   │   └── aws/
│   │       ├── spot_provider.go    # AWS Spot Advisor API client
│   │       ├── instance_specs.go   # EC2 instance catalog
│   │       ├── price_history.go    # DescribeSpotPriceHistory client
│   │       └── real_data_test.go   # Tests proving real data
│   ├── analyzer/
│   │   ├── smart_analyzer.go       # Multi-factor scoring algorithm
│   │   ├── enhanced_scoring.go     # AI-powered enhanced analysis
│   │   ├── predictions.go          # Price predictions & AZ recommendations
│   │   ├── filter.go               # Instance filtering logic
│   │   └── recommendation.go       # Recommendation engine
│   └── cli/
│       └── cli.go                  # Cobra CLI implementation
└── go.mod
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

```bash
# Run all tests
go test -v ./...

# Run data validation tests (proves real API data)
go test -v ./internal/provider/aws/ -run "TestRealData|TestDataNotHardcoded"
```

### Test Coverage

| Test | What It Proves |
|------|----------------|
| `TestRealDataValidation` | Provider data matches direct AWS API call |
| `TestDataNotHardcoded` | Different regions return different data |
| `TestAPIEndpointIsReal` | Using real AWS S3 endpoint |
| `TestInstanceCountReasonable` | Fetches 500-2000 instances |
| `TestSavingsRangeValid` | All values in valid ranges (0-100%, 0-4) |
| `TestPriceHistoryRealData` | DescribeSpotPriceHistory returns real prices |

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
- [x] Web UI dashboard
- [x] Natural language requirements parser
- [x] Use case presets
- [ ] Azure Spot VM support
- [ ] GCP Preemptible VM support
- [ ] Cost estimation calculator
- [ ] Terraform/Pulumi output generation

## 📚 Documentation

- [Web UI Guide](docs/web-ui.md)
- [Natural Language Parser](docs/natural-language.md)
- [Use Case Presets](docs/presets.md)

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

## 🤝 Contributing

Contributions welcome! Please read our contributing guidelines and submit PRs.

---

*Stop guessing, start saving* 💰
