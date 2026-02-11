package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	Version  int            `yaml:"version"`
	Provider ProviderConfig `yaml:"provider"`
	Agents   AgentsConfig   `yaml:"agents"`
	Loop     LoopConfig     `yaml:"loop"`
	Tools    ToolsConfig    `yaml:"tools"`
	Input    InputConfig    `yaml:"input"`
}

// ProviderConfig defines the OpenAI-compatible LLM endpoint.
type ProviderConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

// AgentsConfig holds per-role agent configurations.
type AgentsConfig struct {
	Planner    AgentConfig `yaml:"planner"`
	Subplanner AgentConfig `yaml:"subplanner"`
	Worker     AgentConfig `yaml:"worker"`
}

// AgentConfig defines model and system prompt for an agent role.
type AgentConfig struct {
	Model        ModelConfig `yaml:"model"`
	SystemPrompt string      `yaml:"system_prompt"`
}

// ModelConfig specifies LLM model parameters.
type ModelConfig struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

// LoopConfig controls orchestration loop parameters.
type LoopConfig struct {
	MaxDepth          int           `yaml:"max_depth"`
	MaxWorkers        int           `yaml:"max_workers"`
	MaxSteps          int           `yaml:"max_steps"`
	Timeout           string        `yaml:"timeout"`
	StepDelay         string        `yaml:"step_delay"`
	AutoApprove       bool          `yaml:"auto_approve"`
	TimeoutDuration   time.Duration `yaml:"-"`
	StepDelayDuration time.Duration `yaml:"-"`
}

// ToolsConfig controls which tools are available and their settings.
type ToolsConfig struct {
	WorkDir      string   `yaml:"work_dir"`
	AllowedShell []string `yaml:"allowed_shell"`
	GitEnabled   bool     `yaml:"git_enabled"`
}

// InputConfig holds the user's input prompt.
type InputConfig struct {
	Prompt string `yaml:"prompt"`
}

// Load reads and validates a YAML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)

	// Resolve env: and file: prefixes
	if err := resolveValue(&cfg.Provider.APIKey); err != nil {
		return nil, fmt.Errorf("resolve api_key: %w", err)
	}
	if err := resolveValue(&cfg.Agents.Planner.SystemPrompt); err != nil {
		return nil, fmt.Errorf("resolve planner system_prompt: %w", err)
	}
	if err := resolveValue(&cfg.Agents.Subplanner.SystemPrompt); err != nil {
		return nil, fmt.Errorf("resolve subplanner system_prompt: %w", err)
	}
	if err := resolveValue(&cfg.Agents.Worker.SystemPrompt); err != nil {
		return nil, fmt.Errorf("resolve worker system_prompt: %w", err)
	}

	// Parse durations
	cfg.Loop.TimeoutDuration, err = time.ParseDuration(cfg.Loop.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse timeout duration %q: %w", cfg.Loop.Timeout, err)
	}
	cfg.Loop.StepDelayDuration, err = time.ParseDuration(cfg.Loop.StepDelay)
	if err != nil {
		return nil, fmt.Errorf("parse step_delay duration %q: %w", cfg.Loop.StepDelay, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// ResolvePrompt resolves the input prompt (inline or file:path).
func (c *Config) ResolvePrompt() (string, error) {
	prompt := c.Input.Prompt
	if strings.HasPrefix(prompt, "file:") {
		path := strings.TrimPrefix(prompt, "file:")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read prompt file %q: %w", path, err)
		}
		return string(data), nil
	}
	return prompt, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Loop.MaxDepth == 0 {
		cfg.Loop.MaxDepth = 3
	}
	if cfg.Loop.MaxWorkers == 0 {
		cfg.Loop.MaxWorkers = 4
	}
	if cfg.Loop.MaxSteps == 0 {
		cfg.Loop.MaxSteps = 30
	}
	if cfg.Loop.Timeout == "" {
		cfg.Loop.Timeout = "30m"
	}
	if cfg.Loop.StepDelay == "" {
		cfg.Loop.StepDelay = "2s"
	}
	if cfg.Tools.WorkDir == "" {
		cfg.Tools.WorkDir = "."
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	// GitEnabled defaults to true (Go zero value is false, so we check version)
	// Only set default if the entire tools section was not provided
	// Note: yaml will parse "git_enabled: true" correctly; if omitted, it stays false.
	// For the documented default of true, we handle this via the template config.
}

func resolveValue(s *string) error {
	if s == nil || *s == "" {
		return nil
	}
	if strings.HasPrefix(*s, "env:") {
		envName := strings.TrimPrefix(*s, "env:")
		val, ok := os.LookupEnv(envName)
		if !ok {
			return fmt.Errorf("environment variable %q not set", envName)
		}
		*s = val
	}
	if strings.HasPrefix(*s, "file:") {
		path := strings.TrimPrefix(*s, "file:")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file %q: %w", path, err)
		}
		*s = string(data)
	}
	return nil
}

func validate(cfg *Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("unsupported config version: %d (expected 1)", cfg.Version)
	}
	if cfg.Provider.BaseURL == "" {
		return fmt.Errorf("provider.base_url is required")
	}
	if cfg.Agents.Planner.Model.Model == "" {
		return fmt.Errorf("agents.planner.model.model is required")
	}
	if cfg.Agents.Subplanner.Model.Model == "" {
		return fmt.Errorf("agents.subplanner.model.model is required")
	}
	if cfg.Agents.Worker.Model.Model == "" {
		return fmt.Errorf("agents.worker.model.model is required")
	}
	if cfg.Agents.Planner.SystemPrompt == "" {
		return fmt.Errorf("agents.planner.system_prompt is required")
	}
	if cfg.Agents.Subplanner.SystemPrompt == "" {
		return fmt.Errorf("agents.subplanner.system_prompt is required")
	}
	if cfg.Agents.Worker.SystemPrompt == "" {
		return fmt.Errorf("agents.worker.system_prompt is required")
	}
	if cfg.Loop.MaxWorkers < 1 {
		return fmt.Errorf("loop.max_workers must be >= 1")
	}
	if cfg.Loop.MaxSteps < 1 {
		return fmt.Errorf("loop.max_steps must be >= 1")
	}
	return nil
}
