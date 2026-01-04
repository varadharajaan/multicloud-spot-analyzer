// Package cli implements the command-line interface for the spot analyzer.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spot-analyzer/internal/analyzer"
	"github.com/spot-analyzer/internal/domain"
	"github.com/spot-analyzer/internal/logging"
	"github.com/spot-analyzer/internal/provider"
	awsprovider "github.com/spot-analyzer/internal/provider/aws"
	azureprovider "github.com/spot-analyzer/internal/provider/azure"
)

// CLI encapsulates the command-line interface
type CLI struct {
	rootCmd *cobra.Command
	logger  *logging.Logger
}

// New creates a new CLI instance
func New() *CLI {
	logger, _ := logging.New(logging.Config{
		Level:       logging.INFO,
		LogDir:      "logs",
		EnableFile:  true,
		EnableColor: true,
	})
	cli := &CLI{logger: logger}
	cli.buildCommands()
	return cli
}

// Execute runs the CLI
func (c *CLI) Execute() error {
	return c.rootCmd.Execute()
}

// buildCommands constructs the command tree
func (c *CLI) buildCommands() {
	c.rootCmd = &cobra.Command{
		Use:   "spot-analyzer",
		Short: "Multi-cloud spot instance analyzer by Varadharajan",
		Long: `
   _____ ____   ___ _____     _    _   _    _    _  __   ____________ ____  
  / ___/|  _ \ / _ \_   _|   / \  | \ | |  / \  | | \ \ / /__  / ____|  _ \ 
  \___ \| |_) | | | || |    / _ \ |  \| | / _ \ | |  \ V /  / /|  _| | |_) |
   ___) |  __/| |_| || |   / ___ \| |\  |/ ___ \| |___| |  / /_| |___|  _ < 
  |____/|_|    \___/ |_|  /_/   \_\_| \_/_/   \_\_____|_| /____|_____|_| \_\

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Author: Varadharajan | https://github.com/varadharajaan
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  An intelligent tool for analyzing and recommending optimal spot/preemptible 
  instances across cloud providers (AWS, Azure, GCP).

  Uses real-time spot pricing data, interruption rates, and instance specs 
  to provide data-driven recommendations for your workloads.`,
		Version: "1.0.0",
	}

	// Add subcommands
	c.rootCmd.AddCommand(c.analyzeCmd())
	c.rootCmd.AddCommand(c.regionsCmd())
	c.rootCmd.AddCommand(c.refreshCmd())
	c.rootCmd.AddCommand(c.predictCmd())
	c.rootCmd.AddCommand(c.azCmd())
	c.rootCmd.AddCommand(c.webCmd())
}

// analyzeCmd creates the analyze command
func (c *CLI) analyzeCmd() *cobra.Command {
	var (
		cloudProvider   string
		region          string
		osType          string
		minVCPU         int
		maxVCPU         int
		minMemoryGB     float64
		maxMemoryGB     float64
		requiresGPU     bool
		minGPUCount     int
		gpuType         string
		architecture    string
		category        string
		maxInterruption int
		minSavings      int
		allowBurstable  bool
		allowBareMetal  bool
		families        []string
		topN            int
		outputFormat    string
		enhancedMode    bool
		debugMode       bool
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze spot instances and get recommendations",
		Long: `Analyze available spot instances based on your requirements
and get intelligent recommendations for the best instances to use.

Examples:
  # Find best 2 vCPU instances in us-east-1
  spot-analyzer analyze --vcpu 2 --region us-east-1

  # Find compute-optimized 4 vCPU instances with low interruption
  spot-analyzer analyze --vcpu 4 --category compute --max-interruption 1

  # Find GPU instances with at least 1 GPU
  spot-analyzer analyze --gpu --min-gpu-count 1 --region us-west-2

  # Find ARM-based instances for cost savings
  spot-analyzer analyze --vcpu 4 --arch arm64 --region eu-west-1
  
  # Use enhanced AI analysis with additional scoring factors
  spot-analyzer analyze --vcpu 2 --enhanced`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.logger.Info("Starting analyze command: provider=%s region=%s vcpu=%d-%d memory=%.0f-%.0f enhanced=%v",
				cloudProvider, region, minVCPU, maxVCPU, minMemoryGB, maxMemoryGB, enhancedMode)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Parse and validate inputs
			cp := parseCloudProvider(cloudProvider)
			os := parseOS(osType)
			cat := parseCategory(category)
			maxInt := domain.InterruptionFrequency(maxInterruption)

			requirements := domain.UsageRequirements{
				MinVCPU:           minVCPU,
				MaxVCPU:           maxVCPU,
				MinMemoryGB:       minMemoryGB,
				MaxMemoryGB:       maxMemoryGB,
				RequiresGPU:       requiresGPU,
				MinGPUCount:       minGPUCount,
				GPUType:           gpuType,
				Architecture:      architecture,
				PreferredCategory: cat,
				Region:            region,
				OS:                os,
				MaxInterruption:   maxInt,
				MinSavingsPercent: minSavings,
				AllowBurstable:    allowBurstable,
				AllowBareMetal:    allowBareMetal,
				Families:          families,
				TopN:              topN,
			}

			return c.runAnalysis(ctx, cp, requirements, outputFormat, enhancedMode, debugMode, families)
		},
	}

	// Define flags
	cmd.Flags().StringVar(&cloudProvider, "cloud", "aws", "Cloud provider (aws, azure, gcp)")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "Cloud region")
	cmd.Flags().StringVar(&osType, "os", "linux", "Operating system (linux, windows)")
	cmd.Flags().IntVar(&minVCPU, "vcpu", 2, "Minimum vCPU cores required")
	cmd.Flags().IntVar(&maxVCPU, "max-vcpu", 0, "Maximum vCPU cores (0 = no limit)")
	cmd.Flags().Float64Var(&minMemoryGB, "memory", 0, "Minimum memory in GB")
	cmd.Flags().Float64Var(&maxMemoryGB, "max-memory", 0, "Maximum memory in GB (0 = no limit)")
	cmd.Flags().BoolVar(&requiresGPU, "gpu", false, "Require GPU instances")
	cmd.Flags().IntVar(&minGPUCount, "min-gpu-count", 0, "Minimum number of GPUs")
	cmd.Flags().StringVar(&gpuType, "gpu-type", "", "Preferred GPU type (e.g., 'nvidia', 'a100')")
	cmd.Flags().StringVar(&architecture, "arch", "", "CPU architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&category, "category", "", "Instance category (general, compute, memory, storage)")
	cmd.Flags().IntVar(&maxInterruption, "max-interruption", 2, "Max interruption level (0=<5%, 1=5-10%, 2=10-15%, 3=15-20%, 4=>20%)")
	cmd.Flags().IntVar(&minSavings, "min-savings", 0, "Minimum savings percentage")
	cmd.Flags().BoolVar(&allowBurstable, "allow-burstable", true, "Include burstable instances (t2, t3, etc.)")
	cmd.Flags().BoolVar(&allowBareMetal, "allow-bare-metal", false, "Include bare metal instances")
	cmd.Flags().StringSliceVar(&families, "families", nil, "Filter by instance families (e.g., --families t,m,c)")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of top instances to return")
	cmd.Flags().StringVar(&outputFormat, "output", "table", "Output format (table, json, simple)")
	cmd.Flags().BoolVar(&enhancedMode, "enhanced", false, "Use enhanced AI analysis with volatility, trends, and hidden gem detection")
	cmd.Flags().BoolVar(&debugMode, "debug", false, "Show debug info: raw API data source verification")

	return cmd
}

// regionsCmd creates the regions command
func (c *CLI) regionsCmd() *cobra.Command {
	var cloudProvider string

	cmd := &cobra.Command{
		Use:   "regions",
		Short: "List available regions for a cloud provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cp := parseCloudProvider(cloudProvider)
			return c.listRegions(ctx, cp)
		},
	}

	cmd.Flags().StringVar(&cloudProvider, "cloud", "aws", "Cloud provider (aws, azure, gcp)")

	return cmd
}

// refreshCmd creates the refresh command
func (c *CLI) refreshCmd() *cobra.Command {
	var cloudProvider string

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh cached spot instance data",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			cp := parseCloudProvider(cloudProvider)
			return c.refreshData(ctx, cp)
		},
	}

	cmd.Flags().StringVar(&cloudProvider, "cloud", "aws", "Cloud provider (aws, azure, gcp)")

	return cmd
}

// predictCmd creates the price prediction command
func (c *CLI) predictCmd() *cobra.Command {
	var (
		cloudProvider string
		instanceType  string
		region        string
	)

	cmd := &cobra.Command{
		Use:   "predict",
		Short: "Predict future spot prices for an instance type",
		Long: `Generate price predictions for a specific instance type using
historical price data and trend analysis.

Examples:
  # AWS prediction
  spot-analyzer predict --instance m5.large --region us-east-1

  # Azure prediction
  spot-analyzer predict --cloud azure --instance Standard_D2s_v5 --region eastus`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cp := parseCloudProvider(cloudProvider)
			return c.runPrediction(ctx, cp, instanceType, region)
		},
	}

	cmd.Flags().StringVar(&cloudProvider, "cloud", "aws", "Cloud provider (aws, azure)")
	cmd.Flags().StringVar(&instanceType, "instance", "", "Instance type to predict (required)")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "Cloud region")
	cmd.MarkFlagRequired("instance")

	return cmd
}

// azCmd creates the availability zone recommendation command
func (c *CLI) azCmd() *cobra.Command {
	var (
		cloudProvider string
		instanceType  string
		region        string
	)

	cmd := &cobra.Command{
		Use:   "az",
		Short: "Get availability zone recommendations for an instance type",
		Long: `Analyze spot prices across availability zones and recommend
the best AZ for launching spot instances.

Examples:
  # AWS AZ recommendations
  spot-analyzer az --instance m5.large --region us-east-1

  # Azure AZ recommendations
  spot-analyzer az --cloud azure --instance Standard_D2s_v5 --region eastus`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cp := parseCloudProvider(cloudProvider)
			return c.runAZRecommendation(ctx, cp, instanceType, region)
		},
	}

	cmd.Flags().StringVar(&cloudProvider, "cloud", "aws", "Cloud provider (aws, azure)")
	cmd.Flags().StringVar(&instanceType, "instance", "", "Instance type to analyze (required)")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "Cloud region")
	cmd.MarkFlagRequired("instance")

	return cmd
}

// runPrediction executes price prediction
func (c *CLI) runPrediction(ctx context.Context, cloudProvider domain.CloudProvider, instanceType, region string) error {
	fmt.Printf("🔮 Generating price predictions for %s in %s (%s)...\n\n", instanceType, region, cloudProvider)

	var predEngine *analyzer.PredictionEngine

	switch cloudProvider {
	case domain.Azure:
		priceProvider := azureprovider.NewPriceHistoryProvider(region)
		fmt.Println("☁️ Using Azure Retail Prices API")
		adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, region)

	default: // AWS
		priceProvider, err := awsprovider.NewPriceHistoryProvider(region)
		if err != nil {
			return fmt.Errorf("failed to create price provider: %w", err)
		}

		if priceProvider.IsAvailable() {
			fmt.Println("🔑 Using real AWS price history data")
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			predEngine = analyzer.NewPredictionEngine(adapter, region)
		} else {
			fmt.Println("💡 No AWS credentials - predictions limited")
			predEngine = analyzer.NewPredictionEngine(nil, region)
		}
	}

	prediction, err := predEngine.PredictPrice(ctx, instanceType)
	if err != nil {
		return fmt.Errorf("prediction failed: %w", err)
	}

	// Display prediction
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 PRICE PREDICTION: %s (%s)\n", instanceType, cloudProvider)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("   Current Price:    $%.4f/hr\n", prediction.CurrentPrice)
	fmt.Printf("   Predicted (1h):   $%.4f/hr\n", prediction.PredictedPrice1H)
	fmt.Printf("   Predicted (6h):   $%.4f/hr\n", prediction.PredictedPrice6H)
	fmt.Printf("   Predicted (24h):  $%.4f/hr\n", prediction.PredictedPrice24H)
	fmt.Println()
	fmt.Printf("   📈 Trend:         %s\n", prediction.TrendDirection)
	fmt.Printf("   ⚠️  Volatility:    %s risk\n", prediction.VolatilityRisk)
	fmt.Printf("   🎯 Confidence:    %.0f%%\n", prediction.Confidence*100)
	fmt.Printf("   ⏰ Best Launch:   %s\n", prediction.OptimalLaunchTime)
	fmt.Printf("   📐 Method:        %s\n", prediction.PredictionMethod)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// runAZRecommendation executes AZ analysis
func (c *CLI) runAZRecommendation(ctx context.Context, cloudProvider domain.CloudProvider, instanceType, region string) error {
	fmt.Printf("🌐 Analyzing availability zones for %s in %s (%s)...\n\n", instanceType, region, cloudProvider)

	var predEngine *analyzer.PredictionEngine

	switch cloudProvider {
	case domain.Azure:
		priceProvider := azureprovider.NewPriceHistoryProvider(region)
		fmt.Println("☁️ Using Azure Retail Prices API")
		adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
		predEngine = analyzer.NewPredictionEngine(adapter, region)

	default: // AWS
		priceProvider, err := awsprovider.NewPriceHistoryProvider(region)
		if err != nil {
			return fmt.Errorf("failed to create price provider: %w", err)
		}

		if priceProvider.IsAvailable() {
			fmt.Println("🔑 Using real AWS price history data")
			adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
			predEngine = analyzer.NewPredictionEngine(adapter, region)
		} else {
			fmt.Println("💡 No AWS credentials - AZ analysis limited")
			predEngine = analyzer.NewPredictionEngine(nil, region)
		}
	}

	rec, err := predEngine.RecommendAZ(ctx, instanceType)
	if err != nil {
		return fmt.Errorf("AZ analysis failed: %w", err)
	}

	// Display recommendations
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🌐 AVAILABILITY ZONE RECOMMENDATIONS: %s (%s)\n", instanceType, cloudProvider)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if len(rec.Recommendations) == 0 {
		fmt.Println("   ⚠️ No AZ data available")
		for _, insight := range rec.Insights {
			fmt.Printf("   %s\n", insight)
		}
		return nil
	}

	// Table header
	fmt.Println()
	fmt.Printf("   %-15s %-12s %-12s %-12s %-10s %-6s\n", "AZ", "AVG PRICE", "MIN", "MAX", "VOLATILITY", "RANK")
	fmt.Printf("   %-15s %-12s %-12s %-12s %-10s %-6s\n", "──────────────", "────────────", "────────────", "────────────", "──────────", "──────")

	for _, az := range rec.Recommendations {
		rankEmoji := ""
		if az.Rank == 1 {
			rankEmoji = "🥇"
		} else if az.Rank == 2 {
			rankEmoji = "🥈"
		} else if az.Rank == 3 {
			rankEmoji = "🥉"
		}
		fmt.Printf("   %-15s $%-11.4f $%-11.4f $%-11.4f %-10.2f %s #%d\n",
			az.AvailabilityZone, az.AvgPrice, az.MinPrice, az.MaxPrice, az.Volatility, rankEmoji, az.Rank)
	}

	fmt.Println()
	fmt.Println("   💡 INSIGHTS:")
	for _, insight := range rec.Insights {
		fmt.Printf("      %s\n", insight)
	}

	if rec.PriceDifferential > 0 {
		fmt.Printf("\n   📊 Price spread: %.1f%% between best and worst AZ\n", rec.PriceDifferential)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// runAnalysis executes the analysis and displays results
func (c *CLI) runAnalysis(
	ctx context.Context,
	cloudProvider domain.CloudProvider,
	requirements domain.UsageRequirements,
	outputFormat string,
	enhancedMode bool,
	debugMode bool,
	families []string,
) error {
	if debugMode {
		fmt.Println("🔬 DEBUG MODE: Verifying data sources...")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("📡 DATA SOURCE: https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json")
		fmt.Printf("🌍 REGION: %s\n", requirements.Region)
		fmt.Printf("💻 OS: %s\n", requirements.OS)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
	}

	if enhancedMode {
		fmt.Printf("🧠 Running ENHANCED AI analysis for %s in %s...\n\n", cloudProvider, requirements.Region)
	} else {
		fmt.Printf("🔍 Analyzing spot instances for %s in %s...\n\n", cloudProvider, requirements.Region)
	}

	// Get providers from factory
	factory := provider.GetFactory()

	spotProvider, err := factory.CreateSpotDataProvider(cloudProvider)
	if err != nil {
		return fmt.Errorf("failed to create spot data provider: %w", err)
	}

	specsProvider, err := factory.CreateInstanceSpecsProvider(cloudProvider)
	if err != nil {
		return fmt.Errorf("failed to create specs provider: %w", err)
	}

	// Debug: show raw data for verification
	if debugMode {
		c.showDebugData(ctx, spotProvider, requirements)
	}

	if enhancedMode {
		fmt.Printf("🧠 Running ENHANCED AI analysis for %s in %s...\n\n", cloudProvider, requirements.Region)

		var enhancedAnalyzer *analyzer.EnhancedAnalyzer

		switch cloudProvider {
		case domain.Azure:
			// Use Azure price history provider
			priceProvider := azureprovider.NewPriceHistoryProvider(requirements.Region)
			fmt.Println("☁️ Using Azure Retail Prices API for enhanced analysis")
			adapter := azureprovider.NewPriceHistoryAdapter(priceProvider)
			enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(
				spotProvider, specsProvider, adapter, requirements.Region,
			)

		default: // AWS
			// Try to create price history provider with AWS credentials
			priceProvider, priceErr := awsprovider.NewPriceHistoryProvider(requirements.Region)
			if priceErr == nil && priceProvider.IsAvailable() {
				fmt.Println("🔑 AWS credentials detected - using REAL historical price data")
				adapter := awsprovider.NewPriceHistoryAdapter(priceProvider)
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzerWithPriceHistory(
					spotProvider, specsProvider, adapter, requirements.Region,
				)
			} else {
				fmt.Println("💡 No AWS credentials - using intelligent heuristics for enhanced analysis")
				fmt.Println("   Set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY for real price history")
				enhancedAnalyzer = analyzer.NewEnhancedAnalyzer(spotProvider, specsProvider)
			}
		}

		result, enhancedErr := enhancedAnalyzer.AnalyzeEnhanced(ctx, requirements)
		if enhancedErr != nil {
			return fmt.Errorf("enhanced analysis failed: %w", enhancedErr)
		}
		return c.displayEnhancedTable(result)
	}

	// Create standard analyzer
	smartAnalyzer := analyzer.NewSmartAnalyzer(spotProvider, specsProvider)

	// Run analysis
	result, err := smartAnalyzer.Analyze(ctx, requirements)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Display results
	switch outputFormat {
	case "json":
		return c.displayJSON(result)
	case "simple":
		return c.displaySimple(result)
	default:
		return c.displayTable(result)
	}
}

// displayTable shows results in a formatted table
func (c *CLI) displayTable(result *domain.AnalysisResult) error {
	if len(result.TopInstances) == 0 {
		fmt.Println("❌ No instances found matching your requirements.")
		fmt.Println("\nSuggestions:")
		fmt.Println("  - Try relaxing vCPU or memory requirements")
		fmt.Println("  - Increase maximum interruption level")
		fmt.Println("  - Allow burstable instances with --allow-burstable")
		return nil
	}

	// Print summary
	fmt.Printf("✅ Found %d matching instances (analyzed %d, filtered %d)\n\n",
		len(result.TopInstances), result.TotalAnalyzed, result.FilteredOut)

	// Create table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tINSTANCE TYPE\tvCPU\tMEMORY\tSAVINGS\tINTERRUPTION\tSCORE\tARCH\tGEN")
	fmt.Fprintln(w, "----\t-------------\t----\t------\t-------\t------------\t-----\t----\t---")

	for _, inst := range result.TopInstances {
		genLabel := c.generationLabel(inst.Specs.Generation)
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d%%\t%s\t%.2f\t%s\t%s\n",
			inst.Rank,
			inst.Specs.InstanceType,
			inst.Specs.VCPU,
			formatMemory(inst.Specs.MemoryGB),
			inst.SpotData.SavingsPercent,
			inst.SpotData.InterruptionFrequency.String(),
			inst.Score,
			inst.Specs.Architecture,
			genLabel,
		)
	}
	w.Flush()

	// Print top recommendation details
	if len(result.TopInstances) > 0 {
		top := result.TopInstances[0]
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("🏆 TOP RECOMMENDATION: %s\n", top.Specs.InstanceType)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   📊 Score: %.2f\n", top.Score)
		fmt.Printf("   💰 Savings: %d%% vs On-Demand\n", top.SpotData.SavingsPercent)
		fmt.Printf("   ⚡ Stability: %s interruption rate\n", top.SpotData.InterruptionFrequency.String())
		fmt.Printf("   🔧 Specs: %d vCPU, %s RAM\n", top.Specs.VCPU, formatMemory(top.Specs.MemoryGB))
		fmt.Printf("   📝 %s\n", top.Recommendation)

		if len(top.Warnings) > 0 {
			fmt.Println()
			fmt.Println("   ⚠️  Warnings:")
			for _, warning := range top.Warnings {
				fmt.Printf("      • %s\n", warning)
			}
		}

		// Score breakdown
		fmt.Println()
		fmt.Println("   📈 Score Breakdown:")
		fmt.Printf("      • Savings Score:     %.2f (weight: 30%%)\n", top.ScoreBreakdown.SavingsScore)
		fmt.Printf("      • Stability Score:   %.2f (weight: 25%%)\n", top.ScoreBreakdown.StabilityScore)
		fmt.Printf("      • Fitness Score:     %.2f (weight: 25%%)\n", top.ScoreBreakdown.FitnessScore)
		fmt.Printf("      • Value Score:       %.2f (weight: 20%%)\n", top.ScoreBreakdown.ValueScore)
		if top.ScoreBreakdown.GenerationPenalty > 0 {
			fmt.Printf("      • Generation Penalty: -%.2f\n", top.ScoreBreakdown.GenerationPenalty)
		}
	}

	return nil
}

// displaySimple shows results in a simple list format
func (c *CLI) displaySimple(result *domain.AnalysisResult) error {
	if len(result.TopInstances) == 0 {
		fmt.Println("No instances found matching requirements")
		return nil
	}

	for _, inst := range result.TopInstances {
		fmt.Printf("%d. %s - %d vCPU, %s, %d%% savings, %s interruption, score: %.2f\n",
			inst.Rank,
			inst.Specs.InstanceType,
			inst.Specs.VCPU,
			formatMemory(inst.Specs.MemoryGB),
			inst.SpotData.SavingsPercent,
			inst.SpotData.InterruptionFrequency.String(),
			inst.Score,
		)
	}

	return nil
}

// displayJSON shows results in JSON format
func (c *CLI) displayJSON(result *domain.AnalysisResult) error {
	// Use standard library for simple JSON output
	fmt.Println("{")
	fmt.Printf("  \"region\": \"%s\",\n", result.Region)
	fmt.Printf("  \"cloud_provider\": \"%s\",\n", result.CloudProvider)
	fmt.Printf("  \"total_analyzed\": %d,\n", result.TotalAnalyzed)
	fmt.Printf("  \"filtered_out\": %d,\n", result.FilteredOut)
	fmt.Printf("  \"analyzed_at\": \"%s\",\n", result.AnalyzedAt.Format(time.RFC3339))
	fmt.Println("  \"top_instances\": [")

	for i, inst := range result.TopInstances {
		comma := ","
		if i == len(result.TopInstances)-1 {
			comma = ""
		}
		fmt.Printf("    {\n")
		fmt.Printf("      \"rank\": %d,\n", inst.Rank)
		fmt.Printf("      \"instance_type\": \"%s\",\n", inst.Specs.InstanceType)
		fmt.Printf("      \"vcpu\": %d,\n", inst.Specs.VCPU)
		fmt.Printf("      \"memory_gb\": %.0f,\n", inst.Specs.MemoryGB)
		fmt.Printf("      \"savings_percent\": %d,\n", inst.SpotData.SavingsPercent)
		fmt.Printf("      \"interruption_frequency\": \"%s\",\n", inst.SpotData.InterruptionFrequency.String())
		fmt.Printf("      \"score\": %.4f,\n", inst.Score)
		fmt.Printf("      \"architecture\": \"%s\",\n", inst.Specs.Architecture)
		fmt.Printf("      \"category\": \"%s\",\n", inst.Specs.Category)
		fmt.Printf("      \"has_gpu\": %t,\n", inst.Specs.HasGPU)
		fmt.Printf("      \"recommendation\": \"%s\"\n", strings.ReplaceAll(inst.Recommendation, "\"", "\\\""))
		fmt.Printf("    }%s\n", comma)
	}

	fmt.Println("  ]")
	fmt.Println("}")

	return nil
}

// listRegions displays available regions
func (c *CLI) listRegions(ctx context.Context, cloudProvider domain.CloudProvider) error {
	factory := provider.GetFactory()

	spotProvider, err := factory.CreateSpotDataProvider(cloudProvider)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	regions, err := spotProvider.GetSupportedRegions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get regions: %w", err)
	}

	fmt.Printf("Available regions for %s:\n\n", cloudProvider)
	for _, region := range regions {
		fmt.Printf("  • %s\n", region)
	}
	fmt.Printf("\nTotal: %d regions\n", len(regions))

	return nil
}

// refreshData refreshes cached data
func (c *CLI) refreshData(ctx context.Context, cloudProvider domain.CloudProvider) error {
	factory := provider.GetFactory()

	spotProvider, err := factory.CreateSpotDataProvider(cloudProvider)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	fmt.Printf("Refreshing %s spot data cache...\n", cloudProvider)
	if err := spotProvider.RefreshData(ctx); err != nil {
		return fmt.Errorf("failed to refresh data: %w", err)
	}

	fmt.Println("✅ Cache refreshed successfully")
	return nil
}

// Helper functions
func parseCloudProvider(s string) domain.CloudProvider {
	switch strings.ToLower(s) {
	case "azure":
		return domain.Azure
	case "gcp":
		return domain.GCP
	default:
		return domain.AWS
	}
}

func parseOS(s string) domain.OperatingSystem {
	switch strings.ToLower(s) {
	case "windows":
		return domain.Windows
	default:
		return domain.Linux
	}
}

func parseCategory(s string) domain.InstanceCategory {
	switch strings.ToLower(s) {
	case "compute", "compute_optimized":
		return domain.ComputeOptimized
	case "memory", "memory_optimized":
		return domain.MemoryOptimized
	case "storage", "storage_optimized":
		return domain.StorageOptimized
	case "accelerated", "gpu":
		return domain.AcceleratedComputing
	case "general", "general_purpose":
		return domain.GeneralPurpose
	default:
		return ""
	}
}

func (c *CLI) generationLabel(gen domain.InstanceGeneration) string {
	switch gen {
	case domain.Current:
		return "Current"
	case domain.Previous:
		return "Previous"
	case domain.Legacy:
		return "Legacy"
	case domain.Deprecated:
		return "Deprecated"
	default:
		return "Unknown"
	}
}

// formatMemory formats memory in GB, showing decimals only for sub-GB values
func formatMemory(memGB float64) string {
	if memGB < 1 {
		return fmt.Sprintf("%.1f GB", memGB)
	}
	return fmt.Sprintf("%.0f GB", memGB)
}

// displayEnhancedTable shows enhanced analysis results with AI insights
func (c *CLI) displayEnhancedTable(result *analyzer.EnhancedAnalysisResult) error {
	if len(result.EnhancedInstances) == 0 {
		fmt.Println("❌ No instances found matching your requirements.")
		return nil
	}

	// Print summary with enhanced badge
	fmt.Println("🧠 ENHANCED AI ANALYSIS")
	fmt.Printf("✅ Found %d matching instances (analyzed %d, filtered %d)\n",
		len(result.EnhancedInstances), result.TotalAnalyzed, result.FilteredOut)
	fmt.Printf("📊 Scoring Strategies: %v\n\n", result.ScoringStrategies)

	// Create table with enhanced columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tINSTANCE\tvCPU\tMEM\tSAVINGS\tINTERRUPT\tBASE\tENHANCED\tFINAL")
	fmt.Fprintln(w, "----\t--------\t----\t---\t-------\t---------\t----\t--------\t-----")

	for _, inst := range result.EnhancedInstances {
		// Get enhanced factors
		var enhancedScore float64
		for _, factors := range inst.EnhancedFactors {
			enhancedScore = factors.CombinedEnhancedScore
			break
		}

		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d%%\t%s\t%.2f\t%.2f\t%.2f\n",
			inst.Rank,
			inst.Specs.InstanceType,
			inst.Specs.VCPU,
			formatMemory(inst.Specs.MemoryGB),
			inst.SpotData.SavingsPercent,
			inst.SpotData.InterruptionFrequency.String(),
			inst.Score,
			enhancedScore,
			inst.FinalScore,
		)
	}
	w.Flush()

	// Print detailed analysis for top instance
	if len(result.EnhancedInstances) > 0 {
		top := result.EnhancedInstances[0]
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("🏆 TOP RECOMMENDATION (ENHANCED): %s\n", top.Specs.InstanceType)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   📊 Final Score: %.2f (Base: %.2f + Enhanced Factors)\n", top.FinalScore, top.Score)
		fmt.Printf("   💰 Savings: %d%% vs On-Demand\n", top.SpotData.SavingsPercent)
		fmt.Printf("   ⚡ Stability: %s interruption rate\n", top.SpotData.InterruptionFrequency.String())
		fmt.Printf("   🔧 Specs: %d vCPU, %s RAM, %s\n", top.Specs.VCPU, formatMemory(top.Specs.MemoryGB), top.Specs.Architecture)

		// Enhanced factor breakdown
		for strategyName, factors := range top.EnhancedFactors {
			fmt.Println()
			fmt.Printf("   🧠 Enhanced Scoring (%s):\n", strategyName)
			fmt.Printf("      • Volatility Score:     %.2f (pricing stability)\n", factors.VolatilityScore)
			fmt.Printf("      • Trend Score:          %.2f (popularity trend)\n", factors.TrendScore)
			fmt.Printf("      • Capacity Pool Score:  %.2f (multi-AZ availability)\n", factors.CapacityPoolScore)
			fmt.Printf("      • Time Pattern Score:   %.2f (temporal consistency)\n", factors.TimePatternScore)
			fmt.Printf("      • Popularity Score:     %.2f (hidden gem indicator)\n", factors.PopularityScore)
			fmt.Printf("      ─────────────────────────────────\n")
			fmt.Printf("      • Combined Enhanced:    %.2f\n", factors.CombinedEnhancedScore)
		}

		// Print AI insights
		if len(top.AllInsights) > 0 {
			fmt.Println()
			fmt.Println("   💡 AI INSIGHTS:")
			for _, insight := range top.AllInsights {
				fmt.Printf("      %s\n", insight)
			}
		}
	}

	// Summary comparison
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📋 ENHANCED SCORING EXPLAINED:")
	fmt.Println("   • Base Score (60%):     AWS Spot Advisor data (savings, interruption, fitness)")
	fmt.Println("   • Enhanced Score (40%): AI analysis (volatility, trends, capacity, patterns)")
	fmt.Println("   • Final Score:          Weighted combination for optimal recommendations")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// showDebugData displays raw data from the AWS API for verification
func (c *CLI) showDebugData(ctx context.Context, spotProvider domain.SpotDataProvider, requirements domain.UsageRequirements) {
	fmt.Println("📊 RAW SPOT DATA SAMPLE (first 5 instances from AWS API):")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	spotData, err := spotProvider.FetchSpotData(ctx, requirements.Region, requirements.OS)
	if err != nil {
		fmt.Printf("❌ ERROR fetching spot data: %v\n", err)
		return
	}

	fmt.Printf("✅ Successfully fetched %d instance types from AWS Spot Advisor\n\n", len(spotData))

	// Show first 5 raw entries
	count := 0
	for _, data := range spotData {
		if count >= 5 {
			break
		}
		fmt.Printf("   %s: savings=%d%% (s=%d), interruption=%s (r=%d)\n",
			data.InstanceType,
			data.SavingsPercent,
			data.SavingsPercent,
			data.InterruptionFrequency.String(),
			int(data.InterruptionFrequency),
		)
		count++
	}
	fmt.Println()
	fmt.Println("💡 Verify with: Invoke-RestMethod 'https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json' | Select-Object -ExpandProperty spot_advisor | Select-Object -ExpandProperty '" + requirements.Region + "' | Select-Object -ExpandProperty Linux")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// webCmd creates the web UI command
func (c *CLI) webCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web UI",
		Long: `Start a web-based user interface for the spot analyzer.

The web UI provides:
  - Natural language input for requirements
  - Visual configuration for CPU, RAM, and architecture
  - Preset configurations for common use cases (Kubernetes, Database, ASG)
  - Interactive results with sorting and filtering

Examples:
  # Start web UI on default port 8000
  spot-analyzer web

  # Start on custom port
  spot-analyzer web --port 3000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runWeb(port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8000, "Port to run the web server on")

	return cmd
}

func (c *CLI) runWeb(port int) error {
	// Import dynamically to avoid circular dependency at compile time
	// The web package will be imported here
	fmt.Println("🌐 Starting Spot Analyzer Web UI...")
	fmt.Printf("   Open http://localhost:%d in your browser\n", port)
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	// Start the server - import web package
	web := newWebServer(port)
	return web.Start()
}

// webServer is a minimal interface to avoid import cycles
type webServer interface {
	Start() error
}

func newWebServer(port int) webServer {
	// This will be replaced by actual import in main.go
	return &defaultWebServer{port: port}
}

type defaultWebServer struct {
	port int
}

func (s *defaultWebServer) Start() error {
	fmt.Printf("Web server would start on port %d\n", s.port)
	fmt.Println("Note: Import the web package in main.go for full functionality")
	return nil
}
