# GCP Setup Guide

This guide covers GCP (Google Cloud Platform) integration for Spot Analyzer, enabling analysis and recommendations for GCP Spot VMs.

## Overview

GCP Spot VMs (formerly Preemptible VMs) offer significant cost savings of **60-91%** compared to on-demand pricing. Spot Analyzer supports comprehensive GCP analysis with no authentication required.

| Level | Authentication | Features |
|-------|---------------|----------|
| **Full** | None required | Spot prices, 185+ machine types, zone recommendations, capacity analysis |

## Quick Start

No authentication is required for GCP! Simply run the analyzer:

```powershell
cd c:\spot-analyzer
.\spot-web.exe
```

Select "GCP" in the UI and start analyzing Spot VM options.

## GCP Spot VM Basics

### What are GCP Spot VMs?

GCP Spot VMs (renamed from Preemptible VMs in 2022) are excess compute capacity available at steep discounts:

- **60-91% savings** compared to on-demand pricing
- Can be preempted (terminated) when GCP needs capacity
- No guaranteed availability or minimum runtime
- Ideal for fault-tolerant and batch workloads

### Key Differences from AWS/Azure

| Aspect | GCP Spot VM | AWS Spot | Azure Spot |
|--------|------------|----------|------------|
| Pricing Model | Fixed discount by region | Market-based, variable | Market-based, variable |
| Preemption Notice | 30 seconds | 2 minutes | 30 seconds |
| Max Runtime | No limit (since 2022) | No limit | No limit |
| Price Variability | Low (stable discounts) | High (market-driven) | Medium |

## Supported Machine Types

Spot Analyzer includes 185+ GCP machine types across 12 series:

### General Purpose

| Series | Description | vCPU Range | Memory Ratio |
|--------|-------------|------------|--------------|
| **E2** | Cost-optimized | 2-32 | 0.5-8 GB/vCPU |
| **N2** | Balanced | 2-128 | 1-8 GB/vCPU |
| **N2D** | AMD EPYC | 2-224 | 1-8 GB/vCPU |
| **N1** | Previous gen | 1-96 | 0.9-6.5 GB/vCPU |

### Compute Optimized

| Series | Description | vCPU Range | Use Case |
|--------|-------------|------------|----------|
| **C2** | Intel Cascade Lake | 4-60 | HPC, gaming |
| **C2D** | AMD EPYC Milan | 2-112 | HPC, batch |
| **C3** | Intel Sapphire Rapids | 4-176 | Latest compute |

### Memory Optimized

| Series | Description | vCPU Range | Memory |
|--------|-------------|------------|--------|
| **M2** | Ultra-high memory | 208-416 | Up to 12 TB |
| **M3** | Latest memory-optimized | 32-128 | Up to 4 TB |

### Accelerator Optimized (GPU)

| Series | GPU Type | vCPU | Use Case |
|--------|----------|------|----------|
| **A2** | NVIDIA A100 | 12-96 | ML training |
| **G2** | NVIDIA L4 | 4-96 | ML inference, graphics |

### ARM-based

| Series | Description | vCPU Range | Use Case |
|--------|-------------|------------|----------|
| **T2A** | Ampere Altra | 1-48 | Cost-efficient ARM |
| **T2D** | AMD EPYC | 1-60 | Scale-out workloads |

## Supported Regions

Spot Analyzer supports 40+ GCP regions:

### Americas
- `us-central1` (Iowa)
- `us-east1` (South Carolina)
- `us-east4` (Northern Virginia)
- `us-east5` (Columbus)
- `us-west1` (Oregon)
- `us-west2` (Los Angeles)
- `us-west3` (Salt Lake City)
- `us-west4` (Las Vegas)
- `us-south1` (Dallas)
- `northamerica-northeast1` (Montréal)
- `northamerica-northeast2` (Toronto)
- `southamerica-east1` (São Paulo)
- `southamerica-west1` (Santiago)

### Europe
- `europe-west1` (Belgium)
- `europe-west2` (London)
- `europe-west3` (Frankfurt)
- `europe-west4` (Netherlands)
- `europe-west6` (Zürich)
- `europe-west8` (Milan)
- `europe-west9` (Paris)
- `europe-west10` (Berlin)
- `europe-west12` (Turin)
- `europe-central2` (Warsaw)
- `europe-north1` (Finland)
- `europe-southwest1` (Madrid)

### Asia Pacific
- `asia-east1` (Taiwan)
- `asia-east2` (Hong Kong)
- `asia-northeast1` (Tokyo)
- `asia-northeast2` (Osaka)
- `asia-northeast3` (Seoul)
- `asia-south1` (Mumbai)
- `asia-south2` (Delhi)
- `asia-southeast1` (Singapore)
- `asia-southeast2` (Jakarta)
- `australia-southeast1` (Sydney)
- `australia-southeast2` (Melbourne)

### Middle East & Africa
- `me-central1` (Doha)
- `me-central2` (Dammam)
- `me-west1` (Tel Aviv)
- `africa-south1` (Johannesburg)

## Zone Availability

Each GCP region has multiple zones (typically 3-4):

```
us-central1-a
us-central1-b
us-central1-c
us-central1-f
```

Spot Analyzer analyzes availability and capacity across all zones to recommend optimal placement.

## CLI Usage

### Analyze GCP Spot VMs

```bash
# Find best 4 vCPU instances in us-central1
./spot-analyzer analyze --vcpu 4 --region us-central1 --cloud gcp

# With memory requirements
./spot-analyzer analyze --vcpu 8 --memory 32 --region europe-west1 --cloud gcp

# Specific machine families
./spot-analyzer analyze --vcpu 4 --families n2,c2 --region asia-northeast1 --cloud gcp
```

### AZ Recommendations

```bash
# Get zone recommendations for a specific machine type
./spot-analyzer az --instance n2-standard-4 --region us-central1 --cloud gcp
```

### Price Predictions

```bash
# Predict pricing for a machine type
./spot-analyzer predict --instance n2-standard-8 --region us-east1 --cloud gcp
```

## API Usage

### Analyze Request

```bash
curl -X POST http://localhost:8000/api/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "cloud": "gcp",
    "region": "us-central1",
    "minVcpu": 4,
    "minMemoryGb": 16,
    "families": ["n2", "e2", "c2"]
  }'
```

### AZ Recommendation Request

```bash
curl -X POST http://localhost:8000/api/smart-az \
  -H "Content-Type: application/json" \
  -d '{
    "cloud": "gcp",
    "region": "us-central1",
    "instanceType": "n2-standard-4"
  }'
```

## Discount Rates by Machine Family

GCP Spot VM discounts vary by machine type family:

| Family | Typical Discount | Notes |
|--------|-----------------|-------|
| **E2** | 60-70% | Cost-optimized, shared core |
| **N2/N2D** | 60-70% | General purpose |
| **C2/C2D/C3** | 65-75% | Compute optimized |
| **M2/M3** | 70-80% | Memory optimized |
| **T2D/T2A** | 60-70% | Tau (scale-out) |
| **A2** | 60-70% | A100 GPU |
| **G2** | 65-75% | L4 GPU |

## Smart Zone Selection

Spot Analyzer's smart zone selection considers:

1. **Capacity Score** - Estimated spare capacity in each zone
2. **Machine Type Availability** - Which zones support specific machine types
3. **GPU/ARM Availability** - Special hardware availability per zone
4. **Historical Stability** - Zone reliability patterns

### Zone Restrictions

Some machine types have zone restrictions:

- **GPU instances (A2, G2)** - Not available in all zones
- **ARM instances (T2A)** - Limited to specific zones
- **High-memory (M2, M3)** - Limited availability
- **Latest generation (C3)** - Gradually rolling out

## Best Practices

### Workload Suitability

✅ **Good for Spot VMs:**
- Batch processing jobs
- CI/CD pipelines
- Data processing (Dataflow, Dataproc)
- Stateless web servers with load balancing
- Development/test environments
- Machine learning training

❌ **Not recommended:**
- Databases without replication
- Stateful applications
- Real-time trading systems
- Single-point-of-failure services

### Maximizing Spot VM Success

1. **Use multiple zones** - Spread workloads across zones
2. **Use diverse machine types** - Don't depend on a single type
3. **Implement graceful shutdown** - Handle 30-second preemption notice
4. **Use instance groups** - Managed instance groups handle replacement
5. **Checkpoint frequently** - Save state for long-running jobs

## Comparison: GCP Machine Types vs AWS/Azure

| Use Case | GCP | AWS | Azure |
|----------|-----|-----|-------|
| General Purpose | n2-standard-4 | m5.xlarge | Standard_D4s_v3 |
| Compute Optimized | c2-standard-4 | c5.xlarge | Standard_F4s_v2 |
| Memory Optimized | n2-highmem-4 | r5.xlarge | Standard_E4s_v3 |
| GPU (Training) | a2-highgpu-1g | p3.2xlarge | Standard_NC6s_v3 |
| ARM | t2a-standard-4 | m6g.xlarge | - |

## Troubleshooting

### No Results Returned

1. **Check region format** - Use `us-central1` not `us-central-1`
2. **Verify machine family** - Ensure family exists (e.g., `n2`, not `n5`)
3. **Check vCPU range** - Some families have minimum vCPU requirements

### Zone Not Available

Some newer zones may not support all machine types. Try:
- Selecting a different zone in the same region
- Using a different machine type family
- Checking GCP's [zone resource availability](https://cloud.google.com/compute/docs/regions-zones)

## Resources

- [GCP Spot VMs Documentation](https://cloud.google.com/compute/docs/instances/spot)
- [GCP Pricing Calculator](https://cloud.google.com/products/calculator)
- [Machine Type Comparison](https://cloud.google.com/compute/docs/machine-types)
- [Region and Zone Availability](https://cloud.google.com/compute/docs/regions-zones)
