# Changelog

All notable changes to the Multi-Cloud Spot Analyzer project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2025-01-04

### Added

#### Smart AZ Selection for Azure
- **Dual-Approach Analysis** - Combines SKU API + Zone Capacity Scoring:
  - **Approach 1**: Azure Compute SKUs API for real-time availability checks
  - **Approach 2**: Zone Capacity Score based on VM type diversity per zone
  
- **Multi-Factor Smart Scoring**:
  - Availability Score (25%) - Is VM available in zone?
  - Capacity Score (25%) - From VM type diversity analysis
  - Price Score (20%) - Price competitiveness
  - Stability Score (15%) - Based on zone restrictions
  - Interruption Rate (15%) - Estimated interruption percentage

- **Enhanced API Response Fields**:
  - `combinedScore` - Overall smart score (0-100)
  - `capacityScore` - Zone capacity score
  - `availabilityScore` - SKU availability score
  - `priceScore` - Price-based score
  - `pricePredicted` - Boolean for predicted vs real prices
  - `interruptionRate` - Estimated interruption rate
  - `capacityLevel` - High/Medium/Low capacity indicator
  - `confidence` - Analysis confidence level
  - `dataSources` - List of APIs used for analysis

- **UI Enhancements (V1 & V2)**:
  - Score bar visualization with gradient
  - Capacity badges (High/Medium/Low with colors)
  - Predicted price indicator (~$ prefix)
  - Interruption rate display
  - Confidence badge in header
  - Insights section with smart recommendations
  - Data sources display

### Changed
- Updated AZ table columns: Rank, AZ, Score, Capacity, Price, Int. Rate, Stability
- Improved Azure zone analysis with capacity scoring algorithm
- Documentation updates for Smart AZ Selection feature

### Documentation
- Updated `README.md` with Smart AZ Selection section
- Updated `docs/azure-setup.md` with dual-approach analysis details
- Updated `docs/web-ui.md` with AZ Lookup API documentation

---

## [1.2.0] - 2024-12-31

### Added

#### AWS Lambda SAM Deployment
- **SAM Template** (`template.yaml`) - Full CloudFormation-based deployment:
  - Lambda Function URL with public access (no API Gateway costs)
  - CloudWatch Log Group with 14-day retention
  - IAM policies for EC2 spot price access
  - Environment parameter support (dev/prod)
  
#### Lambda Utility Scripts
- **`sam_deploy.py`** - Automated build and deploy script:
  - Builds Go binary for Linux
  - Runs SAM build and deploy
  - Fetches and saves stack outputs
  
- **`sam_cleanup.py`** - Full stack cleanup script:
  - Deletes CloudFormation stack
  - Removes orphaned log groups
  - Supports `--all` flag to clean all spot-analyzer stacks
  - Dry-run mode for safety
  
- **`show_stack_outputs.py`** - View deployment outputs
- **`tail_logs.py`** - Tail CloudWatch logs in real-time

#### Configuration Updates
- **`samconfig.toml`** - SAM CLI configuration with dev/prod environments
- Default stack name: `spot-analyzer-prod`
- Default region: `us-east-1`

### Changed
- Moved lambda utilities from `tools/` to `utils/lambda/`
- Updated all scripts to use consistent `spot-analyzer-prod` as default stack name
- Lambda Function URL CORS handled in code (not template) for CloudFormation compatibility

### Fixed
- CloudFormation validation errors with Function URL CORS configuration
- Log group conflict errors during redeployment

---

## [1.1.0] - 2024-12-30

### Added

#### Health Monitoring
- **`/api/health` endpoint** - New health check endpoint returning:
  - Cache status (ok/error)
  - AWS credentials availability
  - Server uptime
  - Timestamp and version info

#### Rate Limiting
- **Token bucket rate limiting** on API endpoints:
  - 100 requests per minute per IP address
  - Applied to `/api/analyze`, `/api/az`, `/api/cache/refresh`
  - Returns HTTP 429 when limit exceeded

#### Burstable Instance Support
- **`--allow-burstable` CLI flag** - Explicitly include/exclude T-family (burstable) instances
- **`allowBurstable` API parameter** - Control burstable instances in API requests
- **Config option** - `analysis.allow_burstable` in config.yaml for default behavior

#### UI Improvements
- **Top N selector** - Choose number of results to display (5, 10, 15, 20, 30, 50, 100)
- Added to both Classic UI (v1) and Modern UI (v2)

#### Performance Optimizations
- **Parallel AZ price fetching** - Concurrent goroutines with semaphore (10 max concurrent)
- **AWS connection pooling** - HTTP client connection reuse:
  - MaxIdleConns: 100
  - MaxIdleConnsPerHost: 25
  - MaxConnsPerHost: 50
  - IdleConnTimeout: 90s

#### Testing
- **Comprehensive unit test suite** covering all packages:
  - `internal/domain` - 7 tests for models and validation
  - `internal/config` - 4 tests for configuration loading
  - `internal/analyzer` - 4 tests for filter logic
  - `internal/controller` - 9 tests for API analysis
  - `internal/provider/aws` - 12 tests including mocks and real data validation
  - `internal/web` - 8 tests for health, rate limiter, and handlers

### Fixed

#### Memory Display Bug
- **t3.nano "0 GB RAM" bug** - Fixed memory display for sub-GB instances
  - Added `formatMemory()` helper function
  - t3.nano now correctly shows "0.5 GB" instead of "0 GB"
  - Proper decimal formatting for memory values < 1 GB

### Changed

- Updated OpenAPI specification with:
  - Health endpoint documentation
  - `allowBurstable` parameter
  - Health tag added to API groups
- Enhanced scoring uses parallel processing for improved performance
- AWS price history provider uses connection pooling

## [1.0.0] - 2024-12-15

### Added
- Initial release
- AWS Spot Advisor integration
- Smart multi-factor scoring algorithm
- Enhanced AI analysis with DescribeSpotPriceHistory
- Price predictions using linear regression
- AZ recommendations (best + runner-up)
- Web UI dashboard (v1 classic + v2 modern with themes)
- Natural language requirements parser
- Use case presets (Kubernetes, Database, ASG, Batch, Web, ML)
- Instance family filtering
- Central YAML configuration
- Swagger/OpenAPI documentation
- Controller package for programmatic use
- AWS Lambda deployment with SAM
- Rolling logs with compression
- Dark/Light theme toggle

---

## Version History

| Version | Date | Highlights |
|---------|------|------------|
| 1.1.0 | 2024-12-30 | Health endpoint, rate limiting, burstable support, performance improvements |
| 1.0.0 | 2024-12-15 | Initial release with full AWS support |
