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
- 2026-03-03: Renumbered authoring/context slices so agent context injection is a strict prerequisite sequence (`10C-10F`) before all downstream authoring steps (`10G-10O`), plus fixture-level context quality validation (`12H`).
- 2026-03-03: Marked Steps `10B1` and `10C` as `ACCEPTED` per user direction; `10C` merged via PR #13.
- 2026-03-04: Marked Steps `10A`, `10B`, `10F`, and `10G` as `ACCEPTED` per user direction to begin Step `10H`.
- 2026-03-04: Merged `feature/step-10h-preprocess-need-detection` to `main`; marked Step `10H` as `ACCEPTED` (all steps through `10H` now `ACCEPTED`).
- 2026-03-04: Merged `feature/step-10i-fix-tests` to `main`; marked Step `10I` as `ACCEPTED` after fixing preprocess authoring prompt/test drift.
- 2026-03-04: Landed commit `2b90758` on `main`; marked Steps `10J`, `10K`, `10L`, and `10M` as `ACCEPTED` (input/GT encoder detection + authoring flow now merged).
- 2026-03-04: Added blocking Step `10M1` as the immediate next step after `10M` to fix incorrect requirement ordering and decouple encoder requirement detection from integration-test call inference before continuing to `10N`.
- 2026-03-04: Tightened Step `10M1` to require fixture-ground-truth iterative development: derive encoder contract truth from fixture `post`, detect missing encoder requirements on fixture `pre`, and enforce pre/post contract matching invariants before proceeding.
- 2026-03-05: Step `10M1` execution failed to converge with the post/pre oracle-only requirement-source strategy; replaced it with a research-parity implementation sequence (`10M1A-10M1G`) that ports the proven Python discovery pipeline into Go with mandatory per-step tests.
- 2026-03-05: Added fixture capability regressions for encoder authoring recovery (`TestFixtureCaseMissingInputEncoders_Recovers`, `TestFixtureCaseMissingGTEncoders_Recovers`) and stabilized post-fixture completeness assertions for repositories that intentionally lack local `.onnx/.h5` artifacts.
- 2026-03-05: Implemented the research-parity input/GT discovery pipeline in Go (lead extraction, staged artifacts, normalizer, post-processing, contract-source switch, mapping persistence hooks, and planner order gating) and validated locally with inspect/planner/cli/state suites plus fixture E2E (`go test ./internal/adapters/inspect`, `go test ./internal/adapters/planner`, `go test ./internal/cli`, `go test ./internal/state`, `go test ./internal/e2e/fixtures`, `go test ./...`, `bash scripts/fixtures_run_checks.sh`, `make test`); step statuses remain `PENDING` until commit/push/PR/branch CI per workflow rules.
- 2026-03-05: Completed Step `10M1G` by adding research-target fixture parity coverage (`yolov5_visdrone`, `ultralytics`, `imdb`) and optional runtime signature corroboration (ONNX/Keras best-effort) integrated into discovery comparison reports.
- 2026-03-06: Fixture contamination audit found hidden Tensorleap artifacts surviving in generated `pre` repos (`ultralytics/tensorleap_folder`, `cifar10_resnet/utils.py` with `code_loader` imports). The fixture prep/verify invariant must now cover path-based artifacts and Python `code_loader` imports, not only literal `tensorleap` text.

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
| G1 | README §8/§10: Ensure-step `Fix` must apply real actions | Implemented deterministic filesystem mutations for scaffoldable steps; non-supported steps still require dedicated authoring slices | Closed for scaffold scope; remaining authoring capabilities handled in `10C-10O` | 9A (`ACCEPTED`) |
| G2 | README §10/§11: user approvals + diff review + commit workflow | Implemented runtime git diff approval/reject/commit flow | Closed; retain regression coverage | 9B (`ACCEPTED`) |
| G3 | README §10: agent collaboration for focused objectives | Implemented and merged (`internal/agent/*`, agent executor dispatch, transcript evidence) | Closed; maintain with regression tests only | 9C (`ACCEPTED`) |
| G4 | README §6.2: persistent mutable state (`.concierge/state.json`) | Implemented and persisted with invalidation reasons | Closed; retain regression coverage | 7A (`ACCEPTED`) |
| G5 | README §6.1/§8.2: richer snapshot/inspection coverage | Implemented readiness expansion for runtime/model/CLI/auth/server probes | Partially closed; remaining contract-level detection gaps tracked in `10A-10O` plus blocking input/GT discovery redesign (`10M1A-10M1G`) | 8A (`ACCEPTED`), 10A-10O, 10M1A-10M1G |
| G6 | README §8 planner semantics: deterministic next primary action with gate-aware ordering | Implemented severity-first planner policy with upload gate awareness | Closed for existing issue families; future families added in authoring slices | 8B (`ACCEPTED`) |
| G7 | README §9 + user requirement: preprocess authoring must be detection-driven and individually tested | Deterministic preprocess detection + authoring flow are implemented and merged (`10H`, `10I`) | Remaining gap is capability-level fixture proof that preprocess flows converge end-to-end | 10H (`ACCEPTED`), 10I (`ACCEPTED`), 12C |
| G8 | README §9 + user requirement: input-encoder authoring must be detection-driven and individually tested | Deterministic input-encoder detector + symbol-scoped authoring flow are implemented and merged (`10J`, `10K`) | Previous required-symbol strategy stalled in `10M1`; remaining gap is research-parity input discovery (leads + semantic tracing + normalization + confirmation) before fixture convergence | 10J (`ACCEPTED`), 10K (`ACCEPTED`), 10M1A-10M1G, 12D |
| G9 | README §9 + user requirement: GT-encoder authoring must be detection-driven and individually tested | Deterministic GT-encoder detector + authoring flow are implemented and merged (`10L`, `10M`) | Previous required-symbol strategy stalled in `10M1`; remaining gap is research-parity GT discovery and confirmation pipeline before fixture convergence | 10L (`ACCEPTED`), 10M (`ACCEPTED`), 10M1A-10M1G, 12E |
| G10 | README §2.3/§8.2 model contract + user requirement: model discovery/selection/fixing must be explicit and tested | Model discovery and authoring-first remediation are implemented and merged (`10B`, `10G`) | Remaining gap is capability-level fixture E2E coverage for model remediation | 10B (`ACCEPTED`), 10G (`ACCEPTED`), 12B |
| G11 | README §9: runtime harness is core validation layer | Harness is optional via env and currently stub-script-based | No high-resolution runtime correctness evidence in normal runs | 11A, 11B |
| G12 | README §9.2: integration-test call coverage enforcement | No AST/runtime enforcement of required decorator calls in integration test | False positives possible despite incomplete wiring | 10N, 10O, 12F |
| G13 | README §12: real `leap` workflow + upload gating + explicit confirmation | `concierge run` has no `leap` orchestration path and no push gate | Cannot complete actual Tensorleap integration flow | 13A |
| G14 | README §13.4 + user request: capability-level fixture validation | Fixture E2E checks only artifact-missing deltas and persistence files | No proof that each authoring capability converges independently | 12A-12H |
| G15 | PLAN quality gate requirement + user request: CI must run capability-level fixture E2E | CI prepares/verifies fixtures but does not run authoring-focused fixture suite on PRs | Regressions can merge without capability-level safety net | 13B |
| G16 | README docs requirements + operator handoff | README/docs do not yet describe detection->suggest->author->validate authoring loop by capability | Onboarding and operations remain ambiguous for the core concierge value proposition | 14A |
| G17 | V1 quality gate requirement: commit approval must run after delta-scoped integration checks | Pre-commit stage is implemented with step-local validation and changed-file syntax gates (`10B1`) | Closed for runtime commit ordering and delta quality gates; keep regression coverage | 10B1 (`ACCEPTED`) |
| G18 | README §10 + user requirement: agent tasks need Tensorleap domain context plus strict scoped edits | Agent context pipeline is implemented and merged (knowledge pack, scoped policy, repo context pack, structured prompt wiring) | Closed for context injection baseline; fixture-level context-quality assertions remain pending | 10C-10F (`ACCEPTED`), 12H |
| G19 | User requirement (2026-03-04 + 2026-03-05): enforce mandatory contract order `preprocess -> input encoders -> GT encoders -> integration test` and stop deriving encoder requirements from integration-test calls | Inspector/planner now gate integration-test authoring behind encoder mapping confirmation and no longer derive encoder requirements from integration-test call graphs | Closed for ordering+requirement-source behavior; keep regression coverage | 10M1F (`DONE`) |
| G20 | `RESEARCH.md` conclusions (2026-03-05): replicate Python input/GT discovery success in Go with staged artifacts and semantic-first evaluation | Go runtime now persists staged discovery artifacts (`lead_pack`, `agent_prompt_bundle`, `agent_raw_output`, `normalized_findings`, `comparison_report`) with framework-agnostic extraction + normalizer/post-processing plus optional runtime-signature corroboration notes/diffs | Closed for current research-parity scope; keep regression coverage | 10M1A-10M1G (`DONE`) |

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
| Step 10A: Contract discovery core | `ACCEPTED` | 2026-03-04 (`user-directed state sync`) | Added deterministic entry-file contract discovery for decorators and integration-test call symbols with graceful path-aware failures. |
| Step 10B: Model discovery and need detection | `ACCEPTED` | 2026-03-04 (`user-directed state sync`) | Added deterministic model candidate discovery from `@tensorleap_load_model` and repo search, ambiguity/missing/format/outside-repo issues, and candidate evidence context without enforcing leap.yaml include/exclude for model artifacts. |
| Step 10B1: Pre-commit integration quality gate (delta-scoped) | `ACCEPTED` | 2026-03-03 (`main`) | Run step-local integration validation and changed-file syntax checks before commit approval is offered. |
| Step 10C: Tensorleap knowledge pack baseline | `ACCEPTED` | 2026-03-03 (`main`, PR #13) | Add checked-in Tensorleap integration knowledge pack + source manifest used for agent context injection. |
| Step 10D: Step-scoped domain slice and edit-scope policy | `ACCEPTED` | 2026-03-03 (`main`, PR #14) | Map ensure-steps to minimal Tensorleap rule slices and strict allowed/forbidden edit scope contracts. |
| Step 10E: Repo-specific context pack assembly | `ACCEPTED` | 2026-03-04 (`PR #15`) | Build deterministic repo-facts context bundles from snapshot/inspector/planner evidence for each agent task. |
| Step 10F: Claude prompt/system-context wiring | `ACCEPTED` | 2026-03-04 (`user-directed state sync`) | Inject stable system prompt plus structured step prompt sections (objective, scope, repo facts, Tensorleap rules, acceptance checks). |
| Step 10G: Model contract authoring flow | `ACCEPTED` | 2026-03-04 (`user-directed state sync`) | Add model-specific authoring objectives and deterministic recommendation/evidence path. |
| Step 10H: Preprocess need detection | `ACCEPTED` | 2026-03-04 (`main`) | Emit preprocess-specific issue codes from real contract inspection. |
| Step 10I: Preprocess authoring flow | `ACCEPTED` | 2026-03-04 (`main`) | Add preprocess authoring objective context, approvals, and evidence expectations. |
| Step 10J: Input-encoder need detection | `ACCEPTED` | 2026-03-04 (`main`, commit `2b90758`) | Detect missing input encoders and per-symbol coverage gaps. |
| Step 10K: Input-encoder suggestion and authoring flow | `ACCEPTED` | 2026-03-04 (`main`, commit `2b90758`) | Render missing-input suggestions to users and pass symbol-level context to authoring executor. |
| Step 10L: GT-encoder need detection | `ACCEPTED` | 2026-03-04 (`main`, commit `2b90758`) | Detect GT encoder deficits and labeled-subset contract violations. |
| Step 10M: GT-encoder suggestion and authoring flow | `ACCEPTED` | 2026-03-04 (`main`, commit `2b90758`) | Render GT-target suggestions and enforce labeled-subset constraints in authoring tasks. |
| Step 10M1A: Input/GT discovery pipeline stage contracts + artifact persistence | `DONE` | 2026-03-05 | Port research pipeline stage model into Go with typed artifacts (`fixture_state`, `lead_pack`, `agent_prompt_bundle`, `agent_raw_output`, `normalized_findings`, `comparison_report`) persisted per iteration. |
| Step 10M1B: Framework-agnostic lead extraction in Go | `DONE` | 2026-03-05 | Implement deterministic code/artifact signal extraction and framework detection (`pytorch`, `tensorflow`, `mixed`, `unknown`) with ranked lead output. |
| Step 10M1C: Semantic investigator prompt/run plumbing | `DONE` | 2026-03-05 | Add discovery-specific agent prompt bundle, metadata capture, and quality gates that treat lead-pack read confirmation as informational when leads are injected directly. |
| Step 10M1D: Multi-shape findings normalizer hardening | `DONE` | 2026-03-05 | Accept schema variants and synonymous fields from agent outputs; fail with actionable diagnostics on malformed or empty normalized payloads. |
| Step 10M1E: Branch-aware input/GT post-processing rules | `DONE` | 2026-03-05 | Add deterministic tokenizer-dict splitting and resolvable branch-priority heuristics while retaining conditional alternatives. |
| Step 10M1F: User mapping confirmation + strict contract-order planner gating | `DONE` | 2026-03-05 | Require user confirmation/adjustment of proposed input/GT mappings, persist accepted contracts, enforce `preprocess -> input -> GT -> integration-test` ordering, and remove integration-test call graph as encoder requirement source. |
| Step 10M1G: Fixture parity + optional runtime signature corroboration | `DONE` | 2026-03-05 | Validate Go discovery pipeline against research-success fixtures (`yolov5_visdrone`, `ultralytics`) with `imdb` edge-case coverage and optional ONNX/Keras signature corroboration. |
| Step 10N: Integration-test wiring need detection | `PENDING` | — | Add AST-based required-call detection for `@tensorleap_integration_test` paths. |
| Step 10O: Integration-test wiring authoring flow | `PENDING` | — | Add targeted authoring objective to wire missing integration-test calls deterministically. |
| Step 11A: Real runtime harness core (Layer 2) | `PENDING` | — | Replace stub harness with runtime script, schema-v1 events, and default-on validation. |
| Step 11B: Harness semantic coverage mapping | `PENDING` | — | Map runtime harness failures to preprocess/encoder/validation issues with per-symbol evidence. |
| Step 12A: Fixture mutation framework for capability-isolated cases | `PENDING` | — | Generate fixture variants with exactly one broken capability per case. |
| Step 12B: Capability E2E (model) | `PENDING` | — | Prove missing-model detection and authoring convergence on fixture cases. |
| Step 12C: Capability E2E (preprocess) | `PENDING` | — | Prove missing-preprocess detection and authoring convergence on fixture cases. |
| Step 12D: Capability E2E (input encoders) | `PENDING` | — | Prove missing-input-encoder detection, suggestion, and authoring convergence on fixtures. |
| Step 12E: Capability E2E (GT encoders) | `PENDING` | — | Prove missing-GT-encoder detection and authoring convergence on fixture cases. |
| Step 12F: Capability E2E (integration-test wiring) | `PENDING` | — | Prove missing required integration-test calls are detected and fixed in fixture flow. |
| Step 12G: Multi-step recovery E2E | `PENDING` | — | Prove deterministic multi-capability convergence across ordered ensure-steps. |
| Step 12H: Capability E2E (agent context injection quality) | `PENDING` | — | Prove agent task context bundles include step-relevant Tensorleap knowledge and repo facts while enforcing strict scope boundaries. |
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
5. Automated Tensorleap-doc auto-refresh tooling; knowledge-pack updates are manual/code-agent assisted PRs when needed.

## Detailed Step Specifications

### Step 10M1B: Framework-agnostic lead extraction in Go (`DONE`)

Objective:

Implement the research-proven framework-agnostic lead extractor in Go with deterministic ranking and framework detection, based on the insights and conclusions in RESEARCH.md

Files to add:

1. `internal/adapters/inspect/framework_leads.go`
2. `internal/adapters/inspect/framework_leads_test.go`
3. `internal/adapters/inspect/framework_signal_weights.go`
4. `internal/adapters/inspect/testdata/framework_leads_cases.json`

Files to modify:

1. `internal/adapters/inspect/input_gt_discovery_pipeline.go`
2. `internal/adapters/inspect/baseline_inspector.go`

Locked behavior:

1. Scanner scores files using train/validation/data-flow/model/loss signals across supported Python code patterns.
2. Detector emits one of `pytorch`, `tensorflow`, `mixed`, or `unknown` with evidence-backed confidence.
3. Signal weights are versioned and overridable through checked-in config constants (not ad-hoc runtime flags).
4. Stage output includes machine JSON (`lead_pack`) and a human-readable summary used in prompts/reports.
5. Lead extraction remains repo-local and deterministic for the same workspace snapshot.

Tests:

1. `TestFrameworkLeadExtractorRanksTrainingPathFilesFirst`
2. `TestFrameworkLeadExtractorDetectsPyTorchTensorFlowMixedAndUnknown`
3. `TestFrameworkLeadExtractorProducesStableOrderingForEqualScores`
4. `TestFrameworkLeadSummaryIncludesEvidenceSnippets`

Validation commands:

1. `go test ./internal/adapters/inspect -run FrameworkLeadExtractor -v`
2. `go test ./internal/adapters/inspect`
3. `go test ./...`

Acceptance criteria:

1. Lead extraction output is deterministic and framework-agnostic.
2. Prompt construction can rely on lead summaries without custom per-framework branches.

Rollback boundary:

- Revert framework-lead extraction implementation and weight configuration only.

---

### Step 10M1C: Semantic investigator prompt/run plumbing (`DONE`)

Objective:

Port the Python semantic investigator flow into Go agent orchestration with explicit quality gates and evidence capture.

Files to add:

1. `internal/agent/context/input_gt_discovery_prompt.go`
2. `internal/agent/context/input_gt_discovery_prompt_test.go`
3. `internal/adapters/inspect/input_gt_investigator.go`
4. `internal/adapters/inspect/input_gt_investigator_test.go`

Files to modify:

1. `internal/adapters/inspect/input_gt_discovery_pipeline.go`
2. `internal/adapters/execute/agent_executor.go`
3. `internal/core/ports/interfaces.go`

Locked behavior:

1. Prompt bundle always embeds lead summary directly and requires evidence-backed, uncertainty-explicit semantic tracing.
2. Investigator task is read-only for discovery stages (no repo edits in this step).
3. Hard failures:
   1. agent tool execution errors
   2. permission failures
   3. missing/malformed final payload
4. `lead_pack_read_success` is informational only when the lead summary is already injected in prompt context.
5. Raw agent output + run metadata are persisted as stage artifacts for normalization.

Tests:

1. `TestInputGTDiscoveryPromptInjectsLeadSummary`
2. `TestInputGTInvestigatorForcesReadOnlyTaskScope`
3. `TestInputGTInvestigatorTreatsLeadPackReadSignalAsInformational`
4. `TestInputGTInvestigatorPersistsRawOutputAndRunMetadata`

Validation commands:

1. `go test ./internal/agent/context ./internal/adapters/inspect -run InputGTInvestigator -v`
2. `go test ./internal/adapters/execute ./internal/core/ports`
3. `go test ./...`

Acceptance criteria:

1. Concierge can run the semantic investigator stage with deterministic prompt structure and persisted raw artifacts.
2. Discovery flow fails fast only on reliability-critical errors.

Rollback boundary:

- Revert semantic investigator prompt/run plumbing only.

---

### Step 10M1D: Multi-shape findings normalizer hardening (`DONE`)

Objective:

Harden Go normalization so semantic findings with schema variants still produce deterministic candidate sets.

Files to add:

1. `internal/adapters/inspect/input_gt_normalizer.go`
2. `internal/adapters/inspect/input_gt_normalizer_test.go`
3. `internal/adapters/inspect/testdata/input_gt_normalizer_cases.json`

Files to modify:

1. `internal/adapters/inspect/input_gt_investigator.go`
2. `internal/adapters/inspect/input_gt_discovery_pipeline.go`
3. `internal/adapters/inspect/input_encoder_contract.go`
4. `internal/adapters/inspect/gt_encoder_contract.go`

Locked behavior:

1. Normalizer accepts synonymous fields such as:
   1. `inputs`, `model_inputs`, `candidate_inputs`
   2. `ground_truths`, `targets`, `candidate_ground_truths`
2. Normalizer handles list- and object-shaped payload variants.
3. Empty normalized candidates are blocking errors with actionable diagnostics.
4. Unknown fields are preserved in diagnostics/evidence but do not crash normalization.
5. Normalized output is stable and sorted for deterministic downstream comparisons.

Tests:

1. `TestInputGTNormalizerAcceptsSynonymousInputAndTargetKeys`
2. `TestInputGTNormalizerAcceptsListAndObjectVariants`
3. `TestInputGTNormalizerRejectsEmptyCandidatesWithActionableError`
4. `TestInputGTNormalizerPreservesUnknownFieldsInDiagnostics`

Validation commands:

1. `go test ./internal/adapters/inspect -run InputGTNormalizer -v`
2. `go test ./internal/adapters/inspect`
3. `go test ./...`

Acceptance criteria:

1. Normalization failures are explicit and debuggable instead of silently producing empty mappings.
2. Research-success outputs from `yolov5_visdrone` and `ultralytics` parse without schema rewrites.

Rollback boundary:

- Revert normalizer hardening and contract-consumer wiring only.

---

### Step 10M1E: Branch-aware input/GT post-processing rules (`DONE`)

Objective:

Port high-impact branch/tokenizer heuristics from research into deterministic post-processing before planner recommendations.

Files to add:

1. `internal/adapters/inspect/input_gt_postprocess.go`
2. `internal/adapters/inspect/input_gt_postprocess_test.go`

Files to modify:

1. `internal/adapters/inspect/input_gt_normalizer.go`
2. `internal/adapters/inspect/model_discovery.go`
3. `internal/adapters/inspect/input_encoder_contract.go`
4. `internal/adapters/inspect/gt_encoder_contract.go`

Locked behavior:

1. Transformer tokenizer dict candidates are expanded into per-key inputs (`input_ids`, `attention_mask`, `token_type_ids`) when those keys are evidenced.
2. If branch selection is statically resolvable, active-branch candidates are ranked first.
3. Alternate-branch candidates remain as conditional suggestions (not discarded).
4. Post-processed candidates carry evidence snippets linking each candidate to repository code.
5. Candidate ordering remains deterministic for identical snapshot inputs.

Tests:

1. `TestInputGTPostProcessSplitsTokenizerDictIntoPerKeyInputs`
2. `TestInputGTPostProcessPrioritizesResolvableActiveBranch`
3. `TestInputGTPostProcessRetainsConditionalAlternateBranchCandidates`
4. `TestInputGTPostProcessPreservesDeterministicOrdering`

Validation commands:

1. `go test ./internal/adapters/inspect -run InputGTPostProcess -v`
2. `go test ./internal/adapters/inspect`
3. `go test ./...`

Acceptance criteria:

1. Branch/tokenizer edge cases no longer collapse to single ambiguous input candidates.
2. Planner/reporter receive explicit primary and conditional candidate sets.

Rollback boundary:

- Revert branch-aware post-processing heuristics only.

---

### Step 10M1F: User mapping confirmation + strict contract-order planner gating (`DONE`)

Objective:

Insert explicit user confirmation for discovered mappings, persist accepted contracts, and enforce hard contract ordering before integration-test wiring.

Files to add:

1. `internal/cli/encoder_mapping_prompt.go`
2. `internal/cli/encoder_mapping_prompt_test.go`

Files to modify:

1. `internal/adapters/inspect/baseline_inspector.go`
2. `internal/adapters/inspect/input_encoder_contract.go`
3. `internal/adapters/inspect/gt_encoder_contract.go`
4. `internal/adapters/planner/policy.go`
5. `internal/core/issue_step_map.go`
6. `internal/core/checklist.go`
7. `internal/cli/run.go`
8. `internal/state/types.go`
9. `internal/state/store.go`
10. `internal/adapters/execute/input_encoder_authoring_context.go`
11. `internal/adapters/execute/gt_encoder_authoring_context.go`

Locked behavior:

1. Planner must never select `ensure.integration_test_contract` while blocking preprocess/input/GT issues exist.
2. `integration_test_missing` remains deferred until preprocess/input/GT contracts are satisfied.
3. Input/GT requirement detection must not use integration-test call graph as required-symbol source.
4. CLI presents proposed input/GT mappings with evidence snippets and allows user confirm/adjust/decline.
5. Accepted mapping contract is persisted in `.concierge/state/state.json` with invalidation keyed to relevant file fingerprints.
6. Steps `10K` and `10M` consume persisted confirmed mapping contracts when present.

Tests:

1. `TestPlannerDefersIntegrationTestStepUntilEncoderContractsPass`
2. `TestInputEncoderDetectorIgnoresIntegrationTestCallGraphForRequiredSymbols`
3. `TestGTEncoderDetectorIgnoresIntegrationTestCallGraphForRequiredSymbols`
4. `TestRunPromptsForInputGTMappingConfirmation`
5. `TestRunPersistsAndReloadsConfirmedInputGTMappingContract`
6. `TestConfirmedMappingContractInvalidatesWhenSourceFilesChange`

Validation commands:

1. `go test ./internal/adapters/planner ./internal/adapters/inspect -run 'IntegrationTestStep|EncoderDetector|MappingContract' -v`
2. `go test ./internal/cli ./internal/state ./internal/core`
3. `go test ./...`

Acceptance criteria:

1. Encoder authoring recommendations are derived from confirmed discovery contracts.
2. Concierge cannot jump to integration-test wiring before preprocess/input/GT contracts are satisfied.

Rollback boundary:

- Revert mapping confirmation UX, planner ordering gate, and mapping-contract state wiring only.

---

### Step 10M1G: Fixture parity + optional runtime signature corroboration (`DONE`)

Objective:

Prove Go discovery parity with Python research wins and add optional runtime signature corroboration as a support signal.

Files to add:

1. `internal/adapters/inspect/runtime_signature.go`
2. `internal/adapters/inspect/runtime_signature_test.go`
3. `internal/e2e/fixtures/case_input_gt_discovery_parity_test.go`
4. `internal/e2e/fixtures/testdata/cases/input_gt_discovery_parity/*.golden.json`

Files to modify:

1. `internal/adapters/inspect/input_gt_discovery_pipeline.go`
2. `internal/e2e/fixtures/fixtures_test.go`
3. `scripts/fixtures_prepare.sh`
4. `scripts/fixtures_verify.sh`
5. `scripts/fixtures_run_checks.sh`

Locked behavior:

1. Runtime signature inspection is optional and runs only when model artifacts are present/hydrated.
2. Supported corroboration signals:
   1. ONNX input names/shapes/dtypes
   2. Keras/TensorFlow input tensor signatures when available
3. Runtime disagreement with code-derived findings is reported explicitly; discovery remains code/semantic-first.
4. Fixture parity gate must cover:
   1. `yolov5_visdrone` (`r009`) as required success case
   2. `ultralytics` (`r010`) as required scale case
   3. `imdb` as edge-case regression (including tokenizer-bundle split expectations)
5. Parity assertions are semantic-first (presence, plausibility, shape/dtype hints); exact names are diagnostic only.
6. Do not start Step `10N` until parity suite passes with legacy `10M1` strategy paths removed.

Tests:

1. `TestRuntimeSignatureInspectorReadsONNXInputsWhenAvailable`
2. `TestRuntimeSignatureInspectorReportsDisagreementsWithoutOverridingCodeFindings`
3. `TestFixtureInputGTDiscoveryParity_Yolov5Visdrone`
4. `TestFixtureInputGTDiscoveryParity_Ultralytics`
5. `TestFixtureInputGTDiscoveryParity_IMDBEdgeCase`

Validation commands:

1. `bash scripts/fixtures_prepare.sh`
2. `bash scripts/fixtures_verify.sh`
3. `go test ./internal/adapters/inspect -run RuntimeSignature -v`
4. `go test ./internal/e2e/fixtures -run InputGTDiscoveryParity -v`
5. `bash scripts/fixtures_run_checks.sh`
6. `go test ./...`

Acceptance criteria:

1. Go discovery pipeline reproduces the Python research success pattern on required fixtures.
2. Discovery contracts are stable enough to drive `10K`/`10M` and unblock `10N` without fallback to integration-test call inference.

Rollback boundary:

- Revert runtime-signature corroboration and parity-suite additions only.

---

### Step 10N: Integration-test wiring need detection (`PENDING`)

Objective:

Enforce required decorator-call wiring under `@tensorleap_integration_test` through deterministic analysis.

Files to add:

1. `internal/adapters/validate/integration_test_ast.go`
2. `internal/adapters/validate/integration_test_ast_test.go`

Files to modify:

1. `internal/adapters/validate/baseline_validator.go`
2. `internal/core/issues.go` (if additional call-graph issue granularity is required)

Locked behavior:

1. Precondition: Steps `10M1A-10M1G` are complete and encoder mapping contracts are available from the discovery pipeline/user confirmation flow.
2. Detect decorators defined but not called in integration-test call path.
3. Detect calls to unknown/non-discovered interfaces.
4. Emit precise issue locations when AST traversal can resolve line/column.

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

### Step 10O: Integration-test wiring authoring flow (`PENDING`)

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

1. Precondition: Steps `10C-10F` are complete; integration-test wiring authoring does not run without injected knowledge pack, scope policy, repo context pack, and structured prompt wiring.
2. Suggestion text lists missing required calls and unknown calls separately.
3. Agent objective includes strict scope: repair call wiring only.
4. Evidence captures required-call set before and after authoring.

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

1. `go test ./internal/e2e/fixtures -run MissingModel -v`
2. `bash scripts/fixtures_run_checks.sh`

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

1. `go test ./internal/e2e/fixtures -run MissingPreprocess -v`
2. `bash scripts/fixtures_run_checks.sh`

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

1. `go test ./internal/e2e/fixtures -run MissingInputEncoders -v`
2. `bash scripts/fixtures_run_checks.sh`

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

1. `go test ./internal/e2e/fixtures -run MissingGTEncoders -v`
2. `bash scripts/fixtures_run_checks.sh`

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

1. `go test ./internal/e2e/fixtures -run MissingIntegrationTestCalls -v`
2. `bash scripts/fixtures_run_checks.sh`

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

1. `go test ./internal/e2e/fixtures -run Composite -v`
2. `bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Concierge demonstrates deterministic multi-capability convergence on fixtures.

Rollback boundary:

- Revert composite E2E test/golden files only.

---

### Step 12H: Capability E2E (agent context injection quality) (`PENDING`)

Objective:

Prove that agent task context injection is step-relevant, Tensorleap-correct, and scope-bounded across capability flows.

Files to add:

1. `internal/e2e/fixtures/case_agent_context_quality_test.go`
2. `internal/e2e/fixtures/testdata/cases/agent_context/*.golden.json`

Files to modify:

1. `internal/e2e/fixtures/fixtures_test.go`
2. `scripts/fixtures_run_checks.sh`

Locked assertions:

1. Preprocess authoring case injects preprocess + model-loading Tensorleap rule sections and selected model-path context.
2. Input-encoder authoring case excludes GT/integration-test-only rule sections from injected context.
3. Agent scope policy in prompt/evidence lists allowed/forbidden edit boundaries for the selected capability.
4. Missing knowledge-pack/context prerequisites fail deterministically before agent invocation.

Tests:

1. `TestFixtureCaseAgentContextPreprocessIncludesRequiredDomainSections`
2. `TestFixtureCaseAgentContextInputEncoderExcludesOutOfScopeSections`
3. `TestFixtureCaseAgentContextMissingPackFailsFast`

Validation commands:

1. `go test ./internal/e2e/fixtures -run AgentContext -v`
2. `bash scripts/fixtures_run_checks.sh`

Acceptance criteria:

1. Context injection quality is proven end-to-end with deterministic fixture assertions.
2. Step-specific context boundaries prevent cross-capability prompt contamination.

Rollback boundary:

- Revert agent-context E2E test/golden files only.

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
3. `make test-fixtures`
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
4. `make test-fixtures`
5. Manual smoke: `go run ./cmd/concierge run --max-iterations=3 --persist`
6. Manual smoke (agent-backed step requiring Claude CLI): `go run ./cmd/concierge run --max-iterations=3 --persist`

Acceptance criteria:

1. Steps `1` through `14B` are `ACCEPTED` after merge to `main`.
2. CI is green with capability E2E checks included.
3. README + docs reflect released functionality.

Rollback boundary:

- Revert only hardening and final checklist/documentation updates introduced by this step.

## Validation and Acceptance

Status semantics:

1. `PENDING`: step not implemented yet.
2. `DONE`: implemented, committed, pushed, PR opened, branch CI green.
3. `ACCEPTED`: merged to `main`.

Phase acceptance condition:

1. Steps `1` through `14B` (including `10A0`, `10A`, `10B`, `10B1`, `10C-10O`, `10M1A-10M1G`, `11A-11B`, `12A-12H`, `13A-13B`, `14A-14B`) are `ACCEPTED`.
2. Capability-level fixture E2E jobs are part of required CI and green.
3. Concierge can perform user-approved detection->suggest->author->validate loops for model/preprocess/input-encoder/GT-encoder/integration-test wiring and complete guarded upload workflow end-to-end.

## Idempotence and Recovery

1. Keep one commit per step for clean rollback.
2. If CI fails, fix within current step scope only.
3. Revert only the failing step commit when rollback is required.
4. Fixture scripts and `.concierge` persistence paths must remain safe to rerun.

## Surprises & Discoveries

- Steps `10H-10M` are implemented and merged, so preprocess/input/GT detector + authoring paths now emit capability-specific issue families and recommendations from live contracts.
- The first `10M1` strategy failed to converge because post/pre oracle-only required-symbol inference was not enough to reproduce the Python discovery success pattern.
- The plan now requires a staged discovery port (`10M1A-10M1G`) with explicit tests at each stage before integration-test wiring work continues.
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
- Decision: Sequence detection and authoring steps (`10A-10O`) before runtime harness deepening and capability E2E assertions (`11A-12H`).
  Rationale: Harness and fixture suites must validate capability-level behavior that is already concretely implemented and wired in planner/executor paths.
  Date/Author: 2026-03-03 / assistant.
- Decision: Insert a dedicated delta-scoped pre-commit integration quality gate step (`10B1`) before authoring-heavy slices continue.
  Rationale: Commit approval must run only after step-local integration validation and changed-file syntax checks so broken step deltas are not committed prematurely.
  Date/Author: 2026-03-03 / user + assistant.
- Decision: Use identifier `10B1` for the pre-commit gate.
  Rationale: Keep a dedicated pre-commit quality gate between model discovery and context/authoring slices so commit approval cannot bypass step-local validation.
  Date/Author: 2026-03-03 / assistant.
- Decision: Introduce explicit agent context-injection steps (`10C-10F`) using a checked-in Tensorleap knowledge pack and repo-specific context bundles; do not add runtime auto-refresh tooling.
  Rationale: Agent tasks must be domain-correct and scope-bounded without adding online/runtime volatility; knowledge updates can be manual/code-agent assisted via normal PR flow.
  Date/Author: 2026-03-03 / user + assistant.
- Decision: Insert blocking Step `10M1` immediately after `10M` and before `10N` to correct requirement ordering and redesign encoder requirement sourcing.
  Rationale: Current behavior can jump to integration-test blockers before encoder contracts are grounded, and encoder requirement inference is incorrectly coupled to integration-test calls.
  Date/Author: 2026-03-04 / user + assistant.
- Decision: Implement Step `10M1` with fixture-ground-truth iterative development (`post` as oracle, `pre` as detection target), and gate completion on pre/post contract-match invariants across all fixtures.
  Rationale: Encoder detection quality must be empirically anchored to real repository pairs, not heuristics validated in isolation.
  Date/Author: 2026-03-04 / user + assistant.
- Decision: Replace the single blocking `10M1` step with a research-parity sequence (`10M1A-10M1G`) that ports Python lead extraction, semantic investigation, normalization, branch heuristics, and confirmation flow into Go with per-step tests.
  Rationale: The previous strategy failed; reproducing proven research behavior requires explicit stage parity and deterministic test gates.
  Date/Author: 2026-03-05 / user + assistant.

## Outcomes & Retrospective

Current state: baseline architecture through Step `10M` is accepted and merged; the original `10M1` strategy failed and has been replaced by blocking sequence `10M1A-10M1G` to port Python-proven input/GT discovery into Go before continuing to integration-test wiring (`10N-10O`), runtime harness deepening (`11A-11B`), capability E2E/CI hardening (`12A-13B`), and release documentation/hardening (`14A-14B`).

Primary residual risks:

1. Input/GT discovery quality is currently below research parity until `10M1A-10M1G` lands, which can still mis-prioritize integration-test checks and break encoder authoring flow.
2. Agent discovery output normalization can regress if schema variance and branch heuristics are not tested independently.
3. Capability E2E may become flaky unless fixture mutation tooling and discovery parity cases are strictly deterministic.

Mitigations:

1. Complete `10M1A-10M1G` first (staged discovery parity + strict preprocess->input->GT->integration-test ordering + confirmed mapping contracts), then proceed with `10N-10O`.
2. Keep existing context-injection pipeline (`10C-10F`) under regression coverage while discovery prompt/normalization/post-processing paths are introduced.
3. Introduce deterministic fixture case generation before capability E2E tests (`12A` before `12B-12H`) and require discovery parity fixtures in `10M1G`.

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
