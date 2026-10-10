# CLAUDE.md — backpack

@AGENTS.md

## What Backpack is

A small Go service that keeps an agentic project's **delivery tracking** out
of the project's repository. Agent-driven projects commonly track progress in a
document committed beside the code, so every status change becomes a commit and
the document grows until the current state is hard to find. Backpack holds the
same information behind an HTTP API and an MCP server, for any number of
projects, so tracking is no longer a dependency of the code.

Backpack holds what ClickUp does not:

- the current **checkpoint** — where we are and what is next;
- **phases** with their exit conditions;
- which **ClickUp task** is tied to which branch and pull request, and what is
  in flight;
- the **evidence log** — what each branch changed, decisions and their why,
  tests run;
- an append-only **history** of every change.

**ClickUp stays the home of tasks.** Tasks are created, moved through their
statuses and closed in ClickUp, exactly as today. Backpack refers to tasks by
their ClickUp id and never keeps a second copy of their status.

It runs as one Docker container with an SQLite file on a volume, reached through
a Cloudflare tunnel. Every request is authenticated.

## Read this first, every session

1. Read `docs/PLAN.md` — until Backpack can track itself, it is the tracker for
   this repository: what is built, what is next, what is still undecided.
2. Read `docs/ENGINEERING-STANDARD.md` — TDD, YAGNI, branch and delivery
   hygiene. It governs all work here.
3. Check `git log --oneline -15` and `git status`.

## Default to plan mode

Enter plan mode for any request that writes or changes code. Questions (explain,
what/why/how, status) are answered directly.

Any change touching more than one file, or any new endpoint, MCP tool, table or
migration, is planned and explicitly approved before code is written.

## Plans are written for people

For each item: **what's wrong → why it happens → how we'll fix it**, in plain
language with no file paths or internals in the main body. Files, commands and
tests go in a "Technical appendix" at the end.

## Never guess

If requirements, scope or intent are not settled by the plan, the engineering
standard or the API contract, stop and ask.

## Commands

Not yet built. The Go gates are in `docs/ENGINEERING-STANDARD.md` §7.3.

## Off-limits

- Secrets — the API token and the Cloudflare tunnel credentials come from the
  environment. Never commit them, never echo them, never put them in a file the
  repository tracks.
- Tracked content belongs to other projects and may be confidential. Never
  paste it into this repository, an artifact, a published page, a test fixture
  or any external service. This repository is public.

## Memory and continuity

- **Backpack's own status** lives in `docs/PLAN.md` until Backpack is deployed
  (slice 4), then in Backpack itself.
- **Git history carries the "why."** Consequential decisions go in commit
  messages and pull request descriptions.
- **Auto-memory carries preferences** and context not derivable from the repo.

## Branching

`<type>/<YYYYMMDD>-<short-name>` — type is `bug` or `feature`, kebab-case name.
`main` is the integration branch.
