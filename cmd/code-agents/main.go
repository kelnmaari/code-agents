package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/alexflint/go-arg"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/orchestrator"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/version"
)

//go:embed template.yaml
var templateContent []byte

// Args defines CLI arguments.
type Args struct {
	Config string `arg:"positional" default:"code-agents.yaml" help:"path to configuration file"`
	Init   bool   `arg:"--init" help:"generate a new configuration file"`
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

	// Resolve prompt
	prompt, err := cfg.ResolvePrompt()
	if err != nil {
		log.Fatalf("Error resolving prompt: %v", err)
	}

	log.Printf("Starting code-agents (workers: %d, max_steps: %d, timeout: %s)",
		cfg.Loop.MaxWorkers, cfg.Loop.MaxSteps, cfg.Loop.Timeout)
	log.Printf("Planner model: %s, Worker model: %s",
		cfg.Agents.Planner.Model.Model, cfg.Agents.Worker.Model.Model)

	// Create and run orchestrator
	orch := orchestrator.New(cfg)
	if err := orch.Run(context.Background(), prompt); err != nil {
		log.Fatalf("Error: %v", err)
	}

	log.Println("Code-agents completed successfully.")
}

func generateTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %q already exists", path)
	}
	return os.WriteFile(path, templateContent, 0o644)
}
