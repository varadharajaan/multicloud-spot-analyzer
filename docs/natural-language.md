# Natural Language Parser

The Spot Analyzer includes an intelligent natural language parser that converts human-readable requirements into specific instance configurations.

## How It Works

The parser analyzes your text input and extracts:
1. **Size indicators** (small, medium, large)
2. **Specific numbers** (4 vCPU, 16GB RAM)
3. **Use case keywords** (kubernetes, database, batch)
4. **Architecture preferences** (intel, amd, arm, graviton)
5. **Scale patterns** (weekend, temporary, production)

## Supported Patterns

### Size Keywords

| Keyword | vCPU Range | Memory Range |
|---------|------------|--------------|
| small, tiny, micro | 1-2 | 1-4 GB |
| medium, moderate | 2-4 | 4-16 GB |
| large, big | 4-8 | 16-64 GB |
| xlarge, huge, extra large | 8-32 | 32+ GB |

### Use Case Keywords

| Keywords | Use Case | Stability |
|----------|----------|-----------|
| kubernetes, k8s, cluster | Kubernetes | High |
| database, db, postgres, mysql, mongo, redis | Database | Maximum |
| autoscaling, asg, auto scaling | Auto Scaling | Moderate |
| weekend, batch, job, temporary, short | Batch | Low (cost priority) |
| web, api, server | Web Server | Moderate |

### Architecture Keywords

| Keywords | Architecture |
|----------|--------------|
| intel | Intel x86_64 |
| amd | AMD x86_64 |
| arm, graviton | ARM64 |

### Specific Numbers

The parser extracts numbers from patterns like:
- "4 vCPU" → minVCPU = 4
- "16gb" → minMemory = 16
- "8 cores" → minVCPU = 8

## Examples

### Example 1: Weekend Testing

Input:
```
I need a small system for weekend testing that can handle some interruptions
```

Parsed:
- minVCPU: 1
- maxVCPU: 2
- minMemory: 1
- maxMemory: 4
- useCase: batch
- maxInterruption: 3

### Example 2: Production Kubernetes

Input:
```
Production Kubernetes cluster with 8 cores and ARM architecture for cost savings
```

Parsed:
- minVCPU: 8
- architecture: arm64
- useCase: kubernetes
- maxInterruption: 1

### Example 3: Database Server

Input:
```
MySQL database server with 32GB RAM and maximum stability
```

Parsed:
- minMemory: 32
- useCase: database
- maxInterruption: 0

## Integration

### Web UI

The natural language parser is integrated into the web UI. Simply type your requirements in the text area and click "Parse Requirements".

### API

You can also call the parser API directly:

```bash
curl -X POST http://localhost:8080/api/parse-requirements \
  -H "Content-Type: application/json" \
  -d '{"text": "small kubernetes cluster with ARM"}'
```

Response:
```json
{
  "minVcpu": 1,
  "maxVcpu": 2,
  "minMemory": 4,
  "maxMemory": 4,
  "architecture": "arm64",
  "useCase": "kubernetes",
  "maxInterruption": 1,
  "explanation": "Small instance (1-2 vCPU) | Kubernetes use case: prioritizing stability | ARM/Graviton architecture: better cost efficiency"
}
```

## Extending the Parser

The parser is designed to be easily extensible. To add new patterns:

1. Add keyword detection in `parseNaturalLanguage()` function
2. Map keywords to configuration values
3. Add explanation text for user feedback

The parser is located in `internal/web/server.go`.
