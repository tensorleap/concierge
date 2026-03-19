# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Concierge is a deterministic CLI orchestrator (Go 1.24) that automates Tensorleap integration setup. It runs a loop: Snapshot → Inspect → Plan → Execute → GitManager → Validate → Report. It invokes Claude Code as a subprocess for agent-assisted code authoring.

## Commands

```bash
make build                    # Build binary to bin/concierge
make test                     # Unit tests (Go) + QA tests (Python)
go test ./internal/...        # Go unit tests only
go test ./internal/core -run TestFoo  # Single test
make test-fixtures            # Prepare fixtures + verify + run E2E tests
make test-qa-loop             # Python QA harness tests only
make test-live-claude         # Agent integration tests (needs CONCIERGE_LIVE_CLAUDE=1)
make qa REPO=<name> QA_STEP=<step>  # Interactive QA against a fixture
make clean                    # Remove bin/ directory
make fixtures-prepare         # Clone and create pre/post states
make fixtures-verify          # Verify fixture integrity
make fixtures-reset           # Prepare + verify in one step
```

### `concierge run` Flags

These are commonly needed when testing or developing:

- `--dry-run` — preview stages without making changes
- `--max-iterations N` — limit guided rounds (0 = unlimited)
- `--persist` — write reports/evidence to `.concierge/`
- `--project-root PATH` — override project root
- `--non-interactive` — fail instead of prompting
- `--yes` — auto-approve all mutation prompts
- `--model-path PATH` — preferred model artifact path
- `--no-color` — disable colorized output
- `--debug-output` — show internal debug details

## Architecture

### Port-Adapter Pattern

Core interfaces live in `internal/core/ports/interfaces.go`:
- **Snapshotter** — captures git/workspace state
- **Inspector** — analyzes Tensorleap artifacts for issues
- **Planner** — selects next ensure-step deterministically
- **Executor** — applies one ensure-step (often via Claude Code subprocess)
- **GitManager** — diff review, approval, commit/reject
- **Validator** — post-execution acceptance checks
- **Reporter** — publishes iteration output

Adapters implementing these ports live under `internal/adapters/<stage>/`.

### Two-Level Orchestration Loop

`internal/orchestrator/engine.go` has two layers:
- **`Engine.Run()`** — outer loop that repeats iterations until success, cancellation, user-action-required, agent interruption, or max-iteration limit. Carries blocking validation issues between iterations.
- **`Engine.RunIteration()`** — executes the canonical stage sequence once: Snapshot → Inspect → Plan → Execute → GitManager → Validate → Report. If the executor applied changes, it re-snapshots and re-inspects before validation.

Stop reasons (`RunStopReason`): `success`, `max_iterations`, `cancelled`, `interrupted_step`, `needs_user_action`.

### Key Packages

- `cmd/concierge/` — entry point, delegates to CLI
- `internal/cli/` — Cobra commands (`run`, `doctor`, `version`); `run.go` wires all adapters into the engine and handles interactive prompts (model selection, encoder mapping, runtime confirmation, step/git approval)
- `internal/orchestrator/` — `Engine.Run()` / `Engine.RunIteration()` execute the orchestration loop
- `internal/core/` — domain types, ensure-step definitions, issues, checklist rendering
- `internal/agent/` — Claude Code subprocess invocation with task-scoped authoring contexts and repo-only scope policy
- `internal/gitmanager/` — change review, diff rendering, commit/reject flow; implements `ports.GitManager`
- `internal/observe/` — event recording (file-backed `Recorder`) and live terminal rendering (`HighlightsRenderer`); events emitted by engine, executor, and agent runner
- `internal/state/` — persistent `.concierge/state.json` management with invalidation detection (git HEAD, worktree fingerprint, runtime drift)
- `internal/persistence/` — low-level file persistence helpers
- `internal/e2e/` — fixture-based end-to-end tests

### Ensure-Steps

Ensure-steps are defined in `internal/core/ensure_steps.go`. The `ensureStepPriority` slice defines the canonical ordering. The planner (`internal/adapters/planner/deterministic_planner.go`) picks the first step whose blocking issues are unresolved.

Each step ID follows the `ensure.<name>` pattern (e.g., `ensure.preprocess_contract`). Steps progress from repository context → runtime → CLI/server → layout → authoring milestones → validation → upload.

### Inspector Contract Pipeline

The inspector (`internal/adapters/inspect/`) builds an `IntegrationStatus` with typed contracts:
- Each contract file (`preprocess_contract.go`, `input_encoder_contract.go`, `gt_encoder_contract.go`, `integration_contract.go`, `leap_yaml_contract.go`, etc.) checks one domain concern
- `input_gt_discovery_pipeline.go` orchestrates multi-stage input/GT symbol discovery using framework detection, normalization, and model I/O leads
- Issues are emitted with typed `IssueCode` values that the planner maps to ensure-steps via `core.PreferredEnsureStepForIssue()`

### Executor and Authoring Contexts

The executor (`internal/adapters/execute/`) dispatches steps to specialized handlers:
- **`DispatcherExecutor`** — routes steps to filesystem-based or agent-based execution
- **`AgentExecutor`** — builds `AgentTask` payloads and invokes the agent runner
- **`ApprovalExecutor`** — wraps execution with step-approval gates (CLI prompts in `run.go`)
- **Authoring context builders** — per-step files (`preprocess_authoring_context.go`, `input_encoder_authoring_context.go`, `model_authoring_context.go`, `gt_encoder_authoring_context.go`, `integration_test_authoring_context.go`, `model_acquisition_context.go`) construct task-scoped prompts with domain knowledge, repo context, and scope policy
- **`repo_context_pack.go`** — collects relevant file snippets and repo state for agent prompts

### Agent Integration

`internal/agent/runner.go` invokes Claude Code as a subprocess (15min default timeout). It supports streaming (`stream-json`) with fallback to buffered mode. Each task gets:
- A **system prompt** (`prompt_contract.go`) with Tensorleap integration rules
- A **task prompt** with objective, authoring context, domain knowledge sections, and scope policy
- Domain knowledge loaded from `internal/agent/context/loader.go` with versioned section IDs
- Scope policy (`agent_scope_policy.go`) restricting the agent to repo-only file access

Transcripts and raw streams are written to `.concierge/` for debugging.

## Workflow Rules

See `AGENTS.md` for the full workflow agreement. Key points:
- All implementation on feature branches, merged via PR — never commit directly to main
- Use `git worktree` for branch creation/switching
- Run local verification before commit/push
- One issue-sized scope per commit
- GitHub issues are the source of truth for backlog and progress

## Testing Tiers

1. **Go unit tests** — standard `*_test.go` files throughout `internal/`
2. **Python QA tests** — `QA/tests/test_*.py` (unittest discovery)
3. **Fixture E2E tests** — `internal/e2e/fixtures/` against real Tensorleap Hub repos
4. **Live Claude tests** — gated by `CONCIERGE_LIVE_CLAUDE=1` env var

## Fixture Management

Fixtures are cloned from Tensorleap Hub repos. Manifest at `fixtures/manifest.json`. Pre-state strips integration files (`leap.yaml`, `leap_binder.py`, `leap_custom_test.py`); post-state is the known-good integrated commit.

## Dependencies

Minimal: `cobra` (CLI), `yaml.v3` (config parsing), plus stdlib. No ORM, no HTTP framework.
