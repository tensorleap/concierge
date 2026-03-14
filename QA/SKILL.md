---
name: concierge-qa-loop
description: Run the Concierge QA loop for this repository when asked to perform QA, smoke-test Concierge, manually test a fixture, evaluate terminal UX, or report QA findings. Use for requests like "perform QA", "run QA on ultralytics", "smoke test Concierge", "manually test it", or "evaluate the flow". This skill runs `python3 QA/qa_loop.py`, waits for the saved report under `QA/reports/`, and summarizes the final findings.
---

# Concierge QA Loop

Use this skill when the user wants QA or a manual smoke of Concierge in this repository.

## Default workflow

1. Work from the repository root.
2. If the user wants a built-in fixture and `.fixtures/<id>/pre` is missing, prepare fixtures first:
   `bash scripts/fixtures_prepare.sh`
   `bash scripts/fixtures_verify.sh`
3. Choose a stable run id that includes the fixture or target name plus the date.
4. Run the QA loop.

Built-in fixture:

```bash
python3 QA/qa_loop.py \
  --run-id <run-id> \
  --command-cwd .fixtures/<fixture-id>/pre \
  --fixture-post-path .fixtures/<fixture-id>/post
```

Arbitrary repo:

```bash
python3 QA/qa_loop.py \
  --run-id <run-id> \
  --command-cwd /path/to/target-repo
```

5. Let the run finish. Do not stop early unless the command is clearly wedged and not writing artifacts.
6. Read the final artifacts in this order:
   - `QA/reports/<run-id>.md`
   - `QA/runs/<run-id>/summary.json`
   - `QA/transcripts/<run-id>.terminal.txt` only if the report needs supporting detail
7. Report the findings to the user. Lead with the actual defects or blockers, not the mechanics of the harness run.

## Output expectations

When answering the user after QA:

- State whether the run worked end to end or stopped on a product defect.
- Name the final loop state and stop reason from `summary.json`.
- Summarize the important findings from the markdown report.
- Link the saved report and summary paths.

## Guardrails

- Do not write new test code as part of QA unless the user explicitly asks for it.
- Prefer the QA loop over ad hoc manual testing when the goal is Concierge UX or workflow QA.
- If the report file lags behind `summary.json`, wait for it before replying.
- If the user asks to fix issues found by QA, finish the QA run first, then switch to implementation work.

## References

- Operator guide: `QA/QA_LOOP.md`
- Design doc: `QA/DESIGN.md`
