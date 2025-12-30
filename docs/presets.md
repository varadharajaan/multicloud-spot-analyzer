# Use Case Presets

Spot Analyzer includes pre-configured profiles for common cloud workloads. These presets optimize the instance selection based on typical requirements for each use case.

## Available Presets

### ☸️ Kubernetes Cluster

**Best for:** Kubernetes node groups, EKS/GKE/AKS worker nodes

**Configuration:**
- Minimum vCPU: 2
- Minimum Memory: 4 GB
- Max Interruption: <10%

**Why these settings:**
- Kubernetes requires stable nodes for pod scheduling
- Node failures trigger pod rescheduling which affects application availability
- Memory-intensive for running multiple containers

**Recommended instance families:** m5, m6i, r5, r6i

---

### 🗄️ Database Server

**Best for:** PostgreSQL, MySQL, MongoDB, Redis

**Configuration:**
- Minimum vCPU: 2
- Minimum Memory: 8 GB
- Max Interruption: <5%

**Why these settings:**
- Databases need maximum stability for data integrity
- Higher memory for caching and query performance
- Interruptions can cause data corruption or loss

**Recommended instance families:** r5, r6i, x1e (memory-optimized)

---

### 📈 Auto Scaling Group

**Best for:** Web servers, microservices, stateless applications

**Configuration:**
- Minimum vCPU: 2
- Minimum Memory: 4 GB
- Max Interruption: 10-15%

**Why these settings:**
- ASG can handle node failures by launching replacements
- Stateless workloads recover quickly from interruptions
- Cost optimization is important for scaling efficiency

**Recommended instance families:** m5, c5, m6i, c6i

---

### ⏰ Batch/Weekend Jobs

**Best for:** CI/CD, data processing, ETL, machine learning training

**Configuration:**
- Minimum vCPU: 2
- Minimum Memory: 4 GB
- Max Interruption: 15-20%

**Why these settings:**
- Batch jobs can checkpoint and resume
- Cost is more important than availability
- Workloads are temporary and can tolerate delays

**Recommended instance families:** c5, c6i, m5 (compute-optimized)

---

### 🌐 Web Server/API

**Best for:** REST APIs, GraphQL servers, static sites

**Configuration:**
- Minimum vCPU: 2
- Minimum Memory: 4 GB
- Max Interruption: 10-15%

**Why these settings:**
- Load balancers can route around failed instances
- Stateless design allows quick recovery
- Balance between cost and reliability

**Recommended instance families:** t3, m5, c5

---

### 🤖 ML Training

**Best for:** Machine learning model training, deep learning

**Configuration:**
- Minimum vCPU: 8
- Minimum Memory: 32 GB
- Max Interruption: 10-15%

**Why these settings:**
- ML training requires significant compute resources
- Checkpointing allows recovery from interruptions
- Cost optimization crucial for long training runs

**Recommended instance families:** p3, p4, g4 (GPU), c5, c6i (CPU)

---

## Custom Presets

You can combine presets with additional requirements:

1. Select a preset to set base configuration
2. Adjust individual values as needed
3. Use natural language to add more context

Example: Select "Kubernetes" preset, then type "with ARM architecture for better cost" to add ARM preference.

## API Usage

Get all presets:
```bash
curl http://localhost:8080/api/presets
```

Response:
```json
[
  {
    "id": "kubernetes",
    "name": "Kubernetes Cluster",
    "description": "Stable nodes for K8s workloads",
    "icon": "☸️",
    "minVcpu": 2,
    "minMemory": 4,
    "interruption": 1
  },
  // ... more presets
]
```
