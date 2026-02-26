# Measuring Prompt Quality

This guide explains how to use run logging and comparison tools to objectively measure
the impact of prompt changes on code-agents performance.

## Overview

Each mission run is saved as a JSON file in `runs/` (default) containing:

- Token usage (prompt, completion, total)
- Run duration in milliseconds
- Number of failed/completed tasks
- Number of planner iterations
- The profile and prompt used

## Running with a Named Profile

Profiles allow you to maintain multiple prompt configurations and switch between them
for A/B comparisons.

**Create a profile:**

```bash
mkdir -p profiles
cp code-agents.yaml profiles/baseline.yaml
cp code-agents.yaml profiles/experiment.yaml
# Edit system prompts in profiles/experiment.yaml
```

**Run with a profile:**

```bash
code-agents --profile baseline   "build a REST API with CRUD endpoints"
code-agents --profile experiment "build a REST API with CRUD endpoints"
```

Run logs are saved to `runs/YYYY-MM-DD-<id>.json` automatically after each run.

## Comparing Two Runs

```bash
code-agents compare runs/2024-01-15-abc12345.json runs/2024-01-15-xyz67890.json
```

Example output:

```
┌─────────────────────────────────────────────────────────────┐
│                    RUN COMPARISON                          │
├──────────────────────┬────────────────┬────────────────┬───┤
│ Metric               │ abc12345       │ xyz67890       │ Δ │
├──────────────────────┼────────────────┼────────────────┼───┤
│ profile              │ baseline       │ experiment     │ ~ │
│ success              │ true           │ true           │ = │
│ duration_ms          │ 45230          │ 38110          │ -15.8% │
│ prompt_tokens        │ 12400          │ 10900          │ -12.1% │
│ completion_tokens    │ 3200           │ 2800           │ -12.5% │
│ total_tokens         │ 15600          │ 13700          │ -12.2% │
│ failed_tasks         │ 1              │ 0              │ -100.0% │
│ completed_tasks      │ 7              │ 8              │ +14.3% │
│ planner_iterations   │ 5              │ 4              │ -20.0% │
└──────────────────────┴────────────────┴────────────────┴───┘
```

## What to Look For

| Metric | Better = | Notes |
|---|---|---|
| `total_tokens` | Lower | Direct cost indicator |
| `duration_ms` | Lower | Wall-clock time |
| `failed_tasks` | Lower (0 ideal) | Task reliability |
| `completed_tasks` | Higher | Output completeness |
| `planner_iterations` | Lower | Prompt clarity (fewer re-plans) |
| `success` | `true` | Sanity check |

## Recommended Workflow

1. **Establish a baseline** — run `--profile baseline` on several representative prompts
   and collect the JSON logs.

2. **Make one change at a time** — edit system prompts in your experiment profile,
   then run the same prompts again.

3. **Compare** — use `code-agents compare` to see the delta. Focus on `failed_tasks`,
   `planner_iterations`, and `total_tokens`.

4. **Iterate** — only adopt a change if it improves at least one metric without
   worsening others.

## Run Log Format

```json
{
  "id": "abc12345",
  "profile": "baseline",
  "prompt": "build a REST API with CRUD endpoints",
  "started_at": "2024-01-15T10:00:00Z",
  "completed_at": "2024-01-15T10:00:45Z",
  "duration_ms": 45230,
  "success": true,
  "prompt_tokens": 12400,
  "completion_tokens": 3200,
  "total_tokens": 15600,
  "failed_tasks": 1,
  "completed_tasks": 7,
  "planner_iterations": 5
}
```

Files are stored in `runs/YYYY-MM-DD-<id>.json` for easy organization by date.
Use `--runs-dir <path>` to change the output directory.
