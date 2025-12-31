// verify_spot_data.go - Directly queries AWS Spot Advisor API to verify instance data
// Usage: go run verify_spot_data.go -families m,c,r -region us-east-1
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const spotAdvisorURL = "https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json"

// SpotAdvisorData represents the AWS Spot Advisor JSON structure
type SpotAdvisorData struct {
	SpotAdvisor map[string]map[string]map[string]InstanceData `json:"spot_advisor"`
	Ranges      []RangeData                                   `json:"ranges"`
}

type InstanceData struct {
	R int `json:"r"` // Interruption rate (0=<5%, 1=5-10%, 2=10-15%, 3=15-20%, 4=>20%)
	S int `json:"s"` // Savings percent
}

type RangeData struct {
	Index int    `json:"index"`
	Dots  int    `json:"dots"`
	Max   int    `json:"max"`
	Label string `json:"label"`
}

type InstanceResult struct {
	InstanceType string
	Savings      int
	Interruption string
	Family       string
}

var interruptionLabels = []string{"<5%", "5-10%", "10-15%", "15-20%", ">20%"}

func main() {
	familiesFlag := flag.String("families", "", "Comma-separated list of instance families (e.g., m,c,r,t)")
	regionFlag := flag.String("region", "us-east-1", "AWS region")
	osFlag := flag.String("os", "Linux", "Operating system (Linux or Windows)")
	topFlag := flag.Int("top", 20, "Number of top instances to show per family")
	minSavingsFlag := flag.Int("min-savings", 0, "Minimum savings percentage")
	maxInterruptionFlag := flag.Int("max-interruption", 4, "Maximum interruption level (0-4)")
	jsonOutputFlag := flag.Bool("json", false, "Output as JSON")
	flag.Parse()

	if *familiesFlag == "" {
		fmt.Println("AWS Spot Advisor Data Verification Tool")
		fmt.Println("========================================")
		fmt.Println("\nUsage: go run verify_spot_data.go -families <families> [options]")
		fmt.Println("\nOptions:")
		fmt.Println("  -families    Comma-separated list of instance families (e.g., m,c,r,t)")
		fmt.Println("  -region      AWS region (default: us-east-1)")
		fmt.Println("  -os          Operating system: Linux or Windows (default: Linux)")
		fmt.Println("  -top         Number of instances to show per family (default: 20)")
		fmt.Println("  -min-savings Minimum savings percentage (default: 0)")
		fmt.Println("  -max-interruption Maximum interruption level 0-4 (default: 4)")
		fmt.Println("  -json        Output as JSON")
		fmt.Println("\nExamples:")
		fmt.Println("  go run verify_spot_data.go -families m")
		fmt.Println("  go run verify_spot_data.go -families m,c,r -region eu-west-1")
		fmt.Println("  go run verify_spot_data.go -families t -min-savings 50 -max-interruption 1")
		os.Exit(0)
	}

	families := strings.Split(*familiesFlag, ",")
	for i := range families {
		families[i] = strings.TrimSpace(strings.ToLower(families[i]))
	}

	fmt.Fprintf(os.Stderr, "🔍 Fetching data from AWS Spot Advisor API...\n")
	fmt.Fprintf(os.Stderr, "   URL: %s\n\n", spotAdvisorURL)

	data, err := fetchSpotAdvisorData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error fetching data: %v\n", err)
		os.Exit(1)
	}

	// Get region data
	regionData, ok := data.SpotAdvisor[*regionFlag]
	if !ok {
		fmt.Fprintf(os.Stderr, "❌ Region '%s' not found in Spot Advisor data\n", *regionFlag)
		fmt.Fprintf(os.Stderr, "Available regions: ")
		for r := range data.SpotAdvisor {
			fmt.Fprintf(os.Stderr, "%s ", r)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	// Get OS data
	osData, ok := regionData[*osFlag]
	if !ok {
		fmt.Fprintf(os.Stderr, "❌ OS '%s' not found for region '%s'\n", *osFlag, *regionFlag)
		os.Exit(1)
	}

	// Filter and collect results
	results := make(map[string][]InstanceResult)
	totalInstances := 0

	for instanceType, instData := range osData {
		family := extractFamily(instanceType)

		// Check if family matches
		matchesFamily := false
		for _, f := range families {
			if strings.EqualFold(family, f) {
				matchesFamily = true
				break
			}
		}
		if !matchesFamily {
			continue
		}

		// Apply filters
		if instData.S < *minSavingsFlag {
			continue
		}
		if instData.R > *maxInterruptionFlag {
			continue
		}

		intLabel := "unknown"
		if instData.R >= 0 && instData.R < len(interruptionLabels) {
			intLabel = interruptionLabels[instData.R]
		}

		result := InstanceResult{
			InstanceType: instanceType,
			Savings:      instData.S,
			Interruption: intLabel,
			Family:       family,
		}

		results[family] = append(results[family], result)
		totalInstances++
	}

	// Sort each family's results by savings (descending), then by interruption (ascending)
	for family := range results {
		sort.Slice(results[family], func(i, j int) bool {
			// First by savings (higher is better)
			if results[family][i].Savings != results[family][j].Savings {
				return results[family][i].Savings > results[family][j].Savings
			}
			// Then by interruption (lower is better)
			return getInterruptionIndex(results[family][i].Interruption) < getInterruptionIndex(results[family][j].Interruption)
		})

		// Limit to top N
		if len(results[family]) > *topFlag {
			results[family] = results[family][:*topFlag]
		}
	}

	// Output
	if *jsonOutputFlag {
		outputJSON(results, *regionFlag, *osFlag, families, totalInstances)
	} else {
		outputTable(results, *regionFlag, *osFlag, families, totalInstances)
	}
}

func fetchSpotAdvisorData() (*SpotAdvisorData, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(spotAdvisorURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var data SpotAdvisorData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &data, nil
}

func extractFamily(instanceType string) string {
	for i, c := range instanceType {
		if c >= '0' && c <= '9' {
			return strings.ToLower(instanceType[:i])
		}
	}
	return strings.ToLower(instanceType)
}

func getInterruptionIndex(label string) int {
	for i, l := range interruptionLabels {
		if l == label {
			return i
		}
	}
	return 99
}

func outputTable(results map[string][]InstanceResult, region, osType string, families []string, total int) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║            AWS SPOT ADVISOR - REAL DATA VERIFICATION                        ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  📅 Timestamp: %-62s║\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("║  🌍 Region: %-65s║\n", region)
	fmt.Printf("║  💻 OS: %-69s║\n", osType)
	fmt.Printf("║  📦 Families: %-63s║\n", strings.Join(families, ", "))
	fmt.Printf("║  📊 Total matching instances: %-47d║\n", total)
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Sort families for consistent output
	sortedFamilies := make([]string, 0, len(results))
	for f := range results {
		sortedFamilies = append(sortedFamilies, f)
	}
	sort.Strings(sortedFamilies)

	for _, family := range sortedFamilies {
		instances := results[family]
		if len(instances) == 0 {
			continue
		}

		fmt.Printf("┌─── Family: %s (%d instances) ───────────────────────────────────────────┐\n",
			strings.ToUpper(family), len(instances))
		fmt.Println("│ Instance Type          │ Savings │ Interruption Rate                    │")
		fmt.Println("├────────────────────────┼─────────┼──────────────────────────────────────┤")

		for _, inst := range instances {
			fmt.Printf("│ %-22s │   %3d%% │ %-36s │\n",
				inst.InstanceType, inst.Savings, inst.Interruption)
		}
		fmt.Println("└────────────────────────┴─────────┴──────────────────────────────────────┘")
		fmt.Println()
	}

	fmt.Println("✅ Data verified directly from: " + spotAdvisorURL)
}

func outputJSON(results map[string][]InstanceResult, region, osType string, families []string, total int) {
	output := map[string]interface{}{
		"source":          spotAdvisorURL,
		"timestamp":       time.Now().Format(time.RFC3339),
		"region":          region,
		"os":              osType,
		"families":        families,
		"total_instances": total,
		"instances":       results,
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonBytes))
}
