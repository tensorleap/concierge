# Proposal: Browser-Primary Local UI For Concierge

## Status

This document captures the product thinking from an initial brainstorming session about a web UI for Concierge. It is intentionally focused on product UX and session behavior, not implementation design.

## Problem

Today, Concierge is a terminal-first experience. That keeps the runtime model simple, but it also makes some important parts of the product harder to understand than they should be:

- the user mostly experiences progress as scrolling terminal output
- revisiting earlier evidence or completed steps is cumbersome
- approvals, diffs, rationale, and validation output compete for the same narrow surface
- Concierge can only show one thing well at a time even when the user wants both "what is happening now" and "what already happened"

The core idea is to keep Concierge running locally from a terminal, but have `concierge run --web` spawn a local web server and auto-open a browser UI that becomes the primary interaction surface for that run.

## Recommended Product Shape

The recommended direction is a `Single-Session Local App`.

That means:

- each `concierge run --web` invocation creates one fresh local browser session
- the browser is the primary interaction surface for that run
- the terminal remains the process owner and recovery anchor, not the main UI
- the session is local, single-run, and browser-required
- the UI is optimized for a serial Concierge engine with richer observability, not for parallel execution

This is not just "prettier output." It is a distinct interaction mode with its own UX contract.

## Alternatives Considered

### 1. Thin Companion

A lightweight browser companion that mostly mirrors terminal state, prompts, and diffs.

Why not this:

- it undershoots the main goal
- it does not truly replace terminal scrollback as the primary session record
- it still leaves the terminal as the real product surface

### 2. Single-Session Local App

A browser-primary UI for one live Concierge run, with enough history to revisit what happened during that run.

Why this is the recommendation:

- it matches the desired UX without turning Concierge into a much larger local platform
- it keeps the current mental model of "run Concierge from a terminal" intact
- it makes the browser a real cockpit instead of a passive mirror

### 3. Local Operator Console

A larger local application with multiple sessions, cross-run browsing, stronger session identity, and potentially multi-repo views.

Why not now:

- it is a different ambition level
- it adds product surface that was not requested
- it would push the project from "browser-primary run mode" into "persistent local app platform"

## Core Product Decisions

### Session Model

- `concierge run --web` is a browser-required mode.
- Concierge should auto-open the browser by default.
- If the auto-open fails or the user closes the tab, there is no fallback to terminal interaction in `--web` mode.
- The terminal should still print the local URL clearly so the user can return to the UI.
- If the browser disconnects mid-run, Concierge keeps running headless and the browser can reconnect later.
- Each invocation is a fresh session, not a continuation of the previous run.
- The user should not have to think in terms of strong session identity. A visible session ID is not part of the default UX.

### Execution Model

- Concierge remains serial.
- The web UI does not imply concurrency.
- The value of the browser is richer presentation, revisitable history, and a better decision surface.
- The active part of the UI must always make the current progress legible, even while the user browses older steps.

### Terminal Role

In `--web` mode, the terminal is mainly:

- the launcher
- the owner of the running process
- the place where the local URL is shown again if needed
- a recovery path if the browser is closed and needs to be reopened manually

The terminal is not the primary place for prompts, approvals, or detailed run comprehension in this mode.

### Browser Role

The browser is the primary product surface for:

- current activity
- approvals and prompts
- evidence inspection
- diff review
- step history
- error triage
- final outcome review

All meaningful interaction that would normally happen in terminal prompts should move to the browser in `--web` mode.

## Information Architecture

### Default Screen: Live Cockpit

The browser should open into a live cockpit optimized first for one question:

`What is Concierge doing right now?`

This means:

- current activity is the primary focus
- active step and current substage are prominent
- latest evidence and status are nearby
- the user should not need to inspect logs to understand the current state

The UI should not open on a dashboard grid or a static checklist overview.

### Persistent Step Rail

The session should expose the full expected Concierge workflow from the start.

- all steps are visible immediately
- untouched steps are still shown
- the mapping should use Concierge's existing step model rather than a second user-facing taxonomy

This keeps the UI aligned with the engine and avoids maintaining a second conceptual model that can drift.

### Step Detail Model

Navigation hierarchy:

`step -> attempts`

For a selected step:

- the latest or current attempt should be shown first
- prior attempts should remain accessible as history for that same step
- each attempt should carry its own rationale, evidence, logs, diff, approval state, and validation result

This creates a step dossier rather than a flat event feed.

### Live Presence While Browsing History

Users should be able to inspect older steps without losing awareness of the live run.

The preferred behavior is:

- a persistent live strip or "now" bar visible everywhere
- a persistent decision banner when user input is required
- no forced snap-back to the live cockpit just because the run advances

This allows history browsing without losing the active session context.

## Interaction Model

### Browser-Owned Prompts

In `--web` mode, the browser should own all current prompts that would otherwise happen in the terminal.

Examples include:

- approvals before changes
- diff review decisions
- selection prompts
- confirmations
- any other interactive decisions required during a run

This is important because a mixed authority model between stdin and browser would be confusing and fragile from a UX perspective.

### Approval UX

Approval moments should behave like a review workspace, not a plain modal confirmation.

The preferred shape is:

- concise rationale summary at the top
- relevant diff and evidence immediately below
- decision controls always visible
- the user can inspect context before answering without losing the action controls

The UI should be prominent during a decision, but not so blocking that the user cannot inspect the supporting material first.

### Error UX

Hard failures should open into a triage workspace:

- a plain-language summary first
- relevant raw evidence immediately below it
- enough context to understand both what failed and what the user can do next

The UI should not force the user to choose between a friendly summary and actual evidence.

## Completion Behavior

When the run ends, the browser should become a completion screen.

That completion state should:

- clearly communicate the final outcome
- keep enough session history available to inspect what happened
- not try to become a full multi-session history app

The emphasis is still on guiding a single run well.

## Persistence Expectations

Product-wise, the browser experience assumes local persistence by default, because browser presentation cannot rely on scrollback the way a terminal can.

The important UX conclusion is:

- session state is product data, not just debug output

This proposal intentionally does not choose an implementation yet. It only records the product implication that a browser-primary run needs durable local state for the duration of the run and for post-run inspection.

## Explicit Non-Goals

This proposal does not assume:

- concurrent execution
- multiple live sessions in one UI
- a long-lived local dashboard across runs
- a new step taxonomy separate from Concierge's existing step model
- terminal fallback in `concierge run --web`
- a browser-gated preflight start screen

The run should start immediately when invoked.

## Product Principles

The design so far can be summarized as:

- browser-primary, terminal-launched
- serial execution, richer observability
- current activity first
- step-first navigation
- latest attempt first within each step
- inspectable history without losing live awareness
- browser-owned interaction model
- local, lightweight, single-session feel

## Open Questions Deferred On Purpose

The following were deliberately not resolved yet:

- exact page layout and component design
- transport model between Concierge and the local web UI
- state schema and artifact model
- how diffs, logs, and evidence should be rendered in detail
- how the browser server should be started, discovered, and secured locally
- whether completion history should later grow into richer cross-run browsing

Those are design and implementation questions for a later phase if this direction is pursued.
