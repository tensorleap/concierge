# Bootstrap Concierge Initial Go Foundation

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Purpose / Big Picture

Bootstrap Concierge from design-only state into a stable Go-first repository foundation that supports iterative implementation in small, verifiable steps.

The desired outcome for this phase is a clean baseline with correct project metadata, clean CI ownership, and a tracked implementation plan that survives across sessions.

## Progress

| Item | Status | Updated | Scope |
| --- | --- | --- | --- |
| Step 1: Baseline cleanup | `ACCEPTED` | 2026-02-25 07:28Z (`main`) | Remove inherited Python/aider workflows, add Go module metadata, add minimal Go CI, and add README implementation-status note. |
| Plan tracking bootstrap | `DONE` | 2026-02-25 07:07Z | Establish `PLAN.md` as the cross-session source of truth. |
| Step 2: Cobra CLI bootstrap + release automation | `ACCEPTED` | 2026-02-25 10:57Z (merged to `main`) | Add `root`/`doctor`/`run --dry-run`/`version` with global `--log-level`, plus semver release automation for Linux/macOS amd64+arm64 with release notes. |
| Step 3: Core deterministic contracts | `ACCEPTED` | 2026-02-25 11:24Z (`main`) | Added `internal/core` types + typed errors and `internal/core/ports` context-first interfaces, seeded a broad `IssueCode` catalog, and added deterministic `IssueCode -> EnsureStep` mapping helpers for planner scaffolding. |
| Step 4: Stub orchestration engine | `PENDING` | — | Add `snapshot -> inspect -> plan -> execute -> validate -> report` deterministic stub flow. |
| Step 5: `.concierge` persistence scaffolding | `PENDING` | — | Add `state`, `reports`, and `evidence` with atomic JSON writes. |
| Step 6: Test foundation | `PENDING` | — | Add unit tests and CLI smoke tests. |
| Step 7: Developer tooling and CI expansion | `PENDING` | — | Add tooling (`Makefile`, lint config) and expand CI for lint/test/build. |
| Step 8: Baseline docs | `PENDING` | — | Add `docs/architecture.md`, `docs/dev-setup.md`, and README quickstart. |

## Surprises & Discoveries

- Observation: Repository content was design-document heavy and code-light.
  Evidence: only `README.md` existed as tracked source content before setup work.
- Observation: Existing CI/workflow files were inherited from an unrelated Python/aider template.
  Evidence: removed workflows referenced PyPI, Windows Python tests, Docker images, and Jekyll pages for `aider`.
- Observation: Step 1 changes are currently uncommitted on branch `obey`.
  Evidence: `git status --short --branch` shows new Go CI + module files and deleted old workflows.
- Observation: Local system Go is `1.16.6`, which cannot manage a `go 1.24` module for dependency operations.
  Evidence: `go mod tidy` failed under system Go, then succeeded with an isolated Go `1.26.0` toolchain under `/tmp/concierge-go1.26`.
- Observation: Step 3 contract surfaces are now separated from CLI concerns under `internal/core` and `internal/core/ports`.
  Evidence: added contract-only types/errors/interfaces plus focused tests without modifying command behavior.
- Observation: Issue reporting now supports optional structured locations and non-location contract issues.
  Evidence: `Issue` now includes `scope` and optional `location`, while missing-contract issues (like missing preprocess) can omit location entirely.
- Observation: Step 3 now includes deterministic issue-to-step planning scaffolding.
  Evidence: added a typed ensure-step catalog, issue-step mapping table, and priority-based selection helpers with coverage tests.

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
- Decision: Step 2 CLI scope uses global `--log-level` only; defer output-mode flag.
  Rationale: user explicitly requested to eliminate `--output` until it has functional value.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Step 2 must include release deliverables.
  Rationale: user requires Linux/macOS binaries on amd64+arm64, semantic versioning, and release notes.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Regular CI must publish build artifacts from the start, not only release tags.
  Rationale: user requested binary visibility and cross-platform build confidence on PR/push workflows.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Keep Step 3 contracts adapter-agnostic and context-first.
  Rationale: deterministic orchestration should be testable without CLI/adapters and ready for Step 4 engine wiring.
  Date/Author: 2026-02-25 / assistant.
- Decision: Seed a broad known issue-code catalog now, but keep unknown codes valid.
  Rationale: planners/reporters get stable machine codes while preserving forward compatibility for adapter-specific findings.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Add a deterministic mapping from issue codes to preferred ensure-steps now (before Step 4 engine implementation).
  Rationale: enables planner behavior to remain rules-based and testable as orchestration wiring is introduced.
  Date/Author: 2026-02-25 / user + assistant.

## Outcomes & Retrospective

Current status: Steps 1-3 are `ACCEPTED` on `main`.  
Remaining risk: Step 4 stub engine should preserve deterministic behavior while wiring the new contract layer.

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

Step 2 completion checks (executed):
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go test ./...`
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go run ./cmd/concierge --help`
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go run ./cmd/concierge doctor`
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go run ./cmd/concierge run --dry-run`
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go run ./cmd/concierge version`
- Cross-compile matrix:
  - `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 ... go build ./cmd/concierge`
  - `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 ... go build ./cmd/concierge`
  - `GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 ... go build ./cmd/concierge`
  - `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 ... go build ./cmd/concierge`
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go run github.com/goreleaser/goreleaser/v2@latest check --config .goreleaser.yml`

Step 3 completion checks (executed):
- `GOTOOLCHAIN=local /tmp/concierge-go1.26/go/bin/go test ./...`

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
- Package boundaries (current + near-term): `cmd/concierge`, `internal/cli`, `internal/core`, `internal/core/ports`; `internal/adapters` remains planned.
