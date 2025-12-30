// Package main is the entry point for the spot analyzer CLI.
package main

import (
	"fmt"
	"os"

	"github.com/spot-analyzer/internal/cli"
)

func main() {
	app := cli.New()
	if err := app.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
