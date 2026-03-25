# Claude Concierge QA Loop

## Goal

Provide one shared QA control loop that works the same way locally and in GitHub Actions:

- run Concierge inside a real PTY-backed terminal session
- let Claude observe terminal output and decide the next action
- preserve transcripts, turn logs, and final QA reports
- continue across review-only rerun boundaries until Concierge reaches finished integration, a coherent dead end, or a clear defect

This is a qualitative QA harness, not a deterministic assertion runner.

## Architecture

The loop has three parts:

1. `pty_driver.py`
   Runs Concierge in a pseudo terminal so the harness sees the same prompts and terminal behavior as a user.
2. `qa_loop.py`
   Supervises the session, captures artifacts, executes external shell commands when Claude asks for them, and decides when to stop.
3. Claude structured control/report steps
   Claude receives the cleaned transcript tail plus recent terminal output, then returns schema-validated JSON for both live control and the final report.

## Control Model

Each control turn returns one JSON directive with:

- `action`: `SEND_INPUT`, `WAIT`, or `RUN_COMMAND`
- `input_text`: terminal input or shell command
- `loop_state`: `CONTINUE`, `STOP_REPORT`, `STOP_FIX`, or `STOP_DEADEND`
- `summary`, `issues`, and `next_focus`

The supervisor:

1. reads terminal output until a prompt, quiet period, or timeout
2. asks Claude for the next structured directive
3. sends terminal input or runs the requested external shell command
4. records turn artifacts
5. repeats until the loop state stops or the runtime/idle limits are hit

Clean process exits do not automatically end QA. If Concierge exits after a reviewed step and tells the user to rerun `concierge run`, the supervisor should normally relaunch the command and continue the same QA session instead of treating that single-run boundary as completion.

## Blind-First Policy

The QA agent should behave like a normal user first.

- do not inspect the ground-truth fixture during early turns
- rely on visible terminal behavior while the flow is still making progress
- only after the session stalls may Claude inspect the provided post-fixture path

This keeps the early UX evaluation honest.

## Claude Invocation

The shared runtime path is Claude-only:

- local runs default to `claude`, overridable with `--claude-command` or `CLAUDE_BIN`
- CI installs Claude CLI and uses the same `QA/qa_loop.py` entrypoint
- structured output is validated against JSON schemas for both control and report generation
- the same `ANTHROPIC_API_KEY` powers the host-side supervisor and the fixture container

## Artifacts

Each run writes:

- `QA/runs/<run-id>/summary.json`
- `QA/runs/<run-id>/turns.jsonl`
- `QA/runs/<run-id>/claude/*`
- `QA/runs/<run-id>/docker/*`
- `QA/transcripts/<run-id>.*`
- `QA/reports/<run-id>.json`
- `QA/reports/<run-id>.md`

The `claude/` directory holds the raw structured control/report exchange artifacts. The higher-level transcript and report layout stays stable.

## Shared Workflow

`.github/workflows/qa-loop.yml` exposes a manual `workflow_dispatch` entrypoint for:

- `ref`
- `fixture`
- `step`
- optional `run_id`, `issue_number`, and `pr_number`

The workflow checks out the requested same-repo ref, installs the QA toolchain, runs `scripts/qa_fixture_run.sh`, uploads QA artifacts, writes a concise run summary, and can post the same result back to a linked issue or PR.

## Scope Boundaries

This design intentionally does not include:

- scheduled QA runs
- comment-triggered dispatch
- multi-provider abstraction
- automatic repair execution after `STOP_FIX`

Those can be added in later issue-scoped slices if needed.
