# Log Schema Documentation

This document describes the JSON log schema used by Spot Analyzer, designed for analysis with AWS Athena, Google BigQuery, or similar query engines.

## Log File Format

Logs are written in **JSONL format** (JSON Lines) - one JSON object per line.

**Location:** `logs/spot-analyzer-YYYY-MM-DD.jsonl`

## Schema Definition

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `timestamp` | string | ISO 8601 timestamp with nanoseconds | `2025-12-30T18:30:45.123456789Z` |
| `level` | string | Log level (DEBUG, INFO, WARN, ERROR) | `INFO` |
| `message` | string | Human-readable log message | `Analysis completed` |
| `caller` | string | Source file and line number | `smart_analyzer.go:142` |
| `function` | string | Function name | `Analyze` |
| `component` | string | Component name | `analyzer`, `web`, `cli`, `provider` |
| `request_id` | string | Request trace ID (for HTTP requests) | `abc123-def456` |
| `duration_ms` | float | Operation duration in milliseconds | `1234.56` |
| `region` | string | Cloud region | `us-east-1` |
| `provider` | string | Cloud provider | `aws`, `azure`, `gcp` |
| `instance_type` | string | EC2/VM instance type | `m5.large` |
| `error` | string | Error message (if any) | `connection timeout` |
| `count` | int | Item count | `1067` |
| `fields` | object | Additional structured fields | `{"score": 0.85}` |
| `version` | string | Application version | `1.0.0` |
| `hostname` | string | Machine hostname | `server-01` |

## AWS Athena Setup

### 1. Create Table

```sql
CREATE EXTERNAL TABLE spot_analyzer_logs (
  timestamp STRING,
  level STRING,
  message STRING,
  caller STRING,
  function STRING,
  component STRING,
  request_id STRING,
  duration_ms DOUBLE,
  region STRING,
  provider STRING,
  instance_type STRING,
  error STRING,
  count INT,
  fields MAP<STRING, STRING>,
  version STRING,
  hostname STRING
)
ROW FORMAT SERDE 'org.openx.data.jsonserde.JsonSerDe'
WITH SERDEPROPERTIES (
  'ignore.malformed.json' = 'true'
)
LOCATION 's3://your-bucket/logs/'
TBLPROPERTIES ('has_encrypted_data'='false');
```

### 2. Query Examples

**Count logs by level:**
```sql
SELECT level, COUNT(*) as count
FROM spot_analyzer_logs
WHERE timestamp >= '2025-12-30'
GROUP BY level
ORDER BY count DESC;
```

**Average analysis duration by region:**
```sql
SELECT region, AVG(duration_ms) as avg_duration_ms, COUNT(*) as analyses
FROM spot_analyzer_logs
WHERE component = 'analyzer' AND region IS NOT NULL
GROUP BY region
ORDER BY avg_duration_ms DESC;
```

**Top recommended instances:**
```sql
SELECT instance_type, COUNT(*) as recommendations
FROM spot_analyzer_logs
WHERE message = 'Instance recommended'
GROUP BY instance_type
ORDER BY recommendations DESC
LIMIT 10;
```

**Error rate by component:**
```sql
SELECT 
  component,
  COUNT(CASE WHEN level = 'ERROR' THEN 1 END) as errors,
  COUNT(*) as total,
  ROUND(100.0 * COUNT(CASE WHEN level = 'ERROR' THEN 1 END) / COUNT(*), 2) as error_rate
FROM spot_analyzer_logs
GROUP BY component
ORDER BY error_rate DESC;
```

**Request latency percentiles:**
```sql
SELECT 
  component,
  APPROX_PERCENTILE(duration_ms, 0.50) as p50,
  APPROX_PERCENTILE(duration_ms, 0.90) as p90,
  APPROX_PERCENTILE(duration_ms, 0.99) as p99
FROM spot_analyzer_logs
WHERE duration_ms IS NOT NULL
GROUP BY component;
```

## Google BigQuery Setup

### 1. Create Table

```sql
CREATE TABLE `project.dataset.spot_analyzer_logs` (
  timestamp TIMESTAMP,
  level STRING,
  message STRING,
  caller STRING,
  function STRING,
  component STRING,
  request_id STRING,
  duration_ms FLOAT64,
  region STRING,
  provider STRING,
  instance_type STRING,
  error STRING,
  count INT64,
  fields JSON,
  version STRING,
  hostname STRING
);
```

### 2. Load Data

```bash
bq load --source_format=NEWLINE_DELIMITED_JSON \
  project:dataset.spot_analyzer_logs \
  gs://your-bucket/logs/*.jsonl
```

## Sample Log Entries

```json
{"timestamp":"2025-12-30T18:30:45.123456789Z","level":"INFO","message":"Starting analysis for region=us-east-1","caller":"smart_analyzer.go:59","function":"Analyze","component":"analyzer","region":"us-east-1","provider":"aws","version":"1.0.0","hostname":"dev-machine"}
{"timestamp":"2025-12-30T18:30:46.789012345Z","level":"INFO","message":"Analysis completed","caller":"smart_analyzer.go:142","function":"Analyze","component":"analyzer","region":"us-east-1","provider":"aws","count":1067,"duration_ms":1665.89,"version":"1.0.0","hostname":"dev-machine"}
{"timestamp":"2025-12-30T18:30:46.890123456Z","level":"INFO","message":"Instance recommended","caller":"cli.go:250","function":"runAnalyze","component":"cli","region":"us-east-1","instance_type":"m5.large","fields":{"score":0.87,"savings":76},"version":"1.0.0","hostname":"dev-machine"}
```

## Sync to S3/GCS

To analyze logs in the cloud, sync your logs folder:

**AWS S3:**
```bash
aws s3 sync logs/ s3://your-bucket/spot-analyzer-logs/ --exclude "*.log"
```

**Google Cloud Storage:**
```bash
gsutil -m rsync -r logs/ gs://your-bucket/spot-analyzer-logs/
```

## Grafana/Loki Integration

The JSONL format is compatible with Grafana Loki. Use promtail to ship logs:

```yaml
# promtail config
scrape_configs:
  - job_name: spot-analyzer
    static_configs:
      - targets: [localhost]
        labels:
          job: spot-analyzer
          __path__: /path/to/logs/*.jsonl
    pipeline_stages:
      - json:
          expressions:
            level: level
            component: component
            region: region
      - labels:
          level:
          component:
          region:
```
