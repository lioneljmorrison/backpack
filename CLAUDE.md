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
- Stories move: (Proposed →) Backlog → In progress → In review → Merged
  (complete) → Released → Deployed. A story an agent adds is Proposed until a
  person approves it.
- Every story keeps an **append-only log**.

Two ways in: an **MCP server** for AI agents and an **HTTP API** for a UI.
**FalkorDB** is the store, with one graph per workspace. **Typesense** is a
search index rebuilt from it. It runs in Docker on any host. The dev instance
runs on localhost and is reached at `backpack.eldernerds.us` through a
Cloudflare tunnel. Every request is authenticated. `docs/PLAN.md` has the full
design.

## Read this first, every session

1. The session starts with "where are we" from Backpack (see "How this project
   tracks"). `docs/PLAN.md` is the design record: how the board works and why.
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

```bash
make up                  # build and start backpack, FalkorDB and Typesense
make tunnel              # the same, plus the Cloudflare tunnel (needs CLOUDFLARE_TOKEN)
make down                # stop (data volumes are kept)
make gates               # gofmt, vet, tests against the local stack, build
make token ACTOR=<name>  # issue a bearer token (shown once)
```

Local ports, all bound to 127.0.0.1: backpack 8780, FalkorDB 16379, Typesense
18108. Configuration is environment variables: `BACKPACK_ADDR`,
`BACKPACK_FALKORDB_URL`, `BACKPACK_TYPESENSE_URL`, `BACKPACK_SYSTEM_GRAPH`.
`ENVs.txt` (gitignored, never read into context unless needed) holds deploy
secrets.

## Off-limits

- Secrets — the API token and the Cloudflare tunnel credentials come from the
  environment. Never commit them, never echo them, never put them in a file the
  repository tracks.
- Tracked content belongs to other projects and may be confidential. Never
  paste it into this repository, an artifact, a published page, a test fixture
  or any external service. This repository is public.

## Memory and continuity

- **Backpack's own status** lives in Backpack itself, workspace `backpack`.
- **Git history carries the "why."** Consequential decisions go in commit
  messages and pull request descriptions.
- **Auto-memory carries preferences** and context not derivable from the repo.

## How this project tracks

Delivery is tracked in Backpack, workspace `backpack` (see `.backpack.json`),
through the `backpack` MCP tools, added to Claude Code once per machine (see
`README.md`). There is no tracking document, and tracking is never committed.

- **Orientation.** The session starts with "where are we". Ask `whats_next`
  for the next story and `get_story` for its detail. Don't guess the next task.
- **Record each move as it happens**, with `set_status`:
  - start work: `in_progress`, with the task branch;
  - open the pull request: `in_review`, with its link;
  - merged into `dev`: `merged`, with the merge commit;
  - released when `dev` is merged into `main`.
- **While in progress,** keep the story's activity current with
  `update_story`: planning, building, testing, or waiting (and on what).
- **Log as you go** with `log`: decisions with their reason, notable commits,
  notes. Entries are never edited; fix a mistake with a `correction` that names
  the entry it corrects.
- **Stuck or no longer needed:** `on_hold` or `closed`, with a reason.
- **New work** goes in with `add_story`, placed with `before` or `after`, with
  the `reasoning` for that spot. It waits as **proposed** until a person
  approves it (`set_status` to `backlog`, after `move_story` if they pick
  another spot) or declines it (`closed`, with a reason). Then add its
  dependencies with `add_dependency`. Backpack refuses an order that breaks a
  dependency; read its reason and place the story again.
- **Rough thoughts** that aren't work yet go in with `add_idea`, also with
  `reasoning`, and wait as proposed until a person approves them. Once the
  stories or phase it became exist, `refine_idea` links them and closes the
  idea; `drop_idea` closes one that became nothing, with a reason.

## Branching

`<type>/<YYYYMMDD>-<short-name>` — type is `bug` or `feature`, kebab-case name.
**`dev` is the integration branch.** Every task branch starts from `dev` and
merges back into `dev` through a pull request — never a direct push. `main`
holds released work: `dev` is merged into `main` by a pull request when it is
ready to release. Neither `dev` nor `main` is committed to directly.
