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
- `DONE` means implemented, committed, pushed by the agent, and ready for branch CI and PR review.
- `ACCEPTED` means the step has been merged into the `main` branch.

## Execution Rules

- Implement only one step at a time.
- After finishing a step, update its status in `PLAN.md` to `DONE`.
- The agent may commit and push `DONE` step changes before acceptance.
- The agent should trigger and monitor CI for the pushed branch and fix failures within the step scope.
- Do not mark a step as `ACCEPTED` until it is merged to `main`.
- A merge to `main` is the only acceptance event.
- Commit only one step's scope at a time, then move to the next `PENDING` step.

## Step 2 Release Deliverables (Mandatory)

- Step 2 must produce a Go CLI release pipeline that builds binaries for Linux and macOS on both `amd64` and `arm64`.
- Step 2 must use semantic version tags (e.g. `v0.0.1`) and publish release notes with each release.
