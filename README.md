# Concierge

Concierge is a Go CLI for guiding Tensorleap integration work from the terminal. It inspects a target repository, highlights the next blocking step, runs local checks, and keeps the user in control of code changes and upload decisions.

For Tensorleap teams, Concierge sits between the integration guide and the repetitive operator loop of wiring a real customer repository into a working Tensorleap project. It is meant to drive that loop, not replace the Leap CLI.

## What Concierge Does

- inspects repository state, runtime readiness, and integration gaps
- guides the guide-native Tensorleap authoring flow toward `leap_integration.py` and `leap.yaml`
- validates progress locally and can persist evidence under `.concierge/`
- can delegate bounded editing tasks to a local coding-agent runtime, then review and validate the result before moving on

## Current Product Shape

The implementation is intentionally narrower than the original planning document:

- the target integration layout is root-level `leap_integration.py` plus root-level `leap.yaml`
- local execution currently supports Poetry-managed Python projects only
- the CLI surface is `doctor`, `run`, and `version`
- the agent-backed authoring path currently expects the `claude` CLI on `PATH`
- v1 work is focused on the mandatory Tensorleap onboarding path; optional metadata, visualizers, metrics, loss, and custom-layer help are not the primary target yet

The architecture and current implementation context live in [`DESIGN.md`](DESIGN.md).

## Install

Today the simplest path is to build Concierge from source:

```bash
go build -o bin/concierge ./cmd/concierge
./bin/concierge version
./bin/concierge doctor
```

For a real integration run, you currently need:

- Go 1.24+ to build the binary
- a Poetry-managed target Python project
- the Tensorleap `leap` CLI installed and authenticated
- access to a reachable Tensorleap server when you want to push
- the `claude` CLI on `PATH` for agent-assisted authoring steps

## Basic Usage

```bash
# Check local Concierge and Leap CLI setup
./bin/concierge doctor

# Preview the next steps without making changes
./bin/concierge run --dry-run --project-root /path/to/repo

# Run the guided loop and persist reports under .concierge/
./bin/concierge run --project-root /path/to/repo --persist
```

Useful `run` flags:

- `--yes` to auto-approve mutation and push prompts
- `--non-interactive` to fail instead of prompting
- `--model-path` to pin a preferred model path when multiple candidates exist
- `--max-iterations` to cap the guided loop

## Contributor Start Here

Before changing implementation, read:

1. [`AGENTS.md`](AGENTS.md)
2. [`DESIGN.md`](DESIGN.md)
3. the active GitHub issue body

The repo is organized around issue-scoped changes on feature branches. Durable scope, progress, and next steps belong in GitHub issues rather than local plan files.

## Local Development

Common commands:

```bash
make build
make test
go build -o bin/concierge ./cmd/concierge
```

Key areas of the codebase:

- `cmd/concierge`: CLI entrypoint
- `internal/cli`: Cobra commands, prompts, and terminal rendering
- `internal/orchestrator`: outer run loop
- `internal/core`: stable domain types, ensure-step IDs, issue mapping
- `internal/adapters/*`: snapshot, inspect, planner, execute, validate, and report adapters
- `internal/agent`: agent runner and prompt contract
- `fixtures/`, `scripts/`, `QA/`: fixture prep, QA harness, and support tooling

## Fixtures And QA

Prepare and verify the fixture corpus:

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_mutate_cases.sh
bash scripts/fixtures_verify.sh
make test-fixtures
```

Run the QA loop against a prepared fixture checkpoint:

```bash
make qa
make qa REPO=mnist QA_STEP=preprocess
bash scripts/qa_fixture_run.sh --repo ultralytics --step input_encoders
```

The checkpoint selector surface is guide-native and fixture-first:

- list available fixtures with `python3 scripts/qa_checkpoint_resolver.py list-fixtures --repo-root .`
- list steps for one fixture with `python3 scripts/qa_checkpoint_resolver.py list-steps --repo-root . --fixture-id <fixture-id>`
- generated mutation cases are materialized under `.fixtures/cases/` and resolved through the checkpoint manifest used by the QA runner

If you want to test Concierge on another Tensorleap Hub repository, add a pinned entry to `fixtures/manifest.json`, then re-run fixture prepare and verify before using `make qa`. The fixture onboarding checklist is documented in [`AGENTS.md`](AGENTS.md), and the corpus details live in [`fixtures/README.md`](fixtures/README.md).

## CI And Releases

The current GitHub Actions flow is split into:

- `verify`: runs `make test` plus focused snapshot adapter coverage
- `fixtures`: prepares fixtures, generates mutation cases, verifies them, and runs fixture E2E tests
- `build-binaries`: cross-builds Linux and macOS binaries for `amd64` and `arm64`

Releases are triggered by semantic version tags such as `v0.0.1`. The release workflow runs tests and publishes artifacts through GoReleaser.

## More Docs

- [`DESIGN.md`](DESIGN.md): current architecture and implementation context
- [`GUIDE.md`](GUIDE.md): Tensorleap integration reference tracked in this repo
- [`QA/QA_LOOP.md`](QA/QA_LOOP.md): QA loop operator guide
- [`QA/DESIGN.md`](QA/DESIGN.md): QA harness design notes
- [`fixtures/README.md`](fixtures/README.md): fixture corpus workflow
- [`fixtures/cases/README.md`](fixtures/cases/README.md): generated mutation cases
