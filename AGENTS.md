# Agent Workflow Agreement

This repository is implemented through small, issue-scoped changes.

## Session Start Requirements (Mandatory)

- Before any planning or implementation work, read `README.md` in its entirety.
- Then read the relevant GitHub issue, PR description, or user-provided task context for the work you are about to do.
- If no GitHub issue exists yet, treat the current user request as the temporary source of truth and keep the scope narrow enough to become one issue later.

## Planning and Progress Tracking

- GitHub issues are the source of truth for backlog, prioritization, and progress tracking.
- Do not create, maintain, or rely on `PLAN.md` for new work tracking.
- Prefer one issue per independently reviewable bug, missing piece, or feature slice.
- If a task spans multiple concerns, split it into separate GitHub issues instead of rebuilding a waterfall plan in-repo.

## Execution Rules

- Implement only one issue-sized scope at a time.
- Before any `git commit` or `git push`, stop and let the user review changes locally; proceed only after explicit user approval.
- If a change set does not modify code (for example markdown/docs-only edits), test execution is optional and may be skipped.
- After finishing the scoped work, keep progress in GitHub issue / PR state rather than updating `PLAN.md`.
- The agent may commit and push issue-scoped changes on a non-main feature branch before acceptance.
- The agent should trigger and monitor CI for the pushed branch and fix failures within the issue scope.
- Do not treat work as accepted until it is merged to `main`.
- A merge to `main` is the only acceptance event.
- Commit only one issue's scope at a time, then move to the next issue.

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

- `git worktree add ../concierge-issue-<issue-id>-<short-name> -b feature/issue-<issue-id>-<short-name>`
- `cd ../concierge-issue-<issue-id>-<short-name>`

### Required pre-commit gate

Before every commit, verify branch again:

- `git rev-parse --abbrev-ref HEAD`

If branch is `main` or `master`, stop and do not commit.

### Required post-issue flow

After finishing one issue-sized change:

1. Request and receive explicit user approval after local review.
2. Build the CLI binary locally: `go build -o bin/concierge ./cmd/concierge`.
3. Commit on feature branch.
4. Push feature branch.
5. Open PR to `main`.
6. Monitor CI for that PR branch and fix failures in issue scope.
7. Link the PR to the relevant GitHub issue when one exists.

### Accidental protected-branch commit protocol

If a commit is made on `main` or `master` by mistake:

- Stop immediately.
- Do not continue implementation.
- Ask the user whether to revert and re-apply on a feature branch, or keep as-is.

## Release Deliverables (Mandatory)

- Release-related work must produce a Go CLI release pipeline that builds binaries for Linux and macOS on both `amd64` and `arm64`.
- Release-related work must use semantic version tags (e.g. `v0.0.1`) and publish release notes with each release.

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
- `strip_for_pre`: keep as `["leap.yaml", "leap_binder.py", "leap_custom_test.py"]` unless issue requirements change

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
