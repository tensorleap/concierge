# Concierge Detailed Implementation Plan (Execution-Ready)

This ExecPlan is a living document. Keep `Progress`, `Gap Analysis`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Revision Notes

- 2026-02-26: Re-baselined remaining work after a full README/PLAN/code audit. Added explicit gap analysis and replaced the old two-step tail (`Step 7`, `Step 8`) with a detailed operational completion plan (`Steps 7A-12B`).
- 2026-02-26: Verified current baseline is green locally: `go test ./...`, `bash scripts/fixtures_prepare.sh`, `bash scripts/fixtures_verify.sh`, `bash scripts/fixtures_run_checks.sh`.
- 2026-02-26: Confirmed current implementation is still scaffold-first (stub executor, optional harness gate, no real `leap push` path, no git approval/commit loop in product runtime).

## Purpose / Exit Target

Finish Concierge from deterministic scaffold to fully operational integration assistant that:

1. Executes real ensure-steps (not stub-only).
2. Supports user-approved repo mutations and audited commits on feature branches.
3. Runs Tensorleap readiness checks, harness validation, and guarded upload flow.
4. Produces persistent state/evidence artifacts under `.concierge/`.
5. Is validated end-to-end against fixture repos in CI.

## Gap Analysis (README vs PLAN vs Current Code)

| Gap ID | Requirement Source | Current State | Impact | Closing Step |
| --- | --- | --- | --- | --- |
| G1 | README §8/§10: Ensure-step `Fix` must apply real actions | `internal/adapters/execute/stub_executor.go` always returns `Applied=false` + `not implemented` | Concierge cannot remediate integration issues | 9A |
| G2 | README §10/§11: user approvals + diff review + commit workflow | No runtime git diff approval/accept/reject/commit path in orchestration | Unsafe/non-audited changes; no product commit trail | 9B |
| G3 | README §10: agent collaboration for focused objectives | No `AgentRunner` implementation or executor integration | Complex integration fixes cannot be delegated | 9C |
| G4 | README §6.2: persistent mutable state (`.concierge/state.json`) | Paths exist but state is never read/written | No cross-iteration memory or invalidation semantics | 7A |
| G5 | README §6.1/§8.2: richer snapshot/inspection coverage | Snapshot + inspector only cover baseline files and git identity | Missing critical diagnostics (model/include-exclude/runtime/auth/server) | 8A |
| G6 | README §8 planner semantics: deterministic next primary action with gate-aware ordering | Planner only maps current issue codes; no gate-aware policy for mutation/push readiness | Suboptimal action ordering and completion semantics | 8B |
| G7 | README §9: runtime harness is core validation layer | Harness is optional via env and currently stub-script-based | No high-resolution runtime correctness evidence in normal runs | 10A |
| G8 | README §9.2: integration-test call coverage enforcement | No AST/runtime enforcement of required decorator calls in integration test | False positives possible despite incomplete wiring | 10B |
| G9 | README §12: real `leap` workflow + upload gating + explicit confirmation | `concierge run` has no `leap` orchestration path and no push gate | Cannot complete actual Tensorleap integration flow | 10C |
| G10 | README §13.4: fixture behavior fingerprints, not only file presence | Fixture E2E checks only artifact-missing deltas and persistence files | Insufficient confidence for semantic correctness | 11B |
| G11 | PLAN Step 7: lint/test/build gates | `make lint` and lint config missing; CI lacks lint job | Tooling quality gate gap | 11C |
| G12 | PLAN Step 8 / README docs requirements | `docs/` package absent; README implementation status stale | Onboarding and operational guidance drift | 12A |
| G13 | User request: CI against fixture repos | CI prepares/verifies fixtures but does not run fixture E2E suite | No automated proof of repo-level behavior | 11C |

## Progress

| Item | Status | Updated | Scope |
| --- | --- | --- | --- |
| Plan tracking bootstrap | `DONE` | 2026-02-25 | Established `PLAN.md` as the cross-session source of truth. |
| Step 1: Baseline cleanup | `ACCEPTED` | 2026-02-25 (`main`) | Removed inherited workflows, initialized Go baseline CI. |
| Step 2: Cobra bootstrap + release automation | `ACCEPTED` | 2026-02-25 (`main`) | Added core CLI commands and semver release pipeline (linux/macos amd64+arm64). |
| Step 3: Core deterministic contracts | `ACCEPTED` | 2026-02-25 (`main`) | Added core types, errors, ports, issue catalog, and issue->step mapping. |
| Step 4A: Iteration engine skeleton | `ACCEPTED` | 2026-02-25 (`main`) | Added strict stage-order orchestrator and stage error handling. |
| Step 4B: Dry-run stage metadata wiring | `ACCEPTED` | 2026-02-25 (`main`) | `run --dry-run` now renders canonical stage list from core metadata. |
| Step 4C: Multi-iteration loop | `ACCEPTED` | 2026-02-25 (`main`) | Added max-iteration run loop and deterministic stop reasons. |
| Step 5A: Snapshot adapter baseline | `ACCEPTED` | 2026-02-25 (`main`) | Implemented git identity + worktree fingerprint snapshot. |
| Step 5B: Inspector adapter baseline | `ACCEPTED` | 2026-02-25 (`main`) | Implemented required artifact inventory + minimal `leap.yaml` entry validation. |
| Step 5C: Planner adapter baseline | `ACCEPTED` | 2026-02-25 (`main`) | Deterministic issue->step primary/additional planning and `ensure.complete`. |
| Step 5D: Executor adapter skeleton | `ACCEPTED` | 2026-02-25 (`main`) | Added non-mutating stub executor with deterministic evidence stub. |
| Step 5E: Validator baseline | `ACCEPTED` | 2026-02-25 (`main`) | Added deterministic baseline validator flow and issue aggregation. |
| Step 5F: Reporter baseline | `ACCEPTED` | 2026-02-25 (`main`) | Added one-line summary reporter. |
| Step 5G: Fixture corpus preparation | `ACCEPTED` | 2026-02-25 (`main`) | Added fixture manifest + prepare/verify scripts. |
| Step 5H: CLI non-dry-run pipeline wiring | `ACCEPTED` | 2026-02-26 (`main`) | Wired default adapters into engine loop with `--max-iterations`. |
| Step 6A: Persistence primitives | `ACCEPTED` | 2026-02-26 (`main`) | Added atomic JSON writer + `.concierge` path helpers. |
| Step 6B: Persist reports and evidence | `ACCEPTED` | 2026-02-26 (`main`) | Added file reporter and CLI `--persist` path. |
| Step 6C: Harness invocation baseline | `ACCEPTED` | 2026-02-26 (`main`) | Added env-gated harness runner/parser contract and tests. |
| Step 6D: Anti-stub heuristics baseline | `ACCEPTED` | 2026-02-26 (`main`) | Added deterministic heuristic issue derivation from harness events. |
| Step 6E: Fixture pre-vs-post baseline tests | `ACCEPTED` | 2026-02-26 (`main`) | Added fixture delta/planner/persistence E2E assertions. |
| Step 7A: Persistent run state + invalidation engine | `PENDING` | — | Implement `.concierge/state/state.json` lifecycle and invalidation reasons across iterations. |
| Step 7B: Interactive run session UX (project-root + approvals) | `PENDING` | — | Add prompt-driven flow and non-interactive guard rails for deterministic execution. |
| Step 8A: Snapshot/Inspector expansion to readiness checks | `PENDING` | — | Extend snapshot/inspect to cover env, include/exclude, model, CLI/auth/server findings. |
| Step 8B: Planner policy hardening and gate-aware ordering | `PENDING` | — | Deterministic priority rules aware of blocker severity and push/mutation gates. |
| Step 9A: Deterministic executor implementations (non-agent) | `PENDING` | — | Implement real ensure-step mutations for scaffoldable contracts. |
| Step 9B: GitManager runtime integration (diff/approve/reject/commit) | `PENDING` | — | Add audited branch-safe change-control loop in product runtime. |
| Step 9C: AgentRunner integration for complex ensure-steps | `PENDING` | — | Add task-scoped coding-agent collaboration path and transcript evidence. |
| Step 10A: Real runtime harness (Layer 2) | `PENDING` | — | Replace stub with executable Python harness producing semantic coverage evidence. |
| Step 10B: Integration-test wiring checks + advanced anti-stub rules | `PENDING` | — | Enforce decorator call coverage and strengthen heuristics against skeleton integrations. |
| Step 10C: Upload readiness + guarded `leap push` execution | `PENDING` | — | Implement full Tensorleap CLI readiness and explicit push confirmation path. |
| Step 11A: Fixture governance hardening | `PENDING` | — | Encode fixture suitability gate and manifest validation automation. |
| Step 11B: Fixture behavior-fingerprint E2E suite | `PENDING` | — | Assert pre/post semantic deltas using harness-derived fingerprints, not only missing files. |
| Step 11C: CI expansion with fixture E2E execution | `PENDING` | — | Add lint/test/build + fixture E2E CI jobs with stable retry/timeouts. |
| Step 12A: Documentation sync (README + docs/*) | `PENDING` | — | Align all docs with implemented behavior, trust model, and fixture workflows. |
| Step 12B: Release-readiness hardening | `PENDING` | — | Final polish: deterministic defaults, failure ergonomics, and final acceptance checklist. |

## Scope Boundaries For Remaining Work

In scope:

1. End-to-end operational run loop from detect->fix->validate->report->commit->optional push.
2. Deterministic non-agent fixes plus agent-assisted complex fixes.
3. Fixture-backed semantic validation and CI automation.
4. Documentation required for maintainers and operators.

Out of scope for this release train:

1. Managed cloud brokerage for coding-agent credentials.
2. Full backend enforcement sandbox outside local trust model.
3. Automatic Tensorleap server installation/provisioning.

## Detailed Step Specifications (Active Pending Steps)

### Step 7A: Persistent run state + invalidation engine (`PENDING`)

Objective:

Implement durable run state in `.concierge/state/state.json` and deterministic invalidation rules when dependencies change.

Files to add:

1. `internal/state/store.go`
2. `internal/state/types.go`
3. `internal/state/store_test.go`

Files to modify:

1. `internal/persistence/paths.go` (if additional deterministic directories are needed)
2. `internal/orchestrator/run.go`
3. `internal/cli/run.go`

Interface/API changes:

1. `type RunState struct { Version int; SelectedProjectRoot string; LastSnapshotID string; LastHead string; LastWorktreeFingerprint string; LastPrimaryStep core.EnsureStepID; LastRunAt time.Time; InvalidationReasons []string }`
2. `LoadState(projectRoot string) (RunState, error)` and `SaveState(projectRoot string, state RunState) error`.

Implementation tasks:

1. Load state at run start; initialize defaults if missing.
2. Compute invalidation reasons based on snapshot deltas (HEAD/worktree/root change).
3. Persist state after each iteration with atomic write.
4. Record invalidation reasons in report notes/evidence.

Tests:

1. `TestLoadStateReturnsDefaultWhenMissing`
2. `TestSaveStateAtomicRoundTrip`
3. `TestInvalidationReasonsOnHeadAndWorktreeChange`
4. `TestStatePersistsAcrossMultipleIterations`

Validation commands:

1. `go test ./internal/state ./internal/orchestrator ./internal/cli`
2. `go test ./...`

Acceptance criteria:

1. `.concierge/state/state.json` is created and updated on run.
2. Invalidation reasons are deterministic and observable.

Rollback boundary:

- Revert only `internal/state/*` and direct wiring changes in `internal/cli/run.go` / `internal/orchestrator/run.go`.

---

### Step 7B: Interactive run session UX (project-root + approvals) (`PENDING`)

Objective:

Add interactive gates required by README UX principles: project-root selection, mutation confirmation, and non-interactive safeguards.

Files to add:

1. `internal/cli/prompt.go`
2. `internal/cli/project_root_select.go`
3. `internal/cli/prompt_test.go`

Files to modify:

1. `internal/cli/run.go`
2. `internal/cli/run_test.go`

Interface/API changes:

1. New flags:
   1. `--project-root <path>`
   2. `--non-interactive`
   3. `--yes` (auto-approve mutation/push prompts in interactive-equivalent mode)

Locked behavior:

1. If `--project-root` is unset, detect candidates and prompt user to choose.
2. If mutations are required and `--non-interactive` without `--yes`, fail with actionable error.
3. Prompt text and decisions are recorded in iteration notes.

Tests:

1. `TestRunPromptsForProjectRootWhenAmbiguous`
2. `TestRunNonInteractiveFailsWithoutApprovalOverride`
3. `TestRunYesSkipsApprovalPrompts`

Validation commands:

1. `go test ./internal/cli`
2. `go run ./cmd/concierge run --dry-run`

Acceptance criteria:

1. Project root and approval behavior is deterministic and test-covered.
2. No hidden mutation path exists without explicit approval semantics.

Rollback boundary:

- Revert `internal/cli` prompt/root-selection additions only.

---

### Step 8A: Snapshot/Inspector expansion to readiness checks (`PENDING`)

Objective:

Expand snapshot and inspector beyond artifact existence to cover operational readiness checks defined in README §§6,8,12.

Files to add:

1. `internal/adapters/inspect/leap_yaml_contract.go`
2. `internal/adapters/inspect/model_contract.go`
3. `internal/adapters/inspect/runtime_contract.go`
4. `internal/adapters/inspect/leap_cli_contract.go`

Files to modify:

1. `internal/core/types.go`
2. `internal/core/issues.go`
3. `internal/adapters/snapshot/git_snapshotter.go`
4. `internal/adapters/inspect/baseline_inspector.go`
5. `internal/adapters/inspect/baseline_inspector_test.go`

Interface/API changes:

1. Extend `WorkspaceSnapshot` with optional environment/tool fingerprints (python/leap versions, key file hashes).
2. Add missing issue codes only if not already present in `core/issues.go`.

Locked checks:

1. Validate `leap.yaml` include/exclude contract for required files.
2. Validate `entryFile` is inside repo and not excluded.
3. Validate model path + `.onnx`/`.h5` format where model can be resolved.
4. Inspect runtime prerequisites (python presence/version, requirements file detection).
5. Inspect `leap` availability/auth/server-info reachability (non-destructive probes).

Tests:

1. `TestInspectorDetectsEntryFileExcludedByLeapYAML`
2. `TestInspectorDetectsUnsupportedModelFormat`
3. `TestInspectorDetectsMissingLeapCLI`
4. `TestInspectorDetectsServerInfoFailures`
5. `TestSnapshotIncludesEnvironmentFingerprints`

Validation commands:

1. `go test ./internal/adapters/snapshot ./internal/adapters/inspect`
2. `go test ./...`

Acceptance criteria:

1. Inspector emits deterministic issue set for readiness blockers beyond missing files.
2. Planner inputs now include CLI/server/model/runtime signals.

Rollback boundary:

- Revert snapshot/inspector expansions and newly added issue-code mappings for this step.

---

### Step 8B: Planner policy hardening and gate-aware ordering (`PENDING`)

Objective:

Upgrade planner selection policy to prioritize blockers deterministically and preserve push/mutation gate semantics.

Files to add:

1. `internal/adapters/planner/policy.go`
2. `internal/adapters/planner/policy_test.go`

Files to modify:

1. `internal/adapters/planner/deterministic_planner.go`
2. `internal/core/issue_step_map.go`
3. `internal/core/ensure_steps.go`

Locked behavior:

1. Primary step selection honors blocker severity (`error` > `warning` > `info`) then canonical step priority.
2. Planner must not select `ensure.upload_push` unless upload-readiness issues are clear.
3. `ensure.complete` is returned only when no blocking issues remain.

Tests:

1. `TestPlannerPrioritizesErrorSeverity`
2. `TestPlannerDefersUploadPushUntilReadinessClear`
3. `TestPlannerReturnsCompleteOnlyWhenNoBlockingIssues`

Validation commands:

1. `go test ./internal/adapters/planner ./internal/core`
2. `go test ./...`

Acceptance criteria:

1. Planner decisions are deterministic and policy-explicit.
2. Push step cannot be selected prematurely.

Rollback boundary:

- Revert planner policy files and mapping changes only.

---

### Step 9A: Deterministic executor implementations (non-agent) (`PENDING`)

Objective:

Replace stub-only execution with deterministic, idempotent implementations for scaffoldable ensure-steps.

Files to add:

1. `internal/adapters/execute/filesystem_executor.go`
2. `internal/adapters/execute/templates/leap_yaml.tmpl`
3. `internal/adapters/execute/templates/leap_binder.py.tmpl`
4. `internal/adapters/execute/templates/leap_custom_test.py.tmpl`
5. `internal/adapters/execute/filesystem_executor_test.go`

Files to modify:

1. `internal/adapters/execute/stub_executor.go` (split into dispatcher + fallback)
2. `internal/cli/run.go` (wire configurable executor mode)

Locked behavior:

1. Implement concrete actions for:
   1. `ensure.leap_yaml`
   2. `ensure.integration_script`
   3. `ensure.integration_test_contract`
2. Actions must be idempotent and include evidence files with before/after checksums.
3. Unknown or unsupported step IDs still return typed `KindStepNotApplicable`.

Tests:

1. `TestExecutorCreatesLeapYAMLWhenMissing`
2. `TestExecutorCreatesIntegrationScriptTemplate`
3. `TestExecutorCreatesIntegrationTestTemplate`
4. `TestExecutorIdempotentOnSecondRun`

Validation commands:

1. `go test ./internal/adapters/execute`
2. `go test ./...`

Acceptance criteria:

1. At least three ensure-steps perform real repo-local fixes.
2. Execution evidence proves applied mutations.

Rollback boundary:

- Revert execute adapter/template additions only.

---

### Step 9B: GitManager runtime integration (diff/approve/reject/commit) (`PENDING`)

Objective:

Add product-grade branch safety and audited commit flow for approved changes.

Files to add:

1. `internal/gitmanager/manager.go`
2. `internal/gitmanager/manager_test.go`
3. `internal/gitmanager/messages.go`

Files to modify:

1. `internal/orchestrator/engine.go`
2. `internal/cli/run.go`
3. `internal/core/types.go` (add commit metadata in report if needed)

Locked behavior:

1. Never commit on `main`/`master`.
2. Show diff summary before commit and require explicit approval.
3. Reject path restores pre-step working tree state.
4. Approved path commits with structured message: `concierge(<step-id>): <summary>`.

Tests:

1. `TestGitManagerRejectsMainBranchCommit`
2. `TestGitManagerApproveCreatesCommit`
3. `TestGitManagerRejectRestoresTree`
4. `TestRunFlowPromptsBeforeCommit`

Validation commands:

1. `go test ./internal/gitmanager ./internal/orchestrator ./internal/cli`
2. `go test ./...`

Acceptance criteria:

1. Repo mutation path is auditable with explicit approval and commit hash evidence.
2. Reject path is deterministic and safe.

Rollback boundary:

- Revert `internal/gitmanager/*` and direct orchestrator/CLI wiring for commit flow.

---

### Step 9C: AgentRunner integration for complex ensure-steps (`PENDING`)

Objective:

Add task-scoped coding-agent delegation for steps that cannot be safely templated.

Files to add:

1. `internal/agent/runner.go`
2. `internal/agent/types.go`
3. `internal/agent/runner_test.go`
4. `internal/adapters/execute/agent_executor.go`

Files to modify:

1. `internal/adapters/execute/filesystem_executor.go` (dispatcher fallback to agent runner)
2. `internal/cli/run.go` (feature-flag/availability checks)

Interface/API changes:

1. `type AgentTask struct { Objective string; Constraints []string; RepoRoot string }`
2. `type AgentResult struct { Applied bool; TranscriptPath string; Summary string; Evidence []core.EvidenceItem }`

Locked behavior:

1. One objective per session.
2. Capture transcript path under `.concierge/evidence/<snapshot-id>/agent.transcript.log`.
3. Failure to invoke agent returns deterministic issue and does not silently pass.

Tests:

1. `TestAgentExecutorDispatchesSupportedSteps`
2. `TestAgentExecutorReturnsDeterministicErrorWhenUnavailable`
3. `TestAgentTranscriptPersistedAsEvidence`

Validation commands:

1. `go test ./internal/agent ./internal/adapters/execute`
2. `go test ./...`

Acceptance criteria:

1. Executor can delegate complex steps with deterministic reporting.
2. Agent evidence is persisted and reviewable.

Rollback boundary:

- Revert `internal/agent/*` and agent-executor wiring only.

---

### Step 10A: Real runtime harness (Layer 2) (`PENDING`)

Objective:

Implement a real Python harness that produces semantic validation signals (preprocess subset checks, encoder execution coverage, shape/dtype/finite checks).

Files to add:

1. `scripts/harness_runtime.py`
2. `scripts/harness_lib/__init__.py`
3. `scripts/harness_lib/runner.py`
4. `scripts/harness_lib/events.py`
5. `internal/adapters/validate/harness_runtime_test.go`

Files to modify:

1. `internal/adapters/validate/harness_runner.go`
2. `internal/adapters/validate/harness_parser.go`
3. `internal/adapters/validate/baseline_validator.go`

Locked behavior:

1. Keep timeout default at 120s with CLI override.
2. Emit NDJSON event schema version field (e.g., `schemaVersion: 1`).
3. Map runtime failures to existing issue codes (`harness_preprocess_failed`, `harness_encoder_coverage_incomplete`, `harness_validation_failed`).
4. Harness is enabled by default for non-dry-run unless explicitly disabled (`--disable-harness`).

Tests:

1. `TestHarnessRunnerInvokesRuntimeScriptByDefault`
2. `TestHarnessParserAcceptsSchemaV1Events`
3. `TestValidatorFailsWhenHarnessReportsErrors`
4. `TestValidatorPassesOnCleanHarnessRun`

Validation commands:

1. `go test ./internal/adapters/validate`
2. `go test ./...`

Acceptance criteria:

1. Validator consumes real runtime evidence, not stub-only events.
2. Harness failures deterministically fail validation.

Rollback boundary:

- Revert harness runtime additions and validator wiring for this step.

---

### Step 10B: Integration-test wiring checks + advanced anti-stub rules (`PENDING`)

Objective:

Enforce integration-test decorator call contract and strengthen anti-stub detection rules.

Files to add:

1. `internal/adapters/validate/integration_test_ast.go`
2. `internal/adapters/validate/integration_test_ast_test.go`
3. `internal/adapters/validate/heuristics_advanced.go`
4. `internal/adapters/validate/heuristics_advanced_test.go`

Files to modify:

1. `internal/adapters/validate/heuristics.go`
2. `internal/adapters/validate/baseline_validator.go`

Locked behavior:

1. Detect decorators defined but not called in `@tensorleap_integration_test` path.
2. Emit deterministic issue codes for missing required calls.
3. Add prediction-variation heuristic (`IssueCodeSuspiciousConstantPredictions`) when events exist.

Tests:

1. `TestIntegrationTestAnalyzerDetectsMissingDecoratorCalls`
2. `TestHeuristicsDetectConstantPredictions`
3. `TestHeuristicsNoFalsePositiveOnVaryingPredictions`

Validation commands:

1. `go test ./internal/adapters/validate`
2. `go test ./...`

Acceptance criteria:

1. Integration-test coverage is enforced as a first-class correctness dimension.
2. Heuristic suite catches common skeleton-pattern failures.

Rollback boundary:

- Revert integration-test analyzer and advanced heuristic files only.

---

### Step 10C: Upload readiness + guarded `leap push` execution (`PENDING`)

Objective:

Implement operational Tensorleap CLI flow with explicit upload confirmation.

Files to add:

1. `internal/leap/runner.go`
2. `internal/leap/runner_test.go`
3. `internal/adapters/execute/upload_executor.go`

Files to modify:

1. `internal/cli/run.go`
2. `internal/adapters/inspect/leap_cli_contract.go`
3. `internal/adapters/execute/filesystem_executor.go` (dispatcher)

Locked behavior:

1. Upload push is never implicit.
2. `ensure.upload_readiness` runs non-destructive probes (`leap version`, auth check, `leap server info`).
3. `ensure.upload_push` requires explicit user confirmation and `--allow-push` (or equivalent affirmative flag).
4. Capture command stdout/stderr as evidence logs.

Tests:

1. `TestUploadExecutorRefusesPushWithoutApproval`
2. `TestUploadExecutorRunsReadinessChecksBeforePush`
3. `TestUploadExecutorPersistsCommandEvidence`

Validation commands:

1. `go test ./internal/leap ./internal/adapters/execute ./internal/cli`
2. `go test ./...`

Acceptance criteria:

1. Concierge can run readiness checks and guarded push flow in real environments.
2. All push attempts are auditable from evidence artifacts.

Rollback boundary:

- Revert leap runner/upload executor and related CLI wiring for this step.

---

### Step 11A: Fixture governance hardening (`PENDING`)

Objective:

Automate fixture suitability gate and manifest validation before adding new fixture entries.

Files to add:

1. `scripts/fixtures_check_commit.sh`
2. `scripts/fixtures_manifest_lint.sh`
3. `fixtures/SCHEMA.md`

Files to modify:

1. `scripts/fixtures_prepare.sh`
2. `scripts/fixtures_verify.sh`
3. `fixtures/README.md`

Locked behavior:

1. Validate `post_ref` is a commit SHA and contains required integration files.
2. Fail fast when manifest entries violate schema or suitability gate.
3. Keep scripts idempotent.

Tests:

1. Script-level tests via shell assertions in CI (or Go test wrappers under `internal/e2e/fixtures`).

Validation commands:

1. `bash scripts/fixtures_manifest_lint.sh`
2. `bash scripts/fixtures_prepare.sh`
3. `bash scripts/fixtures_verify.sh`

Acceptance criteria:

1. Fixture manifest updates are mechanically validated before E2E runs.

Rollback boundary:

- Revert fixture governance scripts/docs only.

---

### Step 11B: Fixture behavior-fingerprint E2E suite (`PENDING`)

Objective:

Upgrade fixture E2E from artifact existence checks to semantic behavior fingerprints.

Files to add:

1. `internal/e2e/fixtures/fingerprint_test.go`
2. `internal/e2e/fixtures/testdata/fingerprints/README.md`
3. `internal/e2e/fixtures/testdata/fingerprints/<fixture-id>.golden.json` (initial set for locked fixtures)

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Pre fixtures: must fail readiness/harness with expected blocking issue families.
2. Post fixtures: must clear baseline artifact issues and produce non-empty harness events.
3. Behavior fingerprint deltas (subset counts/encoder coverage/fingerprint variation) must match expected invariants.

Tests:

1. `TestFixtureBehaviorFingerprintsPreVsPost`
2. `TestFixtureHarnessCoverageThresholds`
3. `TestFixtureGoldenFingerprintStability`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -v`

Acceptance criteria:

1. Fixture suite validates semantic behavior differences, not just file presence.
2. Failures provide actionable diff output.

Rollback boundary:

- Revert fixture E2E test and golden data additions only.

---

### Step 11C: CI expansion with fixture E2E execution (`PENDING`)

Objective:

Make CI enforce full quality gates and run fixture repo tests automatically.

Files to add:

1. `.golangci.yml`

Files to modify:

1. `Makefile`
2. `.github/workflows/ci.yml`
3. `.github/workflows/release.yml` (only if needed to preserve compatibility)

Locked CI behavior:

1. Required on PR/push:
   1. `make lint`
   2. `make test`
   3. binary build matrix (linux/macos amd64+arm64)
   4. fixture E2E job (`CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`)
2. Fixture job includes retry and timeout budget.
3. Release workflow remains semver tag-gated.

Make targets (required):

1. `lint`
2. `test`
3. `test-fixtures`
4. `build`

Validation commands:

1. `make lint`
2. `make test`
3. `make test-fixtures`
4. `make build`

Acceptance criteria:

1. CI fails on lint/test/build/fixture regressions.
2. Fixture repos are validated in automation, not only locally.

Rollback boundary:

- Revert CI/lint/makefile changes only.

---

### Step 12A: Documentation sync (README + docs/*) (`PENDING`)

Objective:

Align docs with implemented operational behavior.

Files to add:

1. `docs/architecture.md`
2. `docs/dev-setup.md`
3. `docs/fixtures.md`
4. `docs/operations.md`

Files to modify:

1. `README.md`
2. `fixtures/README.md`

Locked documentation requirements:

1. Update README implementation status to current date and actual feature state.
2. Document full `concierge run` flow including approvals, harness, and upload gates.
3. Document fixture lifecycle and CI expectations.
4. Document trust boundary and security posture (agent + command execution + secrets redaction).

Validation commands:

1. Execute every documented command path or mark as example-only.
2. `go test ./...` after documentation edits.

Acceptance criteria:

1. New contributor can execute setup + fixture workflow using docs only.
2. Docs no longer claim capabilities that are not implemented.

Rollback boundary:

- Revert docs/README changes only.

---

### Step 12B: Release-readiness hardening (`PENDING`)

Objective:

Finalize operational polish and lock phase acceptance criteria before first "fully operational" release.

Files to add or modify:

1. `internal/cli/*` (error UX and exit-code consistency)
2. `internal/core/*` (if error-code normalization is needed)
3. `README.md` (release checklist section)
4. `PLAN.md` (mark completed statuses)

Locked tasks:

1. Normalize user-facing failure messages for common blockers.
2. Ensure every ensure-step emits actionable evidence.
3. Add final acceptance checklist command block.

Final acceptance checklist commands:

1. `go test ./...`
2. `make lint`
3. `make build`
4. `CONCIERGE_RUN_FIXTURE_E2E=1 make test-fixtures`
5. Manual smoke: `go run ./cmd/concierge run --max-iterations=3 --persist`

Acceptance criteria:

1. All steps `7A-12B` are `ACCEPTED` after merge to `main`.
2. CI is green with fixture E2E job included.
3. README + docs reflect released functionality.

Rollback boundary:

- Revert only hardening/doc updates introduced by this step.

## Concrete Step Execution Order

1. Implement Step 7A.
2. Implement Step 7B.
3. Implement Step 8A.
4. Implement Step 8B.
5. Implement Step 9A.
6. Implement Step 9B.
7. Implement Step 9C.
8. Implement Step 10A.
9. Implement Step 10B.
10. Implement Step 10C.
11. Implement Step 11A.
12. Implement Step 11B.
13. Implement Step 11C.
14. Implement Step 12A.
15. Implement Step 12B.

Each step follows this gate:

1. Implement step scope only.
2. Run step validation commands.
3. Stop for explicit user local-review approval before commit/push.
4. Commit/push PR branch and monitor/fix CI for that step scope.
5. Mark step `DONE` only after commit/push/PR/green branch CI.
6. Mark step `ACCEPTED` only after merge to `main`.

## Validation and Acceptance

Status semantics:

1. `PENDING`: step not implemented yet.
2. `DONE`: implemented, committed, pushed, PR opened, branch CI green.
3. `ACCEPTED`: merged to `main`.

Phase acceptance condition:

1. Steps `1` through `12B` are `ACCEPTED`.
2. Fixture E2E job is part of required CI and green.
3. Concierge can perform user-approved fixes and guarded upload workflow end-to-end.

## Idempotence and Recovery

1. Keep one commit per step for clean rollback.
2. If CI fails, fix within current step scope only.
3. Revert only the failing step commit when rollback is required.
4. Fixture scripts and `.concierge` persistence paths must remain safe to rerun.

## Surprises & Discoveries

- Current codebase is functionally coherent but still a scaffold: executor is stub-based, harness is optional/stub-oriented, and upload flow is not operational.
- Fixture scripts are stable and deterministic locally; they can support stronger E2E checks with semantic fingerprints.
- Existing issue catalog and ensure-step map are broad enough to support operationalization without major taxonomy redesign.

## Decision Log

- Decision: Re-baseline remaining work into granular operational steps (`7A-12B`) instead of keeping the old coarse `Step 7` and `Step 8`.
  Rationale: The old tail was too coarse for safe iterative delivery and missed major README requirements.
  Date/Author: 2026-02-26 / assistant.
- Decision: Keep fixture E2E in required CI (not only scheduled/manual).
  Rationale: User explicitly requested CI validation against fixture repos.
  Date/Author: 2026-02-26 / user + assistant.
- Decision: Implement deterministic executor coverage before agent integration.
  Rationale: Preserve deterministic baseline and reduce dependence on agent availability for core flows.
  Date/Author: 2026-02-26 / assistant.

## Outcomes & Retrospective

Current state after re-baseline: baseline architecture through Step 6E is accepted; operational completion work is explicitly decomposed and actionable.

Primary residual risk: runtime harness implementation may surface environment-specific assumptions across fixture repos.

Mitigation: introduce harness behavior incrementally (10A then 10B), and keep fixture E2E deterministic and CI-enforced (11B/11C).

## Interfaces and Dependencies

1. Language/runtime: Go `1.24.x`.
2. CLI framework: Cobra.
3. Core contracts: `internal/core`, `internal/core/ports`.
4. Runtime dependencies for operational steps:
   1. `git`
   2. `python3`
   3. `jq`
   4. `leap` CLI (for readiness/upload steps)
5. Planned package additions:
   1. `internal/state`
   2. `internal/gitmanager`
   3. `internal/agent`
   4. `internal/leap`
