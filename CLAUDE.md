# Code-Agents

Multi-agent hierarchical coding system on Go 1.25+. Three agent roles (planner, subplanner, worker) coordinate through a shared task queue with handoff documents flowing upward.

Module: `gitlab.alexue4.dev/kelnmaari/code-agent`

## Build and Test

```bash
go build -o code-agents ./cmd/code-agents
go test ./...
go test -race ./...
go vet ./...
```

## Architecture

- Hierarchical: Root Planner -> Task Queue -> Workers/Subplanners
- Information flows upward only via Handoff documents
- Shared task queue with mutex + channel notification
- Inner agent loop: LLM call -> tool calls -> execute -> repeat (max 20 iterations)
- Outer orchestrator loop: planner creates tasks, workers execute, planner reassesses
- Termination signal: planner outputs "CODEAGENTS_DONE" + queue.AllDone()
- OpenAI-compatible LLM provider (single base_url, model per agent type)

## Key File Paths

| Area | File | Purpose |
|------|------|---------|
| LLM | internal/llm/types.go | Message types, Completer interface |
| LLM | internal/llm/client.go | OpenAI-compatible HTTP client with retry |
| Tools | internal/tool/tool.go | Tool interface and Registry |
| Tools | internal/tool/file.go | read_file, write_file, list_dir |
| Tools | internal/tool/shell.go | shell_exec with whitelist |
| Tools | internal/tool/git.go | git_status, git_diff, git_commit |
| Tools | internal/tool/task.go | create_task, complete_task, submit_handoff |
| Task | internal/task/types.go | Task, Handoff, status/priority types |
| Task | internal/task/queue.go | Thread-safe task queue with blocking Pull |
| Agent | internal/agent/types.go | Role enum, RunResult |
| Agent | internal/agent/agent.go | Agent struct with inner tool-use loop (Step) |
| Config | internal/config/config.go | YAML loading, validation, env:/file: resolution |
| Orch | internal/orchestrator/orchestrator.go | Main Run() - creates client, queue, goroutines |
| Orch | internal/orchestrator/planner.go | Root planner loop |
| Orch | internal/orchestrator/worker.go | Worker execution loop |
| Orch | internal/orchestrator/subplanner.go | Subplanner recursive loop |
| Runner | internal/runner/runner.go | AgentRunner interface |
| Runner | internal/runner/runner_unix.go | Unix: sh -c + PTY |
| Runner | internal/runner/runner_windows.go | Windows: cmd /C + pipes |
| CLI | cmd/code-agents/main.go | CLI entry point (go-arg) |
| Config | cmd/code-agents/template.yaml | Embedded config template |

## Conventions

- Error handling: expected tool errors as string (1st return), Go errors only for critical failures
- Testing: testify/require for assertions, MockCompleter for LLM tests
- All internal packages under internal/ - not exported
- JSON tags on HTTP boundary types, YAML tags on config types
- nanoid for all generated IDs (agents, tasks)
- Path traversal protection via safePath() on all file operations
- Thread safety: Queue uses sync.Mutex, LLM Client is inherently thread-safe
- Completer interface in llm/types.go enables agent testability without HTTP mocking

## Dependencies

- github.com/alexflint/go-arg - CLI parsing
- gopkg.in/yaml.v3 - Config parsing
- github.com/matoous/go-nanoid/v2 - ID generation
- github.com/creack/pty - PTY for Unix shell (build tag: !windows)
- github.com/stretchr/testify - Testing

## Tool Distribution by Role

| Tool | Planner | Subplanner | Worker |
|------|---------|------------|--------|
| create_task | yes | yes | no |
| submit_handoff | no | yes | no |
| complete_task | no | no | yes |
| read_file | yes | no | yes |
| write_file | no | no | yes |
| edit_file | no | no | yes |
| replace_lines | no | no | yes |
| list_dir | yes | no | yes |
| shell_exec | yes | no | yes |
| git_status, git_diff, git_commit | no | no | yes |

## Documentation

Full project documentation in `docs/`:
- architecture.md, project-structure.md, configuration.md
- interfaces.md, orchestration-loop.md, tools.md, models.md

Reference project (loop orchestrator pattern): `clancy/`
