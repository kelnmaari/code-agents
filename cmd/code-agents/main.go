package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/orchestrator"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runlog"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/version"
)

//go:embed template.yaml
var templateContent []byte

// Args defines CLI arguments.
type Args struct {
	Config  string   `arg:"-c,--config" default:"code-agents.yaml" help:"path to configuration file"`
	Init    bool     `arg:"--init" help:"generate a new configuration file"`
	Profile string   `arg:"--profile" help:"named prompt profile to use (loads profiles/<name>.yaml)"`
	RunsDir string   `arg:"--runs-dir" default:"runs" help:"directory to save run logs"`
	Prompt  []string `arg:"positional" help:"prompt text (overrides config prompt)"`
}

func (Args) Version() string {
	return fmt.Sprintf("code-agents %s (commit: %s, built: %s)", version.Version, version.Commit, version.Date)
}

func (Args) Description() string {
	return "Code-Agents - Multi-Agent Hierarchical Coding System"
}

func main() {
	// Handle compare subcommand before go-arg to avoid positional conflicts.
	if len(os.Args) > 1 && os.Args[1] == "compare" {
		runCompare(os.Args[2:])
		return
	}

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

	// Resolve config path: --profile overrides --config.
	cfgPath := args.Config
	if args.Profile != "" {
		cfgPath = filepath.Join("profiles", args.Profile+".yaml")
		logging.Console.Printf("Using profile: %s (%s)", args.Profile, cfgPath)
	}

	// Load configuration
	cfg, err := config.Load(cfgPath)
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

	// Start run log.
	rl := runlog.New(args.Profile, prompt)

	// Create and run orchestrator
	orch := orchestrator.New(cfg)
	runErr := orch.Run(context.Background(), prompt)

	// Capture results and save run log regardless of success/failure.
	res := orch.Results()
	success := runErr == nil
	rl.Finish(success, runErr,
		res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.TotalTokens,
		res.FailedTasks, res.CompletedTasks, res.PlannerIterations,
	)
	if logPath, saveErr := rl.Save(args.RunsDir); saveErr != nil {
		logging.Console.Printf("Warning: could not save run log: %v", saveErr)
	} else {
		logging.Console.Printf("Run log saved: %s", logPath)
	}

	if runErr != nil {
		logging.Console.Fatalf("Error: %v", runErr)
	}

	logging.Console.Println("Code-agents completed successfully.")
}

// runCompare implements the `code-agents compare <run1.json> <run2.json>` subcommand.
func runCompare(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: code-agents compare <run1.json> <run2.json>\n")
		os.Exit(1)
	}
	cr, err := runlog.Compare(args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cr.Print(os.Stdout)
}

func generateTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %q already exists", path)
	}
	return os.WriteFile(path, templateContent, 0o644)
}
