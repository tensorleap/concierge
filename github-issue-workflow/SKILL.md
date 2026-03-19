---
name: concierge-issue-workflow
description: Use when a Concierge conversation is anchored on one or more GitHub issues, such as "look at issue 41", "resume issue 46", or when durable scope, decisions, progress, and next steps should live in GitHub issue bodies instead of PLAN.md, local notes, or terminal chat.
---

# Concierge GitHub Issue Workflow

Use this skill when GitHub issues are the work anchor for this repository.

## Core Rules

- The GitHub issue body is the durable source of truth for scope, decisions, current status, and next step.
- Terminal chat is transient working memory, not the durable project record.
- Milestone comments provide visibility and audit history, but they are not the primary spec.
- Do not use `PLAN.md`, local task lists, or scratch progress notes for ongoing issue-managed work in this repository.
- If another planning skill suggests a local plan document, keep any durable outcome in the relevant GitHub issue body or issue hierarchy instead.

## When To Trigger

Use this skill when:

- the user starts from a GitHub issue, for example "look at issue 41", "read issue 41 and resume", or "continue issue 46"
- the conversation later becomes anchored on a GitHub issue
- a multi-step effort needs durable state that another agent should be able to resume from GitHub alone

## Start Procedure

1. Read `README.md`.
2. Read the active GitHub issue body.
3. If the active issue is a child issue, also read the umbrella issue body, but treat the active child issue as the primary execution record.
4. Before creating a new issue branch or worktree from `main`, sync the local `main` worktree with `origin/main` and verify it is at tip. Do not branch from a stale local protected branch.
5. Read comments only if the body points to them or if recent milestone context is actually needed.
6. If comments contain durable facts that are missing from the body, fold those facts back into the body before continuing.

## Body Maintenance

- Rewrite the issue body freely so it always reflects the current truth.
- Do not preserve stale wording for history's sake.
- Do not keep an edit log inside the body.
- Avoid boilerplate templates. Natural prose and short lists are preferred if they keep the issue resumable.
- Keep the body sufficient for another agent to resume from "look at issue N and continue" without needing this terminal conversation.

## Milestone Comments

Leave a comment only for milestone-level changes, and only after updating the body.

Good reasons to leave a milestone comment:

- the work is split into umbrella and child issues
- a major decision or scope change lands
- a blocker is discovered or resolved
- an implementation slice is completed
- a PR is opened or CI status becomes materially relevant
- QA reaches a notable result
- the issue body is substantially rewritten to reflect a new project state

Each milestone comment should say what changed in the body and why.

## Issue Shaping

- Prefer one issue per independently reviewable slice.
- If the work spans multiple reviewable slices, split it into child issues instead of rebuilding a local waterfall plan.
- Use the umbrella issue for the overall goal, linked slices, and high-level status.
- Use the active child issue body as the primary execution record for day-to-day work.
- Keep sibling issues independent enough that future agents usually need only the umbrella and active child issue bodies.

## Resume Standard

At any point, another agent should be able to:

1. read `README.md`
2. read the active issue body
3. optionally read the umbrella issue body
4. continue the work without needing prior terminal chat
