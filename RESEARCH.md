# Input/GT Discovery Research (Current State)

Last updated: 2026-03-05

This is the living research brief for input/ground-truth discovery prior to production implementation in Concierge.

## Goal

Recommend correct model inputs and ground truths for Tensorleap encoders in repositories that do not yet have Tensorleap integration.

Core direction:

1. Use static analysis to generate high-quality leads.
2. Use an agent for semantic code tracing from those leads.
3. Ask the user to confirm proposed mappings.
4. Use runtime/model introspection as a support layer, not the only source of truth.

## Current Best Method (Recommended)

1. Prepare clean fixture `pre` repos:
   1. Remove integration files (`leap_*`, manifest-listed integration artifacts).
   2. Remove files containing `tensorleap`.
   3. Remove compiled residue (`__pycache__/leap*.pyc` and compiled matches for stripped modules).
2. Run framework-agnostic lead extraction:
   1. Score files by train/val/data-flow/model/loss signals.
   2. Detect framework (`pytorch`, `tensorflow`, `mixed`, `unknown`) using code + artifact evidence.
3. Run semantic investigator (Claude Opus):
   1. Inject lead summary directly into prompt.
   2. Keep task read-only, evidence-backed, uncertainty-explicit.
4. Normalize output into stable findings schema.
5. Evaluate with semantic-first rubric:
   1. input/GT presence
   2. count plausibility
   3. shape/dtype hint coverage
   4. exact names only as diagnostics
6. Present candidates + mapping proposal to user for confirmation.

## Key Decisions (Active)

1. Input detection is an agent task, not static-only parsing.
2. Framework-agnostic method replaces PyTorch-only method.
3. `lead_pack_read_success` is informational when lead context is already in prompt.
4. Exact input names are secondary; semantics and compatibility are primary.
5. Research runner defaults to Opus (`claude-opus-4-6`).
6. Fixture prep now includes automatic relevant-model LFS hydration (best-effort by default, strict mode optional).

## What We Learned

### Strong positives

1. `yolov5_visdrone` (`r009`) is a strong success case:
   1. Framework detected correctly (`pytorch`, high confidence).
   2. Agent traced native training path (`train.py` + dataloaders + loss) and recovered semantically correct input/GT candidates.
2. `ultralytics` (`r010`) shows the approach scales to larger repos:
   1. Agent produced rich, evidence-backed candidates from native code.
   2. Main failure was output-shape/schema mismatch in normalization, not semantic reasoning quality.
3. Contamination controls were essential and effective:
   1. Removing Tensorleap-authored files and compiled artifacts prevented false positives.

### Important gaps

1. `imdb` is a weak representative fixture for typical customer repos:
   1. Minimal code surface.
   2. No train loop.
   3. Dual model-path ambiguity (dense vs bert).
2. BERT input splitting issue remains:
   1. Agent often keeps BERT token bundle as one conceptual input instead of splitting (`input_ids`, `attention_mask`, `token_type_ids`).
   2. Ground truth inference is generally fine.
3. Normalizer robustness is still a practical risk:
   1. Model outputs can use alternate key shapes (`model_inputs`, `candidate_inputs`, etc.).

## LFS Findings

1. We implemented fixture-level relevant-model LFS hydration in prepare/verify scripts.
2. This improved fixture realism and enables future runtime signature checks where artifacts are available.
3. It did not, by itself, fix IMDB BERT splitting in current code-first prompt flow.
4. One known unresolved remote object remains in `mnist` (`model/mnist_onnx.onnx`):
   1. best-effort mode logs warnings and continues.
   2. strict mode (`STRICT_FIXTURE_LFS=1`) fails fast.

## Repository-by-Repository Status

1. `webinar`:
   1. Early runs were contaminated; later runs were clean.
   2. Useful for testing contamination defenses and sparse-context behavior.
2. `yolov5_visdrone`:
   1. Best current proof-of-value for semantic recovery on real PyTorch training code.
3. `ultralytics`:
   1. Good stress test for scale and mixed signals.
   2. Confirms need for stronger normalizer/schema handling.
4. `imdb`:
   1. Useful edge case for tokenizer-bundle splitting and ambiguous branches.
   2. Not a primary benchmark for mainstream expected customer flows.

## Recommendations For Concierge (Go Implementation)

### 1) Keep a deterministic pipeline with explicit stages

Implement stage outputs as typed Go structs and persist each stage artifact:

1. `fixture_state` / repo snapshot
2. `lead_pack`
3. `agent_prompt_bundle`
4. `agent_raw_output`
5. `normalized_findings`
6. `comparison_report`

This makes failures local and debuggable.

### 2) Build framework-agnostic lead extraction natively in Go

1. Use fast file scanning + regex-based signals (and lightweight ranking).
2. Keep signal weights configurable in a versioned config.
3. Emit both machine JSON and human summary.

### 3) Add branch-aware input inference rules before/after agent

High-impact rule set:

1. If tokenizer output is dict-like and framework is BERT/transformers, expand to per-key candidates.
2. If branch selection is statically resolvable (for example `MODEL_TYPE` assignment), prioritize active branch candidates.
3. Keep alternate-branch candidates as conditional suggestions.

### 4) Harden normalization

1. Accept multiple schema variants from agent output.
2. Map synonymous fields (`inputs`, `model_inputs`, `candidate_inputs`, etc.).
3. Fail with actionable diagnostics, not silent empty candidates.

### 5) Introduce controlled runtime signature support (after code-first pass)

1. If model artifacts exist and are hydrated, optionally inspect runtime signature:
   1. ONNX input names/shapes/dtypes.
   2. Keras/TensorFlow input tensors where feasible.
2. Use runtime as corroboration layer, not sole decision-maker.
3. Report disagreements between code-derived and runtime-derived candidate sets.

### 6) Keep quality gates focused on real reliability

Hard failures:

1. tool errors
2. permission errors
3. malformed/empty final payload

Informational (non-blocking in prompt-injection mode):

1. no explicit `Read(lead_pack.json)` tool call

### 7) UX recommendations for Concierge

1. Always show evidence snippets for each proposed input/GT.
2. Ask user to confirm/adjust mappings before authoring encoders.
3. Make alternative branch choices explicit (for example dense vs bert).

## Implementation Plan (Go, Suggested Sequence)

1. Port lead extraction + framework detection to Go with artifact persistence.
2. Port prompt assembly + run metadata/quality gate plumbing.
3. Port robust normalizer with multi-shape parsing.
4. Add heuristic post-processor for dict-input splitting and branch resolution.
5. Add optional runtime signature inspector (LFS-aware precondition).
6. Integrate into Concierge step flow and user confirmation UI.
7. Validate on fixtures emphasizing `yolov5_visdrone` and `ultralytics`; keep `imdb` as edge-case regression.

## Obsolete Items Removed

This version intentionally drops historical run-by-run narrative and deprecated PyTorch-only framing. Detailed logs remain in `research/input_discovery/results/` and should be treated as supporting evidence, not primary plan text.
