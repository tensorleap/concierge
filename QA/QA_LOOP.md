# QA Loop Guide

`QA/qa_loop.py` runs Concierge inside a Docker container PTY, lets Claude act as the user, and saves a transcript plus a qualitative QA report.
For fixture-backed runs that provide a `post` repo, the loop now also runs a final Codex integration review against the exported generated workspace and the known-good fixture before it can declare true success.

## Prerequisites

- `python3`
- a working local `claude` CLI login, or `CLAUDE_BIN` pointing at the desired Claude CLI binary
- a working local `codex` CLI install for the final integration review, or `CODEX_BIN` pointing at it
- Codex authentication for `codex exec`, typically via `CODEX_API_KEY` for automation or a local `codex` login for interactive runs
- this Concierge repo available locally
- Docker
- Go
- `ANTHROPIC_API_KEY`

You do not need to manually check out a separate fixture repo if you want to use the built-in fixture corpus. Prepare the fixtures once, then use `scripts/qa_fixture_run.sh` or `make qa`; they build a fixture-scoped container image from the clean `pre` repo and run the same Claude-driven harness used in CI.

## Using A Built-In Fixture

Prepare fixtures:

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_verify.sh
```

That creates:

- `.fixtures/<id>/pre`: the stripped repo Concierge should try to integrate
- `.fixtures/<id>/post`: the known integrated version

Typical fixture run:

```bash
bash scripts/qa_fixture_run.sh --repo <fixture-id> --step <guide-step>
```

Shortcut from the repo root:

```bash
make qa
make qa REPO=mnist QA_STEP=preprocess
make qa REPO=ultralytics QA_STEP=input_encoders
```

If you omit `REPO` or `QA_STEP` in an interactive shell, `make qa` and `scripts/qa_fixture_run.sh` show a simple numbered menu. In non-interactive shells, missing selectors are treated as an error and the runner prints the valid fixture ids and guide-native steps.

What this does:

- runs Concierge against `.fixtures/<fixture-id>/pre`
- copies only the clean `pre` repo into a fixture-specific Docker image
- stages any declared runtime prerequisites into a temporary host directory and mounts them read-only under `/runtime-prerequisites`
- keeps Claude in blind-first mode at the start
- only exposes the `post` repo path if progress stalls
- after a terminal success path, compares the exported generated integration against the `post` fixture with Codex and fails the run if the generated code is not judged functionally equivalent or otherwise sound
- when using `make qa`, resets the chosen built-in fixture back to a clean pinned `pre`/`post` state first
- starts a long-lived fixture container and runs Concierge inside it with `docker exec`
- keeps per-turn transcripts, turn logs, and `docker diff` / exported `.concierge` artifacts
- renders live Claude control/report events as readable terminal text while keeping the raw JSON event logs under `QA/runs/<run-id>/claude/`

If a fixture declares runtime prerequisites, configure them locally with env vars or `fixtures/runtime_prerequisites.local.json` as described in `fixtures/README.md`. The QA supervisor receives the safe prerequisite facts, including mounted container paths and operator guidance, from the beginning of the run.

## Using A Running Container

If you already have a prepared container that exposes the target repo at `/workspace`, point `QA/qa_loop.py` at that container:

```bash
python3 QA/qa_loop.py \
  --container-name <running-container> \
  --container-workdir /workspace
```

`QA/qa_loop.py` does not build or start that container for you. It just drives Concierge inside it.

## Default Command Behavior

If you do not pass a command after `--`, `QA/qa_loop.py` runs:

- `/usr/local/bin/concierge run` inside the target container

The fixture helper script cross-builds that Linux binary and bakes it into the image for you.

## Overriding The Command

If you want a custom Concierge invocation, pass it after `--`:

```bash
python3 QA/qa_loop.py \
  --container-name <running-container> \
  --container-workdir /workspace \
  -- \
  /usr/local/bin/concierge run --persist --yes
```

Everything after `--` is treated as the container-internal PTY command.

## Useful Options

- `--container-name NAME`
  Running Docker container to target.
- `--container-workdir PATH`
  Working directory inside the container. Default: `/workspace`.
- `--container-image REF`
  Optional image reference to record in `summary.json`.
- `--docker-bin PATH`
  Docker CLI to use. Default: `docker`.
- `--fixture-post-path PATH`
  Optional known-good repo path that Claude may inspect only after the blind-first phase stalls.
- `--docker-snapshots`
  Optional debug mode. Captures a `docker commit` plus per-turn `diff`/`inspect` metadata after each supervisor turn.
- `--artifacts-root PATH`
  Where `runs/`, `transcripts/`, and `reports/` are written. Defaults to `QA/`.
- `--claude-command STRING`
  Command used to launch Claude. Default: `claude`, or `CLAUDE_BIN` if set.
- `--claude-timeout-seconds FLOAT`
  Timeout for each Claude control step. Default: `300`. Final report synthesis is capped at `120`.
- `--review-command STRING`
  Command used to launch the final Codex integration review. Default: `codex`, or `CODEX_BIN` if set.
- `--review-timeout-seconds FLOAT`
  Timeout for the final Codex integration review. Default: `300`.
- `--model MODEL`
  Optional Claude model override.
- `--run-id ID`
  Set a stable run ID instead of the generated timestamp-based one.
- `--max-iterations N`
  Hard cap on Claude control turns. Default: `50`.
- `--max-idle-turns N`
  Stop after this many no-progress turns. Default: `5`.
- `--max-runtime-seconds N`
  Hard runtime limit. Default: `3600`.
- `--read-quiet-seconds FLOAT`
  How long PTY output must stay quiet before a read cycle is treated as complete.
- `--read-timeout-seconds FLOAT`
  Maximum wait per PTY read cycle.
- `--transcript-tail-chars N`
  How much of the full transcript tail is sent to Claude on each control turn.
- `--latest-output-chars N`
  How much of the most recent terminal output is sent to Claude on each control turn.

## Outputs

By default the harness writes:

- `QA/runs/<run-id>/summary.json`
- `QA/runs/<run-id>/turns.jsonl`
- `QA/runs/<run-id>/claude/*`
- `QA/runs/<run-id>/docker/*`
- `QA/transcripts/<run-id>.full.txt`
- `QA/transcripts/<run-id>.terminal.raw.txt`
- `QA/transcripts/<run-id>.terminal.txt`
- `QA/transcripts/<run-id>.interaction.jsonl`
- `QA/reports/<run-id>.json`
- `QA/reports/<run-id>.md`

The harness prints the absolute path to the full-session transcript at the end of the run. The markdown report is still the easiest artifact to read first.
When the review gate runs, `summary.json` also carries an `integration_review` object with the Codex verdict, equivalence assessment, quality assessment, issues, and confidence.

For fixture runs, each `QA/runs/<run-id>/docker/export/workspace/` directory contains exported container artifacts such as `/workspace/.concierge`, `leap.yaml`, `leap_integration.py`, and other common Tensorleap integration files when they exist. If `--docker-snapshots` is enabled, `QA/runs/<run-id>/docker/` also includes per-turn `docker commit` metadata plus `docker diff` / `docker inspect` outputs.

## GitHub Actions

The shared manual workflow lives at `.github/workflows/qa-loop.yml`.

Dispatch it with:

- `ref`: same-repo branch, tag, or SHA to test
- `fixture`: fixture id from `fixtures/manifest.json`
- `step`: guide-native checkpoint step
- optional `run_id`, `issue_number`, and `pr_number`

The workflow installs Claude CLI, runs `scripts/qa_fixture_run.sh`, uploads the saved QA artifacts, writes a richer `GITHUB_STEP_SUMMARY` with outcome and timeline details, and can post the same result summary back to the linked issue or PR.

## Exit Codes

- `0`: `STOP_REPORT`
- `2`: `STOP_FIX`
- `3`: `STOP_DEADEND`
- `1`: anything unexpected

## Quick Start

If you want the shortest working path with the built-in fixtures:

```bash
make qa
```

If you want the shortest explicit non-interactive path:

```bash
make qa REPO=mnist QA_STEP=preprocess
```
