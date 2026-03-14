# Concierge Remaining Work Plan

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

This file now tracks only the remaining backlog. Completed work was intentionally removed on 2026-03-10 to keep the plan short enough for repeated agent use. Use git history and merged PRs for historical detail.

## Purpose / Big Picture

Finish Concierge as an operational Tensorleap integration assistant that:

1. resolves and records the correct local Python runtime before any Python-dependent action
2. drives integrations toward the guide-native `leap_integration.py` workflow as the only supported layout
3. validates integrations through the same staged local signals described in `GUIDE.md`, then expands to higher-coverage Concierge checks
4. proves the workflow on deterministic fixture repos, then ships guarded upload, CI, and release-ready docs

## Current Baseline

The following baseline already exists on `main` and is intentionally omitted from the step list below:

1. workspace snapshotting, persistence, reporting, and deterministic planner/executor scaffolding
2. git diff approval and commit flow in product runtime
3. agent task integration and Tensorleap knowledge/context injection baseline
4. contract discovery and authoring flows for model, preprocess, input encoders, and GT encoders
5. input/GT discovery parity work plus confirmed mapping persistence

The remaining work starts from that baseline.

## Progress

- [x] (2026-03-10 00:00Z) Replaced the historical plan with a pending-only plan aligned to `GUIDE.md` and the Poetry runtime boundary.
- [x] `ENV1` `ACCEPTED` - Capture runtime facts in snapshots and classify Poetry support deterministically.
- [x] `ENV2` `ACCEPTED` - Resolve and persist a Local Runtime Profile for the selected repo root.
- [x] `ENV3` `ACCEPTED` - Model dependency readiness and Tensorleap-local package readiness as explicit runtime gates.
- [x] `ENV4` `ACCEPTED` - Enforce runtime gating in planner/UX and route all local Python execution through the resolved Poetry boundary.
- [x] `GUIDE1` `ACCEPTED` - Make `leap_integration.py` the canonical integration layout for fresh repos.
- [x] `GUIDE2` `ACCEPTED` - Implement guide-native progressive validator orchestration and reporting.
- [x] `GUIDE3` `ACCEPTED` - Enforce thin `integration_test` rules and targeted authoring for integration-test wiring failures.
- [ ] `VAL1` `PENDING` - Replace the stub harness with multi-sample runtime validation plus issue mapping.
- [x] `FIX1` `DONE` - Separate fixture bootstrap from product runtime and generate guide-native mutation cases.
- [ ] `FIX2` `PENDING` - Add capability and composite fixture E2E coverage, including agent-context quality assertions.
- [ ] `OPS1` `PENDING` - Implement upload readiness checks and guarded `leap push`.
- [ ] `SHIP1` `PENDING` - Finish CI, docs, and release-readiness hardening.
- [ ] `QA1` `PENDING` - Add a Codex-driven PTY QA loop that can exercise Concierge like a human user and emit qualitative reports.

## Surprises & Discoveries

- `GUIDE.md` changes the product shape, not just the docs: `leap_integration.py` is the canonical entrypoint and the local validation harness.
- Validation is call-driven. A decorated interface that is never exercised still looks missing to the validator.
- The first meaningful full-path validation happens when `integration_test(...)` runs and then re-runs in mapping mode.
- The human-oriented exit table appears only when the filename is exactly `leap_integration.py`.
- `LeapLoader.check_dataset()` is the better machine interface than scraping local terminal output.
- The current codebase still hard-codes binder-era names across inspector, scaffolding, agent scope, prompt/context packs, fixture helpers, and a wide slice of tests.
- The old pending plan over-weighted historical step bookkeeping and under-weighted alignment to the new integration model.

## Decision Log

- Decision: Keep only remaining work in `PLAN.md`.
  Rationale: The historical merged-step trail had become too large for repeated agent use.
  Date/Author: 2026-03-10 / user + assistant.
- Decision: Put runtime and environment work first.
  Rationale: Every remaining Python-dependent step is unreliable until Concierge resolves and records the correct Poetry runtime.
  Date/Author: 2026-03-10 / user + assistant.
- Decision: After runtime work, prioritize `GUIDE.md` alignment before harness, fixtures, upload, or release steps.
  Rationale: The design must converge on `leap_integration.py`, staged direct-call validation, and thin integration-test wiring before downstream validation can be trustworthy.
  Date/Author: 2026-03-10 / user + assistant.
- Decision: Fold the old granular pending tail into a shorter phase-based backlog.
  Rationale: The remaining work is easier to reason about as runtime phase -> guide-native phase -> hardening phase.
  Date/Author: 2026-03-10 / assistant.

## Backlog Compression Map

This plan supersedes the old pending identifiers without keeping them inline:

1. `ENV1-ENV4` replace the old runtime workstream (`10M2A-10M2E`).
2. `GUIDE1-GUIDE3` replace the old layout/reporting tail and align the product to the guide-native integration model.
3. `VAL1` replaces the old harness work (`11A-11B`).
4. `FIX1-FIX2` replace the old fixture/bootstrap/capability-E2E tail (`12A0-12H`).
5. `OPS1` replaces guarded upload work (`13A`).
6. `SHIP1` replaces CI, docs, and release hardening (`13B-14B`).
7. `QA1` adds the operator-facing QA harness for subjective terminal evaluation.

## Phase 1: Runtime And Environment

### ENV1

Objective:

Expand snapshot and inspection so runtime facts are first-class state and Poetry support is classified deterministically.

Primary files:

1. `internal/adapters/snapshot/runtime_snapshot.go`
2. `internal/adapters/snapshot/runtime_snapshot_test.go`
3. `internal/adapters/inspect/runtime_contract.go`
4. `internal/adapters/inspect/baseline_inspector.go`
5. `internal/adapters/inspect/baseline_inspector_test.go`

Locked behavior:

1. Snapshot fingerprints `pyproject.toml` and `poetry.lock` separately when present.
2. Inspector recognizes supported Poetry project layouts and records Poetry presence/version.
3. Ambient `VIRTUAL_ENV` and `CONDA_PREFIX` are logged as diagnostics only.
4. Unsupported non-Poetry repos emit a blocking runtime issue with plain-language messaging.

Validation:

1. `go test ./internal/adapters/snapshot ./internal/adapters/inspect -run 'RuntimeSnapshot|PoetryProject' -v`
2. `go test ./internal/adapters/snapshot ./internal/adapters/inspect`
3. `go test ./...`

Acceptance:

Concierge can tell whether the selected repo root is supported for v1 local execution before planning any Python-dependent step.

### ENV2

Objective:

Resolve the project interpreter through Poetry, persist it as a Local Runtime Profile, and invalidate it deterministically on drift.

Primary files:

1. `internal/adapters/inspect/poetry_runtime_profile.go`
2. `internal/adapters/inspect/poetry_runtime_profile_test.go`
3. `internal/state/types.go`
4. `internal/state/store.go`
5. `internal/cli/runtime_prompt.go`
6. `internal/cli/run.go`

Locked behavior:

1. Runtime resolution uses Poetry as the source of truth for interpreter path and Python version.
2. Concierge does not install interpreters and does not create brand-new Poetry environments in product runtime.
3. Runtime profile state invalidates on selected-root drift, `pyproject.toml` or `poetry.lock` changes, interpreter-path changes, or Python-version changes.
4. User confirmation is required only when runtime resolution is ambiguous or suspicious.

Validation:

1. `go test ./internal/adapters/inspect ./internal/state ./internal/cli -run 'PoetryRuntimeProfile|RuntimePrompt' -v`
2. `go test ./internal/adapters/inspect ./internal/state ./internal/cli`
3. `go test ./...`

Acceptance:

Concierge has one persisted, inspectable source of truth for local Python execution.

### ENV3

Objective:

Model runtime readiness as environment readiness plus dependency readiness, including reviewable repo-managed changes for the Tensorleap-local package.

Primary files:

1. `internal/adapters/inspect/runtime_contract.go`
2. `internal/adapters/execute/poetry_dependency_executor.go`
3. `internal/adapters/execute/poetry_dependency_executor_test.go`
4. `internal/adapters/report/file_reporter.go`
5. `internal/cli/run_review_render.go`

Locked behavior:

1. Runtime readiness distinguishes a resolvable Poetry environment from project dependency readiness and Tensorleap-local package readiness.
2. `poetry check` is part of readiness before claiming the dependency graph is usable.
3. Dependency-install or repair actions may target only an already resolved existing Poetry environment and always flow through approval.
4. Tensorleap-local package fixes must appear as reviewable repo diffs such as `pyproject.toml` and `poetry.lock`.

Validation:

1. `go test ./internal/adapters/execute ./internal/adapters/inspect ./internal/adapters/report -run 'PoetryDependency|TensorleapPackage|RuntimeRevalidation' -v`
2. `go test ./internal/adapters/execute ./internal/adapters/inspect ./internal/adapters/report ./internal/cli`
3. `go test ./...`

Acceptance:

Runtime readiness no longer depends on hidden local overlays or untracked environment mutation.

### ENV4

Objective:

Make unresolved runtime the highest-priority planner blocker and enforce Poetry as the shared execution boundary for all local Python actions.

Primary files:

1. `internal/core/issues.go`
2. `internal/core/issue_step_map.go`
3. `internal/core/checklist.go`
4. `internal/adapters/planner/policy.go`
5. `internal/adapters/planner/deterministic_planner.go`
6. `internal/adapters/report/stdout_reporter.go`
7. `internal/adapters/validate/python_runtime_runner.go`
8. `internal/adapters/validate/harness_runner.go`
9. `internal/cli/run.go`

Locked behavior:

1. Planner must not select preprocess, encoder, integration-test, harness, or upload steps while runtime issues remain unresolved.
2. All local Python commands execute through `poetry run ...` or the resolved interpreter from the stored runtime profile.
3. Runtime provenance is attached to command evidence and downstream validation artifacts.
4. User-facing output explains the runtime blocker in plain language.

Validation:

1. `go test ./internal/adapters/planner ./internal/core ./internal/adapters/report -run 'LocalPoetryRuntime|RuntimeBlocker|Checklist' -v`
2. `go test ./internal/adapters/validate ./internal/adapters/report -run 'PythonRuntimeRunner|RuntimeProvenance' -v`
3. `go test ./...`

Acceptance:

No remaining Python-dependent feature can run until the correct runtime is resolved, persisted, and used.

## Phase 2: Guide-Native Integration Model

### GUIDE1

Objective:

Make `leap_integration.py` the only supported integration target for fresh repos.

Primary files:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/adapters/inspect/integration_contract.go`
3. `internal/adapters/inspect/model_discovery.go`
4. `internal/adapters/execute/filesystem_executor.go`
5. `internal/adapters/execute/templates/leap_yaml.tmpl`
6. `internal/adapters/execute/agent_scope_policy.go`
7. `internal/adapters/execute/repo_context_pack.go`
8. `internal/adapters/execute/preprocess_authoring_context.go`
9. `internal/agent/prompt_contract.go`
10. `internal/agent/context/tensorleap_knowledge_v1.md`
11. `internal/core/issues.go`
12. `internal/core/checklist.go`
13. `internal/core/types.go`
14. `internal/adapters/report/stdout_reporter.go`
15. `internal/cli/run.go`
16. `leap.yaml`

Locked behavior:

1. Concierge prefers root-level `leap_integration.py` plus `leap.yaml`.
2. `leap.yaml.entryFile` is validated against the canonical entrypoint and blocking issues are emitted when non-canonical layouts remain.
3. Scaffolds and deterministic repair paths create `leap_integration.py`, not `leap_binder.py`, `integration_test.py`, or `leap_custom_test.py`.
4. Default editable scope, repo context packs, and agent prompt contracts include `leap_integration.py` as a first-class file.
5. Contract discovery and authoring fallbacks start from `entryFile` plus decorators instead of hard-coded binder/test filenames.
6. Binder-era filenames do not participate in discovery, scaffolding, or prompt generation.
7. New scaffolds, repairs, and user guidance converge on the guide-native entrypoint.

Validation:

1. `go test ./internal/adapters/inspect ./internal/adapters/execute ./internal/agent ./internal/core ./internal/adapters/report ./internal/cli -run 'LeapIntegration|EntryFile|LegacyBinder|PromptContract|RepoContext' -v`
2. `go test ./internal/adapters/inspect ./internal/adapters/execute ./internal/agent ./internal/core ./internal/adapters/report ./internal/cli`
3. `go test ./...`

Acceptance:

The planner, reporter, and repair flows all assume `leap_integration.py` as the canonical integration shape.

### GUIDE2

Objective:

Implement the staged local validation loop from `GUIDE.md` so Concierge can reason about partial integrations the same way a human author would.

Primary files:

1. `internal/adapters/validate/baseline_validator.go`
2. `internal/adapters/validate/guide_validator.go`
3. `internal/adapters/validate/guide_validator_test.go`
4. `internal/adapters/report/stdout_reporter.go`
5. `internal/adapters/report/file_reporter.go`
6. `internal/cli/run.go`

Locked behavior:

1. Concierge captures import-time feedback, direct-call milestones, thin `integration_test(...)` runs, and mapping-mode outcomes through the resolved runtime boundary.
2. The human-oriented status table is treated as author/operator feedback, not as the primary machine interface.
3. `Successful!` is recorded as a first-sample milestone only.
4. Structured parser output from `LeapLoader.check_dataset()` is persisted when available and used for machine decisions.
5. Reporter explains the next recommended interface in the same staged order as the guide: preprocess -> minimum inputs -> load model -> thin integration test -> remaining inputs -> GT -> wider sample coverage.

Validation:

1. `go test ./internal/adapters/validate ./internal/adapters/report -run 'GuideValidator|LeapLoader|StatusTable' -v`
2. `go test ./internal/adapters/validate ./internal/adapters/report ./internal/cli`
3. `go test ./...`

Acceptance:

Concierge can evaluate an incomplete `leap_integration.py` and report the next useful validator milestone instead of only generic missing-contract errors.

### GUIDE3

Objective:

Enforce the thin `integration_test` rules from `GUIDE.md` and provide targeted authoring flow for wiring failures.

Primary files:

1. `internal/adapters/validate/integration_test_ast.go`
2. `internal/adapters/validate/integration_test_ast_test.go`
3. `internal/adapters/execute/integration_test_authoring_context.go`
4. `internal/adapters/execute/integration_test_authoring_context_test.go`
5. `internal/adapters/execute/agent_executor.go`
6. `internal/adapters/report/stdout_reporter.go`
7. `internal/cli/run.go`

Locked behavior:

1. Validator detects missing required decorator calls under `@tensorleap_integration_test`.
2. Validator also detects guide-native shape violations such as direct dataset access, arbitrary Python transforms, and manual batching inside the integration-test body.
3. Authoring recommendations separate missing calls from illegal body logic.
4. Agent scope for this step is narrow: repair integration-test wiring and code shape only.

Validation:

1. `go test ./internal/adapters/validate ./internal/adapters/execute ./internal/adapters/report -run 'IntegrationTestAnalyzer|IntegrationTestAuthoring' -v`
2. `go test ./internal/adapters/validate ./internal/adapters/execute ./internal/adapters/report ./internal/cli`
3. `go test ./...`

Acceptance:

Integration-test failures are reported and fixed as guide-native wiring/code-shape problems rather than only as missing decorators.

## Phase 3: Remaining Hardening

### VAL1

Objective:

Replace the stub harness with multi-sample runtime validation that builds on the guide-native first-sample loop and maps failures back to actionable issue families.

Primary files:

1. `scripts/harness_runtime.py`
2. `scripts/harness_lib/runner.py`
3. `scripts/harness_lib/events.py`
4. `internal/adapters/validate/harness_runner.go`
5. `internal/adapters/validate/harness_parser.go`
6. `internal/adapters/validate/harness_issue_mapper.go`
7. `internal/adapters/validate/heuristics_advanced.go`

Locked behavior:

1. Harness runs through the shared Poetry runtime runner.
2. Harness expands beyond first-sample success to bounded multi-sample checks across training and validation subsets.
3. Issue mapping preserves symbol, subset, and runtime-provenance context.
4. Anti-stub heuristics remain deterministic and use richer event data when available.

Validation:

1. `go test ./internal/adapters/validate -run 'Harness|HarnessIssueMapper|Heuristics' -v`
2. `go test ./internal/adapters/validate ./internal/cli`
3. `go test ./...`

Acceptance:

Concierge has both the guide-native local validator loop and a higher-coverage semantic runtime harness.

### FIX1

Objective:

Keep fixture bootstrap separate from production runtime policy and generate deterministic guide-native mutation cases.

Primary files:

1. `scripts/fixtures_bootstrap_poetry.sh`
2. `scripts/fixtures_prepare.sh`
3. `scripts/fixtures_mutate_cases.sh`
4. `scripts/fixtures_verify.sh`
5. `fixtures/manifest.json`
6. `fixtures/cases/README.md`
7. `fixtures/cases/schema.json`

Locked behavior:

1. Fixture bootstrap may use explicit `pyenv` plus Poetry setup, but product runtime paths must never depend on those helpers.
2. Fixture commands opt into bootstrap explicitly instead of relying on shell activation.
3. Mutation tooling creates one-defect-at-a-time cases for guide-native failures: canonical layout, preprocess, minimum inputs, load model, integration-test wiring, GT encoders, and composite recovery.
4. Fixture metadata and strip rules no longer assume binder-era filenames only.
5. Generated fixture repos remain clean and deterministic.

Validation:

1. `bash scripts/fixtures_bootstrap_poetry.sh --help`
2. `bash scripts/fixtures_prepare.sh`
3. `bash scripts/fixtures_mutate_cases.sh`
4. `bash scripts/fixtures_verify.sh`

Acceptance:

Fixture setup is explicit and reproducible, and the case corpus matches the guide-native workflow.

### FIX2

Objective:

Prove capability-level and composite convergence on fixtures, including agent-context quality for the remaining authoring steps.

Primary files:

1. `internal/e2e/fixtures/case_model_test.go`
2. `internal/e2e/fixtures/case_preprocess_test.go`
3. `internal/e2e/fixtures/case_input_encoder_test.go`
4. `internal/e2e/fixtures/case_gt_encoder_test.go`
5. `internal/e2e/fixtures/case_integration_test_wiring_test.go`
6. `internal/e2e/fixtures/case_composite_recovery_test.go`
7. `internal/e2e/fixtures/case_agent_context_quality_test.go`
8. `scripts/fixtures_run_checks.sh`
9. `internal/e2e/fixtures/case_encoder_authoring_test.go`

Locked behavior:

1. Capability cases prove the next-step ordering from the guide-native loop.
2. Composite cases prove deterministic end-to-end convergence across ordered steps.
3. Agent-context assertions prove the prompt includes the right Tensorleap rules and scope boundaries for each step.
4. Fixture E2E results are semantic-first and do not depend on exact repo diffs.

Validation:

1. `go test ./internal/e2e/fixtures -run 'MissingModel|MissingPreprocess|MissingInputEncoders|MissingGTEncoders|MissingIntegrationTestCalls|Composite|AgentContext' -v`
2. `bash scripts/fixtures_run_checks.sh`
3. `go test ./...`

Acceptance:

Remaining authoring and validation behavior is proven end-to-end against deterministic fixture cases.

### OPS1

Objective:

Implement operational Tensorleap readiness checks and guarded `leap push` with full audit evidence.

Primary files:

1. `internal/leap/runner.go`
2. `internal/leap/runner_test.go`
3. `internal/adapters/execute/upload_executor.go`
4. `internal/adapters/execute/upload_executor_test.go`
5. `internal/cli/run.go`

Locked behavior:

1. Concierge runs non-destructive readiness probes before any push attempt.
2. Push requires an explicit allow flag plus interactive confirmation unless `--yes` is set.
3. All readiness and push commands persist stdout/stderr evidence.
4. Push is blocked whenever runtime, guide-native validation, or readiness checks fail.

Validation:

1. `go test ./internal/leap ./internal/adapters/execute ./internal/cli -run Upload -v`
2. `go test ./...`

Acceptance:

Concierge can perform an auditable guarded upload only after the earlier phases succeed.

### SHIP1

Objective:

Finish CI, docs, and release-readiness hardening for the guide-aligned product.

Primary files:

1. `.github/workflows/ci.yml`
2. `.github/workflows/release.yml`
3. `.golangci.yml`
4. `Makefile`
5. `README.md`
6. `docs/architecture.md`
7. `docs/dev-setup.md`
8. `docs/fixtures.md`
9. `docs/operations.md`
10. `docs/authoring-capabilities.md`
11. `internal/cli/*`
12. `internal/core/*`

Locked behavior:

1. CI requires lint, test, build, and fixture E2E.
2. Docs describe the Poetry-only runtime model, `leap_integration.py` workflow, guide-native validation loop, trust boundary, fixture bootstrap separation, and guarded upload flow.
3. Release checklist and user-facing blocker messages are consistent with the shipped behavior.

Validation:

1. `make lint`
2. `make test`
3. `make build`
4. `make test-fixtures`
5. `go test ./...`
6. Manual smoke: `go run ./cmd/concierge run --max-iterations=3 --persist`

Acceptance:

The remaining backlog is fully documented, tested in CI, and ready for release review.

### QA1

Objective:

Implement the standalone Codex QA loop from `QA/DESIGN.md`: run Concierge in a PTY, let Codex act as the user, persist transcripts, and emit a qualitative report.

Primary files:

1. `QA/qa_loop.py`
2. `QA/pty_driver.py`
3. `QA/prompts/role_prompt.md`
4. `QA/prompts/nudge_prompt.md`
5. `QA/prompts/control_schema.json`
6. `QA/prompts/report_schema.json`
7. `QA/tests/test_pty_driver.py`
8. `QA/tests/test_qa_loop.py`
9. `Makefile`
10. `README.md`

Locked behavior:

1. Concierge runs inside a real PTY and the supervisor captures terminal output plus typed input decisions.
2. Codex control turns are structured JSON directives with `action`, `input_text`, and `loop_state`.
3. The loop enforces `max_iterations`, `max_idle_turns`, and `max_runtime` to avoid infinite runs.
4. Blind-first evaluation stays active until progress stalls; only then may the prompt reveal an optional post-fixture path.
5. Each run writes machine-readable turn logs, terminal transcripts, and a final qualitative report.
6. `STOP_FIX` is treated as a stop reason only; the optional repair loop is deferred.

Validation:

1. `python3 -m unittest discover -s tests -p 'test_*.py' -v`
2. `python3 QA/qa_loop.py --help`
3. `make test`

Acceptance:

The repository contains a repeatable local QA harness that can drive Concierge end to end without human input and leave behind actionable UX/product feedback.

## Validation And Acceptance

The backlog is complete when:

1. `ENV1-ENV4`, `GUIDE1-GUIDE3`, `VAL1`, `FIX1-FIX2`, `OPS1`, `SHIP1`, and `QA1` have all moved off `PENDING`.
2. Concierge always resolves runtime before Python-dependent work.
3. Concierge steers repos toward the `leap_integration.py` workflow and reasons about staged validator progress correctly.
4. Fixture E2E and CI prove the guide-native flow end to end.

## Idempotence And Recovery

1. Implement one step at a time.
2. Keep one commit per step.
3. If a step fails validation or CI, fix only that step or revert only that step.
4. Fixture bootstrap, fixture mutation, and `.concierge` persistence paths must stay safe to rerun.

## Outcomes & Retrospective

Current state:

The remaining backlog is now explicitly ordered around runtime correctness first, then alignment to the Tensorleap authoring model from `GUIDE.md`, then the downstream validation, fixture, upload, CI, and release work.

Primary residual risks:

1. Runtime ambiguity still threatens every Python-dependent action until `ENV1-ENV4` land.
2. Concierge still reasons mostly in contract terms, not yet in the full guide-native staged validator loop.
3. Fixture and release confidence remain incomplete until `VAL1`, `FIX1-FIX2`, `OPS1`, and `SHIP1` land.

Mitigations:

1. Do not start any guide-native authoring or harness work before the runtime phase is complete.
2. Treat `GUIDE1-GUIDE3` as the new product center of gravity, not as doc polish.
3. Keep the pending plan compressed so future sessions spend context on implementation, not on historical bookkeeping.

## Interfaces And Dependencies

1. Go `1.24.x`
2. Cobra for the CLI
3. `git`
4. `python3`
5. `poetry`
6. `jq`
7. `leap` CLI
8. optional dev/test-only `pyenv` for fixture bootstrap
qqqq
