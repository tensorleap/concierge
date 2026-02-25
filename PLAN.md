# Concierge Detailed Implementation Plan (Execution-Ready)

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Revision Notes

- 2026-02-25 16:06Z: Completed Step 5A on `feature/step-5a-snapshot-adapter` and opened PR #4 with green branch CI; updated progress/status to advance next actionable step to 5B.
- 2026-02-25 12:28Z: Updated remaining `PENDING` steps to close design/plan gaps: added explicit Step 4C multi-iteration loop and Step 5H non-dry-run CLI wiring; strengthened Step 5A/5B/5C contracts; normalized all remaining steps with explicit interface changes and rollback boundaries.
- 2026-02-25 12:10Z: Replaced the coarse pending-step summary with a decision-complete implementation spec. The new plan defines exact interfaces, file-level edits, test cases, validation commands, fixture preparation strategy, and acceptance criteria for every pending step.

## Purpose / Big Picture

Move Concierge from a contract-only foundation to a deterministic, runnable orchestration system with:

1. A strict stage pipeline (`snapshot -> inspect -> plan -> execute -> validate -> report`).
2. Adapter implementations that can run without external services first.
3. Persistent state/report/evidence artifacts under `.concierge/`.
4. Fixture-based pre-integration vs post-integration comparisons using Tensorleap Hub repositories.

The desired outcome is that another engineer can implement each step with no additional product decisions.

## Progress

| Item | Status | Updated | Scope |
| --- | --- | --- | --- |
| Plan tracking bootstrap | `DONE` | 2026-02-25 07:07Z | Establish `PLAN.md` as the cross-session source of truth. |
| Step 1: Baseline cleanup | `ACCEPTED` | 2026-02-25 07:28Z (`main`) | Remove inherited Python/aider workflows, add Go module metadata, add minimal Go CI, and add README implementation-status note. |
| Step 2: Cobra CLI bootstrap + release automation | `ACCEPTED` | 2026-02-25 10:57Z (`main`) | Add `root`/`doctor`/`run --dry-run`/`version` with global `--log-level`; add semver release automation for Linux/macOS amd64+arm64 with release notes. |
| Step 3: Core deterministic contracts | `ACCEPTED` | 2026-02-25 11:24Z (`main`) | Add `internal/core` types + typed errors and `internal/core/ports` interfaces; seed issue-code catalog and deterministic issue-to-step mapping helpers. |
| Step 4A: Iteration engine skeleton | `ACCEPTED` | 2026-02-25 11:55Z (`main`) | Add `internal/orchestrator` engine and stage-scoped error handling with strict call order and short-circuit semantics. |
| Step 4B: CLI dry-run wiring to engine metadata | `ACCEPTED` | 2026-02-25 12:41Z (`main`) | Remove hardcoded stage string in `run --dry-run`; render stage order from orchestrator/core metadata. |
| Step 4C: Multi-iteration orchestration loop | `DONE` | 2026-02-25 12:49Z (`feature/step-4c-multi-iteration-loop`, PR #3) | Add explicit outer loop with max-iterations + deterministic stop conditions; aggregate per-iteration reports. |
| Step 5A: Snapshot adapter (git and workspace identity) | `DONE` | 2026-02-25 16:06Z (`feature/step-5a-snapshot-adapter`, PR #4) | Implement snapshot adapter for repo root/git root/branch/head plus stable worktree fingerprint for change detection and deterministic snapshot IDs. |
| Step 5B: Inspector adapter (Layer 1 baseline inventory) | `PENDING` | — | Implement deterministic inventory checks for required integration artifacts, plus minimal `leap.yaml` parse + `entryFile` validation, emitting canonical issues. |
| Step 5C: Planner adapter | `PENDING` | — | Implement planner using `IssueCode -> EnsureStep` mapping and deterministic primary/secondary step selection; define terminal "complete" step for no-issue state. |
| Step 5D: Executor adapter skeleton | `PENDING` | — | Add ensure-step dispatcher that returns structured execution results and evidence stubs. |
| Step 5E: Validator adapter baseline | `PENDING` | — | Add deterministic validation checks over execution results (without Python harness yet). |
| Step 5F: Reporter adapter baseline | `PENDING` | — | Add reporter that emits human-readable summary and machine-readable in-memory report payload. |
| Step 5G: Fixture corpus preparation | `PENDING` | — | Add fixture manifest and scripts that materialize post-integration and pre-integration working copies locally. |
| Step 5H: CLI `run` (non-dry-run) wiring | `PENDING` | — | Wire CLI `run` to default adapters + orchestrator multi-iteration loop with `--max-iterations`; no repo mutation beyond reporting in this phase. |
| Step 6A: `.concierge` persistence primitives | `PENDING` | — | Add atomic JSON writer and stable path layout for state/reports/evidence artifacts. |
| Step 6B: Persist reports and evidence from pipeline | `PENDING` | — | Wire reporter/executor outputs to `.concierge/reports` and `.concierge/evidence`; add CLI switch to enable persistence output. |
| Step 6C: Runtime harness baseline (Layer 2) | `PENDING` | — | Add initial Python harness invocation contract and deterministic parsing of harness outputs into `IssueCode`s. |
| Step 6D: Anti-stub heuristics baseline (Layer 3) | `PENDING` | — | Add constant-output and empty-subset heuristics as deterministic validator findings. |
| Step 6E: Fixture-backed pre-vs-post behavior tests | `PENDING` | — | Execute Concierge against prepared fixtures and assert behavior/contract deltas between pre and post variants. |
| Step 7: Developer tooling and CI expansion | `PENDING` | — | Add lint/test/build gates and fixture-test strategy in CI with safe defaults. |
| Step 8: Documentation sync | `PENDING` | — | Update architecture/dev setup/README to match actual implementation and fixture workflow. |

## Scope Boundaries For This Phase

In scope:

1. Deterministic orchestration and adapters in Go.
2. Local fixture preparation and local fixture validation.
3. Machine-readable artifacts and deterministic tests.

Out of scope for this phase:

1. Full live Tensorleap server E2E upload automation in CI.
2. Production-grade agent mediation boundary enforcement.
3. Broad Python model execution matrix beyond fixture needs.

## Detailed Step Specifications

### Step 4A: Iteration engine skeleton (`ACCEPTED`)

Objective:

Implement a deterministic orchestration engine that composes existing `ports` and enforces stage ordering and failure semantics.

Files to add:

1. `internal/orchestrator/engine.go`
2. `internal/orchestrator/errors.go`
3. `internal/orchestrator/engine_test.go`

Files to modify:

1. `internal/core/types.go` only if a new report field is strictly required (default: no change).

Public interface and types (locked decisions):

1. `type Dependencies struct { Snapshotter ports.Snapshotter; Inspector ports.Inspector; Planner ports.Planner; Executor ports.Executor; Validator ports.Validator; Reporter ports.Reporter; Clock func() time.Time }`
2. `func NewEngine(deps Dependencies) (*Engine, error)`
3. `func (e *Engine) RunIteration(ctx context.Context, req core.SnapshotRequest) (core.IterationReport, error)`
4. `type StageError struct { Stage core.Stage; Err error }` with `Error()` and `Unwrap()`.

Implementation tasks:

1. Validate all dependencies in `NewEngine`; return typed missing-dependency errors with operation context.
2. Provide default clock (`time.Now().UTC`) when `Clock` is nil.
3. Run stages strictly in this order: snapshot, inspect, plan, execute, validate, report.
4. On each stage failure, return `*StageError` and do not call later stages.
5. Build `core.IterationReport` from snapshot ID, selected step, validation result, and generated timestamp.
6. Call reporter once on success path; reporting errors should also be wrapped as `StageError{Stage: core.StageReport}`.

Test cases (exact):

1. `TestRunIterationSuccessCallsStagesInOrder`
2. `TestRunIterationSnapshotFailureStopsPipeline`
3. `TestRunIterationInspectFailureStopsPipeline`
4. `TestRunIterationPlanFailureStopsPipeline`
5. `TestRunIterationExecuteFailureStopsPipeline`
6. `TestRunIterationValidateFailureStopsPipeline`
7. `TestRunIterationReportFailureReturnsStageError`
8. `TestNewEngineRejectsMissingDependencies`
9. `TestRunIterationUsesDefaultClock`

Validation commands:

1. `go test ./internal/orchestrator ./internal/core ./internal/core/ports`
2. `go test ./...`

Acceptance criteria:

1. Success path returns a fully populated `core.IterationReport`.
2. Failure path is stage-specific and short-circuited.
3. Tests above exist and pass.

Rollback notes:

1. Revert only files under `internal/orchestrator` if regression appears.

---

### Step 4B: CLI dry-run wiring to engine metadata (`ACCEPTED`)

Objective:

Ensure CLI dry-run output reflects canonical stage metadata from orchestrator/core instead of hardcoded text.

Files to modify:

1. `internal/cli/run.go`
2. `internal/cli/run_test.go`

Optional files to add:

1. `internal/orchestrator/stages.go` if stage formatting helper is introduced.

Interface/API changes (if any):

- None (CLI surface remains the same; internal source-of-truth changes only).

Implementation tasks:

1. Replace hardcoded stage pipeline string in `run --dry-run` with `core.DefaultStages()`-derived rendering.
2. Keep user-visible order exactly: `snapshot -> inspect -> plan -> execute -> validate -> report`.
3. Preserve existing guard behavior: `run` without `--dry-run` still returns not-implemented error.

Exact tests to add/update:

1. `TestRunDryRunPrintsExecutionStages` (assert output generated from stage list, not string literal).
2. `TestRunWithoutDryRunReturnsNotImplementedError` (existing behavior unchanged).

Validation commands:

1. `go test ./internal/cli`
2. `go run ./cmd/concierge run --dry-run`

Acceptance criteria:

1. No hardcoded stage pipeline literal remains.
2. Existing CLI UX remains unchanged except implementation source of truth.

Rollback boundary:

- Revert only changes made to `internal/cli/run.go`, `internal/cli/run_test.go`, and any helper file added for stage rendering.

---

### Step 4C: Multi-iteration orchestration loop (`DONE`)

Objective:

Implement the explicit outer runtime loop required by README section 8: repeatedly execute `RunIteration` until success or max iterations, producing an aggregated run result.

Files to add:

1. `internal/orchestrator/run.go`
2. `internal/orchestrator/run_test.go`

Files to modify:

1. `internal/orchestrator/engine.go` (add method receiver/wiring as needed).
2. `internal/core/ensure_steps.go` (add terminal step ID if missing).

Interface/API changes (if any):

1. Add terminal ensure-step constant in core if missing:
   - `core.EnsureStepComplete` with stable ID `ensure.complete`.
2. Add orchestrator run API:
   - `type RunOptions struct { MaxIterations int }`
   - `type RunStopReason string` with constants `success`, `max_iterations`, and `cancelled`
   - `type RunResult struct { Reports []core.IterationReport; StopReason RunStopReason }`
   - `func (e *Engine) Run(ctx context.Context, req core.SnapshotRequest, opts RunOptions) (RunResult, error)`

Locked behavior:

1. If `opts.MaxIterations <= 0`, treat as `1`.
2. Stop conditions:
   1. Stop with `StopReason=success` when latest iteration selects `core.EnsureStepComplete`.
   2. Stop with `StopReason=max_iterations` when loop reaches `MaxIterations` without `ensure.complete`; return `nil` error.
   3. Stop with `StopReason=cancelled` and return `ctx.Err()` on context cancellation.
3. If `RunIteration` returns an error, return it immediately and do not continue looping.

Exact tests to add/update:

1. `TestEngineRunDefaultsToSingleIterationWhenMaxIterationsZero`
2. `TestEngineRunStopsOnEnsureStepComplete`
3. `TestEngineRunStopsAtMaxIterations`
4. `TestEngineRunPropagatesContextCancellation`
5. `TestEngineRunReturnsErrorWhenRunIterationErrors`

Validation commands:

1. `go test ./internal/orchestrator`
2. `go test ./...`

Acceptance criteria:

1. Multi-iteration behavior exists as a dedicated API and is fully unit-tested.
2. Stop conditions are deterministic and documented in code and plan.
3. Step 4A per-iteration stage ordering semantics remain unchanged.

Rollback boundary:

- Revert only changes in `internal/orchestrator/run.go`, `internal/orchestrator/run_test.go`, and any core ensure-step constants added for terminal completion.

---

### Step 5A: Snapshot adapter (git and workspace identity) (`DONE`)

Objective:

Create a deterministic snapshot implementation that populates `core.WorkspaceSnapshot` from filesystem and git, including a stable worktree fingerprint suitable for iteration-to-iteration change detection.

Files to add:

1. `internal/adapters/snapshot/git_snapshotter.go`
2. `internal/adapters/snapshot/git_snapshotter_test.go`

Files to modify (expected):

1. `internal/core/errors.go` and `internal/core/errors_test.go` (add `KindNotGitRepo`).
2. `internal/core/types.go` and related tests only if `WorktreeFingerprint` is added to `WorkspaceSnapshot`.

Interface/API changes (if any):

- Add one new snapshot field if core snapshot model supports it cleanly:
  - `WorktreeFingerprint string` (SHA-256 hex of `git status --porcelain` output)
- If core snapshot type is not extended, keep fingerprint internal but include it in snapshot ID algorithm.

Locked behavior:

1. Adapter type name: `GitSnapshotter`.
2. Snapshot request behavior:
   1. Use `request.RepoRoot` when provided.
   2. If empty, use current working directory.
3. Resolve absolute repository root.
4. Determine git root via `git rev-parse --show-toplevel`.
5. Determine branch via `git rev-parse --abbrev-ref HEAD`.
6. Determine HEAD via `git rev-parse HEAD`.
7. Determine worktree status via `git status --porcelain` raw output.
8. Worktree fingerprint algorithm:
   - `worktreeFp = sha256_hex(raw porcelain output bytes)`
9. Snapshot ID algorithm: SHA-256 of `root|gitRoot|branch|head|worktreeFp` (hex encoded, lowercase).
10. `CapturedAt` uses injected clock (default UTC now).

Non-git behavior (explicit):

1. If `git rev-parse --show-toplevel` fails, return typed error `KindNotGitRepo` with operation context.
2. Do not defer non-git classification to inspector, because snapshot-stage errors short-circuit by design.

Error mapping:

1. Command execution failures must include command and stderr in wrapped error message.
2. Non-git repository failures must use `KindNotGitRepo`.

Exact tests to add/update:

1. `TestSnapshotCapturesGitIdentity`
2. `TestSnapshotWorktreeFingerprintChangesWhenWorkingTreeChanges`
3. `TestSnapshotIDStableForSameState`
4. `TestSnapshotIDChangesOnHeadChange`
5. `TestSnapshotUsesRequestRepoRoot`
6. `TestSnapshotErrorsOutsideGitRepo` (assert `KindNotGitRepo`)

Validation commands:

1. `go test ./internal/adapters/snapshot`
2. `go test ./...`

Acceptance criteria:

1. Snapshot IDs change when meaningful git/worktree state changes, including dirty-content changes.
2. Adapter works on macOS and Linux with standard git CLI.

Rollback boundary:

- Revert only `internal/adapters/snapshot/*` and any core snapshot field additions made for this step.

---

### Step 5B: Inspector adapter (Layer 1 baseline inventory) (`PENDING`)

Objective:

Implement first-pass artifact inventory checks aligned with README section 8.2 and section 9 Layer 1, including minimal `leap.yaml` parsing and `entryFile` validation.

Files to add:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/adapters/inspect/baseline_inspector_test.go`

Files to modify (expected):

1. `internal/core/issues.go` (or equivalent) to add any missing issue codes listed below.
2. `internal/core/issue_step_map.go` to map new issue codes to ensure steps.

Interface/API changes (if any):

- Add issue codes if missing:
  1. `IssueCodeLeapYAMLUnparseable`
  2. `IssueCodeLeapYAMLEntryFileMissing`
  3. `IssueCodeLeapYAMLEntryFileNotFound`

Locked inspection checks:

1. Required root files: `leap.yaml`, `leap_binder.py`.
2. Required integration-test file: either `leap_custom_test.py` or `integration_test.py`.
3. Emit missing artifacts in `IntegrationStatus.Missing` using canonical labels:
   1. `leap.yaml`
   2. `leap_binder.py`
   3. `integration_test`
4. Emit corresponding issues with these codes:
   1. `IssueCodeLeapYAMLMissing`
   2. `IssueCodeIntegrationScriptMissing`
   3. `IssueCodeIntegrationTestMissing`
5. `leap.yaml` minimum correctness checks:
   1. Unparseable `leap.yaml` -> `IssueCodeLeapYAMLUnparseable`
   2. Missing `entryFile` key -> `IssueCodeLeapYAMLEntryFileMissing`
   3. `entryFile` does not resolve to an existing repo file -> `IssueCodeLeapYAMLEntryFileNotFound`
6. Severity for missing/invalid required artifacts: `error`.
7. Scope mapping:
   1. `leap.yaml` -> `IssueScopeLeapYAML`
   2. binder/test -> `IssueScopeIntegrationScript` or `IssueScopeIntegrationTest`

Exact tests to add/update:

1. `TestInspectorReportsAllMissingArtifacts`
2. `TestInspectorAcceptsEitherIntegrationTestFileName`
3. `TestInspectorNoIssuesWhenArtifactsExist`
4. `TestInspectorIssueScopesAndSeverities`
5. `TestInspectorLeapYAMLUnparseableEmitsIssue`
6. `TestInspectorLeapYAMLEntryFileMissingEmitsIssue`
7. `TestInspectorLeapYAMLEntryFileNotFoundEmitsIssue`

Validation commands:

1. `go test ./internal/adapters/inspect`
2. `go test ./...`

Acceptance criteria:

1. Inspector emits deterministic missing/issue output for equivalent directories.
2. Issue codes align with existing mapping in `internal/core/issue_step_map.go`.
3. `leap.yaml` parse and `entryFile` validation are enforced.

Rollback boundary:

- Revert only `internal/adapters/inspect/*` plus issue-code and issue-to-step mapping additions made for this step.

---

### Step 5C: Planner adapter (`PENDING`)

Objective:

Implement planner adapter that converts inspection issues into deterministic `ExecutionPlan`, including a terminal "complete" step when no issues exist.

Files to add:

1. `internal/adapters/planner/deterministic_planner.go`
2. `internal/adapters/planner/deterministic_planner_test.go`

Files to modify (expected):

1. `internal/core/ensure_steps.go` to ensure `core.EnsureStepComplete` exists with ID `ensure.complete`.

Interface/API changes (if any):

- Ensure presence of `core.EnsureStepComplete` with stable ID `ensure.complete`.

Locked behavior:

1. Planner uses `core.PreferredEnsureStepsForIssues(status.Issues)`.
2. If no issues exist or no steps are returned, planner returns:
   1. `Primary = core.EnsureStepComplete`
   2. `Additional = nil`
3. Primary step = highest-priority step in returned list.
4. Additional steps = remaining returned steps in priority order.
5. Planner does not mutate or reorder issue list except through deterministic priority rules.

Exact tests to add/update:

1. `TestPlannerChoosesPrimaryByPriority`
2. `TestPlannerReturnsAdditionalSteps`
3. `TestPlannerReturnsCompleteWhenNoIssues`
4. `TestPlannerDeterministicAcrossRepeatedCalls`

Validation commands:

1. `go test ./internal/adapters/planner`
2. `go test ./...`

Acceptance criteria:

1. Planner output is deterministic and fully covered by unit tests.
2. A no-issue state yields an explicit terminal step compatible with Step 4C loop semantics.

Rollback boundary:

- Revert only `internal/adapters/planner/*` and any `core.EnsureStepComplete` additions.

---

### Step 5D: Executor adapter skeleton (`PENDING`)

Objective:

Provide a safe executor skeleton that accepts ensure-step requests and returns structured evidence without mutating user repos.

Files to add:

1. `internal/adapters/execute/stub_executor.go`
2. `internal/adapters/execute/stub_executor_test.go`

Interface/API changes (if any):

- None (implements existing `ports.Executor` contract).

Locked behavior:

1. For any known step ID, return `ExecutionResult{Applied:false, Summary:"not implemented"}`.
2. Include one evidence item with key `executor.mode` and value `stub`.
3. Unknown step IDs return `KindStepNotApplicable` typed errors.

Exact tests to add/update:

1. `TestExecutorReturnsStubResultForKnownStep`
2. `TestExecutorRejectsUnknownStep`
3. `TestExecutorReturnsEvidenceStub`

Validation commands:

1. `go test ./internal/adapters/execute`
2. `go test ./...`

Acceptance criteria:

1. Orchestrator can run end-to-end with deterministic non-mutating executor behavior.

Rollback boundary:

- Revert only `internal/adapters/execute/*`.

---

### Step 5E: Validator adapter baseline (`PENDING`)

Objective:

Add deterministic validator baseline for execution-level consistency checks before Python harness exists.

Files to add:

1. `internal/adapters/validate/baseline_validator.go`
2. `internal/adapters/validate/baseline_validator_test.go`

Interface/API changes (if any):

- None (implements existing `ports.Validator` contract).

Locked behavior:

1. If `ExecutionResult.Step.ID` is empty, return failed validation with `IssueCodeHarnessValidationFailed`.
2. If `ExecutionResult.Applied` is false and summary contains `not implemented`, return passed=true with warning note issue `IssueCodeUnknown` severity `info`.
3. If execution returned explicit error upstream, validator is not called (covered by orchestrator tests in Step 4A).

Exact tests to add/update:

1. `TestValidatorFailsOnEmptyStepID`
2. `TestValidatorPassesForStubExecution`
3. `TestValidatorDeterministicOutput`

Validation commands:

1. `go test ./internal/adapters/validate`
2. `go test ./...`

Acceptance criteria:

1. Validator behavior is explicit and deterministic for stub execution phase.

Rollback boundary:

- Revert only `internal/adapters/validate/baseline_validator*`.

---

### Step 5F: Reporter adapter baseline (`PENDING`)

Objective:

Add baseline reporter that can print concise human summary and keep machine report payload intact.

Files to add:

1. `internal/adapters/report/stdout_reporter.go`
2. `internal/adapters/report/stdout_reporter_test.go`

Interface/API changes (if any):

- None (implements existing `ports.Reporter` contract).

Locked behavior:

1. Reporter outputs one line: `snapshot=<id> step=<step-id> validation=<passed|failed>`.
2. Reporter does not write files in this step.
3. Reporter returns error on write failure.

Exact tests to add/update:

1. `TestReporterWritesSingleLineSummary`
2. `TestReporterReturnsWriteError`

Validation commands:

1. `go test ./internal/adapters/report`
2. `go test ./...`

Acceptance criteria:

1. End-to-end pipeline can run with baseline adapters without persistence.

Rollback boundary:

- Revert only `internal/adapters/report/stdout_reporter*`.

---

### Step 5G: Fixture corpus preparation (`PENDING`)

Objective:

Create reproducible local fixture preparation that provides paired pre-integration and post-integration states.

Locked fixture set for initial coverage:

1. `Tensorleap-hub/mnist` @ `1c4c6f0d5c4254cc4ae5c2df5ae8ef9f0a61212b`
2. `Tensorleap-hub/cifar10_resnet` @ `5dd3611ca9efdf34c28279e036a7bb90638dda4d`
3. `Tensorleap-hub/IMDb` @ `b4ce7cf109c89402147cc2e7f071165fc143d1d8`

Important decision:

1. We do not require write access to upstream repos.
2. Pre-integration variants are generated locally from pinned post-integration commits by stripping integration artifacts.

Files to add:

1. `fixtures/manifest.json`
2. `scripts/fixtures_prepare.sh`
3. `scripts/fixtures_verify.sh`
4. `fixtures/README.md`

Interface/API changes (if any):

- None (scripts + data only).

Manifest schema (locked):

```json
{
  "fixtures": [
    {
      "id": "mnist",
      "repo": "https://github.com/tensorleap-hub/mnist.git",
      "post_ref": "1c4c6f0d5c4254cc4ae5c2df5ae8ef9f0a61212b",
      "strip_for_pre": ["leap.yaml", "leap_binder.py", "leap_custom_test.py"]
    }
  ]
}
```

Preparation behavior (locked):

1. Create `.fixtures/<id>/post` by cloning and checking out `post_ref`.
2. Create `.fixtures/<id>/pre` by copying `post` and deleting `strip_for_pre` files.
3. Never modify tracked repository files during fixture materialization.

Verification behavior:

1. Confirm `post` contains all stripped files.
2. Confirm `pre` is missing all stripped files.
3. Confirm both working copies are clean git trees.

Exact tests to add/update:

- None (script-validated). Go fixture tests are added in Step 6E.

Validation commands:

1. `bash scripts/fixtures_prepare.sh`
2. `bash scripts/fixtures_verify.sh`

Acceptance criteria:

1. Fixture pairs are reproducible from manifest only.
2. Preparation process is idempotent.

Rollback boundary:

- Revert only `fixtures/*` and `scripts/fixtures_*.sh`. Generated `.fixtures/` directories are disposable.

---

### Step 5H: CLI `run` (non-dry-run) wiring (`PENDING`)

Objective:

Implement the real `concierge run` path by wiring default adapters into the orchestrator engine and invoking the multi-iteration loop from Step 4C.

Files to add:

- None required (expected to be wiring changes only).

Files to modify:

1. `internal/cli/run.go`
2. `internal/cli/run_test.go`
3. `internal/cli/root.go` only if flag plumbing requires it.

Interface/API changes (if any):

1. Add `--max-iterations` (int) to `concierge run`, default `1`.
2. Keep `run --dry-run` surface unchanged (as completed in Step 4B).

Locked behavior:

1. `concierge run` without `--dry-run`:
   1. Constructs default adapters:
      - snapshot: `GitSnapshotter`
      - inspect: `BaselineInspector`
      - planner: `DeterministicPlanner`
      - executor: `StubExecutor` (non-mutating)
      - validator: `BaselineValidator`
      - reporter: `StdoutReporter`
   2. Builds engine via `orchestrator.NewEngine(...)`.
   3. Calls `engine.Run(ctx, core.SnapshotRequest{RepoRoot: <resolved>}, RunOptions{MaxIterations: flag})`.
2. Exit behavior:
   - `StopReason=success` -> return nil (exit code 0).
   - `StopReason=max_iterations` -> return non-nil error (exit code 1) after reporter output.
   - `StopReason=cancelled` -> return `ctx.Err()`.

Exact tests to add/update:

1. `TestRunNonDryRunExecutesSingleIterationByDefault`
2. `TestRunNonDryRunHonorsMaxIterationsFlag`
3. `TestRunNonDryRunReturnsErrorOnMaxIterationsStop`

Validation commands:

1. `go test ./internal/cli`
2. `go run ./cmd/concierge run --max-iterations=1`
3. `go run ./cmd/concierge run --max-iterations=2`

Acceptance criteria:

1. `concierge run` is implemented and exercises full pipeline with default adapters.
2. Multi-iteration behavior is observable via repeated reporter output.
3. No repo mutation occurs; executor remains stub.

Rollback boundary:

- Revert only CLI wiring changes for `run` and related tests/flags.

---

### Step 6A: `.concierge` persistence primitives (`PENDING`)

Objective:

Implement atomic persistence building blocks used by reporter and state management.

Files to add:

1. `internal/persistence/atomic_json.go`
2. `internal/persistence/paths.go`
3. `internal/persistence/atomic_json_test.go`
4. `internal/persistence/paths_test.go`

Interface/API changes (if any):

- New internal persistence APIs:
  - `WriteJSONAtomic(path string, v any) error`
  - path helpers/types for `.concierge` state/report/evidence layout.

Locked behavior:

1. `WriteJSONAtomic(path string, v any) error` writes to temp file in same directory then renames.
2. Temp filename pattern: `<target>.tmp.<pid>.<nsec>`.
3. Path builder roots everything under `.concierge/` relative to selected project root.
4. Directory layout:
   1. `.concierge/state/state.json` (if changed later to `.concierge/state.json` for README alignment, update this section and Step 8 docs together)
   2. `.concierge/reports/<snapshot-id>.json`
   3. `.concierge/evidence/<snapshot-id>/<evidence-name>.log`

Exact tests to add/update:

1. `TestWriteJSONAtomicCreatesFile`
2. `TestWriteJSONAtomicOverwritesSafely`
3. `TestWriteJSONAtomicRejectsInvalidPath`
4. `TestPathsBuilderReturnsExpectedLayout`

Validation commands:

1. `go test ./internal/persistence`
2. `go test ./...`

Acceptance criteria:

1. Persistence helpers are deterministic and independently tested.

Rollback boundary:

- Revert only `internal/persistence/*`.

---

### Step 6B: Persist reports and evidence (`PENDING`)

Objective:

Wire orchestration outputs to filesystem artifacts using Step 6A primitives, and expose persistence enablement in CLI.

Files to add:

1. `internal/adapters/report/file_reporter.go`
2. `internal/adapters/report/file_reporter_test.go`

Files to modify:

1. `internal/adapters/execute/stub_executor.go` (optional evidence payload enrichment).
2. `internal/cli/run.go` and `internal/cli/run_test.go` (add persistence flag + wiring).
3. `internal/orchestrator/engine.go` only if dependency injection requires reporter mode selection.

Interface/API changes (if any):

1. CLI surface:
   - Add `--persist` (bool) to `concierge run`, default `false` in this phase.

Locked behavior:

1. Reporter writes `core.IterationReport` JSON to `.concierge/reports/<snapshot-id>.json`.
2. Evidence items are written as one file per item under `.concierge/evidence/<snapshot-id>/`.
3. Reporter also prints one-line summary to stdout.
4. CLI wiring:
   - `--persist=true`: use file reporter.
   - `--persist=false`: use stdout reporter.

Exact tests to add/update:

1. `TestFileReporterWritesReportJSON`
2. `TestFileReporterWritesEvidenceFiles`
3. `TestFileReporterPreservesExistingEvidenceDirectory`
4. `TestRunWithPersistWritesConciergeArtifacts`

Validation commands:

1. `go test ./internal/adapters/report ./internal/persistence ./internal/cli`
2. `go test ./...`

Acceptance criteria:

1. Running one pipeline iteration with `--persist` produces report and evidence artifacts on disk.
2. Running without `--persist` does not write `.concierge` outputs.

Rollback boundary:

- Revert only `internal/adapters/report/file_reporter*`, CLI persistence wiring, and any optional executor evidence enrichment.

---

### Step 6C: Runtime harness baseline (Layer 2) (`PENDING`)

Objective:

Add a minimal harness contract invocation path and parser so validator can consume runtime check outputs deterministically.

Files to add:

1. `internal/adapters/validate/harness_runner.go`
2. `internal/adapters/validate/harness_parser.go`
3. `internal/adapters/validate/harness_runner_test.go`
4. `internal/adapters/validate/harness_parser_test.go`
5. `scripts/harness_stub.py`

Interface/API changes (if any):

- Add deterministic harness gate via env var:
  - `CONCIERGE_ENABLE_HARNESS=1` enables harness execution.

Locked behavior:

1. Harness invocation is optional and gated by `CONCIERGE_ENABLE_HARNESS=1` (unset/other values disable it).
2. Harness output format is newline-delimited JSON events.
3. Parser maps events to issue codes:
   1. preprocess failure -> `IssueCodeHarnessPreprocessFailed`
   2. encoder coverage incomplete -> `IssueCodeHarnessEncoderCoverageIncomplete`
   3. generic validation failure -> `IssueCodeHarnessValidationFailed`
4. Harness timeout default: 120 seconds.

Exact tests to add/update:

1. `TestHarnessParserMapsKnownEvents`
2. `TestHarnessParserUnknownEventFallsBackToUnknownIssue`
3. `TestHarnessRunnerTimeout`
4. `TestHarnessRunnerSuccessPath`
5. `TestHarnessRunnerRespectsEnablementEnvVar`

Validation commands:

1. `go test ./internal/adapters/validate`
2. `go test ./...`

Acceptance criteria:

1. Validator can ingest deterministic harness signals into structured issues.
2. Harness invocation is off by default and deterministic when enabled.

Rollback boundary:

- Revert only `internal/adapters/validate/harness_*` and `scripts/harness_stub.py`.

---

### Step 6D: Anti-stub heuristics baseline (Layer 3) (`PENDING`)

Objective:

Detect likely stub integrations with deterministic heuristics and explicit issue codes.

Files to add:

1. `internal/adapters/validate/heuristics.go`
2. `internal/adapters/validate/heuristics_test.go`

Interface/API changes (if any):

- None (uses existing issue-code model; add missing codes in core if required).

Locked heuristics:

1. Constant input fingerprint across sampled indices -> `IssueCodeSuspiciousConstantInputs`.
2. Constant label fingerprint across sampled indices -> `IssueCodeSuspiciousConstantLabels`.
3. Empty train or validation subset in harness report -> `IssueCodePreprocessSubsetEmpty`.

Exact tests to add/update:

1. `TestHeuristicsDetectConstantInputs`
2. `TestHeuristicsDetectConstantLabels`
3. `TestHeuristicsDetectEmptySubset`
4. `TestHeuristicsNoFalsePositiveOnVaryingData`

Validation commands:

1. `go test ./internal/adapters/validate`
2. `go test ./...`

Acceptance criteria:

1. Heuristic issues are deterministic and use existing core issue codes.

Rollback boundary:

- Revert only `internal/adapters/validate/heuristics*`.

---

### Step 6E: Fixture-backed pre-vs-post behavior tests (`PENDING`)

Objective:

Use prepared fixture pairs to prove Concierge distinguishes pre-integration and post-integration states.

Files to add:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `internal/e2e/fixtures/testdata/README.md`
3. `scripts/fixtures_run_checks.sh`

Interface/API changes (if any):

- None (tests and scripts only).

Locked assertions per fixture:

1. Pre variant must emit missing-artifact issues (`leap.yaml`, integration script, integration test).
2. Post variant must not emit those missing-artifact issues.
3. Planner primary step for pre variant must be one of:
   1. `ensure.leap_yaml`
   2. `ensure.integration_script`
   3. `ensure.integration_test_contract`
4. Pipeline report files exist for both variants when persistence is enabled.

Execution policy:

1. Fixture E2E tests are opt-in locally and in CI via explicit flag:
   1. env `CONCIERGE_RUN_FIXTURE_E2E=1`
2. Default `go test ./...` should skip fixture network/materialization work.

Exact tests to add/update:

1. `TestFixturePreVsPostIssueDeltas` (table-driven over fixtures)
2. `TestFixturePlannerPrimaryStepPreVariant` (table-driven)
3. `TestFixturePersistenceArtifactsExistWhenEnabled`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_prepare.sh`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -v`

Acceptance criteria:

1. At least three fixtures execute with deterministic pass/fail behavior.
2. Failing fixture output includes actionable issue code diffs.

Rollback boundary:

- Revert only `internal/e2e/fixtures/*` and `scripts/fixtures_run_checks.sh`.

---

### Step 7: Developer tooling and CI expansion (`PENDING`)

Objective:

Codify local and CI quality gates, including optional fixture E2E execution.

Files to add:

1. `Makefile` targets update (if missing targets).
2. `.golangci.yml` (or equivalent lint config).
3. `.github/workflows/ci.yml` updates.

Interface/API changes (if any):

- None (tooling and CI only).

Locked CI behavior:

1. Required on PR/push:
   1. `go test ./...`
   2. cross-platform build checks for configured matrix
2. Optional scheduled/manual fixture E2E job gated by environment and timeout budget.
3. Release workflow from Step 2 remains unchanged.

Exact tests to add/update:

- None (configuration changes only). Existing test suite must remain green.

Validation commands:

1. `make test`
2. `make lint`
3. `make build`

Acceptance criteria:

1. CI fails on lint/test/build regressions.
2. Fixture E2E path is documented and isolated from default fast CI.

Rollback boundary:

- Revert only CI/workflow and lint configuration changes.

---

### Step 8: Documentation sync (`PENDING`)

Objective:

Make docs reflect actual behavior and implementation boundaries.

Files to add or update:

1. `README.md`
2. `docs/architecture.md`
3. `docs/dev-setup.md`
4. `docs/fixtures.md`

Interface/API changes (if any):

- None (documentation only).

Locked documentation content:

1. Architecture diagram/text for orchestrator + adapter layers.
2. Step-by-step developer setup for Go toolchain and fixture preparation.
3. Exact explanation of `PENDING` / `DONE` / `ACCEPTED` semantics.
4. Clear statement of what is deterministic now vs planned later.

Validation commands:

1. Verify all command snippets in docs execute or are explicitly marked as examples.
2. `go test ./...` after documentation updates (guard against accidental code drift).

Acceptance criteria:

1. README quickstart and docs align with implemented CLI behavior.
2. Fixture workflow is reproducible from docs only.

Rollback boundary:

- Revert only documentation files.

## Concrete Step Execution Order

Note: Step 4A is already `ACCEPTED`. The list below is the intended order for remaining `PENDING` steps.

1. Implement Step 4B and commit.
2. Implement Step 4C and commit.
3. Implement Step 5A and commit.
4. Implement Step 5B and commit.
5. Implement Step 5C and commit.
6. Implement Step 5D and commit.
7. Implement Step 5E and commit.
8. Implement Step 5F and commit.
9. Implement Step 5G and commit.
10. Implement Step 5H and commit.
11. Implement Step 6A and commit.
12. Implement Step 6B and commit.
13. Implement Step 6C and commit.
14. Implement Step 6D and commit.
15. Implement Step 6E and commit.
16. Implement Step 7 and commit.
17. Implement Step 8 and commit.

Each step follows this gate:

1. Implement step scope only.
2. Run step-specific validation commands.
3. Update step status to `DONE` after commit/push/CI pass.
4. Mark step `ACCEPTED` only after merge to `main`.

## Validation and Acceptance

Status semantics:

1. `PENDING`: step not implemented.
2. `DONE`: step implemented, committed, pushed, and branch CI passes.
3. `ACCEPTED`: step merged to `main`.

Phase acceptance condition:

1. Steps 4A through 8 are all `ACCEPTED`.
2. Fixture E2E has at least one green run on all three locked fixtures.

## Idempotence and Recovery

1. Keep one commit per step for clean rollback and bisect.
2. If a step fails CI, fix within that step scope only; do not start next step.
3. If a step introduces regressions, revert that step commit only.
4. Fixture preparation scripts must be safe to rerun without manual cleanup.

## Surprises & Discoveries

- Observation: The previous version of this plan was not implementation-complete for pending steps.
  Evidence: It lacked file-level changes, interface contracts, and test case definitions for each pending item.
- Observation: Tensorleap Hub repositories suitable for fixture seeding are publicly listable and have stable `main` heads.
  Evidence: `mnist`, `cifar10_resnet`, and `IMDb` were resolved with concrete commit SHAs.

## Decision Log

- Decision: Keep step statuses strictly to `PENDING`, `DONE`, and `ACCEPTED`.
  Rationale: Required by repository workflow agreement.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Sequence fixture preparation before persistence-enabled fixture assertions.
  Rationale: Reproducible fixture setup is a dependency for behavior comparison tests.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Define and lock initial fixture repositories and SHAs now.
  Rationale: Removes ambiguity when implementing Step 5G and Step 6E.
  Date/Author: 2026-02-25 / assistant.
- Decision: Keep default `go test ./...` fast by making networked fixture E2E opt-in.
  Rationale: Prevent default CI from becoming flaky or slow.
  Date/Author: 2026-02-25 / assistant.
- Decision: Add a dedicated multi-iteration loop step (4C) and a separate non-dry-run CLI wiring step (5H).
  Rationale: Align implementation sequencing with README loop semantics and make stop conditions explicit.
  Date/Author: 2026-02-25 / user + assistant.
- Decision: Strengthen snapshot identity with a worktree fingerprint instead of a dirty boolean.
  Rationale: Ensure iteration-to-iteration change detection and avoid persistence artifact key collisions.
  Date/Author: 2026-02-25 / user + assistant.

## Outcomes & Retrospective

Current status: foundational steps (1-4C) plus Step 5A are complete through branch/PR status, and pending work remains specified at implementation depth. The next executable atomic step is Step 5B.

Residual risk: runtime harness behavior (Step 6C) may reveal additional Python environment assumptions.

Mitigation: harness invocation is introduced behind an explicit gate and parser contract before broad E2E rollout.

## Artifacts and Notes

Current working tree snapshot before this plan revision commit:

    ## main...origin/main
    M PLAN.md
    ?? bin/

Operational note:

1. Untracked `bin/` remains out of scope for planning commits unless a step explicitly targets release artifact handling.

## Interfaces and Dependencies

1. Language/runtime: Go `1.24.x`.
2. CLI framework: Cobra.
3. Core contracts: `internal/core`, `internal/core/ports`.
4. Planned new packages:
   1. `internal/orchestrator`
   2. `internal/adapters/snapshot`
   3. `internal/adapters/inspect`
   4. `internal/adapters/planner`
   5. `internal/adapters/execute`
   6. `internal/adapters/validate`
   7. `internal/adapters/report`
   8. `internal/persistence`
   9. `internal/e2e/fixtures`
5. External tools used by specific steps:
   1. `git`
   2. `bash`
   3. optional Python runtime for harness step
