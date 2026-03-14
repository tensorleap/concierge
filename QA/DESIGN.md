# Codex Concierge QA Loop

Design & Implementation Plan

## Goal

Create a small autonomous harness that lets **Codex act as a QA
engineer** and drive the **Concierge CLI interactively** in a terminal
exactly like a human user.

The system should: - Run Concierge in a real terminal environment -
Allow Codex to read the exact terminal output - Allow Codex to type
responses like a user - Produce qualitative UX and product-fit
feedback - Stop when it reaches completion, a dead-end, or a major issue

This is not a deterministic test framework. This is a subjective
evaluation harness.

------------------------------------------------------------------------

# Architecture

The system consists of three parts:

1.  Supervisor Loop
2.  PTY Driver
3.  Codex Session

Supervisor ├─ launches Concierge in PTY ├─ launches Codex session ├─
forwards terminal output to Codex ├─ executes Codex actions └─ stops
when Codex signals completion

------------------------------------------------------------------------

# Component 1: PTY Driver

Purpose: Run Concierge inside a pseudo terminal so that Codex sees
exactly what a user sees.

Responsibilities: - spawn Concierge - capture stdout/stderr - send user
input - expose terminal transcript

Example interface:

pty.start(command) pty.read() pty.send(text) pty.stop() pty.is_running()

Recommended implementation: Python using:

-   pty
-   subprocess
-   select

or

-   pexpect

------------------------------------------------------------------------

# Component 2: Codex QA Agent

Codex acts as:

Synthetic QA engineer + synthetic user

Responsibilities:

-   Read terminal output
-   Decide what input to send next
-   Detect confusing UX
-   Detect misleading instructions
-   Drive integration forward
-   Stop when significant issues appear

Codex should produce a control directive at the end of each turn:

LOOP: CONTINUE LOOP: STOP_REPORT LOOP: STOP_FIX LOOP: STOP_DEADEND

------------------------------------------------------------------------

# Component 3: Supervisor Loop

Responsibilities:

-   start PTY
-   start Codex
-   relay terminal output
-   execute Codex commands
-   prevent infinite loops

Supervisor Algorithm

start concierge PTY

start codex session

while iterations \< max_iterations:

    terminal_output = pty.read()

    send terminal_output to codex

    codex returns:
        action
        input_text
        loop_state

    if input_text:
        pty.send(input_text)

    if loop_state != CONTINUE:
        break

stop concierge

generate report

------------------------------------------------------------------------

# Blind‑First Evaluation Strategy

Codex should initially behave like a real user.

Rules:

-   Do NOT inspect the ground-truth fixture
-   Follow Concierge instructions naturally
-   Evaluate wording and UX clarity

Only after progress stalls may Codex:

-   inspect the post-fixture
-   compare expected output
-   assess correctness

------------------------------------------------------------------------

# Report Output

Codex should produce a report including:

Integration Progress - how far Concierge progressed - whether the
workflow felt coherent

UX Clarity - confusing prompts - ambiguous wording - excessive verbosity

Product Issues - misleading instructions - incorrect reasoning - failure
to detect completed tasks

Agent Interaction Issues - miscommunication between Concierge and the
coding agent

Suggestions - improvements to prompts - workflow adjustments - missing
guardrails

------------------------------------------------------------------------

# File Structure

QA/

qa_loop.py pty_driver.py

prompts/ role_prompt.md nudge_prompt.md

runs/ transcripts/ reports/

------------------------------------------------------------------------

# Prompt Design

Role Prompt

Codex should be instructed:

-   You are a QA engineer
-   Act as the user of Concierge
-   Drive the workflow forward
-   Evaluate UX quality
-   Avoid asking humans for guidance

------------------------------------------------------------------------

# Iteration Limits

Prevent infinite loops with:

max_iterations = 30 max_idle_turns = 5 max_runtime = 1h

------------------------------------------------------------------------

# Phase 1 Milestone

Minimal working system:

-   PTY driver
-   Codex session
-   supervisor loop
-   final report

No auto-fix mode yet.

------------------------------------------------------------------------

# Phase 2 (Optional)

Add auto-fix workflow.

If Codex emits:

LOOP: STOP_FIX

Supervisor launches a new Codex repair session against the Concierge
repository.

Repair must:

-   reproduce the issue
-   attempt a fix
-   rerun the QA scenario

------------------------------------------------------------------------

# Acceptance Criteria

The system is successful when:

-   Codex can drive Concierge end‑to‑end
-   Codex produces a useful qualitative report
-   the harness requires no human input
-   the run can be repeated on different fixture repositories

------------------------------------------------------------------------

# End
