# Backpack — Plan

**Status:** Approved 2026-10-10. Slice 3 in review.
**Last updated:** 2026-10-10
**Integration branch:** `dev` (released to `main`)

Until Backpack is deployed and can track itself (slice 5), this file is its
tracker.

## Progress

| Slice | State |
|---|---|
| 1. The empty service | Merged (PR #3) |
| 2. The board | Merged (PR #4) |
| 3. Work moving across the board | In review (PR #5) |
| 4–7 | Not started |

---

## The problem

**What's wrong.** Agent-driven projects commonly track their delivery in a
document kept in the code repository, which agents read at the start of every
session and update at the end of every task. Over time it grows to hundreds of
lines, its "where are we" paragraph becomes a chain of "before that…" history,
and the answer to "what is happening now" gets hard to find. A large share of
the project's commits end up doing nothing but updating it.

**Why it happens.** A document can't answer a question; it can only be read.
To find the next task, an agent reads the whole thing. It can't enforce its own
rules (what comes before what, what is blocked), and every change to it is a
commit.

**How we'll fix it.** Backpack is a Kanban board for agentic projects, built as
a service. Each project has its own workspace. Agents ask it "what's next" or
"where are we" and get a short, direct answer. People and a future UI use the
same board through an API.

## How the board works

**Workspaces.** Every project gets its own workspace, and nothing crosses
between workspaces.

**Ideas.** Everything starts as an idea: a rough thought with little context
and no sense yet of where it fits. Ideas are worked through together. Once
refined, an idea is **closed**, keeping links to the stories or phase it became.

**Phases.** A phase is a large body of work (an epic), with its own
description, goals and stories. Phases are numbered in order.
- A phase is **complete when all its stories are complete.**
- Its goals are listed alongside, each marked as met when it is. If every goal
  is met while stories are still open, Backpack flags the phase: something may
  have gone sideways.

**Stories.** Each story has:
- a description, a goal and a plan;
- a status;
- the branch and pull request it lives in;
- its own log.

A story may belong to a phase, or sit directly in the backlog.

**One backlog, in delivery order.** Each workspace has one ranked backlog. The
top story is the next to be delivered. Rank is order, not urgency.

**Dependencies are different from order.**
- "A comes before B" is just rank.
- "B can't be done until A is complete" is a dependency. It is recorded once
  and read from both ends: A *blocks* B, and B is *blocked by* A.
- A story must rank below everything it depends on.
- A dependency can only point to the same or an earlier phase, never across
  workspaces, and never in a circle.
- Backpack refuses any change that breaks these rules.

**Placing new work.** An agent proposes where a new story belongs, with its
reasoning; a person confirms it or picks another spot. The dependency rules
narrow the choice: the story must go below everything it depends on and above
everything that depends on it.

**Statuses.**

| Column | Meaning | In this repo |
|---|---|---|
| Backlog | Ranked, not started | |
| In progress | Being worked on | task branch |
| In review | Pull request open | PR into `dev` |
| Merged | In the integration branch — **complete** | merged to `dev` |
| Released | In the release branch | `dev` → `main` merged |
| Deployed | Running in production | |

A workspace that releases straight from `main` skips **Released**.

**What an in-progress story is doing:** Planning, Building, Testing, or Waiting
on agents. "Blocked" is never set by hand; it is calculated from dependencies.

**Story logs.** Every story keeps a short log. Each entry records who wrote it
(person or agent), when, and what kind it is: a note, a decision and its
reason, a commit (with its id), a pull request (with its link), a status
change, or a correction. Entries are never edited or deleted. A mistake is
fixed with a correction that points to the entry it corrects. That lets an
agent see what was done in which commit, and what was later found wrong.

**Two quick reads.**
- **What's next:** the highest-ranked backlog story whose dependencies are all
  complete, with its description, goal and plan. It reads nothing else.
- **Where are we:**
  - the current phase and how far along it is;
  - what is in progress and what each story is doing;
  - what is in review;
  - the next three in the backlog.

  The session-start hook shows this.

**Search.** Ideas, phases, stories and logs can be searched by meaning as well
as by keyword, for example "do we already have a story about this?"

## How it will be built

- **A Go service with two ways in:** an **MCP server** for AI agents and an
  **HTTP API** for building a UI. Both call the same operations underneath.
- **FalkorDB** is the board's store, as a graph: workspace, phase, story, idea,
  dependency and log entry. Each workspace is its own graph, so workspaces never
  hold each other up. Every board action is one query, so it either happens
  completely or not at all. FalkorDB saves every write to disk before
  confirming it, and takes regular snapshots for backup.
- **Typesense** provides search. It creates the embeddings itself with a
  built-in model running on our own machine, so no project's content is sent to
  an outside service. It is only an index built from FalkorDB, and can be
  rebuilt from it at any time.
- **Access:** every request carries a token. Each token belongs to an actor, a
  person or an agent, and every change records its actor. There is one person
  today. Logins for more people can be added later without changing the
  record.
- **Where it runs:** on localhost for development, and on `nas` or
  `lab-hermes`, behind a Cloudflare tunnel, for the shared dev and production
  instances.

## Delivery, in slices

Each slice is its own branch and pull request into `dev`.

1. **The empty service.** The service, FalkorDB and Typesense run together
   locally. The service answers a health check, refuses requests without a
   token, and can create tokens. Tests and gates run on every pull request.
2. **The board.** Workspaces, phases (with goals) and stories, the ranked
   backlog, dependencies and the ordering rules, over the HTTP API.
3. **Work moving across the board.** Statuses, what an in-progress story is
   doing, story logs with corrections, phase completion and the goals-met
   flag, **what's next** and **where are we**.
4. **The MCP server.** The same operations as tools for agents.
5. **Live, with the session hook.** Deployed to `nas` or `lab-hermes` behind
   the tunnel, with a drop-in hook and a short "how this project tracks"
   section any project can copy into its CLAUDE.md. **From here Backpack tracks
   itself** and this file stops being the tracker. The remaining slices go in
   as Backpack's own stories.
6. **Ideas and placement proposals.** Ideas, refining them into stories or a
   phase and closing them, and the propose-then-confirm placement of new
   stories. Until this lands, a person places new stories directly.
7. **Search.** Typesense indexing and hybrid search by meaning and keyword.

**Out of scope, each needing its own approval:**
- The UI. The API is shaped for it; building it is separate work.
- Moving any existing project onto Backpack. Each move is its own task in that
  project.
- Logins for more than one person.

## Open questions

**Settled:**
1. **Authentication** (2026-10-10). The MCP and the API use bearer tokens that
   Backpack issues itself, one actor per token, created and revoked from the
   command line. OAuth arrives with the UI, which is out of scope for now.
2. **Phase order and rank** (2026-10-10). A later phase's story may rank above
   an earlier phase's stories when it doesn't depend on them. Only
   dependencies constrain order, so a quick unrelated fix can jump ahead.
3. **Stories with no phase** (2026-10-10). The "same or an earlier phase" rule
   applies only when both stories have a phase. A story with no phase is
   constrained only by rank: it must still rank below what it depends on.

**Slice 2 details** (settled 2026-10-10):
- Until story logs arrive in slice 3, everything created records who created
  it and when. A move, an edit, or a removed dependency records who made it
  and when, on the story or phase it changed.
- A story's phase is set when the story is created and can't be changed yet.
- A story is placed directly before or after a named story, or at the bottom
  of the backlog when neither is given.
- Stories are numbered `S-1`, `S-2` … and goals `G-1` … within each
  workspace. A phase is referred to by its number.
- A workspace has a slug, a name and optionally its repository. Whether it
  has a Released stage arrives with slice 3.

4. **How a story reaches Merged, Released and Deployed** (2026-10-10). The
   agent records it when it happens: the merge, with its commit, or the
   release. Doing so is part of every project's delivery rules. No webhook.

**Slice 3 details** (settled 2026-10-10):
- **Status flow.** A story moves forward one column, or back one before
  Merged (In review → In progress, In progress → Backlog). Nothing moves back
  from Merged on. Merged, Released and Deployed all count as **complete**.
- **On hold.** A story in Backlog, In progress or In review can be put on
  hold with a reason (for example, needs more info). Resuming returns it to
  the column it left. "Blocked" stays reserved for dependencies.
- **Closed.** A story no longer needed is closed with a reason. Closing is
  refused while any story depends on it; those dependencies are removed
  first, deliberately. Closed is final, and nothing can depend on a closed
  story.
- **Blocked work.** Starting a blocked story is refused unless a workaround is
  given (for example, building against mock data). The workaround is kept on
  the story and logged as a decision. A blocked story can never be Merged.
- **Release stage.** Set when a workspace is created; the default is none,
  and Merged then goes straight to Deployed. The workspace's branch names are
  dropped until something reads them.
- **Activity** (Planning, Building, Testing, Waiting on agents, with what it
  is waiting on) exists only while a story is in progress. It starts as
  Planning and is cleared on leaving In progress.
- **Logging.** Only status changes write log entries automatically. Moves,
  edits and dependency changes keep their who/when fields on the story.
- **Goals** can be marked met and unmarked. A phase is complete when it has a
  story and every story that isn't closed is complete. With nothing left in
  it (no stories, or all closed), it is complete once it has goals and all
  are met. A phase is flagged when it has goals, all are met, and some of its
  stories are still open.
- **Where are we** also lists what is on hold and why. Plain text is chosen
  with `Accept: text/plain`.

**Needed before slice 5:**

5. **Which host runs the shared instances:** `nas`, `lab-hermes`, or one each
   for dev and production? And which hostname should the tunnel use? (No
   domain has been created yet; the tunnel token exists.)

**Needed before slice 6:**

6. **Are ideas ranked,** or simply listed newest first? **Proposed:** newest
   first. Ideas have no delivery order until they become stories.

---

## Technical appendix

### Stack (versions checked 2026-10-10)

- Go 1.26 (installed locally: `go1.26.0`). HTTP via standard library
  `net/http` routing; no framework.
- FalkorDB server `falkordb/falkordb-server:v4.20.7` (2026-09-24), the latest
  4.x release more than two weeks old. The `-server` image is the same
  database without the bundled web browser UI, which a public service doesn't
  need. 6.0 shipped
  2026-09-29 and is held back until it has settled; revisit once slice 2 is
  done. Go client: `github.com/FalkorDB/falkordb-go/v2` v2.1.0.
- Typesense server `typesense/typesense:30.2` (31.0 is still in release
  candidates). Go client: `github.com/typesense/typesense-go/v3` v3.2.0.
  Auto-embedding with the built-in `ts/all-MiniLM-L12-v2` model, on CPU.
- MCP: `github.com/modelcontextprotocol/go-sdk` v1.8.0, streamable HTTP at
  `/mcp` on the same server.

### FalkorDB persistence

`appendonly yes`, `appendfsync always` (the default `everysec` can lose up to
one second of writes; this is the only copy of the board), plus RDB snapshots on
the data volume. Backup copies the RDB file off the host.

### Graph model (one graph per workspace, `ws:<slug>`)

| Node | Key properties |
|---|---|
| `Workspace` | `slug`, `next_story`, `next_goal`, `next_log` (counters for `S-n`, `G-n` and `L-n` IDs), `has_release_stage` |
| `Phase` | `number`, `title`, `description`, `created_by`, `created_at`, `updated_by`, `updated_at` |
| `Goal` | `id` (`G-n`), `text`, `created_by`, `created_at`, `met_at`, `met_by` |
| `Story` | `id` (`S-n`), `title`, `description`, `goal`, `plan`, `rank`, `status`, `created_by`, `created_at`, `moved_by`, `moved_at`, `updated_by`, `updated_at` (an edit, or a dependency removed), `held_from`, `hold_reason`, `activity`, `waiting_on`, `branch`, `pr_url`, `workaround` |
| `Idea` | `id`, `title`, `body`, `state` (`open`/`closed`), `closed_at` |
| `LogEntry` | `id` (`L-n`), `at`, `actor`, `kind`, `text`, `reason`, `commit`, `pr_url`, `branch`, `from`, `to` |

| Edge | Meaning |
|---|---|
| `(Phase)-[:HAS_GOAL]->(Goal)` | |
| `(Phase)-[:CONTAINS]->(Story)` | optional; a story without one sits directly in the backlog |
| `(Story)-[:DEPENDS_ON {created_by, created_at}]->(Story)` | read in reverse as "blocks" |
| `(Story)-[:LOGGED]->(LogEntry)` | |
| `(LogEntry)-[:CORRECTS]->(LogEntry)` | |
| `(Idea)-[:BECAME]->(Story\|Phase)` | |

A separate `backpack` graph holds actors with hashed tokens and the workspace
registry: `WorkspaceEntry {slug, name, repo_url, has_release_stage,
created_by, created_at}`. `has_release_stage` is also kept on the workspace
graph's `Workspace` node, so the status query can read it.

- **Rank** is a fractional string key over `0-9A-Za-z` (ASCII order, so Go
  and FalkorDB compare keys alike). Between two neighbours there is always a
  key, so a move rewrites only the moved story. `Story.rank` is deliberately
  not indexed: FalkorDB 4.20 answers a range comparison on an indexed string
  property with no rows.
- **Each action is one query.** Its rule checks compute a `violation`, and the
  write runs inside `FOREACH (… CASE WHEN violation IS NULL …)`, so a refused
  change writes nothing. The query returns the violation so that the API can
  say why. Path checks count matches with a pattern comprehension, because
  FalkorDB's `exists(pattern)` is true even when nothing matches.
- **Placement:** a read-only query finds the neighbours' ranks, and Go computes
  the key. The write rechecks that the gap is still empty and the anchor still
  in place. If another write took the gap, it retries (up to 3 times).
- **Ordering checks.** For a move: no `DEPENDS_ON` target may rank below the
  new position, and no dependent may rank above it. For a new dependency: the
  target ranks above the dependent and adds no cycle. When both stories have a
  phase, the target is in the same or an earlier phase.
- **Text** containing characters Go would escape when quoting (control and
  invisible formatting characters, apart from newline, carriage return and
  tab) is refused. The FalkorDB client inlines values as Go-quoted literals,
  and FalkorDB doesn't read those escapes, so such text would otherwise be
  stored changed, without an error.
- **Creating a workspace** is the one action that spans two graphs. The
  workspace graph is prepared first, and the registry entry, written last, is
  what makes the workspace exist. An interrupted create can be repeated. Start-up
  migrates every registered workspace graph.
- **Status changes follow the column order.** Each change writes a `status`
  log entry in the same query, plus a `decision` entry when a blocked story is
  started with a workaround.
- **Statuses:** `backlog`, `in_progress`, `in_review`, `merged`, `released`,
  `deployed`, `on_hold`, `closed`. Complete is `merged`, `released` or
  `deployed`; `blocked` is computed on read.
- **Empty lists:** `all(…)` over an empty list is true, so phase completion
  and the goals flag guard against no stories and no goals.

### HTTP API sketch (contract in `api/openapi.yaml`)

```
GET  /healthz                                        (unauthenticated)
GET  /v1/workspaces                                  POST
GET  /v1/workspaces/{ws}/next                        what's next
GET  /v1/workspaces/{ws}/status                      where are we (JSON, or text/plain for the hook)
GET  /v1/workspaces/{ws}/board                       stories by column, ranked
GET  /v1/workspaces/{ws}/phases                      POST, PATCH /phases/{id}
POST /v1/workspaces/{ws}/phases/{n}/goals            PATCH /goals/{id} (met)
GET  /v1/workspaces/{ws}/stories                     POST (with position)
PATCH /v1/workspaces/{ws}/stories/{id}               fields, activity, branch, pr_url
POST /v1/workspaces/{ws}/stories/{id}/move           new position
POST /v1/workspaces/{ws}/stories/{id}/status         to (+ branch / pr_url / commit / reason / workaround)
POST /v1/workspaces/{ws}/stories/{id}/dependencies   DELETE /dependencies/{dep}
GET  /v1/workspaces/{ws}/stories/{id}/log            POST
GET  /v1/workspaces/{ws}/ideas                       POST, /ideas/{id}/refine     (slice 6)
POST /v1/workspaces/{ws}/stories/proposals           propose, confirm             (slice 6)
GET  /v1/workspaces/{ws}/search?q=                                                (slice 7)
```

Auth: `Authorization: Bearer <token>`. Tokens (`bp_` + 32 random bytes) are
created with `backpack token create --actor <name>`, stored as SHA-256 hashes
in the `backpack` system graph, and shown once. `backpack token revoke --actor
<name>` disables every live token that actor holds. `GET /v1/whoami` returns
the token's actor: it is how a client checks its configuration.

### MCP tools (slice 4)

`whats_next`, `where_are_we`, `get_story`, `add_story`, `move_story`,
`set_status`, `set_activity`, `add_dependency`, `log`, `list_phases`,
`add_phase`, `mark_goal_met`. Ideas, proposals and search tools are added with
slices 6 and 7.

### Per-project config and hook (slice 5)

The committed file `.backpack.json` holds `{ "workspace": "my-app", "url":
"https://backpack.<domain>" }`. The token comes from `BACKPACK_TOKEN` in the
environment. The hook is `curl -fsS --max-time 3` against `/status` as plain
text. On any failure it prints "Backpack unreachable — orientation skipped" and
exits 0.

### Deployment

`docker-compose.yml` runs four services:
- `backpack`;
- `falkordb`, with its data volume;
- `typesense`, with its data volume and the API key from the environment;
- `cloudflared`, under a compose profile so it is off on localhost.

Only the tunnel reaches `backpack`. FalkorDB and Typesense are on the internal
network only.

### Gates and CI (slice 1)

`gofmt -l .`, `go vet ./...`, `go test -race ./...` and `go build ./...` in
GitHub Actions, with FalkorDB and Typesense as service containers so storage
tests run against the real thing.

### Layout

```
cmd/backpack/        main: serve, token create|revoke (reindex in slice 7)
internal/falkor/     connection and numbered migrations, applied at start-up
internal/auth/       tokens, the actor store, the bearer middleware
internal/health/     dependency checks
internal/board/      the operations and their rules (Cypher)
internal/search/     Typesense indexing and queries
internal/httpapi/    handlers
internal/mcpserver/  tool definitions
api/openapi.yaml
deploy/              Dockerfile, docker-compose.yml, hook script
```
