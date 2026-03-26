# Concierge Design

`README.md` is the repository entry point for users and contributors. This file is the implementation-context document that maintainers and coding agents should read before planning or changing code.

It preserves the parts of the original README that are still useful for implementation, but trims away design assumptions that no longer match the current product shape.

## Product Goal

Concierge is a deterministic terminal orchestrator for Tensorleap integration work. Its job is to help an operator move a real repository from partial or missing Tensorleap integration toward a validated, upload-ready state.

At a high level, Concierge is responsible for:

- inspecting repository and runtime state
- identifying the next blocking integration step
- executing that step deterministically or through a task-scoped agent run
- validating the result with local checks
- keeping repo changes reviewable and user-approved

Concierge is not meant to replace the Leap CLI. It wraps the authoring and validation loop that usually happens before `leap push`.

## Current Product Boundary

As of 2026-03-26, the implemented shape is narrower and more concrete than the original design draft:

- target layout: root-level `leap_integration.py` and root-level `leap.yaml`
- local runtime boundary: Poetry-managed Python projects only
- model expectations: `.onnx` or `.h5`, with batching-compatible inputs and outputs
- v1 emphasis: the mandatory onboarding path for preprocess, input encoders, GT encoders, model loading, integration test wiring, and upload readiness
- optional metadata, visualizers, metrics, loss, and custom-layer support are still secondary surfaces rather than the main authoring target
- the current agent runner shells out to the local `claude` CLI

These boundaries matter because much of the planner, validator, and user messaging assumes guide-native Tensorleap workflows rather than binder-era layouts or arbitrary Python environment managers.

## Operator Flow

The outer loop is:

1. capture a fresh workspace snapshot
2. inspect layout, runtime readiness, model leads, and integration status
3. choose one deterministic ensure-step
4. execute it through a deterministic action or a task-scoped agent run
5. validate the result
6. report evidence and either stop, prompt the user, or continue to the next iteration

The stop conditions exposed today are success, max iterations reached, cancellation, interrupted agent execution, required user action, and repeated no-progress iterations.

## Code Map

The codebase is organized around a deterministic core plus adapters:

- `cmd/concierge`
  - CLI entrypoint
- `internal/cli`
  - Cobra commands, prompts, terminal copy, and output rendering
- `internal/orchestrator`
  - outer `run` loop and stop-reason handling
- `internal/core`
  - shared types, ensure-step IDs, issue catalogs, and domain logic
- `internal/adapters/snapshot`
  - workspace and runtime snapshotting
- `internal/adapters/inspect`
  - repo inspection, runtime signals, model discovery, and integration-shape detection
- `internal/adapters/planner`
  - deterministic next-step selection
- `internal/adapters/execute`
  - deterministic file/template actions, approval flow, and agent task packaging
- `internal/adapters/validate`
  - guide validation, harness execution, heuristics, and issue mapping
- `internal/adapters/report`
  - persisted and terminal-facing reports
- `internal/observe`
  - structured live-event rendering and recording
- `internal/agent`
  - Claude runner, guarded repo view, prompt contract, and transcript artifacts
- `internal/e2e/fixtures`, `fixtures/`, `scripts/`, `QA/`
  - fixture corpus, support scripts, QA harness, and higher-level validation flows

When looking for product behavior, start from the CLI command in `internal/cli`, follow the orchestration entrypoint in `internal/orchestrator`, and then trace into the relevant adapter family.

## Design Assumptions That Still Matter

### Runtime Is Part Of Correctness

Concierge should not treat the ambient shell environment as the source of truth. The target repository's Poetry runtime is part of the product contract.

Current expectations:

- local Python execution goes through the selected Poetry environment
- runtime readiness is validated before relying on local checks
- dependency or interpreter drift is part of the evidence, not background noise

### Guide-Native Layout Is The Authoring Target

The planner and validators are built around the guide-native entrypoint-first workflow:

- `leap.yaml` is mandatory
- `leap.yaml.entryFile` should point at `leap_integration.py`
- newly scaffolded or repaired integrations should converge on the canonical root-level entrypoint
- validation is call-driven, so a decorator definition is not enough by itself
- `@tensorleap_integration_test` should be scaffolded early and extended continuously as preprocess, input encoders, GT encoders, model loading, and optional surfaces are added
- guide status rows should be interpreted as end-to-end wiring signals, not as a plain inventory of which decorators exist in the file

### Concierge Stays Deterministic Even When An Agent Helps

The product design assumes:

- the planner chooses the next step
- agent tasks are scoped to one objective
- the agent can explore inside the prepared repo view, but that does not make it the orchestrator
- repo diffs and validation results decide whether a step is accepted

This is why the code is split between deterministic adapters and a relatively narrow `internal/agent` package.

## Validation Model

Concierge should avoid declaring success based on skeleton files alone. The important layers are:

- surface inventory for repo diagnostics and planning
- guide-native local validation for `leap_integration.py`
- structured parse and harness-level checks
- bounded multi-sample coverage across preprocess, input encoders, GT encoders, and model wiring
- heuristics that catch suspiciously shallow integrations

The validator should use different signals for different questions:

- `check_dataset()` and related parser payloads are the primary contract-level truth for preprocess, input encoder, and GT encoder presence plus first-sample execution
- `@tensorleap_integration_test` and model-loading checks are the primary end-to-end wiring truth
- local guide status tables are supporting evidence, but they are not authoritative enough to override stronger parser or runtime proof on their own

This matters because guide status rows can regress when downstream wiring breaks. Concierge should not snap back to the earliest red row if more concrete downstream evidence already identifies the current blocker.

The validator should answer both:

- what failed
- how far the repo actually progressed through the intended integration flow

Persisted evidence under `.concierge/` is part of that debugging surface when `concierge run --persist` is used.

## Agent Execution Model

The current agent path is intentionally constrained:

- Concierge runs the local `claude` CLI for task-scoped objectives
- the prepared agent view is limited to the relevant repo content
- bare environment mutation commands are guarded against in agent tasks
- Concierge treats the agent's work as tentative until diff review and validation pass

This design keeps the agent useful for repo exploration and edits without moving orchestration authority away from Concierge itself.

## Testing And QA Surfaces

There are three main verification layers in the repo:

- `make test`
  - Go unit and adapter tests plus Python QA harness tests
- `make test-fixtures`
  - fixture preparation, verification, and fixture E2E coverage
- `make qa`
  - subjective terminal QA through `QA/qa_loop.py`

The fixture system exists to give Concierge deterministic before-and-after integration states. The QA loop exists to evaluate the real terminal experience rather than only internal APIs.

Useful companion docs:

- `QA/QA_LOOP.md`
- `fixtures/README.md`
- `fixtures/cases/README.md`
- `GUIDE.md`

## Deferred Or Open Areas

These topics still exist, but they should not be treated as already solved:

- supporting local runtime managers beyond Poetry
- broader help for optional Tensorleap assets
- stronger sandboxing and repo-boundary enforcement for agent runs
- a broader public installation and packaging story beyond building from source and tag-driven releases

If code or docs imply these are complete, that is usually a sign that the docs drifted away from the current implementation.
