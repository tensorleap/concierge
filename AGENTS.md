# Agent Workflow Agreement

This repository is implemented step by step.

## Session Start Requirements (Mandatory)

- Before any planning or implementation work, read `README.md` in its entirety.
- Immediately after that, read `PLAN.md`.
- This read order is mandatory for every new session on this project.

## Planning and Progress Tracking

- Keep a living implementation document in `PLAN.md`.
- Represent each implementation step with exactly one status: `PENDING`, `DONE`, or `ACCEPTED`.
- `PENDING` means not implemented yet.
- `DONE` means implemented, committed and pushed on a non-main feature branch, PR opened, branch CI green, and ready for review.
- `ACCEPTED` means the step has been merged into the `main` branch.

## Execution Rules

- Implement only one step at a time.
- After finishing a step, update its status in `PLAN.md` to `DONE` only after commit, push, PR creation, and passing branch CI.
- The agent may commit and push step changes on a non-main feature branch before acceptance.
- The agent should trigger and monitor CI for the pushed branch and fix failures within the step scope.
- Do not mark a step as `ACCEPTED` until it is merged to `main`.
- A merge to `main` is the only acceptance event.
- Commit only one step's scope at a time, then move to the next `PENDING` step.

## Branch Safety (Mandatory, Hard Stop)

- Never commit directly to `main` or `master`.
- Never push directly to `main` or `master`.
- All implementation work must be done on a feature branch and merged via PR.

### Required pre-implementation git checks

Run these before any file edits or commits:

1. `git rev-parse --abbrev-ref HEAD`
2. `git status --short --branch`

If current branch is `main` or `master`, create and switch immediately:

- `git checkout -b feature/step-<step-id>-<short-name>`

### Required pre-commit gate

Before every commit, verify branch again:

- `git rev-parse --abbrev-ref HEAD`

If branch is `main` or `master`, stop and do not commit.

### Required post-step flow

After finishing one step:

1. Commit on feature branch.
2. Push feature branch.
3. Open PR to `main`.
4. Monitor CI for that PR branch and fix failures in step scope.

Only then update step status to `DONE`.

### Accidental protected-branch commit protocol

If a commit is made on `main` or `master` by mistake:

- Stop immediately.
- Do not continue implementation.
- Ask the user whether to revert and re-apply on a feature branch, or keep as-is.

## Step 2 Release Deliverables (Mandatory)

- Step 2 must produce a Go CLI release pipeline that builds binaries for Linux and macOS on both `amd64` and `arm64`.
- Step 2 must use semantic version tags (e.g. `v0.0.1`) and publish release notes with each release.
