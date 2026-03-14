# Concierge — Terminal Integration Assistant for Tensorleap

**Document type:** Design document (handoff-ready for implementation)
**Primary audience:** Engineering + agent-assisted coding implementation
**Status:** Draft (implementation-oriented, no step-by-step implementation plan)
**Implementation status (2026-03-10):** Concierge v1 scope remains focused on mandatory onboarding requirements. The design now assumes the guide-native `leap_integration.py` workflow plus a Poetry-only local runtime boundary. Optional asset assistance (metadata, visualizers, metrics, loss, custom layers) is deferred to v2.

---

## 1) Problem statement

Tensorleap integrations are powerful but often require a pre-sales/solutions engineer to sit with a prospective customer and incrementally wire up:

* dataset preprocessing and sample fetching
* input encoders and ground-truth encoders
* optional metadata/visualizers/metrics/loss (deferred from Concierge v1 workflow)
* a mandatory integration test that connects these interfaces to model inference
* CLI upload and server-side validation

This work is hard to “one-shot” with an LLM prompt because the required context spans:

* the customer repository (often large, multi-project, inconsistent)
* the repository’s Python runtime (interpreter, Poetry environment, dependency state)
* Tensorleap’s integration contracts and rules
* step ordering and prerequisites (server mounts, `leap.yaml` include/exclude, CLI auth)
* iterative debugging when real code and data paths don’t match assumptions

**Goal:** build a deterministic, cross-platform terminal tool called **Concierge** that drives the integration loop, delegates *small, focused* editing tasks to a coding agent (starting with **Claude Code**), and provides a guided user experience with explicit confirmations and auditable git commits.

As Concierge moves from planning to authoring and local execution, runtime selection becomes part of correctness. Local validation is only meaningful if Concierge knows which project Python interpreter and environment it is using, whether that environment belongs to the selected repo, and whether the required dependencies are actually present there.

---

## 2) Background: what “Tensorleap integration” requires

Concierge must align with Tensorleap’s documented integration flow and runtime expectations.

### 2.1 Expected integration artifacts and flow

`GUIDE.md` and the current decorator-based Tensorleap workflow establish a canonical authoring shape:

* `leap_integration.py` — canonical entry script and local validator harness
* `leap.yaml` — mandatory configuration, with `entryFile` pointing at `leap_integration.py`
* decorated interfaces inside `leap_integration.py`, including preprocess, input encoders, GT encoders, model loading, and integration-test wiring

Concierge v1 assumes a fresh guide-native layout rooted at `leap_integration.py`. Binder-style layouts are not a supported authoring target and should not shape planning, scaffolding, or validation behavior.

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

### 2.4 Decorator-based integration structure

Tensorleap’s current authoring model is decorator-first and entrypoint-first:

* `leap_integration.py` is both the integration module and the runnable local validation entrypoint.
* **Preprocess function**: runs once, takes no arguments, and returns a list of `PreprocessResponse` objects for dataset slices.
  At least **train + validation are mandatory**.
* **Input encoders**: called per-sample `(sample_id, preprocess_response)` and registered with `@tensorleap_input_encoder(...)`.
* **Ground truth encoders**: called per-sample and registered with `@tensorleap_gt_encoder(...)`.
* `@tensorleap_load_model(...)` loads an `.onnx` or `.h5` model and declares prediction semantics.
* `@tensorleap_integration_test()` is the thin wiring layer that connects decorated interfaces into one model path.
* Optional hooks remain available for metadata, custom visualizers, custom metrics, loss, and custom layers.

Nuance: for the *unlabeled* set, GT encoders + metrics/loss won’t run.

**Concierge v1 scope note:** although Tensorleap supports optional hooks, Concierge v1 does not validate, enforce, or auto-generate optional assets. Concierge v1 targets only the mandatory contracts required to reach a working upload, while still understanding the validator signals emitted by optional interfaces.

### 2.5 Validation is call-driven, not definition-driven

The guide-native workflow changes what “present” means:

* a decorator being defined is not enough; validation happens only when the decorated function is actually called
* early `__main__` blocks should directly call new interfaces such as `preprocess()`, input encoders, GT encoders, and `load_model()`
* as soon as a minimal real model path exists, `__main__` should switch to calling `integration_test(...)`

This matters because `@tensorleap_integration_test` re-runs itself in mapping mode, then triggers binder-level checks. A clean run can print `Successful!`, but that still proves only first-sample wiring, not full dataset health.

### 2.6 `leap_integration.py` is a progressive local validator

Concierge should mirror the staged validator behavior described in `GUIDE.md`:

* import-time/decorator-time feedback catches duplicate names and invalid decorator arguments
* direct calls validate function signatures, return types, dtypes, ranks, and batching mistakes
* the exit hook prints a status table only when the entry script filename is exactly `leap_integration.py`
* the status table and “recommended next interface” message are useful human-oriented progress signals
* `LeapLoader.check_dataset()` is the better machine-oriented interface because it returns structured parse results instead of only terminal text

This makes local execution part of the authoring loop, not a final smoke test after the integration is “done.”

### 2.7 Guide-native authoring order

Concierge should drive users through the same order that unlocks progressively more informative validation:

1. create `leap_integration.py` and `leap.yaml`
2. implement preprocess
3. inspect model I/O contract
4. implement the minimum required input encoder set
5. implement `@tensorleap_load_model`
6. add a minimal, thin `@tensorleap_integration_test`
7. add remaining required input encoders
8. add GT encoders
9. expand from first-sample success to multiple training/validation samples

Optional assets remain out of Concierge v1 scope, but the planner should still preserve this contract order.

### 2.8 CLI upload prerequisites and common failures

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
Correctness is primarily established via runtime behavior and contract validation, not semantic branch comparison by another LLM. Those checks must run in an explicit project runtime, not whatever interpreter happens to be active in the shell.

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

### 4.4 User-facing language (mandatory)

Any text shown to end users (terminal printouts, prompts, errors, confirmations) must use user-language, not internal implementation jargon.

Rules:

* avoid internal terms like “contract,” “ensure-step,” “issue code,” or “planner policy” unless they are immediately explained in plain words
* describe what Concierge observed and what the user should do next in direct language
* when a technical term is unavoidable, add a short explanation in the same message
* prefer “required input names” over “required input encoder symbols” in user-facing copy

---

## 5) High-level architecture

### 5.1 Core components (Go)

**1) Snapshotter**
Captures a **Workspace Snapshot** (see §6) including git state, file hashes for relevant files, and key config fingerprints.

**2) RuntimeResolver**
Resolves the selected repo’s **Local Runtime Profile**:

* determines whether the repo is a supported Poetry project in v1
* discovers the Poetry-managed interpreter and Python version
* verifies runtime readiness (environment present, dependencies installed, Tensorleap-local dependency ready)
* treats ambient virtual-environment state as diagnostic evidence, not as the source of truth

**3) Inspector**
Reads the snapshot and produces an **Integration Status Report**:

* what artifacts exist (`leap.yaml`, entry file, integration test, binder, requirements)
* what’s missing or invalid
* where failures occurred (CLI, server, validation)
* what evidence exists (logs, error traces)

**4) Planner (deterministic)**
Selects the next action as an ordered set of **Ensure Steps**. Planner chooses *one primary next action* at a time to preserve reliability.

**5) Executor**
Performs the chosen ensure-step via:

* deterministic action (write templates, run CLI commands)
* agent-assisted action (Claude Code session)
* collects evidence and returns a structured result

**6) AgentRunner (agent host / collaboration session)**
Runs Claude Code as a subprocess in a *collaborative* mode:

* the agent is allowed to **explore the repository** (read/search/navigate) to complete tasks reliably
* the agent may request to run commands (e.g., `grep`, `find`, tests); Concierge can gate these through **user approval** (recommended)
* the agent edits files in-repo; Concierge reviews changes via `git diff`
* agent interaction may be **multi-turn** (the agent can ask clarifying questions), while Concierge still controls the outer orchestration loop

> Design intent: leverage Claude Code’s strengths (repo investigation + tooling), while keeping Concierge as the deterministic driver and keeping the user in control of impactful actions.

**7) GitManager**
Ensures clean working tree, creates branch, captures diffs, supports revert-on-reject, and commits approved changes with structured messages.

**8) Validator**
Runs local validation:

* Tensorleap integration test checks (mandatory)
* Concierge runtime harness (coverage + semantic checks) (§9)
* optional CLI-level checks where appropriate

**9) Reporter**
Presents status/diffs/validation results and writes machine-readable artifacts for debugging and tests.

### 5.2 Local Runtime Profile (Poetry-only v1)

The runtime subsystem is a first-class correctness boundary.

For v1:

* Concierge supports **Poetry-managed Python projects only** for local developer-machine execution.
* Production Concierge does **not** install Python interpreters and does **not** create brand-new project environments as part of normal behavior.
* Concierge must resolve and persist an explicit **Local Runtime Profile** for the selected project root.
* All local Python execution must proceed through Poetry (`poetry run ...`) and the resolved interpreter path, not shell activation.
* Runtime readiness includes both **project dependency readiness** and **Tensorleap-local dependency readiness**.
* If Concierge changes repo dependency state to add the Tensorleap-local package, those changes belong in normal diff/approval/commit flow via repo-managed files such as `pyproject.toml` and `poetry.lock`.
* Dev-time fixture bootstrap is a separate concern: the test harness may use tools such as `pyenv` + Poetry, but that must not silently become production product behavior.

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
  * dependency/runtime files (present + hash): `pyproject.toml`, `poetry.lock`, `requirements.txt`, etc.
* **Environment**

  * OS/arch
  * Poetry presence + version (discoverable)
  * whether the selected root is a supported Poetry project
  * Poetry-reported interpreter path via `poetry env info --executable` (when resolvable)
  * effective Python version for the resolved project runtime
  * ambient virtual-environment indicators such as `VIRTUAL_ENV` / `CONDA_PREFIX` (diagnostic only)
  * other discoverable python executable(s) + versions (diagnostic only)
  * leap CLI presence + version (discoverable)
* **Tensorleap connectivity**

  * ability to reach server (CLI probe, port expectations)
  * auth presence (via CLI status checks)

### 6.2 Persistent Concierge State (mutable across iterations)

Store in e.g. `.concierge/state.json`:

* selected project root (for monorepos)
* chosen integration layout
* resolved local runtime profile:

  * runtime kind (v1: `poetry`)
  * Poetry executable/version
  * resolved interpreter path
  * Python version
  * whether the runtime choice required user confirmation
  * whether project dependencies are known-ready
  * whether Tensorleap-local dependency is known-ready
  * runtime fingerprint / invalidation token
* “integration plan” expectations:

  * expected dataset subsets and their semantics
  * expected inputs and GT targets (as discovered/confirmed)
  * expected model artifact strategy
* last successful ensure-step
* last known good validation outputs

State must be invalidated if relevant files change, including project-root changes, `pyproject.toml`, `poetry.lock`, or resolved-interpreter drift.

### 6.3 Evidence bundle

Each iteration should produce:

* command outputs (stdout/stderr) for executed CLI commands
* runtime resolution evidence (Poetry detection, interpreter resolution, dependency-readiness checks)
* validation logs
* runtime provenance for each local Python validation action
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

### 7.2 Canonical integration layout

Concierge v1 has one canonical authoring target:

* root-level `leap_integration.py`
* root-level `leap.yaml`
* `leap.yaml.entryFile = leap_integration.py`

Design stance:

* require the canonical root-level `leap_integration.py` entrypoint
* treat non-canonical `entryFile` settings as blocking layout errors
* normalize newly scaffolded or repaired integrations toward the guide-native `leap_integration.py` flow
* preserve repo-specific helper modules only when `leap_integration.py` remains the entrypoint

---

## 8) Orchestration loop and ensure-steps

Concierge implements the iterative loop:

* capture snapshot
* inspect integration state
* ensure repository prerequisites and the local Poetry runtime
* ensure the canonical `leap_integration.py` / `leap.yaml` layout
* progress through the next guide-native authoring milestone
* validate first through the built-in local validator, then through higher-coverage Concierge validation
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

**B) Local Python runtime (Poetry-only v1)**

* ensure the selected root is a supported Poetry project
* ensure Poetry is installed and usable
* ensure a Local Runtime Profile is resolved and recorded
* ensure project dependencies are installed in the resolved Poetry environment
* ensure the required Tensorleap-local Python dependency is ready
* treat shell activation as diagnostic state only; execute local Python through Poetry
* ask the user only when automatic runtime resolution is ambiguous or suspicious

**C) Tensorleap CLI and server**

* ensure CLI installed and authenticated
* ensure server reachable; `leap server info` yields mounted dataset volumes
* ensure secrets context (if needed) is correctly configured

**D) Canonical guide-native layout**

* `leap.yaml` exists, is parseable, and points `entryFile` at `leap_integration.py`
* include/exclude covers all runtime-read files
* binder-era filenames are not part of the supported authoring surface
* initial integration nuance: don’t force populated IDs prematurely

**E) Progressive authoring milestones**

* preprocess runs directly and returns at least train+validation `PreprocessResponse`
* model I/O contract is understood before expecting useful model-path validation
* the minimum required input encoder set can execute on a real sample
* `@tensorleap_load_model` executes directly and declares model semantics
* a thin `@tensorleap_integration_test` exists as soon as one real model path exists
* remaining required input encoders and GT encoders are added in guide-native order

**F) Integration-test mapping and coverage**

* integration test exists and follows guide-native rules
* it calls the relevant decorators that must be used in analysis
* it stays thin and declarative so mapping-mode re-execution succeeds
* it avoids direct dataset access, arbitrary Python transforms, and manual batch-dimension manipulation in the integration-test body

**G) Multi-stage validation**

* human-oriented `python leap_integration.py` runs reach the next expected validator milestone
* machine-oriented `LeapLoader.check_dataset()` results are captured for tooling
* Concierge expands validation from first-sample success to bounded multi-sample coverage

**H) Upload readiness**

* requirements correctness (if used)
* model artifact is `.onnx`/`.h5` and compatible
* explicit user confirmation before `leap push`

---

## 9) Validation strategy: avoid “skeleton-only” false success

A critical requirement: Concierge must not flatten the guide-native validator into a single binary pass/fail check, and it must not treat “decorators exist” + “a minimal test passes once” as completion.

Validation is only meaningful when it is tied to an explicit Local Runtime Profile. Any local Python-based check must execute through the resolved Poetry runtime and record which runtime profile was used.

### 9.1 Five layers of validation

**Layer 1: Surface inventory (fast signal, not sufficient)**
Discover what integration interfaces appear present across the upload boundary defined by `leap.yaml` include/exclude + entry file. This may use:

* static scanning (best-effort)
* runtime import and introspection through the resolved Poetry runtime profile

Purpose:

* diagnostics
* planning (what’s missing)
* user messaging

**Layer 2: Guide-native progressive local validation**
Concierge should execute and interpret the built-in local validator surface in the same staged order described in `GUIDE.md`:

* import-time/decorator-time validation
* direct calls to preprocess, input encoders, GT encoders, and `load_model()`
* thin `integration_test(...)` execution
* mapping-mode re-execution and binder-level checks
* human-oriented status-table output when the entry script is exactly `leap_integration.py`

Important nuance:

* `Successful!` is a milestone, not final proof
* the exit status table is for authors and operator UX, not the primary machine interface
* failures in mapping mode are often integration-test-body design failures, not just missing decorators

**Layer 3: Structured parser validation**
Concierge should capture machine-readable parse results from `LeapLoader.check_dataset()` (or equivalent structured parser surface) whenever available.

Purpose:

* avoid brittle scraping of human-only terminal output
* distinguish import/setup/model/setup errors from handler-level failures
* record payload-level evidence for planning and reporting

**Layer 4: Concierge runtime coverage harness (semantic + high-resolution)**
Concierge runs a deterministic harness that:

* executes through the resolved Poetry runtime profile
* runs preprocess and validates subsets; train+validation are mandatory
* selects multiple indices per subset (bounded)
* calls every input encoder across those indices
* calls every GT encoder across those indices for labeled subsets
* optionally records guide-native validator milestones alongside harness results
* optionally calls metadata/visualizers if defined (planned for Concierge v2, not v1)
* enforces invariants:

  * no exceptions
  * finite outputs
  * sensible dtypes
  * shapes compatible with the model (batch dimension expectations)

This creates **high-resolution evidence** like:

* “input encoder X executed successfully on indices [0,1,2]”
* “GT encoder Y missing or throws exception at index 1”
* “model input shape mismatch”

**Layer 5: Stub-detection heuristics (“anti-cheat”)**
Practical heuristics to catch stubby integrations:

* variation checks across indices (detect constant outputs)
* non-empty train/val warnings
* suspicious constant labels
* file/path existence checks and server-mount compatibility checks

### 9.2 Integration test correctness is both coverage and code-shape

Tensorleap states only decorators called in `@tensorleap_integration_test` are used in analysis, and `GUIDE.md` adds a second constraint: the integration-test body must stay thin enough to survive mapping-mode re-execution. Concierge must therefore:

* track what must be wired in the integration test and in what order
* enforce that the relevant decorators are actually called
* enforce guide-native code-shape rules for the integration-test body
* treat direct dataset access, arbitrary Python transforms, or manual batching inside `integration_test` as correctness failures, not stylistic warnings

### 9.3 Runtime provenance is part of correctness

The validator and evidence bundle should record the runtime profile used for every local validation action, including:

* Poetry executable/version
* resolved interpreter path
* Python version
* whether the runtime was auto-resolved or user-confirmed
* enough fingerprint data to detect later drift

This makes runtime drift distinguishable from code regressions.

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

Production Concierge and fixture bootstrap must stay separate:

* production behavior attaches to an existing Poetry-managed project environment and does not create a fresh one
* fixture/dev bootstrap may create clean environments with `pyenv` + Poetry when needed for deterministic tests
* that fixture convenience must not become an implicit production assumption

### 13.3 Test tiers

**Tier 1: Unit tests (no agent, no Docker required)**

* snapshots, invalidation rules
* Poetry project detection, runtime-profile resolution, and ambiguity handling
* parsing and include/exclude resolution
* planner step selection
* git operations (diff, revert, commit)
* evidence recording

**Tier 2: Orchestrator integration tests (no live agent)**

* stub AgentRunner behavior deterministically
* validate Poetry runtime gating, dependency-readiness checks, and runtime-profile invalidation
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
* **Explicit runtime boundary:** local Python commands run through the resolved Poetry runtime, not ambient shell activation.
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

### 17.3 Extending beyond Poetry-managed projects

This is no longer open for v1: local developer-machine runtime support is fixed to Poetry-managed Python projects, and Concierge does not create interpreters or brand-new project environments in production.

The remaining open question is when and how to extend the runtime subsystem beyond Poetry, for example to:

* `requirements.txt`-only repos
* pip-tools
* Conda
* Hatch
* `uv`

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

## 18) Codex QA Loop

The repository also includes a standalone QA harness for subjective terminal evaluation under `QA/`, with the entrypoint at `QA/qa_loop.py`.

For the design document and operator guide, see `QA/DESIGN.md` and `QA/QA_LOOP.md`.

What it does:

* launches Concierge inside a real PTY
* asks `codex exec` to act as a QA engineer and synthetic user
* forwards terminal output to Codex and types Codex's chosen replies back into Concierge
* stops on completion, dead-end, fix-needed, idle-limit, runtime-limit, or iteration-limit
* writes structured turn logs plus human-readable reports under `QA/runs/`, `QA/transcripts/`, and `QA/reports/`

Minimal usage:

```bash
python3 QA/qa_loop.py --command-cwd /path/to/fixture
```

If no command is supplied, the harness defaults to this repository's Concierge binary (or `go run ./cmd/concierge run` when the binary is missing). For a blind-first comparison flow, pass `--fixture-post-path /path/to/post-fixture`; the prompt will not reveal that path to Codex until the run stalls.

Requirements:

* `python3`
* a local `codex` CLI login that can run `codex exec`
* a runnable Concierge command

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
