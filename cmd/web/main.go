// Package main is the entry point for the spot analyzer web server.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spot-analyzer/internal/web"
)

func main() {
	port := flag.Int("port", 8000, "Port to run the web server on")
	flag.Parse()

	fmt.Println("🚀 Multi-Cloud Spot Analyzer - Web UI")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	server := web.NewServer(*port)
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
