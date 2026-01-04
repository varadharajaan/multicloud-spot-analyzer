// Package main is the entry point for the spot analyzer web server.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spot-analyzer/internal/config"
	"github.com/spot-analyzer/internal/web"
)

func main() {
	port := flag.Int("port", 8000, "Port to run the web server on")
	flag.Parse()

	fmt.Println()
	fmt.Println("   _____ ____   ___ _____     _    _   _    _    _  __   ____________ ____ ")
	fmt.Println("  / ___/|  _ \\ / _ \\_   _|   / \\  | \\ | |  / \\  | | \\ \\ / /__  / ____|  _ \\")
	fmt.Println("  \\___ \\| |_) | | | || |    / _ \\ |  \\| | / _ \\ | |  \\ V /  / /|  _| | |_) |")
	fmt.Println("   ___) |  __/| |_| || |   / ___ \\| |\\  |/ ___ \\| |___| |  / /_| |___|  _ < ")
	fmt.Println("  |____/|_|    \\___/ |_|  /_/   \\_\\_| \\_/_/   \\_\\_____|_| /____|_____|_| \\_\\")
	fmt.Println()
	fmt.Println("  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  Author: Varadharajan | https://github.com/varadharajaan")
	fmt.Println("  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Log Azure credential status at startup
	cfg := config.Get()
	if cfg.Azure.TenantID != "" && cfg.Azure.ClientID != "" {
		fmt.Println("  ✓ Azure SKU API credentials loaded")
	} else {
		fmt.Println("  ⚠ Azure SKU API not configured (using default zones)")
	}
	fmt.Println()

	server := web.NewServer(*port)
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
