package main

import (
	"flag"
	"fmt"
	"os"
)

// main is the entry point of the application. It parses CLI arguments and starts the orchestrator.
func main() {
	inputFolder := flag.String("input", "./input", "Path to the input folder")
	outputFolder := flag.String("output", "./output", "Path to the output folder")
	dryRun := flag.Bool("dry-run", false, "Enable dry-run mode (no filesystem changes)")
	overwrite := flag.Bool("overwrite", false, "Overwrite existing destination files")

	// Custom help flag
	help := flag.Bool("help", false, "Display help information")
	flag.Parse()

	if *help {
		fmt.Println("Usage: transcode-daemon [OPTIONS]")
		fmt.Println("\nOptions:")
		fmt.Println("  -input      Path to the input folder (default: ./input)")
		fmt.Println("  -output     Path to the output folder (default: ./output)")
		fmt.Println("  -dry-run    Enable dry-run mode (no filesystem changes)")
		fmt.Println("  -overwrite  Overwrite existing destination files")
		fmt.Println("  -help       Display this help information")
		os.Exit(0)
	}

	if *dryRun {
		fmt.Println("[Dry-Run] Mode: No filesystem changes will be made.")
	}

	StartOrchestrator(*inputFolder, *outputFolder, *dryRun, *overwrite)
}
