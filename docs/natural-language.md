# Natural Language Parser

The Spot Analyzer includes an intelligent natural language parser that converts human-readable requirements into specific instance configurations.

## How It Works

The parser uses a rule-based approach with pattern matching to extract meaningful configuration from natural language input. It analyzes your text in the following order:

### Processing Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│                    Natural Language Input                        │
│  "I need a small Kubernetes cluster with ARM for testing"       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    1. Text Normalization                         │
│  - Convert to lowercase                                          │
│  - Tokenize into words                                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    2. Size Detection                             │
│  Keywords: small, tiny, micro, medium, large, xlarge             │
│  → Sets MinVCPU, MaxVCPU, MinMemory, MaxMemory                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    3. Numeric Extraction                         │
│  Patterns: "4 vCPU", "16gb", "8 cores"                           │
│  → Overrides size-based values with exact numbers                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    4. Use Case Classification                    │
│  Keywords: kubernetes, database, batch, web, autoscaling         │
│  → Sets UseCase and MaxInterruption                              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    5. Architecture Detection                     │
│  Keywords: intel, amd, arm, graviton                             │
│  → Sets Architecture preference                                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    6. Explanation Generation                     │
│  Builds human-readable explanation of parsed values              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Configuration Output                          │
│  {minVcpu: 1, maxVcpu: 2, architecture: "arm64", ...}            │
└─────────────────────────────────────────────────────────────────┘
```

## Parser Algorithm

The parser extracts:
1. **Size indicators** (small, medium, large)
2. **Specific numbers** (4 vCPU, 16GB RAM)
3. **Use case keywords** (kubernetes, database, batch)
4. **Architecture preferences** (intel, amd, arm, graviton)
5. **Scale patterns** (weekend, temporary, production)

### Default Values

When no patterns are detected, the parser uses these defaults:
- `minVcpu`: 2
- `maxVcpu`: 0 (unlimited)
- `minMemory`: 4 GB
- `maxInterruption`: 2 (10-15%)

## Supported Patterns

### Size Keywords

| Keyword | vCPU Range | Memory Range |
|---------|------------|--------------|
| small, tiny, micro | 1-2 | 1-4 GB |
| medium, moderate | 2-4 | 4-16 GB |
| large, big | 4-8 | 16-64 GB |
| xlarge, huge, extra large | 8-32 | 32+ GB |

### Use Case Keywords

| Keywords | Use Case | Stability | MaxInterruption |
|----------|----------|-----------|-----------------|
| kubernetes, k8s, cluster | Kubernetes | High | 1 (5-10%) |
| database, db, postgres, mysql, mongo, redis | Database | Maximum | 0 (<5%) |
| autoscaling, asg, auto scaling | Auto Scaling | Moderate | 2 (10-15%) |
| weekend, batch, job, temporary, short | Batch | Low | 3 (15-20%) |
| web, api, server | Web Server | Moderate | 2 (10-15%) |

### Architecture Keywords

| Keywords | Architecture | Notes |
|----------|--------------|-------|
| intel | Intel x86_64 | Traditional Intel processors |
| amd | AMD x86_64 | AMD EPYC processors |
| arm, graviton | ARM64 | AWS Graviton (better cost efficiency) |

### Specific Numbers

The parser extracts numbers from patterns like:
- "4 vCPU" → minVCPU = 4
- "16gb" → minMemory = 16
- "8 cores" → minVCPU = 8

## Code Implementation

The parser is implemented in `internal/web/server.go` in the `parseNaturalLanguage()` function:

```go
func parseNaturalLanguage(text string) ParseRequirementsResponse {
    text = strings.ToLower(text)
    resp := ParseRequirementsResponse{
        MinVCPU:         2,
        MaxVCPU:         0,
        MinMemory:       4,
        MaxInterruption: 2,
    }
    
    // 1. Parse size keywords
    if strings.Contains(text, "small") || strings.Contains(text, "tiny") {
        resp.MinVCPU = 1
        resp.MaxVCPU = 2
        resp.MinMemory = 1
        resp.MaxMemory = 4
    }
    
    // 2. Extract numeric values
    for _, word := range strings.Fields(text) {
        if strings.HasSuffix(word, "gb") {
            numStr := strings.TrimSuffix(word, "gb")
            if num, err := strconv.Atoi(numStr); err == nil {
                resp.MinMemory = num
            }
        }
    }
    
    // 3. Detect use case
    if strings.Contains(text, "kubernetes") || strings.Contains(text, "k8s") {
        resp.UseCase = "kubernetes"
        resp.MaxInterruption = 1
    }
    
    // 4. Detect architecture
    if strings.Contains(text, "arm") || strings.Contains(text, "graviton") {
        resp.Architecture = "arm64"
    }
    
    return resp
}
```

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

Explanation: "Small instance (1-2 vCPU) | Batch/temporary use case: prioritizing cost savings"

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

Explanation: "Detected 8 vCPU requirement | Kubernetes use case: prioritizing stability | ARM/Graviton architecture: better cost efficiency"

### Example 3: Database Server

Input:
```
MySQL database server with 32GB RAM and maximum stability
```

Parsed:
- minMemory: 32
- useCase: database
- maxInterruption: 0

Explanation: "Detected 32GB memory requirement | Database use case: maximum stability required"

## Integration

### Web UI

The natural language parser is integrated into both UI versions:
- **Classic UI (v1)**: Type in the "Describe Your Needs" text area
- **Modern UI (v2)**: Type in the "Natural Language Query" card

Click "Parse & Configure" to automatically populate the configuration fields.

### API

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

### Programmatic API (Controller)

```go
import "github.com/spot-analyzer/internal/controller"

ctrl := controller.New()

// Parse and analyze in one step
result, err := ctrl.Analyze(ctx, controller.AnalyzeRequest{
    NaturalLanguage: "medium kubernetes cluster with ARM",
})
```

## Extending the Parser

The parser is designed to be easily extensible. To add new patterns:

1. Add keyword detection in `parseNaturalLanguage()` function
2. Map keywords to configuration values
3. Add explanation text for user feedback

### Example: Adding GPU Detection

```go
// Detect GPU requirements
if strings.Contains(text, "gpu") || strings.Contains(text, "cuda") ||
   strings.Contains(text, "ml") || strings.Contains(text, "machine learning") {
    resp.UseCase = "gpu"
    resp.MinVCPU = 4
    resp.MinMemory = 16
    explanations = append(explanations, "GPU/ML use case: selecting GPU-capable instances")
}
```

## Future Improvements

- [ ] Use regex for more flexible number extraction
- [ ] Add support for instance family preferences
- [ ] Implement fuzzy matching for typo tolerance
- [ ] Add price range detection ("under $0.10/hour")
- [ ] Support for region preferences ("in Europe")

