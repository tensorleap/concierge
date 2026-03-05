# Input Discovery Research Workspace (Temporary)

This directory is for temporary research artifacts only. It is not production Concierge code.

## Purpose

Validate agent-led PyTorch input/GT discovery using static lead extraction + semantic analysis.

## Structure

- `scripts/`: temporary research scripts.
- `schemas/`: JSON schemas used by research scripts and agent output validation.
- `prompts/`: static prompt drafts for read-only semantic investigator runs.
- `results/`: experiment outputs (lead packs, summaries, comparisons).

## Experiment Naming Convention

Use this pattern:

`<repo-id>__<repo-variant>__<method-version>__<run-id>`

Examples:

- `webinar__pre__pytorch-leads-v1__r001`
- `yolov5_visdrone__pre__pytorch-leads-v1__r002`

Field rules:

1. `repo-id`: fixture id (for example `webinar`).
2. `repo-variant`: usually `pre` for discovery research; use `post` only for comparison/debugging.
3. `method-version`: algorithm identifier (starts with `pytorch-leads-v1`).
4. `run-id`: monotonic run label (`r001`, `r002`, ...).

## Research-Only Rules

1. Keep scripts lightweight and disposable.
2. No CI wiring.
3. No TDD requirement.
4. Prefer explicit outputs over hidden state.
