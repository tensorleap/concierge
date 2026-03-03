# Concierge Detailed Implementation Plan (Execution-Ready)

This ExecPlan is a living document. Keep `Progress`, `Gap Analysis`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Revision Notes

- 2026-02-26: Merged `feature/step-doctor-ui-polish` to `main`; accepted Steps `7A`, `7B`, `8A`, `8B`, `9A`, and `9B`.
- 2026-02-26: Re-baselined remaining work after a full README/PLAN/code audit. Added explicit gap analysis and replaced the old two-step tail (`Step 7`, `Step 8`) with a detailed operational completion plan (`Steps 7A-12B`).
- 2026-02-26: Verified current baseline is green locally: `go test ./...`, `bash scripts/fixtures_prepare.sh`, `bash scripts/fixtures_verify.sh`, `bash scripts/fixtures_run_checks.sh`.
- 2026-02-26: Confirmed current implementation is still scaffold-first (stub executor, optional harness gate, no real `leap push` path, no git approval/commit loop in product runtime).
- 2026-03-03: Locked v1 product scope to mandatory onboarding contracts only. Optional asset assistance (metadata/visualizers/metrics/loss/custom layers) is deferred to v2 and removed from v1 ensure-step taxonomy.
- 2026-03-03: Step `9C` was merged to `main` and is now `ACCEPTED`. Replaced the remaining tail with an authoring-first sequence (`10A0-14B`) that breaks model, preprocess, input-encoder, GT-encoder, and integration-test wiring into independent detection + authoring + fixture-E2E validation steps.
- 2026-03-03: Integrated `VALIDATION.md` as Step `10B1` (delta-scoped pre-commit integration quality gate) and wired it into gap analysis, progress tracking, execution order, and phase acceptance criteria.
- 2026-03-03: Completed Step `10A0` plan synchronization by marking it `ACCEPTED` and aligning execution-order metadata with the already-merged authoring-first sequence.

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
| G1 | README §8/§10: Ensure-step `Fix` must apply real actions | Implemented deterministic filesystem mutations for scaffoldable steps; non-supported steps still require dedicated authoring slices | Closed for scaffold scope; remaining authoring capabilities handled in `10C-10K` | 9A (`ACCEPTED`) |
| G2 | README §10/§11: user approvals + diff review + commit workflow | Implemented runtime git diff approval/reject/commit flow | Closed; retain regression coverage | 9B (`ACCEPTED`) |
| G3 | README §10: agent collaboration for focused objectives | Implemented and merged (`internal/agent/*`, agent executor dispatch, transcript evidence) | Closed; maintain with regression tests only | 9C (`ACCEPTED`) |
| G4 | README §6.2: persistent mutable state (`.concierge/state.json`) | Implemented and persisted with invalidation reasons | Closed; retain regression coverage | 7A (`ACCEPTED`) |
| G5 | README §6.1/§8.2: richer snapshot/inspection coverage | Implemented readiness expansion for runtime/model/CLI/auth/server probes | Partially closed; remaining contract-level detection gaps tracked in `10A-10J` | 8A (`ACCEPTED`), 10A-10J |
| G6 | README §8 planner semantics: deterministic next primary action with gate-aware ordering | Implemented severity-first planner policy with upload gate awareness | Closed for existing issue families; future families added in authoring slices | 8B (`ACCEPTED`) |
| G7 | README §9 + user requirement: preprocess authoring must be detection-driven and individually tested | No inspector/validator emission path currently produces preprocess-specific issue codes from real repo analysis | Planner cannot deterministically trigger preprocess authoring from real deficits | 10D, 10E, 12C |
| G8 | README §9 + user requirement: input-encoder authoring must be detection-driven and individually tested | No deterministic detector currently emits missing/coverage issues for specific input encoders from contract discovery | Concierge cannot reliably suggest missing input encoders before authoring | 10F, 10G, 12D |
| G9 | README §9 + user requirement: GT-encoder authoring must be detection-driven and individually tested | No deterministic detector currently emits GT-specific deficits from discovered integration contracts | GT authoring remains under-specified and under-tested | 10H, 10I, 12E |
| G10 | README §2.3/§8.2 model contract + user requirement: model discovery/selection/fixing must be explicit and tested | Inspector validates declared model path but lacks authoring-first model discovery strategy for missing/ambiguous model wiring | Model contract failures can remain opaque and difficult to fix deterministically | 10B, 10C, 12B |
| G11 | README §9: runtime harness is core validation layer | Harness is optional via env and currently stub-script-based | No high-resolution runtime correctness evidence in normal runs | 11A, 11B |
| G12 | README §9.2: integration-test call coverage enforcement | No AST/runtime enforcement of required decorator calls in integration test | False positives possible despite incomplete wiring | 10J, 10K, 12F |
| G13 | README §12: real `leap` workflow + upload gating + explicit confirmation | `concierge run` has no `leap` orchestration path and no push gate | Cannot complete actual Tensorleap integration flow | 13A |
| G14 | README §13.4 + user request: capability-level fixture validation | Fixture E2E checks only artifact-missing deltas and persistence files | No proof that each authoring capability converges independently | 12A-12G |
| G15 | PLAN quality gate requirement + user request: CI must run capability-level fixture E2E | CI prepares/verifies fixtures but does not run authoring-focused fixture suite on PRs | Regressions can merge without capability-level safety net | 13B |
| G16 | README docs requirements + operator handoff | README/docs do not yet describe detection->suggest->author->validate authoring loop by capability | Onboarding and operations remain ambiguous for the core concierge value proposition | 14A |
| G17 | V1 quality gate requirement: commit approval must run after delta-scoped integration checks | Engine currently calls git commit flow before validator result and has no step-local syntax gate over changed files | Broken step-local integration edits can be committed and approved too early | 10B1 |

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
| Step 7A: Persistent run state + invalidation engine | `ACCEPTED` | 2026-02-26 (`main`) | Implemented `.concierge/state/state.json` lifecycle and deterministic invalidation reasons across iterations. |
| Step 7B: Interactive run session UX (project-root + approvals) | `ACCEPTED` | 2026-02-26 (`main`) | Added prompt-driven flow with project-root selection and explicit approval gating semantics. |
| Step 8A: Snapshot/Inspector expansion to readiness checks | `ACCEPTED` | 2026-02-26 (`main`) | Expanded snapshot/inspect coverage for runtime, model, and CLI/auth/server readiness signals. |
| Step 8B: Planner policy hardening and gate-aware ordering | `ACCEPTED` | 2026-02-26 (`main`) | Added deterministic planner policy with severity ordering and upload gate behavior. |
| Step 9A: Deterministic executor implementations (non-agent) | `ACCEPTED` | 2026-02-26 (`main`) | Implemented deterministic filesystem-backed ensure-step fixes for scaffoldable contracts. |
| Step 9B: GitManager runtime integration (diff/approve/reject/commit) | `ACCEPTED` | 2026-02-26 (`main`) | Added audited branch-safe diff review, approval/reject handling, and commit flow in runtime. |
| Step 9C: AgentRunner integration for complex ensure-steps | `ACCEPTED` | 2026-03-03 (`main`) | Added task-scoped coding-agent delegation with transcript evidence integration in executor flow. |
| Step 10A0: Plan state sync after 9C merge | `ACCEPTED` | 2026-03-03 (`main`) | Synchronized plan tracking after `9C` merge and verified authoring-first tail consistency (`10A-14B`). |
| Step 10A: Contract discovery core | `DONE` | 2026-03-03 (`PR #11`) | Added deterministic entry-file contract discovery for decorators and integration-test call symbols with graceful path-aware failures. |
| Step 10B: Model discovery and need detection | `DONE` | 2026-03-03 (`PR #12`) | Added deterministic model candidate discovery (leap.yaml + load-model analysis + repo search), ambiguity/missing/format/outside-repo issues, and candidate evidence context without enforcing leap.yaml include/exclude for model artifacts. |
| Step 10B1: Pre-commit integration quality gate (delta-scoped) | `PENDING` | — | Run step-local integration validation and changed-file syntax checks before commit approval is offered. |
| Step 10C: Model contract authoring flow | `PENDING` | — | Add model-specific authoring objectives and deterministic recommendation/evidence path. |
| Step 10D: Preprocess need detection | `PENDING` | — | Emit preprocess-specific issue codes from real contract inspection. |
| Step 10E: Preprocess authoring flow | `PENDING` | — | Add preprocess authoring objective context, approvals, and evidence expectations. |
| Step 10F: Input-encoder need detection | `PENDING` | — | Detect missing input encoders and per-symbol coverage gaps. |
| Step 10G: Input-encoder suggestion and authoring flow | `PENDING` | — | Render missing-input suggestions to users and pass symbol-level context to authoring executor. |
| Step 10H: GT-encoder need detection | `PENDING` | — | Detect GT encoder deficits and labeled-subset contract violations. |
| Step 10I: GT-encoder suggestion and authoring flow | `PENDING` | — | Render GT-target suggestions and enforce labeled-subset constraints in authoring tasks. |
| Step 10J: Integration-test wiring need detection | `PENDING` | — | Add AST-based required-call detection for `@tensorleap_integration_test` paths. |
| Step 10K: Integration-test wiring authoring flow | `PENDING` | — | Add targeted authoring objective to wire missing integration-test calls deterministically. |
| Step 11A: Real runtime harness core (Layer 2) | `PENDING` | — | Replace stub harness with runtime script, schema-v1 events, and default-on validation. |
| Step 11B: Harness semantic coverage mapping | `PENDING` | — | Map runtime harness failures to preprocess/encoder/validation issues with per-symbol evidence. |
| Step 12A: Fixture mutation framework for capability-isolated cases | `PENDING` | — | Generate fixture variants with exactly one broken capability per case. |
| Step 12B: Capability E2E (model) | `PENDING` | — | Prove missing-model detection and authoring convergence on fixture cases. |
| Step 12C: Capability E2E (preprocess) | `PENDING` | — | Prove missing-preprocess detection and authoring convergence on fixture cases. |
| Step 12D: Capability E2E (input encoders) | `PENDING` | — | Prove missing-input-encoder detection, suggestion, and authoring convergence on fixtures. |
| Step 12E: Capability E2E (GT encoders) | `PENDING` | — | Prove missing-GT-encoder detection and authoring convergence on fixture cases. |
| Step 12F: Capability E2E (integration-test wiring) | `PENDING` | — | Prove missing required integration-test calls are detected and fixed in fixture flow. |
| Step 12G: Multi-step recovery E2E | `PENDING` | — | Prove deterministic multi-capability convergence across ordered ensure-steps. |
| Step 13A: Upload readiness and guarded `leap push` execution | `PENDING` | — | Implement full readiness probes and explicit push gate with auditable command evidence. |
| Step 13B: CI expansion with capability E2E execution | `PENDING` | — | Add lint/test/build + capability-level fixture E2E as required CI jobs. |
| Step 14A: Documentation sync (README + docs/*) | `PENDING` | — | Document capability-by-capability authoring workflow, trust model, and operational guidance. |
| Step 14B: Release-readiness hardening | `PENDING` | — | Finalize UX/evidence/checklist and lock acceptance criteria for first fully operational release. |

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
4. Optional integration asset assistance (metadata/visualizers/metrics/loss/custom layers), which is deferred to v2.

## Detailed Step Specifications

### Step 7A: Persistent run state + invalidation engine (`ACCEPTED`)

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

### Step 7B: Interactive run session UX (project-root + approvals) (`ACCEPTED`)

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

### Step 8A: Snapshot/Inspector expansion to readiness checks (`ACCEPTED`)

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

### Step 8B: Planner policy hardening and gate-aware ordering (`ACCEPTED`)

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

### Step 9A: Deterministic executor implementations (non-agent) (`ACCEPTED`)

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

### Step 9B: GitManager runtime integration (diff/approve/reject/commit) (`ACCEPTED`)

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

### Step 9C: AgentRunner integration for complex ensure-steps (`ACCEPTED`)

Objective:

Add task-scoped coding-agent delegation for steps that cannot be safely templated.

Files added:

1. `internal/agent/runner.go`
2. `internal/agent/types.go`
3. `internal/agent/runner_test.go`
4. `internal/adapters/execute/agent_executor.go`

Files modified:

1. `internal/adapters/execute/stub_executor.go` (dispatcher fallback to agent runner)
2. `internal/cli/run.go` (feature-flag and availability checks)

Interface/API changes:

1. `type AgentTask struct { Objective string; Constraints []string; RepoRoot string; TranscriptPath string }`
2. `type AgentResult struct { Applied bool; TranscriptPath string; Summary string; Evidence []core.EvidenceItem }`

Delivered behavior:

1. One objective per session.
2. Transcript captured under `.concierge/evidence/<snapshot-id>/agent.transcript.log`.
3. Agent command lookup failure is explicit and deterministic.

Regression tests:

1. `TestAgentExecutorDispatchesSupportedSteps`
2. `TestAgentExecutorReturnsDeterministicErrorWhenUnavailable`
3. `TestAgentTranscriptPersistedAsEvidence`

Validation commands:

1. `go test ./internal/agent ./internal/adapters/execute`
2. `go test ./...`

Acceptance criteria:

1. Executor delegates complex steps with deterministic reporting.
2. Agent evidence is persisted and reviewable.

Rollback boundary:

- Revert `internal/agent/*` and agent-executor wiring only.

---

### Step 10A0: Plan state sync after Step 9C merge (`ACCEPTED`)

Objective:

Reconcile planning metadata with current repo truth before implementing additional code.

Files to modify:

1. `PLAN.md`

Locked behavior:

1. Mark Step `9C` as `ACCEPTED`.
2. Replace remaining pending tail with authoring-first execution plan (`10A-14B`).
3. Ensure every unresolved gap maps to at least one concrete step.

Validation commands:

1. `rg -n "Step 9C|PENDING|ACCEPTED|Concrete Step Execution Order|Phase acceptance condition" PLAN.md`

Acceptance criteria:

1. No stale `PENDING` marker remains for `9C`.
2. Execution order, acceptance criteria, and detailed steps are internally consistent.

Rollback boundary:

- Revert only `PLAN.md` planning edits for this synchronization step.

---

### Step 10A: Contract discovery core (`DONE`)

Objective:

Introduce deterministic contract discovery from integration entry file so planner decisions are driven by discovered interfaces, not only file presence.

Files to add:

1. `internal/adapters/inspect/integration_contract.go`
2. `internal/adapters/inspect/integration_contract_test.go`

Files to modify:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/core/types.go`
3. `internal/core/issues.go` (only if additional code-level issue constants are required)

Interface/API changes:

1. Add `IntegrationContracts` to `core` types with fields:
   1. `EntryFile`
   2. `LoadModelFunctions`
   3. `PreprocessFunctions`
   4. `InputEncoders`
   5. `GroundTruthEncoders`
   6. `IntegrationTestFunctions`
   7. `IntegrationTestCalls`
2. Extend `IntegrationStatus` with optional `Contracts *IntegrationContracts`.

Locked behavior:

1. Parse `leap.yaml` `entryFile`, then parse/inspect entry python module for relevant decorators and function symbols.
2. Discovery errors are deterministic and include file-path context.
3. Discovery supports both `leap_custom_test.py` and `integration_test.py` naming conventions.

Tests:

1. `TestContractDiscoveryFindsDecoratedFunctions`
2. `TestContractDiscoveryCapturesIntegrationTestCalls`
3. `TestContractDiscoveryGracefullyHandlesMissingEntryFile`
4. `TestContractDiscoveryGracefullyHandlesSyntaxErrors`

Validation commands:

1. `go test ./internal/adapters/inspect -run ContractDiscovery -v`
2. `go test ./internal/adapters/inspect ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. `IntegrationStatus` includes machine-readable discovered contracts when possible.
2. Contract discovery failures are surfaced without panics.

Rollback boundary:

- Revert contract discovery files and `core/types.go` contract extensions only.

---

### Step 10B: Model discovery and need detection (`DONE`)

Objective:

Detect model requirements and gaps even when `leap.yaml` model fields are absent or ambiguous.

Files to add:

1. `internal/adapters/inspect/model_discovery.go`
2. `internal/adapters/inspect/model_discovery_test.go`

Files to modify:

1. `internal/adapters/inspect/model_contract.go`
2. `internal/adapters/inspect/baseline_inspector.go`
3. `internal/core/issues.go` (if new deterministic ambiguity issue code is needed)

Locked behavior:

1. Resolve model candidates from:
   1. `leap.yaml` (`modelPath` / `model`)
   2. discovered `@tensorleap_load_model` call sites
   3. deterministic repo search for `.onnx` / `.h5` files
2. Emit deterministic issues for:
   1. no resolvable model
   2. ambiguous model candidates
   3. unsupported extension
   4. path outside repo or excluded by upload boundary
3. Attach candidate list to evidence/recommendation context.

Tests:

1. `TestModelDiscoveryFromLeapYAML`
2. `TestModelDiscoveryFromLoadModelDecorator`
3. `TestModelDiscoveryReportsAmbiguousCandidates`
4. `TestModelDiscoveryRejectsUnsupportedFormat`
5. `TestModelDiscoveryRespectsIncludeExcludeRules`

Validation commands:

1. `go test ./internal/adapters/inspect -run ModelDiscovery -v`
2. `go test ./internal/adapters/inspect ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Model contract needs are detected deterministically across fixture variants.
2. Planner can select `ensure.model_contract` based on real discovery results.

Rollback boundary:

- Revert model discovery additions and linked inspector wiring only.

---

### Step 10B1: Pre-commit integration quality gate (delta-scoped) (`PENDING`)

Objective:

Add a v1 pre-commit gate so commit approval is offered only after delta-scoped integration quality checks pass.

Locked decisions:

1. This step is part of v1 scope.
2. Commit must be offered only after pre-commit checks run.
3. Blocking scope is integration-only, not full-repo lint/test/build.
4. If nice-to-have checker infrastructure fails (for example interpreter/tool missing), that checker is silently skipped.
5. If pre-commit checks fail, keep uncommitted changes in the working tree (no auto-revert).

Scope:

1. Add pre-commit gating to runtime orchestration.
2. Keep CI lint/test/build expansion in Step `13B`.
3. Do not add full-repo blocking gates to `concierge run` in this step.

Non-scope:

1. Full repository lint enforcement in interactive run loop.
2. Mandatory compile/test for arbitrary user languages.
3. Branch/PR workflow semantic changes.

Files to modify:

1. `internal/core/types.go`
2. `internal/core/ports/interfaces.go`
3. `internal/orchestrator/engine.go`
4. `internal/orchestrator/engine_test.go`
5. `internal/gitmanager/manager.go`
6. `internal/gitmanager/manager_test.go`
7. `internal/adapters/report/stdout_reporter.go`
8. `internal/adapters/report/stdout_reporter_test.go` (if output contract assertions require updates)

Interface/API changes:

1. Add `StageCommit` in core stage enum and `DefaultStages()`.
2. Update `ports.GitManager` signature to accept validation context:
   1. From: `Handle(ctx, snapshot, result)`
   2. To: `Handle(ctx, snapshot, result, validation)`
3. Keep CLI flags unchanged for this step (no new user-facing knobs).

Architecture and flow changes:

1. Update orchestration stage order to:
   1. `snapshot`
   2. `inspect`
   3. `plan`
   4. `execute`
   5. `validate`
   6. `commit`
   7. `report`
2. Run validator before commit decision.
3. Run pre-commit gate inside git manager using:
   1. Step-local validation blockers only: `SeverityError` issues whose preferred ensure-step matches current primary step.
   2. Changed-file syntax checks only:
      1. `*.py`: `python3 -m py_compile` (fallback `python`).
      2. `*.yaml` / `*.yml`: parse with Go YAML parser.
4. If pre-commit gate has blocking failures:
   1. Skip commit prompt.
   2. Do not commit.
   3. Keep working tree changes.
   4. Add report notes/evidence stating commit was skipped due to pre-commit failures.
5. If checker infrastructure fails (tool missing/exec failure unrelated to file syntax), silently skip that checker and continue evaluating remaining checks.

Separation from unrelated repository debt:

1. No full-repo lint/test/build gate in runtime loop.
2. Only current-step integration validation blockers can block commit.
3. Only files changed in this iteration are syntax-checked.
4. Existing unrelated repository issues remain visible in inspect/report but do not block commit in this step.

Tests:

1. `TestDefaultStagesIncludesCommitAfterValidate`
2. `TestEngineValidatesBeforeGitManagerHandle`
3. `TestGitManagerBlocksCommitWhenCurrentStepValidationHasErrors`
4. `TestGitManagerDoesNotBlockOnOtherStepErrors`
5. `TestGitManagerBlocksCommitOnPythonSyntaxFailure`
6. `TestGitManagerBlocksCommitOnYAMLSyntaxFailure`
7. `TestGitManagerSilentlySkipsMissingInterpreterChecker`
8. `TestGitManagerSkipsApprovalPromptWhenPreCommitGateFails`
9. `TestGitManagerPreservesDirtyWorktreeOnPreCommitFailure`
10. `TestGitManagerRejectFlowStillRestoresTree`
11. `TestGitManagerHappyPathStillCommits`
12. `TestStdoutReporterShowsCommitSkippedDueToPreCommitChecks`

Validation commands:

1. `go test ./internal/orchestrator ./internal/gitmanager ./internal/adapters/report`
2. `go test ./internal/core ./internal/core/ports`
3. `go test ./...`

Acceptance criteria:

1. Concierge never offers commit approval before running pre-commit integration quality checks.
2. Unrelated repo-wide quality debt does not block step commits.
3. Checker infrastructure failures are silently skipped.
4. Failed pre-commit checks keep uncommitted edits and produce clear report evidence.
5. Existing stepwise commit audit trail remains intact for passing steps.

Rollback boundary:

- Revert pre-commit gate wiring in `core`, `orchestrator`, `gitmanager`, and reporter updates for this step only.

---

### Step 10C: Model contract authoring flow (`PENDING`)

Objective:

Create a dedicated authoring path for model contract remediation with explicit user-facing recommendations.

Files to add:

1. `internal/adapters/execute/model_authoring_context.go`
2. `internal/adapters/execute/model_authoring_context_test.go`

Files to modify:

1. `internal/adapters/execute/agent_executor.go`
2. `internal/cli/run.go` (approval prompt context)
3. `internal/core/types.go` (authoring recommendation payload, if needed)

Interface/API changes:

1. Add `AuthoringRecommendation` structure to execution evidence payload conventions.
2. Extend agent task context to include:
   1. resolved/ambiguous model candidate list
   2. include/exclude mismatch details
   3. model format constraints (`.onnx` / `.h5`)

Locked behavior:

1. For model issues, CLI prompt must show recommended model target before approval.
2. Agent objective must include a strict “do not touch unrelated training logic” constraint.
3. Execution evidence must include selected model path and rationale.

Tests:

1. `TestModelAuthoringRecommendationRenderedInApprovalPrompt`
2. `TestModelAuthoringAgentTaskIncludesCandidateContext`
3. `TestModelAuthoringEvidenceContainsSelectedModelPath`

Validation commands:

1. `go test ./internal/adapters/execute ./internal/cli -run ModelAuthoring -v`
2. `go test ./...`

Acceptance criteria:

1. Model remediation is independently actionable and auditable.
2. User sees explicit rationale for model-path suggestions before approving edits.

Rollback boundary:

- Revert model authoring context files and related prompt/agent wiring only.

---

### Step 10D: Preprocess need detection (`PENDING`)

Objective:

Emit preprocess-specific issue codes from deterministic contract analysis and lightweight static checks.

Files to add:

1. `internal/adapters/inspect/preprocess_contract.go`
2. `internal/adapters/inspect/preprocess_contract_test.go`

Files to modify:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/core/issues.go` (only if additional deterministic preprocess detection code is required)
3. `internal/core/issue_step_map.go` (if new preprocess-related code is introduced)

Locked behavior:

1. Detect missing preprocess function from discovered contracts.
2. Detect obvious invalid preprocess signatures/return semantics via static guardrails.
3. Emit existing preprocess issue codes (`preprocess_function_missing`, `preprocess_response_invalid`, etc.) with actionable messages.

Tests:

1. `TestPreprocessDetectorEmitsMissingFunctionIssue`
2. `TestPreprocessDetectorEmitsInvalidSignatureIssue`
3. `TestPreprocessDetectorDoesNotFlagValidDefinitions`

Validation commands:

1. `go test ./internal/adapters/inspect -run Preprocess -v`
2. `go test ./internal/adapters/inspect ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Planner deterministically selects `ensure.preprocess_contract` when preprocess is absent/invalid.
2. Detection results are stable across repeated runs on unchanged snapshots.

Rollback boundary:

- Revert preprocess detector files and inspector mappings only.

---

### Step 10E: Preprocess authoring flow (`PENDING`)

Objective:

Provide a dedicated preprocess authoring objective with deterministic constraints and review evidence.

Files to add:

1. `internal/adapters/execute/preprocess_authoring_context.go`
2. `internal/adapters/execute/preprocess_authoring_context_test.go`

Files to modify:

1. `internal/adapters/execute/agent_executor.go`
2. `internal/cli/run.go`
3. `internal/gitmanager/messages.go` (review focus text)

Locked behavior:

1. Agent objective for preprocess must include:
   1. mandatory train + validation subsets
   2. deterministic, non-empty subset expectation when feasible
   3. prohibition against unrelated refactors
2. Approval prompt must explicitly describe preprocess-specific intended change.
3. Evidence must capture preprocess symbol(s) targeted.

Tests:

1. `TestPreprocessAuthoringTaskIncludesTrainValidationConstraint`
2. `TestPreprocessAuthoringReviewFocusHighlightsSubsetRequirement`
3. `TestPreprocessAuthoringEvidenceCapturesTargetSymbols`

Validation commands:

1. `go test ./internal/adapters/execute ./internal/cli ./internal/gitmanager -run PreprocessAuthoring -v`
2. `go test ./...`

Acceptance criteria:

1. Preprocess authoring is independently triggerable and auditable.
2. Suggested changes are clearly explained before user approval.

Rollback boundary:

- Revert preprocess authoring context/prompt/message changes only.

---

### Step 10F: Input-encoder need detection (`PENDING`)

Objective:

Detect missing input encoders and incomplete input coverage per model input contract.

Files to add:

1. `internal/adapters/inspect/input_encoder_contract.go`
2. `internal/adapters/inspect/input_encoder_contract_test.go`

Files to modify:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/core/issue_step_map.go` (only if additional input-encoder issue granularity is needed)

Locked behavior:

1. Derive expected input encoder set from discovered model inputs + integration contracts.
2. Emit issues for:
   1. missing input encoder function(s)
   2. incomplete symbol coverage
   3. impossible mapping due to unresolved model contract
3. Issue messages must include symbol-level detail.

Tests:

1. `TestInputEncoderDetectorEmitsMissingEncoderIssue`
2. `TestInputEncoderDetectorEmitsCoverageIncompleteIssue`
3. `TestInputEncoderDetectorNoFalsePositiveWhenCoverageComplete`

Validation commands:

1. `go test ./internal/adapters/inspect -run InputEncoder -v`
2. `go test ./internal/adapters/inspect ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Planner can select `ensure.input_encoders` from deterministic detector output.
2. Detector output identifies exactly which encoder symbols are missing.

Rollback boundary:

- Revert input-encoder detector files and related inspector wiring only.

---

### Step 10G: Input-encoder suggestion and authoring flow (`PENDING`)

Objective:

Render symbol-level encoder suggestions to the user and pass the same context to authoring execution.

Files to add:

1. `internal/adapters/execute/input_encoder_authoring_context.go`
2. `internal/adapters/execute/input_encoder_authoring_context_test.go`

Files to modify:

1. `internal/adapters/execute/agent_executor.go`
2. `internal/cli/run.go`
3. `internal/adapters/report/stdout_reporter.go` (or equivalent recommendation output path)

Locked behavior:

1. Approval prompt lists missing input encoder symbols.
2. Agent objective includes those exact symbols and model-shape constraints.
3. Evidence captures recommendation list and resolved authored symbols.

Tests:

1. `TestInputEncoderRecommendationListsMissingSymbols`
2. `TestInputEncoderAuthoringTaskCarriesSymbolList`
3. `TestInputEncoderAuthoringEvidenceIncludesRecommendationAndResult`

Validation commands:

1. `go test ./internal/adapters/execute ./internal/cli ./internal/adapters/report -run InputEncoderAuthoring -v`
2. `go test ./...`

Acceptance criteria:

1. Input-encoder authoring workflow is explicit, reviewable, and symbol-specific.
2. User-facing suggestions and agent context are consistent.

Rollback boundary:

- Revert input-encoder authoring context and reporting/prompt wiring only.

---

### Step 10H: GT-encoder need detection (`PENDING`)

Objective:

Detect missing or invalid ground-truth encoders as a separate capability from input encoders.

Files to add:

1. `internal/adapters/inspect/gt_encoder_contract.go`
2. `internal/adapters/inspect/gt_encoder_contract_test.go`

Files to modify:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/core/issue_step_map.go` (if additional GT issue specificity is introduced)

Locked behavior:

1. Detect missing GT encoders for labeled subsets.
2. Detect contract mismatch where GT encoders are configured but incompatible with discovered targets.
3. Preserve existing v1 rule that unlabeled subsets must not require GT execution.

Tests:

1. `TestGTEncoderDetectorEmitsMissingIssue`
2. `TestGTEncoderDetectorEmitsContractMismatchIssue`
3. `TestGTEncoderDetectorRespectsUnlabeledSubsetRule`

Validation commands:

1. `go test ./internal/adapters/inspect -run GTEncoder -v`
2. `go test ./internal/adapters/inspect ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Planner can select `ensure.ground_truth_encoders` from deterministic detector output.
2. Detector output separates GT concerns from input-encoder concerns.

Rollback boundary:

- Revert GT detector files and related inspector wiring only.

---

### Step 10I: GT-encoder suggestion and authoring flow (`PENDING`)

Objective:

Add GT-specific recommendation and authoring flow with labeled-subset constraints.

Files to add:

1. `internal/adapters/execute/gt_encoder_authoring_context.go`
2. `internal/adapters/execute/gt_encoder_authoring_context_test.go`

Files to modify:

1. `internal/adapters/execute/agent_executor.go`
2. `internal/cli/run.go`
3. `internal/gitmanager/messages.go` (review focus messaging)

Locked behavior:

1. Prompt and agent context must distinguish GT targets from input targets.
2. Agent objective must include “run on labeled subsets only” guidance.
3. Evidence must record GT symbols authored/repaired.

Tests:

1. `TestGTEncoderAuthoringTaskIncludesLabeledSubsetConstraint`
2. `TestGTEncoderReviewFocusMentionsGroundTruthTargets`
3. `TestGTEncoderAuthoringEvidenceContainsTargetSymbols`

Validation commands:

1. `go test ./internal/adapters/execute ./internal/cli ./internal/gitmanager -run GTEncoderAuthoring -v`
2. `go test ./...`

Acceptance criteria:

1. GT authoring is independently triggerable and auditable.
2. Constraints prevent conflating GT with input encoder work.

Rollback boundary:

- Revert GT authoring context and prompt/review message changes only.

---

### Step 10J: Integration-test wiring need detection (`PENDING`)

Objective:

Enforce required decorator-call wiring under `@tensorleap_integration_test` through deterministic analysis.

Files to add:

1. `internal/adapters/validate/integration_test_ast.go`
2. `internal/adapters/validate/integration_test_ast_test.go`

Files to modify:

1. `internal/adapters/validate/baseline_validator.go`
2. `internal/core/issues.go` (if additional call-graph issue granularity is required)

Locked behavior:

1. Detect decorators defined but not called in integration-test call path.
2. Detect calls to unknown/non-discovered interfaces.
3. Emit precise issue locations when AST traversal can resolve line/column.

Tests:

1. `TestIntegrationTestAnalyzerDetectsMissingRequiredCalls`
2. `TestIntegrationTestAnalyzerDetectsUnknownInterfaceCalls`
3. `TestIntegrationTestAnalyzerReportsStableLocations`

Validation commands:

1. `go test ./internal/adapters/validate -run IntegrationTestAnalyzer -v`
2. `go test ./internal/adapters/validate ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Integration-test wiring gaps are surfaced as deterministic validation failures.
2. Planner receives integration-test call-coverage issues with actionable detail.

Rollback boundary:

- Revert integration-test AST analyzer and validator wiring only.

---

### Step 10K: Integration-test wiring authoring flow (`PENDING`)

Objective:

Provide targeted authoring objective for fixing integration-test wiring gaps without broad refactors.

Files to add:

1. `internal/adapters/execute/integration_test_authoring_context.go`
2. `internal/adapters/execute/integration_test_authoring_context_test.go`

Files to modify:

1. `internal/adapters/execute/agent_executor.go`
2. `internal/cli/run.go`
3. `internal/adapters/report/stdout_reporter.go`

Locked behavior:

1. Suggestion text lists missing required calls and unknown calls separately.
2. Agent objective includes strict scope: repair call wiring only.
3. Evidence captures required-call set before and after authoring.

Tests:

1. `TestIntegrationTestAuthoringRecommendationListsMissingCalls`
2. `TestIntegrationTestAuthoringTaskContainsRequiredCallSet`
3. `TestIntegrationTestAuthoringEvidenceContainsBeforeAfterCallSets`

Validation commands:

1. `go test ./internal/adapters/execute ./internal/cli ./internal/adapters/report -run IntegrationTestAuthoring -v`
2. `go test ./...`

Acceptance criteria:

1. Integration-test wiring fixes are reviewable as a dedicated step.
2. Authoring evidence proves which calls were added or repaired.

Rollback boundary:

- Revert integration-test authoring context and recommendation/report wiring only.

---

### Step 11A: Real runtime harness core (Layer 2) (`PENDING`)

Objective:

Replace stub harness with executable runtime harness that validates semantic runtime behavior by default for non-dry-run flows.

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
4. `internal/cli/run.go` (`--disable-harness`, `--harness-timeout` wiring)

Locked behavior:

1. Default timeout remains 120 seconds.
2. Harness event schema includes `schemaVersion: 1`.
3. Harness runs by default for non-dry-run unless explicitly disabled.
4. Parser rejects unsupported schema versions with actionable errors.

Tests:

1. `TestHarnessRunnerInvokesRuntimeScriptByDefault`
2. `TestHarnessParserAcceptsSchemaV1Events`
3. `TestHarnessParserRejectsUnsupportedSchema`
4. `TestValidatorFailsWhenHarnessReportsErrors`
5. `TestValidatorPassesOnCleanHarnessRun`

Validation commands:

1. `go test ./internal/adapters/validate -run Harness -v`
2. `go test ./internal/adapters/validate ./internal/cli`
3. `go test ./...`

Acceptance criteria:

1. Runtime validator consumes real harness evidence, not stub-only output.
2. Harness failures are deterministic blockers.

Rollback boundary:

- Revert runtime harness scripts and validator/CLI harness wiring only.

---

### Step 11B: Harness semantic coverage mapping (`PENDING`)

Objective:

Map harness events to preprocess/input/GT/validation issue families with per-symbol evidence.

Files to add:

1. `internal/adapters/validate/harness_issue_mapper.go`
2. `internal/adapters/validate/harness_issue_mapper_test.go`
3. `internal/adapters/validate/heuristics_advanced.go`
4. `internal/adapters/validate/heuristics_advanced_test.go`

Files to modify:

1. `internal/adapters/validate/harness_parser.go`
2. `internal/adapters/validate/heuristics.go`
3. `internal/adapters/validate/baseline_validator.go`

Locked behavior:

1. Map runtime failures to existing issue families:
   1. preprocess
   2. encoder coverage
   3. validation failure
2. Preserve anti-stub heuristics and add prediction-variation checks when prediction events are available.
3. Include symbol/subset context in issue messages.

Tests:

1. `TestHarnessIssueMapperMapsPreprocessFailures`
2. `TestHarnessIssueMapperMapsEncoderCoverageFailures`
3. `TestHeuristicsDetectConstantPredictionsWhenEventsExist`
4. `TestHeuristicsNoFalsePositiveWithVaryingPredictions`

Validation commands:

1. `go test ./internal/adapters/validate -run HarnessIssueMapper -v`
2. `go test ./internal/adapters/validate`
3. `go test ./...`

Acceptance criteria:

1. Harness output drives precise issue generation for authoring steps.
2. Validation issues contain enough context to produce deterministic suggestions.

Rollback boundary:

- Revert harness issue mapper and advanced heuristics wiring only.

---

### Step 12A: Fixture mutation framework for capability-isolated cases (`PENDING`)

Objective:

Create deterministic fixture mutation tooling to produce one-defect-at-a-time cases for each authoring capability.

Files to add:

1. `scripts/fixtures_mutate_cases.sh`
2. `fixtures/cases/README.md`
3. `fixtures/cases/schema.json`

Files to modify:

1. `scripts/fixtures_prepare.sh`
2. `scripts/fixtures_verify.sh`
3. `fixtures/manifest.json` (case descriptors)

Locked behavior:

1. Generate case variants from post fixture commits for:
   1. missing model
   2. missing preprocess
   3. missing input encoder
   4. missing GT encoder
   5. missing integration-test required calls
2. Each case mutates only one capability.
3. Generated case repos remain clean git trees after mutation commit.

Tests:

1. Shell-level assertions (CI) for deterministic case generation.
2. Optional Go wrapper tests under `internal/e2e/fixtures`.

Validation commands:

1. `bash scripts/fixtures_prepare.sh`
2. `bash scripts/fixtures_mutate_cases.sh`
3. `bash scripts/fixtures_verify.sh`

Acceptance criteria:

1. Case variants exist per fixture and per capability.
2. Case generation is idempotent and deterministic.

Rollback boundary:

- Revert fixture mutation tooling and case metadata only.

---

### Step 12B: Capability E2E (model) (`PENDING`)

Objective:

Prove missing-model detection + authoring convergence in fixture automation.

Files to add:

1. `internal/e2e/fixtures/case_model_test.go`
2. `internal/e2e/fixtures/testdata/cases/model/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Model-missing case emits model-specific blocking issue.
2. Planner primary step includes `ensure.model_contract`.
3. Post-authoring validation clears model-specific blocking issue family.

Tests:

1. `TestFixtureCaseMissingModel_Recovers`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run MissingModel -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Model authoring flow is proven end-to-end on fixtures.

Rollback boundary:

- Revert model capability E2E tests and related golden data only.

---

### Step 12C: Capability E2E (preprocess) (`PENDING`)

Objective:

Prove missing-preprocess detection + authoring convergence in fixture automation.

Files to add:

1. `internal/e2e/fixtures/case_preprocess_test.go`
2. `internal/e2e/fixtures/testdata/cases/preprocess/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Preprocess-missing case emits preprocess-specific blocking issue.
2. Planner primary step includes `ensure.preprocess_contract`.
3. Post-authoring validation clears preprocess issue family.

Tests:

1. `TestFixtureCaseMissingPreprocess_Recovers`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run MissingPreprocess -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Preprocess authoring flow is proven end-to-end on fixtures.

Rollback boundary:

- Revert preprocess capability E2E tests and related golden data only.

---

### Step 12D: Capability E2E (input encoders) (`PENDING`)

Objective:

Prove input-encoder detection, suggestion, and authoring convergence in fixture automation.

Files to add:

1. `internal/e2e/fixtures/case_input_encoder_test.go`
2. `internal/e2e/fixtures/testdata/cases/input_encoders/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Missing-input-encoder case emits symbol-specific input encoder issues.
2. Recommendation output lists missing encoder symbols.
3. Post-authoring validation clears input encoder issue family.

Tests:

1. `TestFixtureCaseMissingInputEncoders_Recovers`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run MissingInputEncoders -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Input-encoder authoring flow is proven end-to-end on fixtures.

Rollback boundary:

- Revert input-encoder capability E2E tests and related golden data only.

---

### Step 12E: Capability E2E (GT encoders) (`PENDING`)

Objective:

Prove GT-encoder detection + authoring convergence in fixture automation.

Files to add:

1. `internal/e2e/fixtures/case_gt_encoder_test.go`
2. `internal/e2e/fixtures/testdata/cases/gt_encoders/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Missing-GT-encoder case emits GT-specific issues.
2. Planner selects `ensure.ground_truth_encoders`.
3. Post-authoring validation clears GT issue family without regressing input-encoder checks.

Tests:

1. `TestFixtureCaseMissingGTEncoders_Recovers`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run MissingGTEncoders -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. GT-encoder authoring flow is proven end-to-end on fixtures.

Rollback boundary:

- Revert GT-encoder capability E2E tests and related golden data only.

---

### Step 12F: Capability E2E (integration-test wiring) (`PENDING`)

Objective:

Prove integration-test required-call detection + authoring convergence in fixture automation.

Files to add:

1. `internal/e2e/fixtures/case_integration_test_wiring_test.go`
2. `internal/e2e/fixtures/testdata/cases/integration_test_wiring/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Missing-required-call case emits `integration_test_missing_required_calls`.
2. Planner selects `ensure.integration_test_contract`.
3. Post-authoring validation clears integration-test wiring issue family.

Tests:

1. `TestFixtureCaseMissingIntegrationTestCalls_Recovers`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run MissingIntegrationTestCalls -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Integration-test wiring authoring flow is proven end-to-end on fixtures.

Rollback boundary:

- Revert integration-test wiring capability E2E tests and related golden data only.

---

### Step 12G: Multi-step recovery E2E (`PENDING`)

Objective:

Prove deterministic convergence across multiple missing capabilities in one run sequence.

Files to add:

1. `internal/e2e/fixtures/case_composite_recovery_test.go`
2. `internal/e2e/fixtures/testdata/cases/composite/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Composite case triggers ordered sequence of primary steps (model -> preprocess -> encoders -> integration-test wiring -> harness validation).
2. Each iteration emits auditable evidence for the executed capability.
3. Final state reaches `ensure.complete` with no blocking issues.

Tests:

1. `TestFixtureCaseCompositeMissingContracts_Converges`

Validation commands:

1. `CONCIERGE_RUN_FIXTURE_E2E=1 go test ./internal/e2e/fixtures -run Composite -v`
2. `CONCIERGE_RUN_FIXTURE_E2E=1 bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Concierge demonstrates deterministic multi-capability convergence on fixtures.

Rollback boundary:

- Revert composite E2E test/golden files only.

---

### Step 13A: Upload readiness and guarded `leap push` execution (`PENDING`)

Objective:

Implement operational Tensorleap CLI flow with explicit push approval and full audit evidence.

Files to add:

1. `internal/leap/runner.go`
2. `internal/leap/runner_test.go`
3. `internal/adapters/execute/upload_executor.go`
4. `internal/adapters/execute/upload_executor_test.go`

Files to modify:

1. `internal/cli/run.go`
2. `internal/adapters/inspect/leap_cli_contract.go`
3. `internal/adapters/execute/stub_executor.go` (dispatcher routing for upload steps)

Locked behavior:

1. `ensure.upload_readiness` runs non-destructive probes:
   1. `leap --version`
   2. `leap auth whoami`
   3. `leap server info`
2. `ensure.upload_push` requires explicit affirmative gate (`--allow-push`) and interactive confirmation unless `--yes` is set.
3. Every readiness/push command stores stdout/stderr as evidence.
4. Push is never attempted when readiness checks fail.

Tests:

1. `TestUploadExecutorRefusesPushWithoutAllowPush`
2. `TestUploadExecutorRefusesPushWithoutApproval`
3. `TestUploadExecutorRunsReadinessChecksBeforePush`
4. `TestUploadExecutorPersistsCommandEvidence`

Validation commands:

1. `go test ./internal/leap ./internal/adapters/execute ./internal/cli -run Upload -v`
2. `go test ./...`

Acceptance criteria:

1. Concierge can run readiness checks and guarded push in real environments.
2. All upload attempts are auditable through persisted evidence.

Rollback boundary:

- Revert leap runner/upload executor and related CLI/dispatcher wiring only.

---

### Step 13B: CI expansion with capability E2E execution (`PENDING`)

Objective:

Make CI enforce lint/test/build and capability-level fixture E2E suites on PR/push.

Files to add:

1. `.golangci.yml`

Files to modify:

1. `Makefile`
2. `.github/workflows/ci.yml`
3. `.github/workflows/release.yml` (only if compatibility adjustment is required)

Locked CI behavior:

1. Required PR checks:
   1. `make lint`
   2. `make test`
   3. `make build`
   4. `make test-fixtures` (capability E2E enabled)
2. Fixture job must include timeout + retry budget to reduce flakiness.
3. Release workflow remains semver-tag gated.

Make targets (required):

1. `lint`
2. `test`
3. `test-fixtures`
4. `build`

Validation commands:

1. `make lint`
2. `make test`
3. `CONCIERGE_RUN_FIXTURE_E2E=1 make test-fixtures`
4. `make build`

Acceptance criteria:

1. CI fails on any lint/test/build/capability-E2E regression.
2. Capability-level fixture tests are mandatory for merge.

Rollback boundary:

- Revert CI/lint/makefile changes only.

---

### Step 14A: Documentation sync (README + docs/*) (`PENDING`)

Objective:

Align documentation with implemented authoring-first behavior and operational guardrails.

Files to add:

1. `docs/architecture.md`
2. `docs/dev-setup.md`
3. `docs/fixtures.md`
4. `docs/operations.md`
5. `docs/authoring-capabilities.md`

Files to modify:

1. `README.md`
2. `fixtures/README.md`
3. `PLAN.md` (status updates only, as steps are completed)

Locked documentation requirements:

1. Document capability-specific flow for:
   1. model contract
   2. preprocess
   3. input encoders
   4. GT encoders
   5. integration-test wiring
2. Document detection -> suggestion -> authoring -> validation loop with examples.
3. Document harness default behavior and disable/timeout flags.
4. Document trust boundary, command approvals, and evidence artifacts.

Validation commands:

1. Execute all documented command paths or mark explicitly as example-only.
2. `go test ./...` after documentation edits.

Acceptance criteria:

1. New contributor can reproduce capability-level fixture workflow using docs alone.
2. Documentation does not claim unimplemented behavior.

Rollback boundary:

- Revert docs/README updates only.

---

### Step 14B: Release-readiness hardening (`PENDING`)

Objective:

Finalize operational polish and lock release acceptance criteria after capability-level authoring and tests are complete.

Files to add or modify:

1. `internal/cli/*` (error UX and exit-code consistency)
2. `internal/core/*` (error-code normalization, if needed)
3. `README.md` (final acceptance checklist)
4. `PLAN.md` (status transitions to `DONE` / `ACCEPTED`)

Locked tasks:

1. Normalize top blocker messages for common failure categories:
   1. model
   2. preprocess
   3. input encoder
   4. GT encoder
   5. integration-test wiring
   6. upload readiness
2. Ensure every ensure-step emits actionable evidence payloads.
3. Freeze final acceptance checklist command block.

Final acceptance checklist commands:

1. `go test ./...`
2. `make lint`
3. `make build`
4. `CONCIERGE_RUN_FIXTURE_E2E=1 make test-fixtures`
5. Manual smoke: `go run ./cmd/concierge run --max-iterations=3 --persist`
6. Manual smoke (agent-enabled): `go run ./cmd/concierge run --enable-agent --max-iterations=3 --persist`

Acceptance criteria:

1. Steps `1` through `14B` are `ACCEPTED` after merge to `main`.
2. CI is green with capability E2E checks included.
3. README + docs reflect released functionality.

Rollback boundary:

- Revert only hardening and final checklist/documentation updates introduced by this step.

## Concrete Step Execution Order

1. Implement Step 7A.
2. Implement Step 7B.
3. Implement Step 8A.
4. Implement Step 8B.
5. Implement Step 9A.
6. Implement Step 9B.
7. Step 9C is already `ACCEPTED` (no implementation action required).
8. Step 10A0 is already `ACCEPTED` (no implementation action required).
9. Implement Step 10A.
10. Implement Step 10B.
11. Implement Step 10B1.
12. Implement Step 10C.
13. Implement Step 10D.
14. Implement Step 10E.
15. Implement Step 10F.
16. Implement Step 10G.
17. Implement Step 10H.
18. Implement Step 10I.
19. Implement Step 10J.
20. Implement Step 10K.
21. Implement Step 11A.
22. Implement Step 11B.
23. Implement Step 12A.
24. Implement Step 12B.
25. Implement Step 12C.
26. Implement Step 12D.
27. Implement Step 12E.
28. Implement Step 12F.
29. Implement Step 12G.
30. Implement Step 13A.
31. Implement Step 13B.
32. Implement Step 14A.
33. Implement Step 14B.

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

1. Steps `1` through `14B` (including `10A0`, `10A`, `10B`, `10B1`, `10C-10K`, `11A-11B`, `12A-12G`, `13A-13B`, `14A-14B`) are `ACCEPTED`.
2. Capability-level fixture E2E jobs are part of required CI and green.
3. Concierge can perform user-approved detection->suggest->author->validate loops for model/preprocess/input-encoder/GT-encoder/integration-test wiring and complete guarded upload workflow end-to-end.

## Idempotence and Recovery

1. Keep one commit per step for clean rollback.
2. If CI fails, fix within current step scope only.
3. Revert only the failing step commit when rollback is required.
4. Fixture scripts and `.concierge` persistence paths must remain safe to rerun.

## Surprises & Discoveries

- Step `9C` is implemented and merged, but planner-triggered authoring for preprocess/input/GT/integration-test wiring is still under-specified because detector paths do not yet emit those issue families from live repo contracts.
- Existing fixture E2E covers artifact deltas and persistence, but it does not yet prove capability-isolated authoring convergence (model/preprocess/input/GT/wiring) one-by-one.
- Runtime harness integration exists only as a stub baseline; it must become default runtime evidence before capability E2E can assert semantic behavior robustly.

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
- Decision: Exclude optional asset guidance/checks from Concierge v1 runtime and defer to a dedicated v2 enhancement workflow.
  Rationale: v1 goal is successful mandatory onboarding with minimal distraction and deterministic contracts.
  Date/Author: 2026-03-03 / user + assistant.
- Decision: Mark Step `9C` as `ACCEPTED` and re-plan remaining work around authoring-first capability slices instead of batching broad runtime/fixture changes.
  Rationale: The primary product value is guided code authoring for mandatory onboarding contracts; each capability must be independently detectable, fixable, and testable.
  Date/Author: 2026-03-03 / user + assistant.
- Decision: Sequence detection and authoring steps (`10A-10K`) before runtime harness deepening and capability E2E assertions (`11A-12G`).
  Rationale: Harness and fixture suites must validate capability-level behavior that is already concretely implemented and wired in planner/executor paths.
  Date/Author: 2026-03-03 / assistant.
- Decision: Insert a dedicated delta-scoped pre-commit integration quality gate step (`10B1`) before authoring-heavy slices continue.
  Rationale: Commit approval must run only after step-local integration validation and changed-file syntax checks so broken step deltas are not committed prematurely.
  Date/Author: 2026-03-03 / user + assistant.
- Decision: Use identifier `10B1` for the pre-commit gate instead of renumbering downstream authoring steps.
  Rationale: Preserve referential stability for already-reviewed `10C-14B` capability slices while inserting the gate at the correct sequence point between `10B` and `10C`.
  Date/Author: 2026-03-03 / assistant.

## Outcomes & Retrospective

Current state: baseline architecture through Step `10A0` is accepted and merged; remaining work is now decomposed into capability-level authoring slices with explicit detection, suggestion, authoring, and fixture validation phases.

Primary residual risks:

1. Detector-to-planner mapping gaps may prevent expected authoring steps from being selected.
2. Agent-authoring prompts may be too generic unless they receive symbol-level context from detectors.
3. Capability E2E may become flaky unless fixture mutation tooling is strictly deterministic.

Mitigations:

1. Implement detector slices first (`10A-10J`) and assert mapped primary-step expectations in unit tests.
2. Gate authoring context with explicit recommendation payload tests (`10C`, `10E`, `10G`, `10I`, `10K`).
3. Introduce deterministic fixture case generation before capability E2E tests (`12A` before `12B-12G`).

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
