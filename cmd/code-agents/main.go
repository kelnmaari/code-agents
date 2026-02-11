package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/alexflint/go-arg"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/orchestrator"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/version"
)

//go:embed template.yaml
var templateContent []byte

// Args defines CLI arguments.
type Args struct {
	Config string   `arg:"-c,--config" default:"code-agents.yaml" help:"path to configuration file"`
	Init   bool     `arg:"--init" help:"generate a new configuration file"`
	Prompt []string `arg:"positional" help:"prompt text (overrides config prompt)"`
}

func (Args) Version() string {
	return fmt.Sprintf("code-agents %s (commit: %s, built: %s)", version.Version, version.Commit, version.Date)
}

func (Args) Description() string {
	return "Code-Agents - Multi-Agent Hierarchical Coding System"
}

func main() {
	var args Args
	arg.MustParse(&args)

	// Set up log file (truncated on each run)
	logFile, err := os.OpenFile("code-agents.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Fatalf("Error creating log file: %v", err)
	}
	defer logFile.Close()
	logging.Init(logFile)

	if args.Init {
		if err := generateTemplate(args.Config); err != nil {
			log.Fatalf("Error generating config: %v", err)
		}
		fmt.Printf("Configuration file created: %s\n", args.Config)
		return
	}

	// Load configuration
	cfg, err := config.Load(args.Config)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Resolve prompt (base from config if not provided via CLI)
	var prompt string
	if len(args.Prompt) > 0 {
		prompt = strings.Join(args.Prompt, " ")
	} else {
		var err error
		prompt, err = cfg.ResolvePrompt()
		if err != nil {
			log.Fatalf("Error resolving prompt: %v", err)
		}
	}

	if prompt == "" {
		log.Fatalf("No prompt provided (neither in config nor via command-line)")
	}

	logging.Console.Printf("Starting code-agents (workers: %d, timeout: %s)",
		cfg.Loop.MaxWorkers, cfg.Loop.Timeout)
	logging.Console.Printf("Planner: %s | Worker: %s",
		cfg.Agents.Planner.Model.Model, cfg.Agents.Worker.Model.Model)
	logging.File.Printf("Starting code-agents (workers: %d, max_steps: %d, timeout: %s)",
		cfg.Loop.MaxWorkers, cfg.Loop.MaxSteps, cfg.Loop.Timeout)

	// Create and run orchestrator
	orch := orchestrator.New(cfg)
	if err := orch.Run(context.Background(), prompt); err != nil {
		logging.Console.Fatalf("Error: %v", err)
	}

	logging.Console.Println("Code-agents completed successfully.")

}

func generateTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %q already exists", path)
	}
	return os.WriteFile(path, templateContent, 0o644)
}
