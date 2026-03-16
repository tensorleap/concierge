# QA Loop Guide

`QA/qa_loop.py` is a local harness that runs Concierge inside a Docker container PTY, lets `codex exec` act as the user, and saves a transcript plus a qualitative QA report.

## Prerequisites

- `python3`
- a working local `codex` CLI login
- this Concierge repo available locally
- Docker
- Go
- `ANTHROPIC_API_KEY`

You do not need to manually check out a separate fixture repo if you want to use the built-in fixture corpus. Prepare the fixtures once, then use `scripts/qa_fixture_run.sh` or `make qa`; they build a fixture-scoped container image from the clean `pre` repo and run the harness against that container.

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
bash scripts/qa_fixture_run.sh --repo <fixture-id>
```

Shortcut from the repo root:

```bash
make qa
make qa REPO=mnist
```

What this does:

- runs Concierge against `.fixtures/<fixture-id>/pre`
- copies only the clean `pre` repo into a fixture-specific Docker image
- keeps Codex in blind-first mode at the start
- only exposes the `post` repo path if progress stalls
- when using `make qa`, resets the chosen built-in fixture back to a clean pinned `pre`/`post` state first
- starts a long-lived fixture container and runs Concierge inside it with `docker exec`
- records Docker snapshots after every supervisor turn
- renders live Codex control/report events as readable terminal text while keeping the raw JSON event logs under `QA/runs/<run-id>/codex/`

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
  Optional known-good repo path that Codex may inspect only after the blind-first phase stalls.
- `--artifacts-root PATH`
  Where `runs/`, `transcripts/`, and `reports/` are written. Defaults to `QA/`.
- `--codex-command STRING`
  Command used to launch Codex. Default: `codex`.
- `--model MODEL`
  Optional Codex model override.
- `--run-id ID`
  Set a stable run ID instead of the generated timestamp-based one.
- `--max-iterations N`
  Hard cap on Codex control turns. Default: `30`.
- `--max-idle-turns N`
  Stop after this many no-progress turns. Default: `5`.
- `--max-runtime-seconds N`
  Hard runtime limit. Default: `3600`.
- `--read-quiet-seconds FLOAT`
  How long PTY output must stay quiet before a read cycle is treated as complete.
- `--read-timeout-seconds FLOAT`
  Maximum wait per PTY read cycle.
- `--transcript-tail-chars N`
  How much of the full transcript tail is sent to Codex on each control turn.
- `--latest-output-chars N`
  How much of the most recent terminal output is sent to Codex on each control turn.

## Outputs

By default the harness writes:

- `QA/runs/<run-id>/summary.json`
- `QA/runs/<run-id>/turns.jsonl`
- `QA/runs/<run-id>/codex/*`
- `QA/transcripts/<run-id>.full.txt`
- `QA/transcripts/<run-id>.terminal.raw.txt`
- `QA/transcripts/<run-id>.terminal.txt`
- `QA/transcripts/<run-id>.interaction.jsonl`
- `QA/reports/<run-id>.json`
- `QA/reports/<run-id>.md`

The harness prints the absolute path to the full-session transcript at the end of the run. The markdown report is still the easiest artifact to read first.

For fixture runs, each `QA/runs/<run-id>/docker/` directory also contains per-turn `docker commit` metadata and any exported container artifacts such as `/workspace/.concierge`.

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
