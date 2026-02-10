package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

const validConfig = `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "test-key"
agents:
  planner:
    model:
      model: "Qwen3-14B"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "You are a planner."
  subplanner:
    model:
      model: "Qwen3-8B"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "You are a subplanner."
  worker:
    model:
      model: "Qwen2.5-Coder-14B"
      temperature: 0.2
      max_tokens: 8192
    system_prompt: "You are a worker."
loop:
  max_depth: 2
  max_workers: 2
  max_steps: 10
  timeout: "5m"
  step_delay: "1s"
tools:
  work_dir: "/tmp/test"
  git_enabled: true
input:
  prompt: "Build the project"
`

func TestLoad_FullConfig(t *testing.T) {
	path := writeConfig(t, validConfig)

	cfg, err := Load(path)
	require.NoError(t, err)

	require.Equal(t, 1, cfg.Version)
	require.Equal(t, "http://localhost:8080/v1", cfg.Provider.BaseURL)
	require.Equal(t, "test-key", cfg.Provider.APIKey)
	require.Equal(t, "Qwen3-14B", cfg.Agents.Planner.Model.Model)
	require.Equal(t, 0.3, cfg.Agents.Planner.Model.Temperature)
	require.Equal(t, 4096, cfg.Agents.Planner.Model.MaxTokens)
	require.Equal(t, "You are a planner.", cfg.Agents.Planner.SystemPrompt)
	require.Equal(t, "Qwen2.5-Coder-14B", cfg.Agents.Worker.Model.Model)
	require.Equal(t, 2, cfg.Loop.MaxDepth)
	require.Equal(t, 2, cfg.Loop.MaxWorkers)
	require.Equal(t, 10, cfg.Loop.MaxSteps)
	require.Equal(t, 5*time.Minute, cfg.Loop.TimeoutDuration)
	require.Equal(t, 1*time.Second, cfg.Loop.StepDelayDuration)
	require.Equal(t, "/tmp/test", cfg.Tools.WorkDir)
	require.True(t, cfg.Tools.GitEnabled)
	require.Equal(t, "Build the project", cfg.Input.Prompt)
}

func TestLoad_Defaults(t *testing.T) {
	minConfig := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, minConfig)

	cfg, err := Load(path)
	require.NoError(t, err)

	// Check defaults
	require.Equal(t, 3, cfg.Loop.MaxDepth)
	require.Equal(t, 4, cfg.Loop.MaxWorkers)
	require.Equal(t, 30, cfg.Loop.MaxSteps)
	require.Equal(t, 30*time.Minute, cfg.Loop.TimeoutDuration)
	require.Equal(t, 2*time.Second, cfg.Loop.StepDelayDuration)
	require.Equal(t, ".", cfg.Tools.WorkDir)
}

func TestLoad_EnvResolution(t *testing.T) {
	t.Setenv("TEST_API_KEY", "resolved-key")
	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "env:TEST_API_KEY"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "resolved-key", cfg.Provider.APIKey)
}

func TestLoad_EnvNotSet(t *testing.T) {
	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "env:NONEXISTENT_VAR_12345"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)

	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not set")
}

func TestLoad_FileResolution(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	require.NoError(t, os.WriteFile(promptPath, []byte("Do the work"), 0o644))

	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "file:` + filepath.ToSlash(promptPath) + `"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "Do the work", cfg.Agents.Planner.SystemPrompt)
}

func TestLoad_MissingBaseURL(t *testing.T) {
	config := `
version: 1
provider:
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "base_url")
}

func TestLoad_MissingModel(t *testing.T) {
	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: ""}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "planner.model.model")
}

func TestLoad_MissingPrompt(t *testing.T) {
	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: ""
`
	path := writeConfig(t, config)
	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input.prompt")
}

func TestLoad_BadVersion(t *testing.T) {
	config := `
version: 99
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported config version")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read config file")
}

func TestResolvePrompt_Inline(t *testing.T) {
	cfg := &Config{Input: InputConfig{Prompt: "Build it"}}
	prompt, err := cfg.ResolvePrompt()
	require.NoError(t, err)
	require.Equal(t, "Build it", prompt)
}

func TestResolvePrompt_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	require.NoError(t, os.WriteFile(path, []byte("Build from file"), 0o644))

	cfg := &Config{Input: InputConfig{Prompt: "file:" + path}}
	prompt, err := cfg.ResolvePrompt()
	require.NoError(t, err)
	require.Equal(t, "Build from file", prompt)
}

func TestResolvePrompt_FileMissing(t *testing.T) {
	cfg := &Config{Input: InputConfig{Prompt: "file:/nonexistent/task.md"}}
	_, err := cfg.ResolvePrompt()
	require.Error(t, err)
}

func TestLoad_InvalidDuration(t *testing.T) {
	config := `
version: 1
provider:
  base_url: "http://localhost:8080/v1"
  api_key: "key"
agents:
  planner:
    model: {model: "m1"}
    system_prompt: "p1"
  subplanner:
    model: {model: "m2"}
    system_prompt: "p2"
  worker:
    model: {model: "m3"}
    system_prompt: "p3"
loop:
  timeout: "invalid"
input:
  prompt: "hello"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse timeout duration")
}
