package analyzer

import (
	"testing"
)

func TestNLPParserRuleBased(t *testing.T) {
	parser := NewNLPParser()

	testCases := []struct {
		name            string
		input           string
		expectedMinCPU  int
		expectedMinMem  int
		expectedUseCase string
	}{
		{
			name:            "Quantum computing workload",
			input:           "I need workload for quantum computing",
			expectedMinCPU:  32,
			expectedMinMem:  128,
			expectedUseCase: "hpc",
		},
		{
			name:            "HPC scientific simulation",
			input:           "Scientific simulation for research",
			expectedMinCPU:  32,
			expectedMinMem:  128,
			expectedUseCase: "hpc",
		},
		{
			name:            "ML training workload",
			input:           "Machine learning training for deep neural networks",
			expectedMinCPU:  16,
			expectedMinMem:  64,
			expectedUseCase: "ml",
		},
		{
			name:            "LLM inference",
			input:           "Running GPT inference with transformers",
			expectedMinCPU:  8,
			expectedMinMem:  64,
			expectedUseCase: "ml",
		},
		{
			name:            "Kubernetes cluster - standard",
			input:           "Kubernetes cluster for container workloads",
			expectedMinCPU:  4,
			expectedMinMem:  8,
			expectedUseCase: "kubernetes",
		},
		{
			name:            "Database server - standard",
			input:           "PostgreSQL database server",
			expectedMinCPU:  4,
			expectedMinMem:  32,
			expectedUseCase: "database",
		},
		{
			name:            "Small development instance",
			input:           "Small development environment for testing",
			expectedMinCPU:  1,
			expectedMinMem:  2,
			expectedUseCase: "",
		},
		{
			name:            "Video encoding",
			input:           "Video transcoding and encoding pipeline",
			expectedMinCPU:  8,
			expectedMinMem:  16,
			expectedUseCase: "batch",
		},
		{
			name:            "CI/CD pipeline",
			input:           "Jenkins build pipeline for CI/CD",
			expectedMinCPU:  4,
			expectedMinMem:  8,
			expectedUseCase: "batch",
		},
		{
			name:            "Gaming server",
			input:           "Game server for multiplayer gaming",
			expectedMinCPU:  4,
			expectedMinMem:  16,
			expectedUseCase: "general",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use rule-based parsing directly to avoid network calls in tests
			result := parser.parseWithRules(tc.input)

			if result.MinVCPU != tc.expectedMinCPU {
				t.Errorf("MinVCPU: expected %d, got %d", tc.expectedMinCPU, result.MinVCPU)
			}
			if result.MinMemory != tc.expectedMinMem {
				t.Errorf("MinMemory: expected %d, got %d", tc.expectedMinMem, result.MinMemory)
			}
			if tc.expectedUseCase != "" && result.UseCase != tc.expectedUseCase {
				t.Errorf("UseCase: expected %s, got %s", tc.expectedUseCase, result.UseCase)
			}
			if result.Explanation == "" {
				t.Error("Explanation should not be empty")
			}
		})
	}
}

func TestNLPParserHeavyWorkloads(t *testing.T) {
	parser := NewNLPParser()

	testCases := []struct {
		name           string
		input          string
		minExpectedCPU int
		minExpectedMem int
		expectedUseCase string
	}{
		{
			name:           "Heavy Kubernetes workload",
			input:          "kubernetes heavy workload for production tuning",
			minExpectedCPU: 16,
			minExpectedMem: 64,
			expectedUseCase: "kubernetes",
		},
		{
			name:           "Weather forecasting (scientific domain)",
			input:          "weather forecast prediction system",
			minExpectedCPU: 32,
			minExpectedMem: 128,
			expectedUseCase: "hpc",
		},
		{
			name:           "Heavy Kubernetes with weather forecasting",
			input:          "kubernetes heavy workload for tuning and weather forecast",
			minExpectedCPU: 32,  // Should detect weather forecast as HPC
			minExpectedMem: 128,
			expectedUseCase: "hpc",
		},
		{
			name:           "Production database",
			input:          "production heavy PostgreSQL database for enterprise",
			minExpectedCPU: 16,
			minExpectedMem: 128,
			expectedUseCase: "database",
		},
		{
			name:           "Intensive data processing",
			input:          "intensive data processing pipeline with real-time analytics",
			minExpectedCPU: 16,
			minExpectedMem: 64,
			expectedUseCase: "",
		},
		{
			name:           "Monte Carlo simulation",
			input:          "monte carlo simulation for financial risk",
			minExpectedCPU: 32,
			minExpectedMem: 128,
			expectedUseCase: "hpc",
		},
		{
			name:           "Genomics processing",
			input:          "genomic sequencing and bioinformatics pipeline",
			minExpectedCPU: 32,
			minExpectedMem: 128,
			expectedUseCase: "hpc",
		},
		{
			name:           "Enterprise Kubernetes",
			input:          "enterprise-grade kubernetes cluster for mission-critical applications",
			minExpectedCPU: 16,
			minExpectedMem: 64,
			expectedUseCase: "kubernetes",
		},
		{
			name:           "Light dev workload",
			input:          "small development kubernetes for testing and prototyping",
			minExpectedCPU: 1,
			minExpectedMem: 2,
			expectedUseCase: "kubernetes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.parseWithRules(tc.input)

			if result.MinVCPU < tc.minExpectedCPU {
				t.Errorf("MinVCPU: expected at least %d, got %d (explanation: %s)", 
					tc.minExpectedCPU, result.MinVCPU, result.Explanation)
			}
			if result.MinMemory < tc.minExpectedMem {
				t.Errorf("MinMemory: expected at least %d, got %d (explanation: %s)", 
					tc.minExpectedMem, result.MinMemory, result.Explanation)
			}
			if tc.expectedUseCase != "" && result.UseCase != tc.expectedUseCase {
				t.Errorf("UseCase: expected %s, got %s", tc.expectedUseCase, result.UseCase)
			}
			t.Logf("Parsed '%s' -> vCPU: %d-%d, Memory: %d-%dGB, UseCase: %s, Explanation: %s",
				tc.input, result.MinVCPU, result.MaxVCPU, result.MinMemory, result.MaxMemory, 
				result.UseCase, result.Explanation)
		})
	}
}

func TestWorkloadIntensityDetection(t *testing.T) {
	testCases := []struct {
		input            string
		expectedIntensity WorkloadIntensity
	}{
		{"light workload for testing", IntensityLight},
		{"small development environment", IntensityLight},
		{"heavy production workload", IntensityHeavy},
		{"intensive data processing", IntensityHeavy},
		{"enterprise-grade system", IntensityHeavy},
		{"massive scale processing", IntensityExtreme},
		{"planet-scale database", IntensityExtreme},
		{"moderate workload", IntensityMedium},
		{"standard kubernetes cluster", IntensityMedium},
		{"just a simple web app", IntensityDefault},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			intensity, keyword := detectWorkloadIntensity(tc.input)
			if intensity != tc.expectedIntensity {
				t.Errorf("Expected intensity %v, got %v (matched: '%s')", 
					tc.expectedIntensity, intensity, keyword)
			}
		})
	}
}

func TestDomainWorkloadDetection(t *testing.T) {
	testCases := []struct {
		input          string
		expectScientific bool
		expectedDesc   string
	}{
		{"weather forecast system", true, "Forecasting/prediction"},
		{"climate modeling simulation", true, "Climate simulation"},
		{"genomic sequencing pipeline", true, "Genomics processing"},
		{"monte carlo risk analysis", true, "Monte Carlo simulation"},
		{"CFD aerodynamics simulation", true, "Computational Fluid Dynamics"},
		{"bioinformatics analysis", true, "Bioinformatics"},
		{"protein folding computation", true, "Protein folding"},
		{"simple web application", false, ""},
		{"kubernetes cluster", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			isScientific, _, desc := detectDomainWorkload(tc.input)
			if isScientific != tc.expectScientific {
				t.Errorf("Expected scientific=%v, got %v", tc.expectScientific, isScientific)
			}
			if tc.expectScientific && desc != tc.expectedDesc {
				t.Errorf("Expected desc '%s', got '%s'", tc.expectedDesc, desc)
			}
		})
	}
}

func TestNLPParserExplicitNumbers(t *testing.T) {
	parser := NewNLPParser()

	testCases := []struct {
		name           string
		input          string
		expectedMinCPU int
		expectedMinMem int
	}{
		{
			name:           "Explicit CPU count",
			input:          "I need 8 vcpu for my workload",
			expectedMinCPU: 8,
			expectedMinMem: 4, // default
		},
		{
			name:           "Explicit memory",
			input:          "Need instances with 32gb ram",
			expectedMinCPU: 2, // default
			expectedMinMem: 32,
		},
		{
			name:           "Both explicit",
			input:          "16 cores and 64gb memory",
			expectedMinCPU: 16,
			expectedMinMem: 64,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.parseWithRules(tc.input)

			if result.MinVCPU != tc.expectedMinCPU {
				t.Errorf("MinVCPU: expected %d, got %d", tc.expectedMinCPU, result.MinVCPU)
			}
			if result.MinMemory != tc.expectedMinMem {
				t.Errorf("MinMemory: expected %d, got %d", tc.expectedMinMem, result.MinMemory)
			}
		})
	}
}

func TestNLPParserArchitecture(t *testing.T) {
	parser := NewNLPParser()

	testCases := []struct {
		name         string
		input        string
		expectedArch string
	}{
		{
			name:         "Intel preference",
			input:        "Intel processors for compatibility",
			expectedArch: "intel",
		},
		{
			name:         "AMD preference",
			input:        "AMD instances for cost savings",
			expectedArch: "amd",
		},
		{
			name:         "ARM/Graviton preference",
			input:        "Graviton ARM instances for best value",
			expectedArch: "arm64",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.parseWithRules(tc.input)

			if result.Architecture != tc.expectedArch {
				t.Errorf("Architecture: expected %s, got %s", tc.expectedArch, result.Architecture)
			}
		})
	}
}

func TestNLPParserGPU(t *testing.T) {
	parser := NewNLPParser()

	testCases := []struct {
		name         string
		input        string
		expectedGPU  bool
		expectedType string
	}{
		{
			name:         "Explicit GPU request",
			input:        "Need GPU instances for ML",
			expectedGPU:  true,
			expectedType: "nvidia-t4",
		},
		{
			name:         "NVIDIA A100",
			input:        "NVIDIA A100 GPU for training",
			expectedGPU:  true,
			expectedType: "nvidia-a100",
		},
		{
			name:         "CUDA workload",
			input:        "CUDA deep learning workload",
			expectedGPU:  true,
			expectedType: "nvidia-t4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.parseWithRules(tc.input)

			if result.NeedsGPU != tc.expectedGPU {
				t.Errorf("NeedsGPU: expected %v, got %v", tc.expectedGPU, result.NeedsGPU)
			}
			if tc.expectedGPU && result.GPUType != tc.expectedType {
				t.Errorf("GPUType: expected %s, got %s", tc.expectedType, result.GPUType)
			}
		})
	}
}
