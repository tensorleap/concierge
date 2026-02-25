# Concierge High-Resolution Implementation Plan

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Purpose / Big Picture

Move Concierge from "contracts only" into an executable deterministic orchestrator with adapter seams, persistent artifacts, and fixture-backed behavior validation against Tensorleap Hub repositories.

The outcome for this phase is a reliable path from `concierge run --dry-run` to a real iteration engine with auditable reports and repeatable tests that compare pre-integration vs post-integration behavior.

## Progress

| Item | Status | Updated | Scope |
| --- | --- | --- | --- |
| Plan tracking bootstrap | `DONE` | 2026-02-25 07:07Z | Establish `PLAN.md` as the cross-session source of truth. |
| Step 1: Baseline cleanup | `ACCEPTED` | 2026-02-25 07:28Z (`main`) | Remove inherited Python/aider workflows, add Go module metadata, add minimal Go CI, and add README implementation-status note. |
| Step 2: Cobra CLI bootstrap + release automation | `ACCEPTED` | 2026-02-25 10:57Z (`main`) | Add `root`/`doctor`/`run --dry-run`/`version` with global `--log-level`; add semver release automation for Linux/macOS amd64+arm64 with release notes. |
| Step 3: Core deterministic contracts | `ACCEPTED` | 2026-02-25 11:24Z (`main`) | Add `internal/core` types + typed errors and `internal/core/ports` interfaces; seed issue-code catalog and deterministic issue-to-step mapping helpers. |
| Step 4A: Iteration engine skeleton | `PENDING` | — | Add `internal/orchestrator` engine with strict stage order: `snapshot -> inspect -> plan -> execute -> validate -> report`. |
| Step 4B: CLI run wiring for engine contract | `PENDING` | — | Wire `run --dry-run` to use orchestrator stage metadata so CLI reflects engine contract. |
| Step 5A: Snapshot adapter (repo identity baseline) | `PENDING` | — | Implement snapshot adapter for repo root/git root/branch/head/dirty + deterministic snapshot IDs/timestamps. |
| Step 5B: Inspector stub adapter | `PENDING` | — | Implement deterministic inspector stub returning structured status/issues for engine integration. |
| Step 5C: Planner adapter | `PENDING` | — | Implement planner adapter using `IssueCode -> EnsureStep` mapping to choose primary/secondary steps. |
| Step 5D: Fixture corpus preparation | `PENDING` | — | Prepare Tensorleap Hub fixtures with `pre-integration` and `post-integration` refs and manifest mapping. |
| Step 6A: `.concierge` persistence primitives | `PENDING` | — | Add atomic JSON persistence primitives for `state`, `reports`, and `evidence`. |
| Step 6B: Persist reports/evidence from engine | `PENDING` | — | Persist iteration reports and evidence artifacts produced by orchestration stages. |
| Step 6C: Fixture-backed behavior tests | `PENDING` | — | Run Concierge on prepared `pre-integration` fixtures and compare behavior/contract fingerprints to `post-integration` refs. |
| Step 7: Developer tooling and CI expansion | `PENDING` | — | Expand CI for lint/test/build and codify local dev commands. |
| Step 8: Docs sync with implementation | `PENDING` | — | Add architecture and developer setup docs; align README quickstart with real behavior. |

## Surprises & Discoveries

- Observation: Repository content started design-heavy and code-light.
  Evidence: initial code surface had only foundational files.
- Observation: Existing CI workflows were inherited from an unrelated Python template.
  Evidence: removed workflows referenced PyPI and unrelated Docker/page pipelines.
- Observation: Local system Go (`1.16.6`) cannot manage module operations for a `go 1.24` module.
  Evidence: dependency operations required a newer isolated Go toolchain.
- Observation: Step 3 already established adapter-agnostic contracts that are ready for engine wiring.
  Evidence: `internal/core` and `internal/core/ports` compile and pass tests.
- Observation: The previous plan was too coarse between "engine stubs" and "tests".
  Evidence: fixture preparation timing and behavior-ground-truth testing were not explicitly sequenced.

## Decision Log

- Decision: Use `PLAN.md` as the persistent cross-session implementation log.
  Rationale: explicit workflow agreement in `AGENTS.md`.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Status values remain strictly `PENDING`, `DONE`, or `ACCEPTED`.
  Rationale: keep lifecycle semantics machine-checkable and aligned with repo policy.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Acceptance is merge-based (`main`), not branch-based.
  Rationale: branch CI pass means `DONE`; merge to `main` means `ACCEPTED`.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Keep deterministic core adapter-agnostic and context-first.
  Rationale: enables isolated testing of orchestrator decisions independent of external tools.
  Date/Author: 2026-02-25 / assistant.
- Decision: Insert fixture preparation immediately after planner adapter implementation (Step 5C).
  Rationale: snapshot/inspect/plan seams must exist before creating realistic pre/post fixture workflows.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Run pre-vs-post fixture assertions after persistence/reporting primitives (Step 6C).
  Rationale: behavior comparison needs stable report/evidence artifacts, not only in-memory results.
  Date/Author: 2026-02-25 / user + assistant.

## Outcomes & Retrospective

Current status: Steps 1-3 are `ACCEPTED` on `main`; the repository baseline is stable.  
Current risk: implementing an engine without strong stage-level tests can hide ordering/regression bugs.  
Mitigation: Step 4A is explicitly test-first and stage-order strict before adapter implementation starts.

## Context and Orientation

Repository root: `/Users/assaf/Dropbox/tensorleap/concierge`

Current key files:
- `/Users/assaf/Dropbox/tensorleap/concierge/README.md`
- `/Users/assaf/Dropbox/tensorleap/concierge/PLAN.md`
- `/Users/assaf/Dropbox/tensorleap/concierge/cmd/concierge/main.go`
- `/Users/assaf/Dropbox/tensorleap/concierge/internal/cli/run.go`
- `/Users/assaf/Dropbox/tensorleap/concierge/internal/core/types.go`
- `/Users/assaf/Dropbox/tensorleap/concierge/internal/core/ports/interfaces.go`

Definitions:
- Iteration engine: deterministic orchestration runner for one full loop.
- Fixture corpus: curated repositories with both pre-integration and post-integration references for behavior comparison.

## Plan of Work

Implement the deterministic orchestration seam first (Step 4A), then align CLI dry-run behavior to that seam (Step 4B). Add minimal concrete adapters for snapshot/inspect/plan (Steps 5A-5C) so the orchestration path can execute with real data structures. At that point, prepare fixture repositories (Step 5D) and only then add persistence/reporting (Steps 6A-6B), so fixture assertions can rely on stable output artifacts. Finally, add fixture-backed behavior tests (Step 6C), followed by CI/tooling hardening and documentation sync.

Each step remains atomic, independently testable, and commit-scoped.

## Concrete Steps

Step 4A completion checks:
- `go test ./internal/orchestrator ./internal/core ./internal/core/ports`
- `go test ./...`

Step 4B completion checks:
- `go run ./cmd/concierge run --dry-run`
- `go test ./internal/cli`

Step 5A-5C completion checks:
- `go test ./internal/adapters/... ./internal/orchestrator ./internal/core`
- `go test ./...`

Step 5D completion checks:
- Fixture manifest exists and is parseable (for example `fixtures/manifest.json`).
- Each fixture has both `pre-integration` and `post-integration` refs recorded.
- Dry validation command confirms all fixture refs resolve.

Step 6A-6B completion checks:
- Atomic writer tests pass under forced interruption scenarios.
- Iteration report and evidence files are written under `.concierge/` with deterministic structure.

Step 6C completion checks:
- Fixture test runner executes Concierge over at least one prepared pre-integration fixture.
- Reported behavior fingerprints/contract checks are compared against corresponding post-integration references.
- Regression test fails when known contract invariants are intentionally violated in a fixture branch.

Step 7 completion checks:
- CI runs lint + test + build matrix on PR/push.
- Local developer commands in `Makefile` are documented and verified.

Step 8 completion checks:
- `docs/architecture.md` and `docs/dev-setup.md` exist and reflect current package boundaries and workflows.
- README quickstart matches implemented CLI behavior and current step status.

## Validation and Acceptance

Status semantics:
- `PENDING`: step not implemented.
- `DONE`: step implemented, committed, pushed, and branch CI passes.
- `ACCEPTED`: step merged to `main`.

Step 4A is accepted for merge readiness when:
- Engine executes stages in strict order.
- Stage failure short-circuits downstream stages with typed stage context.
- Success path returns a populated `core.IterationReport`.
- Existing CLI tests remain green.

Overall phase is accepted when Steps 4A-8 are `ACCEPTED`.

## Idempotence and Recovery

- Keep one commit per step scope to simplify review and rollback.
- If a step regresses behavior, revert that step’s commit without modifying accepted earlier steps.
- Re-running non-mutating checks should not modify repository-tracked files.

## Artifacts and Notes

Current working tree snapshot at plan update time:

    ## main...origin/main
    ?? bin/

Operational note:
- Untracked `bin/` is currently ignored for step-scoped commits unless the step explicitly includes release artifact handling.

## Interfaces and Dependencies

- Language/runtime: Go `1.24.x`
- CLI framework: Cobra
- Existing core contracts: `internal/core`, `internal/core/ports`
- Planned package additions: `internal/orchestrator`, `internal/adapters/*`, fixture test utilities
- CI platform: GitHub Actions
