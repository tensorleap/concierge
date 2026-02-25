# Bootstrap Concierge Initial Go Foundation

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Purpose / Big Picture

Bootstrap Concierge from design-only state into a stable Go-first repository foundation that supports iterative implementation in small, verifiable steps.

The desired outcome for this phase is a clean baseline with correct project metadata, clean CI ownership, and a tracked implementation plan that survives across sessions.

## Progress

- Step 1: Baseline cleanup (`DONE`, 2026-02-25 07:04Z). Scope: remove inherited Python/aider workflows, add Go module metadata, add minimal Go CI, and add README implementation status note.
- Plan tracking bootstrap: `DONE` (2026-02-25 07:07Z). Scope: establish `PLAN.md` as the cross-session source of truth.
- Step 2: Add Cobra CLI bootstrap (`root`, `doctor`, `run --dry-run`, `version`) with global output/log flags. Status: `PENDING`.
- Step 3: Add core deterministic contracts (`types`, `ports`, typed errors, context-first APIs). Status: `PENDING`.
- Step 4: Add stub orchestration engine (`snapshot -> inspect -> plan -> execute -> validate -> report`) with deterministic outputs. Status: `PENDING`.
- Step 5: Add `.concierge` persistence scaffolding (`state`, `reports`, `evidence`) with atomic JSON writes. Status: `PENDING`.
- Step 6: Add test foundation (unit tests + CLI smoke tests). Status: `PENDING`.
- Step 7: Add developer tooling (`Makefile`, lint config) and expand CI for lint/test/build. Status: `PENDING`.
- Step 8: Add baseline docs (`docs/architecture.md`, `docs/dev-setup.md`) and README quickstart. Status: `PENDING`.

## Surprises & Discoveries

- Observation: Repository content was design-document heavy and code-light.
  Evidence: only `README.md` existed as tracked source content before setup work.
- Observation: Existing CI/workflow files were inherited from an unrelated Python/aider template.
  Evidence: removed workflows referenced PyPI, Windows Python tests, Docker images, and Jekyll pages for `aider`.
- Observation: Step 1 changes are currently uncommitted on branch `obey`.
  Evidence: `git status --short --branch` shows new Go CI + module files and deleted old workflows.

## Decision Log

- Decision: Use `PLAN.md` (not `PLANS.md`) as the persistent plan file name.
  Rationale: explicit user preference and updated `work-planning` skill guidance.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Execute only Step 1 first before any scaffolding code.
  Rationale: keep setup incremental and auditable.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Baseline stack is Go with version `1.24.x`.
  Rationale: aligns with implementation plan and keeps toolchain target explicit.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Acceptance policy is merge-based.
  Rationale: a step may be `DONE` after implementation+commit+push+CI on a branch, and only merge to `main` counts as `ACCEPTED`.
  Date/Author: 2026-02-25 / user + assistant.

## Outcomes & Retrospective

Current status: foundational cleanup is complete and tracked.  
Remaining risk: the repository still lacks executable Concierge code until Step 2 begins.

## Context and Orientation

Repository root: `/Users/assaf/Dropbox/tensorleap/concierge`

Current key files:
- `/Users/assaf/Dropbox/tensorleap/concierge/README.md`
- `/Users/assaf/Dropbox/tensorleap/concierge/go.mod`
- `/Users/assaf/Dropbox/tensorleap/concierge/.github/workflows/ci.yml`
- `/Users/assaf/Dropbox/tensorleap/concierge/PLAN.md`

Definitions:
- ExecPlan: a living implementation plan with progress and decisions.
- Baseline setup: repository hygiene and project metadata, not feature implementation.

## Plan of Work

After committing Step 1, implement remaining steps sequentially to keep review surface small: CLI bootstrap first, then deterministic interfaces, then engine stubs, then persistence, then tests and CI expansion, and finally docs.  

Each step should end with command-level validation and a concise progress update in this file.

## Concrete Steps

Step 1 completion checks (already done):
- `go list -m`
- `ls .github/workflows`
- `git status --short --branch`

Next-step kickoff checks (for Step 2):
- `go version`
- `go test ./...` (expected to fail until packages/tests are added)
- `git status --short --branch` (ensure working tree state is understood before coding)

## Validation and Acceptance

Step 1 is `DONE` when:
- Old inherited workflows are removed from `.github/workflows/`.
- A Go module exists and resolves via `go list -m`.
- A minimal Go CI workflow exists and targets Go `1.24.x`.
- The README contains a brief implementation-status note with date.

Status semantics:
- `PENDING`: step not yet implemented.
- `DONE`: step implemented, committed, pushed, and branch CI loop passes.
- `ACCEPTED`: changes from the step are merged into `main`.

Overall bootstrap phase is accepted when all implementation steps are `ACCEPTED`.

## Idempotence and Recovery

- If setup commands are rerun, resulting files should remain consistent (`go.mod`, CI workflow, docs).
- If a future step introduces regressions, revert only that step’s changes and keep this plan plus accepted earlier steps intact.
- Keep commits step-scoped to simplify rollback and bisect.

## Artifacts and Notes

Current working tree snapshot (at plan creation time):

    ## obey
     D .github/workflows/check_pypi_version.yml
     D .github/workflows/docker-build-test.yml
     D .github/workflows/docker-release.yml
     D .github/workflows/issues.yml
     D .github/workflows/pages.yml
     D .github/workflows/pre-commit.yml
     D .github/workflows/release.yml
     D .github/workflows/ubuntu-tests.yml
     D .github/workflows/windows-tests.yml
     D .github/workflows/windows_check_pypi_version.yml
     M README.md
    ?? .github/workflows/ci.yml
    ?? go.mod
    ?? PLAN.md

## Interfaces and Dependencies

- Language/runtime: Go `1.24.x`
- CLI framework target: Cobra (starting in Step 2)
- CI platform: GitHub Actions
- Near-term package boundaries (to be created): `cmd/concierge`, `internal/cli`, `internal/core`, `internal/adapters`
