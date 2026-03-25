---
name: concierge-qa-loop
description: Run the Concierge QA loop for this repository when asked to perform QA, smoke-test Concierge, manually test a fixture, evaluate terminal UX, or report QA findings. Use for requests like "perform QA", "run QA on ultralytics", "smoke test Concierge", "manually test it", or "evaluate the flow". This skill runs `python3 QA/qa_loop.py`, waits for the saved report under `QA/reports/`, and summarizes the final findings.
---

# Concierge QA Loop

Use this skill when the user wants QA or a manual smoke of Concierge in this repository.

## Default workflow

1. Work from the repository root.
2. Treat built-in QA fixtures as the repos declared in `fixtures/manifest.json` only.
   Use fixture ids from that manifest and the corresponding prepared paths under `.fixtures/<id>/pre` and `.fixtures/<id>/post`.
   Do not pick repos under `.fixtures/cases/` as built-in QA fixtures. Those generated case repos are for automated validation coverage, not the default manual/QA loop fixture corpus.
   If the user explicitly asks to run QA against a generated case repo, treat it as an arbitrary repo path instead of a built-in fixture.
3. If the user wants a built-in fixture and `.fixtures/<id>/pre` is missing, prepare fixtures first:
   `bash scripts/fixtures_prepare.sh`
   `bash scripts/fixtures_verify.sh`
4. Choose a stable run id that includes the fixture or target name plus the date.
5. Run the QA loop.

Built-in fixture:

```bash
bash scripts/qa_fixture_run.sh --repo <fixture-id> --step <guide-step> -- \
  --run-id <run-id>
```

Already-running container:

```bash
python3 QA/qa_loop.py \
  --run-id <run-id> \
  --container-name <running-container> \
  --container-workdir /workspace
```

6. Let the run finish. Do not stop early unless the command is clearly wedged and not writing artifacts.
7. Read the final artifacts in this order:
   - `QA/reports/<run-id>.md`
   - `QA/runs/<run-id>/summary.json`
   - `QA/transcripts/<run-id>.terminal.txt` only if the report needs supporting detail
8. Report the findings to the user. Lead with the actual defects or blockers, not the mechanics of the harness run.
9. If the QA run should become a GitHub issue or PR update, generate the inline evidence bundle first:
   `python3 scripts/qa_issue_evidence.py --run-id <run-id>`
   Paste that markdown into the issue or PR body. Local artifact paths may stay as secondary breadcrumbs, but they cannot be the only durable evidence.

## Output expectations

When answering the user after QA:

- State whether the run worked end to end or stopped on a product defect.
- Name the final loop state and stop reason from `summary.json`.
- Summarize the important findings from the markdown report.
- Link the saved report and summary paths.
- When promoting a QA run into a GitHub issue or PR description, include the helper-generated inline evidence bundle instead of relying only on local file paths.

## Guardrails

- Do not write new test code as part of QA unless the user explicitly asks for it.
- Prefer the QA loop over ad hoc manual testing when the goal is Concierge UX or workflow QA.
- When choosing a built-in fixture automatically, select only from `fixtures/manifest.json`, never from `.fixtures/cases/`.
- If the report file lags behind `summary.json`, wait for it before replying.
- If the user asks to fix issues found by QA, finish the QA run first, then switch to implementation work.
- Built-in fixtures should run through `scripts/qa_fixture_run.sh` so the target container is built from the clean `pre` repo only.

## References

- Operator guide: `QA/QA_LOOP.md`
- Design doc: `QA/DESIGN.md`
