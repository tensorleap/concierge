# QA Loop Guide

`QA/qa_loop.py` is a local harness that runs Concierge in a real PTY, lets `codex exec` act as the user, and saves a transcript plus a qualitative QA report.

## Prerequisites

- `python3`
- a working local `codex` CLI login
- this Concierge repo available locally
- a repo for Concierge to operate on

You do not need to manually check out a separate fixture repo if you want to use the built-in fixture corpus. Prepare the fixtures once, then point `QA/qa_loop.py` at the generated `pre` repo.

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
python3 QA/qa_loop.py \
  --command-cwd .fixtures/<fixture-id>/pre \
  --fixture-post-path .fixtures/<fixture-id>/post
```

Shortcut from the repo root:

```bash
make qa
make qa REPO=mnist
```

What this does:

- runs Concierge against `.fixtures/<fixture-id>/pre`
- keeps Codex in blind-first mode at the start
- only exposes the `post` repo path if progress stalls
- when using `make qa`, resets the chosen built-in fixture back to a clean pinned `pre`/`post` state first
- renders live Codex control/report events as readable terminal text while keeping the raw JSON event logs under `QA/runs/<run-id>/codex/`

## Using Another Repo

If you already have a repo checked out somewhere else, point `--command-cwd` at it:

```bash
python3 QA/qa_loop.py --command-cwd /path/to/target-repo
```

`QA/qa_loop.py` does not prepare that repo for you. It just runs Concierge there.

## Default Command Behavior

If you do not pass a command after `--`, `QA/qa_loop.py` runs:

- `bin/concierge run` if `bin/concierge` exists in this repo
- otherwise `go run /absolute/path/to/this/repo/cmd/concierge run`

That means a local `go build` is optional for normal usage.

## Overriding The Command

If you want a custom Concierge invocation, pass it after `--`:

```bash
python3 QA/qa_loop.py \
  --command-cwd .fixtures/<fixture-id>/pre \
  -- \
  /absolute/path/to/concierge run --persist --yes
```

Everything after `--` is treated as the PTY command.

## Useful Options

- `--command-cwd PATH`
  Run Concierge in this repo or fixture directory.
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
