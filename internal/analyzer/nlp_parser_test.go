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
			name:            "Kubernetes cluster",
			input:           "Kubernetes cluster for container workloads",
			expectedMinCPU:  2,
			expectedMinMem:  4,
			expectedUseCase: "kubernetes",
		},
		{
			name:            "Database server",
			input:           "PostgreSQL database server",
			expectedMinCPU:  2,
			expectedMinMem:  8,
			expectedUseCase: "database",
		},
		{
			name:            "Small development instance",
			input:           "Small development environment for testing",
			expectedMinCPU:  1,
			expectedMinMem:  1,
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
