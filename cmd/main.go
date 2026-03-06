// Package main provides the command-line interface for openmodel.
//
// Usage:
//
//	openmodel serve     Start the OpenModel server (default)
//	openmodel test      Test configured models
//	openmodel -h        Show help
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server"
	"github.com/macedot/openmodel/internal/state"
)

// TestResult represents the result of testing a provider.
type TestResult struct {
	Model      string            `json:"model"`
	Provider   string            `json:"provider"`
	ListModels *ListModelsResult `json:"listModels,omitempty"`
}

// ListModelsResult represents the result of the ListModels API call.
type ListModelsResult struct {
	Success bool     `json:"success"`
	Error   string   `json:"error,omitempty"`
	Latency string   `json:"latency"`
	Models  []string `json:"models,omitempty"`
}

// MethodResult represents the result of testing a single API method.
type MethodResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Latency string `json:"latency"`
}

// TestSummary represents the summary of all test results.
type TestSummary struct {
	TotalTests int          `json:"total_tests"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Results    []TestResult `json:"results"`
}

func main() {
	command := "serve"
	args := os.Args[1:]

	for i, arg := range args {
		if arg == "-h" || arg == "--help" {
			printUsage()
			os.Exit(0)
		}
		if !strings.HasPrefix(arg, "-") {
			command = arg
			args = args[i+1:]
			break
		}
	}

	if command == "test" {
		modelName := flag.String("model", "", "Model name to test (tests all if omitted)")
		jsonOutput := flag.Bool("check", false, "Output results in JSON format")

		err := flag.CommandLine.Parse(args)
		if err != nil {
			if err == flag.ErrHelp {
				printTestUsage()
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			printTestUsage()
			os.Exit(1)
		}
		runTest(modelName, jsonOutput)
		return
	}

	if command != "serve" {
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}

	flag.CommandLine.Init("serve", flag.ContinueOnError)
	err := flag.CommandLine.Parse(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printServerUsage()
		os.Exit(1)
	}
	runServer()
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [command]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve    Start the OpenModel server (default)\n")
	fmt.Fprintf(os.Stderr, "  test     Test configured models\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  -h, --help    Show help\n")
	fmt.Fprintf(os.Stderr, "\nRun '%s <command> -h' for more information on a command.\n", os.Args[0])
}

func runServer() {
	serveFlag := flag.Bool("h", false, "Show help")
	if *serveFlag {
		printServerUsage()
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Info("Config loaded", "config_path", config.GetConfigPath())

	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	providers := make(map[string]provider.Provider)
	for name, pc := range cfg.Providers {
		providers[name] = provider.NewOpenAIProvider(name, pc.URL, pc.APIKey)
		logger.Info("Provider initialized", "name", name, "url", pc.URL)
	}

	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := server.New(cfg, providers, stateMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Shutting down...")
		srv.Stop(ctx)
		cancel()
	}()

	logger.Info("Starting openmodel", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error", "error", err)
	}
}

func printTestUsage() {
	modelName := flag.String("model", "", "Model name to test (tests all if omitted)")
	_ = modelName
	jsonOutput := flag.Bool("check", false, "Output results in JSON format")
	_ = jsonOutput
	fmt.Fprintf(os.Stderr, "Usage: %s test [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func printServerUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s serve [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func runTest(modelName *string, jsonOutput *bool) {
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", flag.Arg(0))
		printTestUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Initializing providers")

	providers := make(map[string]provider.Provider)
	for name, pc := range cfg.Providers {
		providers[name] = provider.NewOpenAIProvider(name, pc.URL, pc.APIKey)
		logger.Info("Provider initialized", "name", name, "url", pc.URL)
	}

	if *modelName != "" {
		logger.Info("Testing model", "model", *modelName)
	} else {
		logger.Info("Testing all configured providers")
	}

	summary := runTests(providers)

	if *jsonOutput {
		printJSON(summary)
	} else {
		printText(summary)
	}

	logger.Info("Test completed", "total", summary.TotalTests, "passed", summary.Passed, "failed", summary.Failed)

	if summary.Failed > 0 {
		os.Exit(1)
	}
}

func runTests(providers map[string]provider.Provider) TestSummary {
	summary := TestSummary{
		Results: make([]TestResult, 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Test each provider's /v1/models endpoint
	for providerName, prov := range providers {
		summary.TotalTests++

		logger.Info("Testing provider", "provider", providerName)

		result := TestResult{
			Model:    "-",
			Provider: providerName,
		}

		listResult := testListModels(ctx, prov)
		if listResult.Success {
			summary.Passed++
			logger.Info("ListModels test passed", "provider", providerName, "latency", listResult.Latency, "models", listResult.Models)
		} else {
			summary.Failed++
			logger.Error("ListModels test failed", "provider", providerName, "error", listResult.Error)
		}

		result.ListModels = listResult
		summary.Results = append(summary.Results, result)
	}

	return summary
}

func testListModels(ctx context.Context, prov provider.Provider) *ListModelsResult {
	start := time.Now()

	resp, err := prov.ListModels(ctx)
	latency := time.Since(start)

	if err != nil {
		return &ListModelsResult{
			Success: false,
			Error:   err.Error(),
			Latency: latency.String(),
		}
	}

	models := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		models = append(models, m.ID)
	}

	return &ListModelsResult{
		Success: true,
		Latency: latency.String(),
		Models:  models,
	}
}

func printText(summary TestSummary) {
	fmt.Println()
	fmt.Println("==============================================")
	fmt.Println("           Provider Test Results                ")
	fmt.Println("==============================================")
	fmt.Println()

	for _, result := range summary.Results {
		fmt.Printf("Provider: %s\n", result.Provider)
		fmt.Println(strings.Repeat("-", 50))

		if result.ListModels != nil {
			status := "PASS"
			if !result.ListModels.Success {
				status = "FAIL"
			}
			fmt.Printf("  ListModels: [%s] %s\n", status, result.ListModels.Latency)
			if result.ListModels.Success {
				fmt.Printf("             Models: %v\n", result.ListModels.Models)
			} else {
				fmt.Printf("             Error: %s\n", result.ListModels.Error)
			}
		}

		fmt.Println()
	}

	fmt.Println("==============================================")
	fmt.Printf("Total: %d | Passed: %d | Failed: %d\n", summary.TotalTests, summary.Passed, summary.Failed)
	fmt.Println("==============================================")
}

func printJSON(summary TestSummary) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
