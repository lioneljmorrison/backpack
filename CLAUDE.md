# CLAUDE.md — backpack

@AGENTS.md

## What Backpack is

A Kanban board for agentic projects, built as a Go service. Agent-driven
projects commonly track progress in a document committed beside the code, so
every status change becomes a commit and the document grows until the current
state is hard to find. Backpack replaces that document: agents ask it "what's
next" or "where are we" and get a short answer, so tracking is no longer a
dependency of the code.

- Each project has its own **workspace**.
- Work starts as **ideas**, which are refined into **stories**, and into
  **phases** (epics with a description, goals and stories).
- Each workspace has **one backlog ranked in delivery order**. Rank is order,
  not urgency.
- **Dependencies** are explicit and read both ways (blocks / blocked by). A
  story always ranks below what it depends on.
- Stories move: Backlog → In progress → In review → Merged (complete) →
  Released → Deployed.
- Every story keeps an **append-only log**.

Two ways in: an **MCP server** for AI agents and an **HTTP API** for a UI.
**FalkorDB** is the store, with one graph per workspace. **Typesense** is a
search index rebuilt from it. It runs in Docker: on localhost for development,
and behind a Cloudflare tunnel on `nas` or `lab-hermes`. Every request is
authenticated. `docs/PLAN.md` has the full design.

## Read this first, every session

1. Read `docs/PLAN.md` — until Backpack can track itself, it is the tracker for
   this repository: what is built, what is next, what is still undecided.
2. Read `docs/ENGINEERING-STANDARD.md` — TDD, YAGNI, branch and delivery
   hygiene. It governs all work here.
3. Check `git log --oneline -15` and `git status`.

## Default to plan mode

Enter plan mode for any request that writes or changes code. Questions (explain,
what/why/how, status) are answered directly.

Any change touching more than one file, or any new endpoint, MCP tool, graph
node or edge, index or migration, is planned and explicitly approved before code is written.

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
  (slice 5), then in Backpack itself.
- **Git history carries the "why."** Consequential decisions go in commit
  messages and pull request descriptions.
- **Auto-memory carries preferences** and context not derivable from the repo.

## Branching

`<type>/<YYYYMMDD>-<short-name>` — type is `bug` or `feature`, kebab-case name.
**`dev` is the integration branch.** Every task branch starts from `dev` and
merges back into `dev` through a pull request — never a direct push. `main`
holds released work: `dev` is merged into `main` by a pull request when it is
ready to release. Neither `dev` nor `main` is committed to directly.
