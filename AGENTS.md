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
- Before any `git commit` or `git push`, stop and let the user review changes locally; proceed only after explicit user approval.
- If a change set does not modify code (for example markdown/docs-only edits), test execution is optional and may be skipped.
- After finishing a step, update its status in `PLAN.md` to `DONE` only after commit, push, PR creation, and passing branch CI.
- The agent may commit and push step changes on a non-main feature branch before acceptance.
- The agent should trigger and monitor CI for the pushed branch and fix failures within the step scope.
- Do not mark a step as `ACCEPTED` until it is merged to `main`.
- A merge to `main` is the only acceptance event.
- Commit only one step's scope at a time, then move to the next `PENDING` step.

## Branch Safety (Mandatory, Hard Stop)

- Never commit directly to `main` or `master` unless specifically instructed to do so by the user.
- Never push directly to `main` or `master` unless specifically instructed to do so by the user.
- All implementation work must be done on a feature branch and merged via PR.
- Use `git worktree` as the default mechanism for branch creation and branch switching.
- Do not rely on regular branch checkouts (`git checkout` / `git switch`) for implementation flow between branches.

### Required pre-implementation git checks

Run these before any file edits or commits:

1. `git rev-parse --abbrev-ref HEAD`
2. `git status --short --branch`

If current branch is `main` or `master`, create and switch immediately using a new worktree:

- `git worktree add ../concierge-step-<step-id>-<short-name> -b feature/step-<step-id>-<short-name>`
- `cd ../concierge-step-<step-id>-<short-name>`

### Required pre-commit gate

Before every commit, verify branch again:

- `git rev-parse --abbrev-ref HEAD`

If branch is `main` or `master`, stop and do not commit.

### Required post-step flow

After finishing one step:

1. Request and receive explicit user approval after local review.
2. Build the CLI binary locally: `go build -o bin/concierge ./cmd/concierge`.
3. Commit on feature branch.
4. Push feature branch.
5. Open PR to `main`.
6. Monitor CI for that PR branch and fix failures in step scope.

Only then update step status to `DONE`.

### Accidental protected-branch commit protocol

If a commit is made on `main` or `master` by mistake:

- Stop immediately.
- Do not continue implementation.
- Ask the user whether to revert and re-apply on a feature branch, or keep as-is.

## Step 2 Release Deliverables (Mandatory)

- Step 2 must produce a Go CLI release pipeline that builds binaries for Linux and macOS on both `amd64` and `arm64`.
- Step 2 must use semantic version tags (e.g. `v0.0.1`) and publish release notes with each release.

## Tensorleap Hub Fixture Onboarding

Use this process when adding another `tensorleap-hub` repository to `fixtures/manifest.json`.

### 1) Pick and pin a stable post-integration commit

- Choose a concrete commit SHA from the target repo. Do not use branches or tags as `post_ref`.
- Prefer a commit that already represents a working integrated state.

### 2) Suitability gate (mandatory before manifest edit)

For the candidate `<repo>` and `<sha>`, verify all required integration artifacts exist at repo root:

- `leap.yaml`
- `leap_binder.py`
- `leap_custom_test.py`

Example check:

```bash
tmp="$(mktemp -d)"
git clone --filter=blob:none --no-checkout "https://github.com/tensorleap-hub/<repo>.git" "$tmp/repo"
cd "$tmp/repo"
git cat-file -e "<sha>^{commit}"
for f in leap.yaml leap_binder.py leap_custom_test.py; do
  git ls-tree --name-only "<sha>" -- "$f" | grep -qx "$f"
done
```

If any file is missing, do not add the fixture until a suitable commit is identified.

### 3) Add manifest entry

Add one object under `fixtures[].` with:

- `id`: lowercase stable identifier (letters, numbers, underscore preferred)
- `repo`: full HTTPS clone URL
- `post_ref`: full 40-char commit SHA
- `strip_for_pre`: keep as `["leap.yaml", "leap_binder.py", "leap_custom_test.py"]` unless Step requirements change

### 4) Validate fixture generation end to end

Run:

```bash
bash scripts/fixtures_prepare.sh
bash scripts/fixtures_verify.sh
```

Both commands must pass. `prepare` must create:

- `.fixtures/<id>/post` at `post_ref`
- `.fixtures/<id>/pre` derived from `post` with stripped integration files

`verify` must confirm:

- stripped files exist in `post`
- stripped files do not exist in `pre`
- both repos are clean git trees

## Project Skills

This repository also provides a project-local skill:

- `concierge-qa-loop`
  Path: `QA/SKILL.md`
  Use when the user asks to perform QA, smoke-test Concierge, manually test a fixture, evaluate terminal UX, or report QA findings. The skill runs `python3 QA/qa_loop.py`, waits for the saved report under `QA/reports/`, and summarizes the final findings from the generated artifacts.
