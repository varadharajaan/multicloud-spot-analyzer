# Natural Language Parser

The Spot Analyzer includes an intelligent natural language parser that converts human-readable requirements into specific instance configurations.

## How It Works

The parser uses a **multi-provider architecture** with automatic fallback:

```
┌─────────────────────────────────────────────────────────────────┐
│                    NLP Provider Priority                         │
├─────────────────────────────────────────────────────────────────┤
│  1️⃣  Embedded     │ Pure Go TF-IDF ML (PRIMARY, always works)  │
│  2️⃣  Ollama       │ Local LLM (optional, better quality)       │
│  3️⃣  OpenAI       │ Cloud API (if API key configured)          │
│  4️⃣  HuggingFace  │ Cloud API (free, rate-limited)             │
│  5️⃣  Rules        │ Pattern matching (final fallback)          │
└─────────────────────────────────────────────────────────────────┘
```

### Provider Details

| Provider | Description | Network | API Key | Best For |
|----------|-------------|---------|---------|----------|
| **Embedded** | Pure Go TF-IDF ML classifier | ❌ No | ❌ No | Default, offline use |
| **Ollama** | Local LLM (llama3.2, mistral) | ❌ No | ❌ No | Better semantic understanding |
| **OpenAI** | GPT-3.5/4 API | ✅ Yes | ✅ Yes | Highest accuracy |
| **HuggingFace** | Zero-shot classification | ✅ Yes | Optional | Free cloud option |
| **Rules** | Keyword pattern matching | ❌ No | ❌ No | Simple fallback |

### Embedded ML Provider (Default)

The **Embedded provider** is the primary NLP engine. It uses **TF-IDF (Term Frequency-Inverse Document Frequency)** text classification - a proven ML technique that runs entirely in Go with **zero external dependencies**.

**Features:**
- 12 workload categories with weighted keywords
- Intensity detection (light, medium, heavy, extreme)
- Domain-specific HPC detection (weather, genomics, CFD, etc.)
- Confidence scoring
- No network required, works completely offline

**Example output:**
```
🧠 [Embedded ML] HPC/Scientific workload (78% confidence) | 💪 Heavy | 32-96 vCPU, 128-512GB RAM
```

### Workload Categories

The parser classifies input into these workload categories:

| Category | vCPU Range | Memory Range | GPU | Use Case |
|----------|------------|--------------|-----|----------|
| HPC/Scientific | 32-96 | 128-512 GB | Optional | hpc |
| ML Training | 16-64 | 64-256 GB | ✅ T4 | ml |
| LLM/Inference | 8-32 | 64-256 GB | ✅ A10G | ml |
| Data Analytics | 8-32 | 32-128 GB | ❌ | batch |
| Media Processing | 8-32 | 16-64 GB | ❌ | batch |
| Gaming Servers | 4-16 | 16-64 GB | ❌ | general |
| 3D Rendering | 16-64 | 32-128 GB | ✅ T4 | batch |
| CI/CD Pipelines | 4-16 | 8-32 GB | ❌ | batch |
| Kubernetes | 4-16 | 8-32 GB | ❌ | kubernetes |
| Database | 4-16 | 32-128 GB | ❌ | database |
| Web/API | 2-8 | 4-16 GB | ❌ | general |
| Dev/Test | 1-4 | 2-8 GB | ❌ | batch |

### Intensity Modifiers

The parser detects workload intensity and scales resources accordingly:

| Intensity | Keywords | CPU Multiplier | Memory Multiplier |
|-----------|----------|----------------|-------------------|
| 🚀 Extreme | massive, planet-scale, hyperscale | 4x (min 64) | 4x (min 256GB) |
| 💪 Heavy | heavy, production, enterprise, intensive | 2x (min 16) | 2x (min 64GB) |
| ⚖️ Medium | moderate, standard, typical | 1.5x | 1.5x |
| 🌱 Light | small, dev, testing, poc, prototype | 0.5x | 0.5x |

### Domain-Specific Detection

The parser recognizes specialized scientific/HPC domains:

| Domain | Keywords | Detected As |
|--------|----------|-------------|
| Weather/Climate | weather, climate, forecast, atmospheric | HPC |
| Life Sciences | genomic, bioinformatics, protein, dna, rna | HPC |
| Physics/Engineering | cfd, fea, molecular dynamics, quantum | HPC |
| Finance | monte carlo, risk calculation, quant, trading | HPC |
| Rendering | render farm, ray tracing, path tracing | HPC |

## Processing Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│                    Natural Language Input                        │
│  "kubernetes heavy workload for weather forecasting"            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    1. Provider Selection                         │
│  Try: Embedded → Ollama → OpenAI → HuggingFace → Rules          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    2. Text Classification (Embedded ML)          │
│  - Tokenization & TF-IDF scoring                                │
│  - Category matching (12 workload types)                        │
│  - Confidence calculation                                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    3. Intensity Detection                        │
│  Keywords: heavy, production, enterprise, light, dev, test      │
│  → Apply resource multipliers                                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    4. Domain Detection                           │
│  Scientific: weather, genomics, CFD, monte carlo, etc.          │
│  → Override to HPC workload (32-96 vCPU, 128-512GB RAM)         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    5. Explicit Number Extraction                 │
│  Patterns: "8 vcpu", "32gb", "16 cores"                         │
│  → Override category defaults with exact values                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    6. Architecture & GPU Detection               │
│  Keywords: intel, amd, arm, graviton, gpu, nvidia, cuda         │
│  → Set preferences                                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Configuration Output                          │
│  {minVcpu: 32, maxVcpu: 96, minMemory: 128, useCase: "hpc"}     │
│  Explanation: "🧠 [Embedded ML] HPC workload (78% confidence)"  │
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

The NLP system is implemented in `internal/nlp/` package with multiple providers:

```
internal/nlp/
├── provider.go      # Provider interface & types
├── manager.go       # Provider manager & routing
├── embedded.go      # TF-IDF ML classifier (PRIMARY)
├── ollama.go        # Local LLM provider
├── openai.go        # OpenAI API provider
├── huggingface.go   # HuggingFace API provider
└── rules.go         # Pattern matching fallback
```

### Using the Parser

```go
import "github.com/spot-analyzer/internal/analyzer"

// Create parser (auto-selects best available provider)
parser := analyzer.NewNLPParser()

// Parse natural language
result, err := parser.Parse("kubernetes heavy workload for weather forecasting")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("vCPU: %d-%d\n", result.MinVCPU, result.MaxVCPU)
fmt.Printf("Memory: %d-%d GB\n", result.MinMemory, result.MaxMemory)
fmt.Printf("Use Case: %s\n", result.UseCase)
fmt.Printf("Explanation: %s\n", result.Explanation)

// Output:
// vCPU: 32-96
// Memory: 128-512 GB
// Use Case: hpc
// Explanation: 🧠 [Embedded ML] HPC/Scientific workload (78% confidence) | 💪 Heavy | 32-96 vCPU, 128-512GB RAM
```

### Custom Provider Configuration

```go
import "github.com/spot-analyzer/internal/nlp"

// Force specific provider
config := nlp.Config{
    Provider: nlp.ProviderEmbedded,  // or ProviderOllama, ProviderOpenAI, etc.
    TimeoutSeconds: 30,
}

parser := analyzer.NewNLPParserWithConfig(config)
```

### Available Providers

```go
// Check which providers are available
providers := parser.GetAvailableProviders()
fmt.Println(providers) // ["embedded", "rules"] or ["embedded", "ollama/llama3.2", "rules"]
```

## Examples

### Example 1: Heavy Kubernetes with Scientific Workload

Input:
```
kubernetes heavy workload for weather forecasting
```

Parsed:
- minVCPU: 32
- maxVCPU: 96
- minMemory: 128
- maxMemory: 512
- useCase: hpc
- maxInterruption: 2

Explanation: "🧠 [Embedded ML] HPC/Scientific workload (78% confidence) | 💪 Heavy | 32-96 vCPU, 128-512GB RAM"

**Why HPC?** The parser detected "weather forecasting" as a scientific domain, which takes priority over "kubernetes".

### Example 2: Production Database

Input:
```
production heavy PostgreSQL database for enterprise
```

Parsed:
- minVCPU: 16
- maxVCPU: 64
- minMemory: 128
- maxMemory: 512
- useCase: database
- maxInterruption: 0

Explanation: "🧠 [Embedded ML] Database workload (85% confidence) | 💪 Heavy | 16-64 vCPU, 128-512GB RAM"

### Example 3: Light Development Testing

Input:
```
small development kubernetes for testing and prototyping
```

Parsed:
- minVCPU: 1
- maxVCPU: 4
- minMemory: 2
- maxMemory: 8
- useCase: kubernetes
- maxInterruption: 1

Explanation: "🧠 [Embedded ML] Kubernetes workload (72% confidence) | 🌱 Light | 1-4 vCPU, 2-8GB RAM"

### Example 4: ML Training with GPU

Input:
```
deep learning training with NVIDIA A100 GPU
```

Parsed:
- minVCPU: 16
- maxVCPU: 64
- minMemory: 64
- maxMemory: 256
- useCase: ml
- needsGPU: true
- gpuType: nvidia-a100
- maxInterruption: 2

Explanation: "🧠 [Embedded ML] ML Training workload (91% confidence) | 16-64 vCPU, 64-256GB RAM | GPU: nvidia-a100"

### Example 5: Genomics Processing

Input:
```
genomic sequencing and bioinformatics pipeline
```

Parsed:
- minVCPU: 32
- maxVCPU: 96
- minMemory: 128
- maxMemory: 512
- useCase: hpc
- maxInterruption: 2

Explanation: "🧠 [Embedded ML] HPC/Scientific workload (82% confidence) | 32-96 vCPU, 128-512GB RAM"

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

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OLLAMA_ENDPOINT` | Ollama API endpoint | `http://localhost:11434` |
| `OLLAMA_MODEL` | Ollama model to use | `llama3.2` |
| `OPENAI_API_KEY` | OpenAI API key | (none) |
| `OPENAI_MODEL` | OpenAI model | `gpt-3.5-turbo` |
| `HUGGINGFACE_TOKEN` | HuggingFace API token | (none, uses free tier) |

### Installing Ollama (Optional)

For better semantic understanding, install Ollama:

```bash
# Windows
winget install Ollama.Ollama

# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# Pull a model
ollama pull llama3.2
```

The parser will automatically detect and use Ollama when available.

## Future Improvements

- [x] ~~Use regex for more flexible number extraction~~
- [x] ~~AI-powered classification with multiple providers~~
- [x] ~~Intensity detection (heavy, light, etc.)~~
- [x] ~~Domain-specific HPC detection~~
- [x] ~~GPU detection and type selection~~
- [x] ~~Pure Go embedded ML (no external dependencies)~~
- [ ] Add support for instance family preferences
- [ ] Implement fuzzy matching for typo tolerance
- [ ] Add price range detection ("under $0.10/hour")
- [ ] Support for region preferences ("in Europe")
- [ ] Multi-language support

