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
  (complete, one column per branch: Merged (dev), Merged (main)) → Released
  (in an official release, cut with `cut_release`). A story an agent adds is
  Proposed until a person approves it.
- Every story keeps an **append-only log**.

Three ways in: an **MCP server** for AI agents, an **HTTP API**, and **web
pages** for people (sign in once with a pasted token).
**FalkorDB** is the store, with one graph per workspace. **Typesense** is a
search index rebuilt from it. It runs in Docker on any host. The dev instance
runs on a separate Docker host on the LAN, driven from here through a Docker
context, and is reached at `backpack.eldernerds.us` through a Cloudflare
tunnel. Every request is authenticated. `docs/PLAN.md` has the full
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
Working through the backlog (below) is the one exception: there, an approved
story's plan is the approved plan.

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
make forward             # bring a remote stack's ports here over SSH
make gates               # gofmt, vet, tests against the stack's ports here, build
make token ACTOR=<name>  # issue a bearer token (shown once)
make reindex             # rebuild the search index from the board
```

Ports, all bound to the stack host's 127.0.0.1: backpack 8780, FalkorDB 16379,
Typesense 18108. The commands drive the local Docker unless
`BACKPACK_DOCKER_CONTEXT` names another Docker host (a `docker context`, set
in the shell profile, never in a tracked file). With a remote stack, run
`make forward` before `make gates`. `make tunnel` refuses to start when the
public address is already served from another host. Configuration is environment variables: `BACKPACK_ADDR`,
`BACKPACK_FALKORDB_URL`, `BACKPACK_TYPESENSE_URL`,
`BACKPACK_TYPESENSE_API_KEY`, `BACKPACK_SYSTEM_GRAPH`.
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
through the `backpack` MCP tools (set up: `docs/ADOPTING.md`). There is no
tracking document, and tracking is never committed. `actions` lists the rarer
actions; a rarer change runs through `batch`.

- **Orientation.** The session hook has already shown "where are we"; trust it.
  Call `where_are_we` only if the hook didn't run. `whats_next` gives the next
  story and `get_story` its detail.
- **Record each move as it happens** with `set_status`: `in_progress` with the
  task branch, `in_review` with the pull request link, `merged` with the merge
  commit. `merge_branch` moves `dev`'s stories to `main`; `cut_release` records a
  release (you create the git tag).
- **While in progress,** keep `update_story` activity current.
- **Log as you go** with `log`: decisions with their reason. Entries are never
  edited; fix one with a `correction`. Stuck or not needed: `on_hold` or
  `closed`, with a reason.
- **Upstream changed.** Check each marked story against the new plan; update its
  plan, or `mark_reviewed` with a reason.
- **Search first,** before `add_story` or `add_idea`. New work goes in with
  `add_story` and its `reasoning`, waits as proposed for a person (a bug with a
  clear fix: `add_bug_fix`), then `add_dependency`. Break an `add_idea`
  thought into proposed stories and any phase yourself, then `refine_idea` it.
- **Many changes at once** go in one `batch`; read every answer.

## Working through the backlog

"Work the backlog", "complete the backlog" or "build backlog" means: deliver
every ready story, one at a time, with a sub-agent doing each build. This
instruction is the standing request to use sub-agents for it. A story in the
backlog was approved by a person, so its plan is the approved plan. Merging
into `dev` is authorised; merging `dev` into `main` and deploying are not.

The main session coordinates and keeps Backpack current; it does not write
the code. For each story:

1. `whats_next`. If no story is ready, stop and report.
2. If the story's plan is thin or leaves a question open, hand it to the
   `planner` and bring its plan to the person for approval; work on nothing
   else until they answer.
3. Cut the task branch from an up-to-date `origin/dev`; `set_status`
   `in_progress` with the branch.
4. Hand the story to the `executor`, one at a time, never in parallel. Give
   it the story's id, goal and plan, the branch, and these rules: follow the
   plan and `docs/ENGINEERING-STANDARD.md`; tests first; `make gates` must
   pass (after `make forward` when the stack runs on another host); if the
   change alters how Backpack is built, run, configured or used, or adds a
   known limitation, update README.md, CLAUDE.md or `docs/` to match and list
   the doc changes in the report; commit on the branch (don't push); report
   what changed, any deviation from the plan and why, and any gap or new work
   it found. Keep the story's activity current with `update_story` as it moves.
5. Review the diff against the goal (`git diff origin/dev...`) and check the
   docs match. Send it back to the same sub-agent (`SendMessage`) for fixes.
6. Push, open the pull request into `dev` (`in_review`), merge it once the
   gates pass (`merged`, with the merge commit). Log decisions and deviations.
7. New work the sub-agent found: `search`, then `add_story` as proposed.
   Don't wait for approval; continue with the next ready story.
8. If the build fails twice or the plan doesn't hold, `on_hold` with the
   reason and move on.

When a story needs a decision from the person, ask them then, log the question
on the story, and only then set it waiting.

Internal refactors and bug fixes that don't change behaviour need no doc
change. Finish with a summary: stories merged, stories on hold and why,
proposed stories awaiting approval.

## Branching

`<type>/<YYYYMMDD>-<short-name>` — type is `bug` or `feature`, kebab-case name.
**`dev` is the integration branch.** Every task branch starts from `dev` and
merges back into `dev` through a pull request — never a direct push. `main`
holds released work: `dev` is merged into `main` by a pull request when it is
ready to release. Neither `dev` nor `main` is committed to directly.
