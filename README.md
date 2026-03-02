# Concierge — Terminal Integration Assistant for Tensorleap

**Document type:** Design document (handoff-ready for implementation)
**Primary audience:** Engineering + agent-assisted coding implementation
**Status:** Draft (implementation-oriented, no step-by-step implementation plan)
**Implementation status (2026-02-25):** Step 1 baseline is complete (Go module initialized, inherited CI removed, minimal Go CI added with Go `1.24.x`).

---

## 1) Problem statement

Tensorleap integrations are powerful but often require a pre-sales/solutions engineer to sit with a prospective customer and incrementally wire up:

* dataset preprocessing and sample fetching
* input encoders and ground-truth encoders
* optional metadata/visualizers/metrics/loss
* a mandatory integration test that connects these interfaces to model inference
* CLI upload and server-side validation

This work is hard to “one-shot” with an LLM prompt because the required context spans:

* the customer repository (often large, multi-project, inconsistent)
* Tensorleap’s integration contracts and rules
* step ordering and prerequisites (server mounts, `leap.yaml` include/exclude, CLI auth)
* iterative debugging when real code and data paths don’t match assumptions

**Goal:** build a deterministic, cross-platform terminal tool called **Concierge** that drives the integration loop, delegates *small, focused* editing tasks to a coding agent (starting with **Claude Code**), and provides a guided user experience with explicit confirmations and auditable git commits.

---

## 2) Background: what “Tensorleap integration” requires

Concierge must align with Tensorleap’s documented integration flow and runtime expectations.

### 2.1 Expected integration artifacts and flow

Tensorleap documents an expected folder structure for code integration that includes:

* `leap_binder.py` — integration script
* `leap.yaml` — mandatory configuration
* `integration_test.py` — local integration test script (name may vary, concept is mandatory)

This structure is explicitly presented as the “expected file structure” for a Tensorleap code integration. (See **Tensorleap Integration** guide.)

### 2.2 `leap.yaml` is mandatory and defines upload boundaries

`leap.yaml` is mandatory and located at project root. It includes, among other fields:

* `entryFile`: path to a python file that includes the integration test
* `include` / `exclude`: which files are uploaded; supports wildcards
* optional `projectId` and `secretId`
* `pythonVersion` (used when supplying requirements.txt)

Important rule: for initial integration / uploading to a new location, `leap.yaml` should **not** include identifiers such as `projectId`, `secretId`, etc., because the CLI can prompt and populate them interactively.

### 2.3 Model constraints

Tensorleap-compatible models are:

* `.onnx` or `.h5`
* inputs and outputs must include a batch dimension: `[Batch, ...]` (dynamic or fixed batch supported)

### 2.4 Integration script structure: preprocess + encoders (+ optional hooks)

Tensorleap describes the integration script as:

* **Preprocess function**: runs once, returns a list of `PreprocessResponse` objects for dataset slices (train/validation/test/unlabeled).
  At least **train + validation are mandatory**.
* **Input encoders**: called per-sample `(idx, preprocess_response)`; registered with `@tensorleap_input_encoder(...)`.
* **Ground truth encoders**: called per-sample; registered with `@tensorleap_gt_encoder(...)`.
* Optional: metadata functions, custom visualizers, metrics, loss, custom layers.

Nuance: for the *unlabeled* set, GT encoders + metrics/loss won’t run.

### 2.5 Integration test is mandatory and has strict rules

Tensorleap’s **integration test** is mandatory and serves two purposes:

* locally simulate and validate the data flow (types/shapes/values/visualizations)
* instruct Tensorleap how to connect integration interfaces to model inference and analysis

It requires implementing:

* `@tensorleap_load_model(...)` (supported model formats `.h5` or `.onnx`)
* `@tensorleap_integration_test()` (wires everything together)

Critical constraint: only decorators **called within** `@tensorleap_integration_test` are used in platform analysis; decorators defined but not called won’t be executed.

Tensorleap also documents specific “dos/don’ts” for the integration test (what logic belongs where, and avoiding manual batch-dim manipulation).

### 2.6 CLI upload prerequisites and common failures

Concierge must understand and validate the CLI upload path:

* requires CLI installed & authenticated
* requires a reachable Tensorleap server via port **4589**
* upload from repo root containing `leap.yaml` using `leap push`
* code uploads first, then model uploads
* server runs preprocess, fetches input + GT, runs metadata for index 0 as part of validation

Common failures include:

* contract mismatch (shapes/types)
* missing files not included in `leap.yaml`
* data path access problems: assets must live under folders mounted to the server; `leap server info` lists mounted folders under `datasetvolumes`

Secrets: `leap secrets set` associates a secret; integration code accesses it via `AUTH_SECRET`.

---

## 3) Goals and non-goals

### 3.1 Goals

**G1 — Deterministic orchestration, iterative convergence**
Concierge controls the integration workflow as a deterministic loop: inspect → diagnose gaps → propose next step → apply fix → validate → repeat. The user is not responsible for “what next” ordering.

**G2 — Focused agent tasks (small scope; not “big chains”)**
Concierge delegates bounded tasks to a coding agent (Claude Code first). Tasks should be narrow in *objective*, but the agent must be allowed to explore the repo sufficiently to succeed.

**G3 — User-in-the-loop with explicit confirmations**
Concierge asks for confirmation before applying repo changes and before pushing assets to a Tensorleap server. Concierge also supports the user participating in edits in the same branch.

**G4 — Cross-platform standalone binary**
Implement in Go for portability and easy installation (single binary).

**G5 — Auditable repo changes with git diffs and step commits**
Concierge operates on a new git branch and commits after each accepted step, producing an audit trail.

**G6 — High-resolution validation without “LLM equivalence oracle”**
Correctness is primarily established via runtime behavior and contract validation, not semantic branch comparison by another LLM.

**G7 — Test-driven development friendliness**
Architecture supports deterministic tests for the orchestrator independent of LLM variability.

### 3.2 Non-goals (initial scope)

* Fully automated end-to-end integration without user input
* Replacing the Tensorleap CLI
* Building a full IDE or GUI
* Installing the Tensorleap server automatically (Concierge validates and guides)

---

## 4) Product experience and UX principles

### 4.1 Core interaction model

Concierge behaves like an interactive terminal “integration driver”:

* continuously evaluates repository state (and detects changes between iterations)
* proposes the next deterministic action
* delegates code edits to the coding agent only when needed
* always shows diffs and asks the user to approve changes
* commits each approved step to the integration branch

### 4.2 Minimal, structured user prompts

Prefer:

* multiple-choice and confirm/deny
* “select project root” from a short list of detected candidates
* explicit “approve these changes?” gates

### 4.3 Repository changes mid-run

Concierge assumes the repo can change at any time (user edits). Therefore:

* each loop iteration begins by capturing a fresh snapshot
* cached decisions are invalidated if their dependencies change (files, git HEAD, requirements)
* Concierge tolerates partial progress

---

## 5) High-level architecture

### 5.1 Core components (Go)

**1) Snapshotter**
Captures a **Workspace Snapshot** (see §6) including git state, file hashes for relevant files, and key config fingerprints.

**2) Inspector**
Reads the snapshot and produces an **Integration Status Report**:

* what artifacts exist (`leap.yaml`, entry file, integration test, binder, requirements)
* what’s missing or invalid
* where failures occurred (CLI, server, validation)
* what evidence exists (logs, error traces)

**3) Planner (deterministic)**
Selects the next action as an ordered set of **Ensure Steps**. Planner chooses *one primary next action* at a time to preserve reliability.

**4) Executor**
Performs the chosen ensure-step via:

* deterministic action (write templates, run CLI commands)
* agent-assisted action (Claude Code session)
* collects evidence and returns a structured result

**5) AgentRunner (agent host / collaboration session)**
Runs Claude Code as a subprocess in a *collaborative* mode:

* the agent is allowed to **explore the repository** (read/search/navigate) to complete tasks reliably
* the agent may request to run commands (e.g., `grep`, `find`, tests); Concierge can gate these through **user approval** (recommended)
* the agent edits files in-repo; Concierge reviews changes via `git diff`
* agent interaction may be **multi-turn** (the agent can ask clarifying questions), while Concierge still controls the outer orchestration loop

> Design intent: leverage Claude Code’s strengths (repo investigation + tooling), while keeping Concierge as the deterministic driver and keeping the user in control of impactful actions.

**6) GitManager**
Ensures clean working tree, creates branch, captures diffs, supports revert-on-reject, and commits approved changes with structured messages.

**7) Validator**
Runs local validation:

* Tensorleap integration test checks (mandatory)
* Concierge runtime harness (coverage + semantic checks) (§9)
* optional CLI-level checks where appropriate

**8) Reporter**
Presents status/diffs/validation results and writes machine-readable artifacts for debugging and tests.

---

## 6) Data model: snapshots, state, and evidence

### 6.1 Workspace Snapshot (immutable per iteration)

Snapshot should include:

* **Repo identity**

  * absolute path
  * git root
  * current branch
  * HEAD commit hash
  * working tree status (clean/dirty)
* **Key files + fingerprints**

  * `leap.yaml` (if present): hash + parsed structure
  * entry file referenced by `leap.yaml.entryFile` (if resolvable)
  * requirements files (present + hash): `requirements.txt`, `pyproject.toml`, etc.
* **Environment**

  * OS/arch
  * python executable(s) + versions (discoverable)
  * leap CLI presence + version (discoverable)
* **Tensorleap connectivity**

  * ability to reach server (CLI probe, port expectations)
  * auth presence (via CLI status checks)

### 6.2 Persistent Concierge State (mutable across iterations)

Store in e.g. `.concierge/state.json`:

* selected project root (for monorepos)
* chosen integration layout
* “integration plan” expectations:

  * expected dataset subsets and their semantics
  * expected inputs and GT targets (as discovered/confirmed)
  * expected model artifact strategy
* last successful ensure-step
* last known good validation outputs

State must be invalidated if relevant files change.

### 6.3 Evidence bundle

Each iteration should produce:

* command outputs (stdout/stderr) for executed CLI commands
* validation logs
* diff summaries and commit hashes
* agent session transcript (optional but recommended for debuggability)

---

## 7) Integration layouts and repo variability

### 7.1 Repo variability assumptions

Concierge must handle:

* monorepos with multiple models/datasets
* non-working or partially broken training code
* multiple entry points
* custom layers and external code requirements
* secrets required for data access

### 7.2 Integration layout modes

Tensorleap documents a root-level expected structure (`leap_binder.py`, `leap.yaml`, `integration_test.py`). Some quickstarts reference a `.tensorleap/` structure in CLI workflows.

Concierge must **not** hardcode one layout:

* detect existing integration artifacts
* if none exist, offer a default scaffold aligned with the “expected folder structure”
* store the chosen layout in Concierge state

---

## 8) Orchestration loop and ensure-steps

Concierge implements the iterative loop:

* capture snapshot
* inspect integration state
* ensure prerequisites and required artifacts
* detect inputs/targets, ensure encoders
* ensure integration test coverage and local validation
* confirm upload and perform `leap push`
* loop until success or max iterations

### 8.1 Ensure-step design pattern

Every ensure-step:

* **Check:** deterministic condition check
* **Explain:** what’s missing and why it matters
* **Fix:** deterministic action or agent-assisted session
* **Verify:** acceptance checks
* **Record:** evidence + optional commit

### 8.2 Canonical ensure-step categories

**A) Repository context**

* ensure git repo
* ensure clean working tree (or user decision)
* ensure integration branch

**B) Tensorleap CLI and server**

* ensure CLI installed and authenticated
* ensure server reachable; `leap server info` yields mounted dataset volumes
* ensure secrets context (if needed) is correctly configured

**C) `leap.yaml` correctness**

* exists, parseable, has valid `entryFile`
* include/exclude covers required uploaded files
* initial integration nuance: don’t force populated IDs prematurely

**D) Integration script correctness**

* preprocess returns at least train+validation `PreprocessResponse`
* input encoders exist and execute reliably across multiple indices
* GT encoders exist and execute on labeled subsets
* optional hooks where useful

**E) Integration test contract and coverage**

* integration test exists and follows documented rules
* it calls all relevant decorators that must be used in analysis

**F) Upload readiness**

* requirements correctness (if used)
* model artifact is `.onnx`/`.h5` and compatible
* explicit user confirmation before `leap push`

---

## 9) Validation strategy: avoid “skeleton-only” false success

A critical requirement: Concierge must not treat “decorators exist” + “a minimal test passes once” as completion.

### 9.1 Three layers of validation

**Layer 1: Surface inventory (fast signal, not sufficient)**
Discover what integration interfaces appear present across the upload boundary defined by `leap.yaml` include/exclude + entry file. This may use:

* static scanning (best-effort)
* runtime import and introspection (when environment supports)

Purpose:

* diagnostics
* planning (what’s missing)
* user messaging

**Layer 2: Concierge runtime coverage harness (semantic + high-resolution)**
Concierge runs a deterministic harness that:

* runs preprocess and validates subsets; train+validation are mandatory
* selects multiple indices per subset (bounded)
* calls every input encoder across those indices
* calls every GT encoder across those indices for labeled subsets
* optionally calls metadata/visualizers if defined
* enforces invariants:

  * no exceptions
  * finite outputs
  * sensible dtypes
  * shapes compatible with the model (batch dimension expectations)

This creates **high-resolution evidence** like:

* “input encoder X executed successfully on indices [0,1,2]”
* “GT encoder Y missing or throws exception at index 1”
* “model input shape mismatch”

**Layer 3: Stub-detection heuristics (“anti-cheat”)**
Practical heuristics to catch stubby integrations:

* variation checks across indices (detect constant outputs)
* non-empty train/val warnings
* suspicious constant labels
* file/path existence checks and server-mount compatibility checks

### 9.2 Integration test coverage is a correctness dimension

Tensorleap states only decorators called in `@tensorleap_integration_test` are used in analysis. Concierge must therefore:

* track what must be wired (expected encoders/predictions/metrics)
* enforce that the integration test calls them
* ensure integration test follows documented constraints

---

## 10) Agent collaboration model (coding agent session)

### 10.1 Design intent

Concierge should **unleash the selected coding agent’s strengths** (Claude Code in v1):

* repo navigation and understanding
* searching (grep/ripgrep), reading multiple files
* making reasonable refactors across files when necessary for a sensible integration

At the same time:

* Concierge remains the deterministic orchestrator
* the user remains in control of impactful operations (especially command execution and commits)
* correctness is validated by Concierge (runtime harness + integration test), not by trusting the agent’s claims

### 10.2 Session model: interactive, task-scoped, user-visible

AgentRunner should support a task-scoped session that can be **multi-turn**:

* Concierge starts a session with a specific objective (“Implement input encoder for input X”) and constraints (“must satisfy Tensorleap decorator + harness invariants”).
* The agent may ask questions or request repo exploration.
* Concierge surfaces agent requests to the user (and can require confirmation for certain actions).
* Concierge ends the session when the acceptance check passes or when the user chooses to stop.

**Important:** “Focused” means *one objective*, not “agent can’t explore.” The agent can explore broadly within the repo to accomplish the objective.

### 10.3 Repo access policy: prioritize “in-repo freedom”, restrict “outside repo”

Concierge should allow the agent to:

* read any file **inside the repository**
* edit any file **inside the repository** as needed (including refactors)
* run repo-local analysis commands (search, tests) **with user approval** (recommended)

Concierge should explicitly prohibit:

* reading/writing outside the repo root (unless user explicitly opts in)
* accessing secrets or user home directories
* network exfiltration (if the agent tooling supports network, this should be addressed explicitly in security posture)

**Note:** enforcing “repo-only” access is easiest when Concierge *mediates* file reads and command execution; if running an external agent tool that can execute commands itself, Concierge must document that hard guarantees may be limited without OS/container sandboxing.

### 10.4 Change application: prefer `git diff` review over patch-format enforcement

Concierge does **not** require a patch output format as a primary mechanism.

Recommended flow:

1. agent edits files in the working tree
2. Concierge shows `git diff` (and a file list + summary)
3. user approves or rejects
4. on approval → Concierge commits with structured message
5. on rejection → Concierge reverts changes (hard reset to last commit on branch)

Optional: if the agent tooling naturally emits patches, Concierge may support patch-apply mode, but it is not required.

### 10.5 Guardrails that still matter (without arbitrary “max files” limits)

Instead of imposing arbitrary limits (max files touched), Concierge should use **outcome-based guardrails**:

* every task has deterministic acceptance checks
* any changes must be reviewable via `git diff`
* user approves before commit
* Concierge runs validations immediately after changes
* if validations regress, Concierge either reverts or enters a guided fix loop

This preserves power while maintaining reliability.

### 10.6 Agent dependency and credential strategy (v1 baseline)

For **v1**, Concierge assumes the coding-agent runtime is already available in the user environment (starting with Claude Code), including any required user authentication/API credentials.

Rationale for v1:

* fastest path to a deterministic orchestrator with clear trust boundaries
* avoids introducing backend credential brokerage in the first release
* keeps ownership of provider spend and access with the user

Future provisioning models are intentionally deferred. Candidate directions (managed access, BYOK-first, or hybrid) are tracked as open questions in section 17.

---

## 11) Git workflow and change control

### 11.1 Branch workflow (default)

Concierge should:

* require the repo to be clean OR ask user to resolve/stash/commit
* create a new branch (e.g. `concierge/<timestamp>` or user-specified)
* apply changes stepwise, showing `git diff` each time
* request explicit approval for each diff
* commit each approved step with structured messages

This produces an auditable, bisectable history and supports “user participates in editing.”

### 11.2 Worktrees are an open question

Some teams may prefer `git worktree` isolation; others prefer working directly in the user’s current workspace so the user can actively edit alongside Concierge.

**Design stance:** treat worktrees as an explicitly documented open decision:

* keep the architecture compatible with a future “sandbox worktree mode”
* do not require worktrees for initial implementation

---

## 12) Upload and platform interaction

### 12.1 CLI operations Concierge must support (conceptually)

Concierge doesn’t reimplement `leap`, but must run it and guide users through:

* initialization/login flows (`leap init`, `leap auth login`) per CLI quickstart
* `leap push` from repo root containing `leap.yaml`
* `leap server info` to identify mounted dataset folders (`datasetvolumes`)
* secrets association via `leap secrets set` and `AUTH_SECRET`

### 12.2 Upload gating

Concierge must never push by default. Require:

* integration test success (mandatory)
* Concierge harness success (coverage + semantic checks)
* server mount/path accessibility checks where possible
* explicit user confirmation

---

## 13) Testing strategy (designed for TDD and determinism)

### 13.1 Fixture repos as test data

Your organization `tensorleap-hub` contains many integrated example repositories that can serve as gold fixtures.

Approach:

* create “pre-integration” branches/tags by removing integration artifacts
* run Concierge against the pre-integration variant
* validate outcomes against behavior + contract, not exact code diffs

### 13.2 Docker-based test isolation

Use Docker to run tests in clean environments:

* clone fixture repo at known tag
* install Concierge binary
* run Concierge in non-interactive mode with scripted answers
* run harness + integration test
* optionally validate CLI interactions against a test server

### 13.3 Test tiers

**Tier 1: Unit tests (no agent, no Docker required)**

* snapshots, invalidation rules
* parsing and include/exclude resolution
* planner step selection
* git operations (diff, revert, commit)
* evidence recording

**Tier 2: Orchestrator integration tests (no live agent)**

* stub AgentRunner behavior deterministically
* validate state-machine correctness, gating, and validations

**Tier 3: Live-agent end-to-end tests (optional / slower)**

* real Claude Code sessions
* scripted user answers (as much as possible)
* correctness asserted via harness + integration test + upload boundary checks

### 13.4 Behavior fingerprints vs “truth branch”

Exact repo diffs are brittle. For fixtures, compare runtime behavior:

* run preprocess + encoders on fixed indices
* capture fingerprints: subset sizes, shapes, dtypes, stats
* compare against known-good integration branch

This provides high-resolution validation without requiring an LLM equivalence judge.

---

## 14) Security and privacy considerations

### 14.1 Secrets and sensitive data

* never print secrets
* never include secret values in agent context
* implement redaction rules for logs and agent transcripts

### 14.2 Agent boundary posture

Concierge should make the boundary explicit:

* **Preferred (strong boundary):** Concierge mediates file reads and command execution, enforcing “repo-only” access and explicit user approval for commands.
* **Alternate (weaker boundary):** external agent tool runs directly in the workspace; Concierge relies on user review and git diff gating but cannot provide strong guarantees without sandboxing.

Document this clearly so users understand the trust model.

### 14.3 Safe command execution

When commands are executed (CLI, python, grep, tests):

* show the command (or log it)
* require user approval for non-trivial commands (recommended)
* apply timeouts where feasible
* store outputs in evidence bundle

---

## 15) Observability and reporting

Concierge should produce:

1. **Human-readable summary**:

* current status
* missing pieces
* proposed next step
* diff summary
* validation results

2. **Machine-readable report** (JSON):

* snapshot ID
* executed ensure-step
* agent session metadata (optional)
* validation results and error traces
* commit hashes

---

## 16) Methodological guidance for implementation (without an implementation plan)

### 16.1 Engineering principles

* **Deterministic core:** planner/executor/validator are deterministic and testable without the agent.
* **Small, composable ensure-steps:** each step is idempotent and has a precise acceptance check.
* **TDD-first:** ensure-steps are added with tests (Tier 1/2).
* **Agent as collaborator, not driver:** agent helps edit/understand; Concierge decides ordering and completion.
* **Evidence-driven progress:** “done” is defined by harness outputs + integration test + optional push success.

### 16.2 Agent-session hygiene rules (revised)

* One objective per session (task-scoped), but allow repo-wide exploration inside the repo.
* Encourage the agent to ask clarifying questions when repo ambiguity blocks progress.
* Use git diff + user approval as the primary change-control mechanism.
* Run acceptance checks immediately after changes.
* Prefer mediating command execution through Concierge (user-approved) when feasible.
* Keep transcripts and evidence for debugging and repeatability.

---

## 17) Open questions and design risks

### 17.1 Worktrees vs branch-in-place

Open decision. Default to branch-in-place, but keep design extensible for sandbox/worktree mode.

### 17.2 Multi-project repos and project root selection

Need heuristics to propose candidates; user-confirmed selection.

### 17.3 Python environment strategy

Concierge must run harness/integration test. Options include:

* reuse user environment
* create managed venv under `.concierge/`
* Docker validation mode

### 17.4 Agent boundary enforcement

How strongly Concierge can enforce “repo-only access” depends on the agent integration mode:

* mediated tool-execution allows strong enforcement
* direct external agent tool may require sandboxing for strong guarantees

### 17.5 Custom layers and external code

Models with custom layers may require special handling; Concierge should treat this as an advanced ensure-step.

### 17.6 Coding agent provisioning and rollout model (v2/v3)

Open decision. v1 is fixed to local tool availability + user-managed credentials.

Questions to resolve before broader rollout:

* should Concierge offer a **gated managed mode** for select users who do not yet have coding-agent tooling/accounts
* should public rollout be **BYOK-only** or **BYOK + optional managed fallback**
* if managed mode exists, what backend architecture should issue short-lived usage tokens and enforce spend/rate limits
* what policy should determine model/backend selection when preferred local tools are unavailable

---

## Appendix A — Key Tensorleap references (URLs)

```text
Guides index:
https://docs.tensorleap.ai/guides

Tensorleap Integration (expected folder structure + flow):
https://docs.tensorleap.ai/tensorleap-integration

Writing Integration Code (overall structure + secrets + persistent storage):
https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code

Preprocess Function (train+val mandatory):
https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/preprocess-function

Input Encoder:
https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/input-encoder
https://docs.tensorleap.ai/tensorleap-integration/python-api/code_loader/decorators/tensorleap_input_encoder

Ground Truth Encoder:
https://docs.tensorleap.ai/tensorleap-integration/writing-integration-code/ground-truth-encoder
https://docs.tensorleap.ai/tensorleap-integration/python-api/code_loader/decorators/tensorleap_gt_encoder

Model Integration (onnx/h5 + batch dimension requirement):
https://docs.tensorleap.ai/tensorleap-integration/model-integration

leap.yaml (entryFile + include/exclude + initial-integration rule):
https://docs.tensorleap.ai/tensorleap-integration/leap.yaml

Integration test (mandatory + rules + only-called-decorators used):
https://docs.tensorleap.ai/tensorleap-integration/integration-test
https://docs.tensorleap.ai/tensorleap-integration/python-api/code_loader/decorators/tensorleap_integration_test
https://docs.tensorleap.ai/tensorleap-integration/python-api/code_loader/decorators/tensorleap_load_model

Uploading with CLI / CLI Assets upload (leap push + prerequisites + server info + secrets):
https://docs.tensorleap.ai/tensorleap-integration/uploading-with-cli/cli-assets-upload

Secrets Management:
https://docs.tensorleap.ai/user-interface/secrets-management

Dataset Parse Process:
https://docs.tensorleap.ai/user-interface/project/menu-bar/runs-and-processes/process-types/dataset-parse-process

Quickstart using CLI:
https://docs.tensorleap.ai/getting-started/quickstart/quickstart-using-cli

Quickstart using Leap Hub:
https://docs.tensorleap.ai/getting-started/quickstart/quickstart-using-leap-hub

Leap CLI repo:
https://github.com/tensorleap/leap-cli
```

---

## Appendix B — Test fixture corpus

```text
Tensorleap Hub example repositories (fixture corpus):
https://github.com/tensorleap-hub
```
