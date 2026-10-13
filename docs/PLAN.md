# Backpack — Plan

**Status:** Approved 2026-10-10. Slices 1–6 merged; slice 7 (search) in review.
**Last updated:** 2026-10-13
**Integration branch:** `dev` (merged into `main`, the release branch)

**Backpack now tracks itself** in its own workspace, `backpack`, at
`https://backpack.eldernerds.us`. This file is the design record: how the board
works, why, and what was settled. It is no longer the tracker. Status, what's
next and open questions about remaining work live on the stories.

## Progress (until Backpack took over)

| Slice | State |
|---|---|
| 1. The empty service | Merged (PR #3) |
| 2. The board | Merged (PR #4) |
| 3. Work moving across the board | Merged (PR #5) |
| 4. The MCP server | Merged (PR #7) |
| 5–7 | In Backpack: S-1, S-2, S-3 |

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
everything that depends on it. A proposed story sits in **Proposed**, ahead of
Backlog, at the spot the agent chose, with the reasoning as the first entry in
its log. Approving moves it to Backlog; declining closes it with a reason.

**Statuses.**

| Column | Meaning | In this repo |
|---|---|---|
| Proposed | Added by an agent, waiting for a person to approve it | |
| Backlog | Ranked, not started | |
| In progress | Being worked on | task branch |
| In review | Pull request open | PR into `dev` |
| Merged (*branch*) | In that branch — **complete**. One column per branch, in merge order, each named by its branch (for example Merged (dev), Merged (main)) | merged to `dev`, then `dev` merged to `main` |
| Released | In an official release (R-n) — the last step | a release cut from `main` |

The Merged columns come from the workspace's branches, not from a switch (see
"Branches and releases", 2026-10-13). A workspace with one branch has one
Merged column. Before 2026-10-13 the board had a fixed Released column that
meant "merged into `main`", skipped by workspaces without a release stage, and
Deployed as the last step. Both are replaced: Released now means an official
release, and Deployed is gone.

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

**Live updates.** The board page and a story's page update on their own when
the board changes, whoever changed it, so a person watching sees an agent's
move within a moment. Every board change sends a notice naming only what
changed. A hub inside Backpack passes it to each page watching that
workspace, and the page fetches itself again. Nothing is sent through an
outside service: Backpack is one process, and board content stays on the
host. A page never replaces something a person is typing or choosing; it
says the page has changed instead. It also waits for a drag to finish. On
the board, a card that moves slides to its new place (except one the person
just dropped, which appears where they put it), a new card fades in and
a card that leaves fades out, all over the same half second; with reduced
motion asked for, the board redraws without animation. A person's own moves
on the board (a drop, the move prompt, a card's arrows) redraw in place too:
the page sends the move in the background and swaps in the board the server
returns, so the page does not reload. If something is typed on the page, or
the reply is not the board (signed out), it falls back to a plain form post.

## How it will be built

- **A Go service with three ways in:** an **MCP server** for AI agents, an
  **HTTP API** for scripts and other tools, and **web pages** for people. All
  three call the same operations underneath.
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
- **Where it runs:** in Docker, on any host. The dev instance runs on a
  separate Docker host on the LAN and is reached through a Cloudflare tunnel.

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
5. **Live, with the session hook.** Reachable through the tunnel, with a drop-in hook and a short "how this project tracks"
   section any project can copy into its CLAUDE.md. **From here Backpack tracks
   itself** and this file stops being the tracker. The remaining slices go in
   as Backpack's own stories.
6. **Ideas and placement proposals.** Ideas, refining them into stories or a
   phase and closing them, and the propose-then-confirm placement of new
   stories. Until this lands, a person places new stories directly. Split in
   Backpack into S-2 (placement proposals) and S-4 (ideas).
7. **Search.** Typesense indexing and hybrid search by meaning and keyword.
8. **The UI.** Web pages served by Backpack itself: sign in with a token,
   see the board, then act on it. Tracked in Backpack as the Visual UI phase.

**Out of scope, each needing its own approval:**
- Moving any existing project onto Backpack. Each move is its own task in that
  project.
- Logins for more than one person.

## Open questions

**Settled:**
1. **Authentication** (2026-10-10). The MCP and the API use bearer tokens that
   Backpack issues itself, one actor per token, created and revoked from the
   command line. The UI uses the same tokens: a person pastes one once and the
   browser keeps it in a cookie. OAuth waits until a second person needs a
   login.
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
  has a Released stage arrives with slice 3. (Replaced by branches on
  2026-10-13; see "Branches and releases".)

4. **How a story reaches Merged and beyond** (2026-10-10). The agent records
   it when it happens: the merge, with its commit, or the release. Doing so is
   part of every project's delivery rules. No webhook. (Written for the
   original Merged, Released and Deployed columns; the columns changed on
   2026-10-13, the rule did not.)

**Slice 3 details** (settled 2026-10-10):
- **Status flow.** A story moves forward one column, or back one before
  Merged (In review → In progress, In progress → Backlog). Nothing moves back
  from Merged on. Merged and everything after it count as **complete**.
  (Originally Merged, Released and Deployed; see 2026-10-13.)
- **On hold.** A story in Proposed, Backlog, In progress or In review can be put on
  hold with a reason (for example, needs more info). Resuming returns it to
  the column it left. "Blocked" stays reserved for dependencies.
- **Closed.** A story no longer needed is closed with a reason. Closing is
  refused while any story depends on it; those dependencies are removed
  first, deliberately. Closed is final, and nothing can depend on a closed
  story.
- **Blocked work.** Starting a blocked story is refused unless a workaround is
  given (for example, building against mock data). The workaround is kept on
  the story and logged as a decision. A blocked story can never be Merged.
- **Release stage** (history, replaced 2026-10-13 by branches). It was set
  when a workspace was created; the default was none, and Merged then went
  straight to Deployed. The workspace's branch names were dropped until
  something read them.
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

**Slice 4 details** (settled 2026-10-10):
- **Tools.** The twelve first listed, plus what agents need and that list
  left out: changing a story's fields, branch or PR link, adding a goal,
  unmarking a goal, removing a dependency and reading a log. What a story
  is doing is one of its fields, so there is no separate activity tool.
  Creating workspaces, the full board, editing a phase and listing every
  story stay HTTP-only until an agent needs them. Listing workspaces became
  a tool on 2026-10-11 (S-8): a client without the project's
  `.backpack.json`, such as chat, had no way to learn a workspace's slug.
- **Workspace.** Every tool takes the workspace's slug, so one MCP address
  serves every project.
- **Answers.** Every tool answers with the same JSON as the HTTP API, "where
  are we" included. A refusal comes back as a tool error with the board's
  reason.
- **No sessions.** Every call carries its token and stands alone.

5. **Where it runs** (2026-10-10). This is dev: the instance runs on
   localhost and is reached at `backpack.eldernerds.us` through the Cloudflare
   tunnel, using the development tunnel token. The stack runs on any Docker
   host; no particular host is part of the design.
   **2026-10-12:** the dev instance moved to a separate Linux Docker host on
   the LAN. The WSL 6.18 kernel is built without per-process I/O accounting,
   so `/proc/<pid>/io` is missing, and Typesense's bundled metrics library
   crashes at startup without it. The commands drive the remote host through
   a Docker context named by `BACKPACK_DOCKER_CONTEXT`; `make forward` brings
   its ports here over SSH for the tests. The whole stack moved together, so
   board content still never leaves the host it runs on.

**Slice 6 details, placement proposals** (settled 2026-10-11, S-2):
- **A proposal is a story** in a new status, **Proposed**, before Backlog. It
  is not a separate record, so the ordering and dependency rules apply to it
  unchanged, and what's next and the next three, which read only Backlog,
  skip it.
- **Agents propose; a person may place.** `add_story` over the MCP always
  proposes and needs the reasoning for its spot, logged as a decision in the
  same query. This is the one automatic log entry besides status changes
  (slice 3's "only status changes" rule). `POST /stories` over HTTP still places directly into Backlog.
- **Anyone approves**, over MCP or HTTP: Proposed → Backlog. Picking another
  spot is a move, then approval. Declining is Proposed → Closed, with a
  reason. A story never goes back to Proposed, except a held proposal
  resuming (2026-10-13 decision below).
- **Where are we** lists what is waiting for approval. A proposed story in a
  phase counts as open work.

**Slice 6 details, ideas** (settled 2026-10-11, S-4):
- **Their own order.** Ideas are ranked like stories, in a list of their
  own; stories and ideas never share an order, and what's next never reads
  ideas. The ranking code is shared, keyed by the node label.
- **No approval, but context.** Every idea goes in `open`, over the MCP or
  HTTP alike. Approval gates the backlog, so only stories need it; an idea
  is not work yet. Instead the idea carries **context**: anything that helps
  understand it and turn it into work (notes, links, documents,
  background). It is optional, but `add_idea` encourages it because an idea
  without context is only as good as its title, and `refine_idea` tells the
  agent to build the stories or phase from it. Where are we has no ideas
  section. (An earlier design proposed agents' ideas with a reasoning for
  their spot; migration 6 folded that reasoning and the old notes into
  context.)
- **Worked on over time.** Title and context can be edited, and the idea
  moved, until it is closed.
- **Refine links, it doesn't create.** The stories and phase are added the
  usual way; refining an open idea links it to them with `BECAME` and closes
  it, in one query, refused whole if a story is missing or closed or the
  phase is missing. Dropping closes it with a reason. Closed is final.
- **No idea log.** The context lives on the idea; nothing yet needs more.

**Slice 7 details, search** (settled 2026-10-11, S-3):
- **Hybrid, with a local model.** Keywords and meaning together. Meaning
  uses Typesense's built-in `ts/all-MiniLM-L12-v2`, which runs inside the
  stack, so tracked content never leaves the host. The model is downloaded
  once per Typesense volume.
- **What is indexed.** One entry per story, idea and phase, in one collection
  per workspace. Meaning reads the title and summary (a story's description
  and goal, an idea's context, a phase's description and goals); keywords also
  read the detail (a story's plan and log text, an idea's drop reason). Closed stories and dropped ideas stay in, with
  their state.
- **Results.** Up to ten, best first: kind, id, title and state. A match by
  meaning further than a cosine distance of 0.75 is left out, measured on
  invented fixtures; a keyword match always stays.
- **Kept current after each write.** The board reports which item a
  successful write changed, and the index reads it back and replaces its
  entry. Moves and dependencies report nothing. If Typesense can't be
  reached, the board change stands and the failure is logged with "reindex
  needed".
- **Rebuilt at every start and on demand** (`backpack reindex`). A rebuild
  drops and recreates the collection, so a change to the search schema
  applies at the next start whatever was there before: that is its
  migration.

**Drag cards between columns** (settled 2026-10-13, S-18):
- **A proposal can be put on hold.** Proposed → On hold, with a reason like
  any hold, through MCP, API and web. It resumes to Proposed and is then
  approved as usual; while held it is not listed as waiting for approval.
  Why: dragging cards on the board needs to be able to park a proposal
  without approving or declining it. This is the only way a story returns to
  Proposed.

**Branches and releases** (settled 2026-10-13, I-2; phases 2 and 3):

*What was wrong.* The board had a fixed "has a release stage" switch, set once
when a workspace was created. Its Released column only meant "merged into
`main`", which reads like a GitHub release (a tagged version) and isn't one.
Deployed was the last step, though nothing is deployed without a release. The
switch couldn't describe how a project really branches.

*Why it happened.* Statuses were a fixed list and the project's branches were
not part of the board.

*What was decided.*
- **Branches.** A workspace has branches. Each names the branch it merges
  into, and exactly one, the **release branch** (normally `main`), merges
  nowhere. The branch stories are merged into first is the **first branch**
  (normally `dev`); with one branch, it is the release branch. A branch can't
  merge into itself or into anything that leads back to it. Existing
  workspaces carry over: no release stage becomes one branch (`main`), a
  release stage becomes two (`dev` merging into `main`).
- **Where a merged story is.** A story that has been merged stores the name of
  the furthest branch it reached, as `into`. (`branch` already means the task
  branch the story is built on.) It is a name, not a step number, so adding a
  branch never moves a story. Each step's merge commit is logged on the story.
- **Columns.** One Merged column per branch, in merge order, named by branch:
  Merged (dev), Merged (main). No column says Released for a merge. Merged
  into any branch is **complete**.
- **Recording a merge.** The agent names the branch the story was merged into;
  with none named it means the first branch, which is what "merged" meant
  before. A merge moves **one step at a time**: a story goes only to the branch
  its current one merges into, so a merge into `main` straight from In review
  is refused. Dragging a card between Merged columns records a merge into that
  branch.
- **Branch merged.** One action, given a branch and the merge commit, moves
  every story merged into that branch to the branch it merges into, as one
  query, with the commit logged on each. Stories still in progress or in
  review stay put. Merging the release branch is refused. Cutting a release
  candidate changes nothing on the board.
- **Changing branches later.** A branch can be renamed (every reference
  follows, stories stay in their columns, past log entries keep the old name
  because logs are never edited), given a display **alias** (only the web
  column heading changes; tools, the API, logs and refusals use the real
  name), added (nothing moves), re-pointed to merge elsewhere (nothing
  moves), and removed only when nothing refers to it: no story merged into it
  and no branch merging into it. The release branch and the first branch are
  never removed. An abandoned branch's stories are put on hold or closed
  first.
- **Setup.** When an agent first connects a repository it works out the
  branching (default, integration and protected branches, how pull requests
  flow), proposes it in conversation, and a person approves or adjusts it.
  Then the agent applies it with Backpack's tools. Nothing pending is stored
  in Backpack. Later changes an agent suggests work the same way.
- **Releases (R-n).** Cutting a release is its own action, because merging
  into the release branch doesn't make one. It records a version, git tag,
  commit, date and notes, and in one query takes every story merged into the
  release branch that isn't in a release yet, marks it **Released**, links it
  to the release and logs it. **Released replaces Deployed** as the final
  step: there is no deployment without a release. Existing deployed stories
  read as released, with no release record. Backpack doesn't talk to GitHub;
  the agent records the tag as it records commits.
- **Old statuses.** Setting `released` (the old "merged into main") or
  `deployed` is refused, with a message naming the new form: record a merge
  into the release branch, or cut a release.
- **Later idea: sub-boards.** A long-lived branch for a revision (V2) could be
  a sub-board of the same workspace, with its own ranked backlog and
  `whats_next` per board, and closing it would move its stories to the parent
  branch's column at the merge commit. Not designed or scheduled; for now one
  workspace has one backlog.
- **Order of delivery.** Phase 2 (Branches) brings branches, `into` and the
  branch tools while Deployed still exists, so Deployed is allowed only from
  the release branch until phase 3 (Releases) removes it.

Questions about remaining work are logged on their stories in Backpack.

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
  candidates). No Go client: the index needs five REST calls, made with
  `net/http` (decided 2026-10-11, in place of `typesense-go`).
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
| `Workspace` | `slug`, `next_story`, `next_goal`, `next_log`, `next_idea` (counters for `S-n`, `G-n`, `L-n` and `I-n` IDs), and `next_release` (the `R-n` counter) once releases exist. `has_release_stage` is history: replaced by `Branch` nodes |
| `Branch` | `name`, `into` (the name of the branch it merges into, a property rather than an edge; none for the release branch), `alias` (display only), `created_by`, `created_at` |
| `Phase` | `number`, `title`, `description`, `created_by`, `created_at`, `updated_by`, `updated_at` |
| `Goal` | `id` (`G-n`), `text`, `created_by`, `created_at`, `met_at`, `met_by` |
| `Story` | `id` (`S-n`), `title`, `description`, `goal`, `plan`, `rank`, `status`, `created_by`, `created_at`, `moved_by`, `moved_at`, `updated_by`, `updated_at` (an edit, or a dependency removed), `held_from`, `hold_reason`, `activity`, `waiting_on`, `branch`, `pr_url`, `workaround`, `into` (the furthest branch a merged story reached, a `Branch.name`) |
| `Release` | `id` (`R-n`), `version`, `tag`, `commit`, `notes`, `created_by`, `created_at` |
| `Idea` | `id` (`I-n`), `title`, `context`, `rank`, `state` (`open`/`closed`), `drop_reason`, `created_by`, `created_at`, `moved_by`, `moved_at`, `updated_by`, `updated_at`, `closed_by`, `closed_at` |
| `LogEntry` | `id` (`L-n`), `at`, `actor`, `kind`, `text`, `reason`, `commit`, `pr_url`, `branch`, `into` (the branch a merge entry reached), `from`, `to` |

| Edge | Meaning |
|---|---|
| `(Phase)-[:HAS_GOAL]->(Goal)` | |
| `(Phase)-[:CONTAINS]->(Story)` | optional; a story without one sits directly in the backlog |
| `(Story)-[:DEPENDS_ON {created_by, created_at}]->(Story)` | read in reverse as "blocks" |
| `(Story)-[:LOGGED]->(LogEntry)` | |
| `(LogEntry)-[:CORRECTS]->(LogEntry)` | |
| `(Idea)-[:BECAME]->(Story\|Phase)` | |
| `(Story)-[:IN_RELEASE]->(Release)` | |

A separate `backpack` graph holds actors with hashed tokens and the workspace
registry: `WorkspaceEntry {slug, name, repo_url, has_release_stage,
created_by, created_at}`. Until 2026-10-13 `has_release_stage` was also kept
on the workspace graph's `Workspace` node, so the status query could read it.
Branches replace it: the status query reads `Branch` nodes, and a new
workspace's branches are seeded from the creation setting (one branch, or
`dev` into `main`).

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
- **Statuses:** `proposed`, `backlog`, `in_progress`, `in_review`, `merged`,
  `released`, `on_hold`, `closed`. A merged story's `into` says which branch
  column it is in. Complete is `merged` or `released`; `blocked` is computed
  on read. `released` means an official release (R-n). The old meaning of
  `released` (merged into `main`) and the old `deployed` status are refused
  with a message naming the new form. Migrations turn old released stories
  into merged into the release branch, and old deployed stories into released.
  Until releases exist (2026-10-13, S-24 built the merge half), `deployed` remains,
  allowed only from the release branch, and the migration turns old released
  stories into merged into the release branch and gives old deployed stories
  `into` the release branch. A merge to the next branch is a `merged` to
  `merged` change that names it (`into`) and carries its own commit.
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
GET  /v1/workspaces/{ws}/ideas                       POST
GET  /v1/workspaces/{ws}/ideas/{id}                  PATCH; POST /move, /refine, /drop
GET  /v1/workspaces/{ws}/search?q=                   stories, ideas and phases by keyword and meaning
POST /v1/workspaces/{ws}/batch                       1–100 POST/PATCH/DELETE requests, each sent to its own route
```

Auth: `Authorization: Bearer <token>`. Tokens (`bp_` + 32 random bytes) are
created with `backpack token create --actor <name>`, stored as SHA-256 hashes
in the `backpack` system graph, and shown once. `backpack token revoke --actor
<name>` disables every live token that actor holds. `GET /v1/whoami` returns
the token's actor: it is how a client checks its configuration.

### MCP tools (slice 4)

Served at `/mcp` (streamable HTTP, stateless) behind the same bearer check
as `/v1/`; the actor reaches each tool through the request context.
`whats_next`, `where_are_we`, `get_story`, `add_story`, `update_story`,
`move_story`, `set_status`, `add_dependency`, `remove_dependency`, `log`,
`read_log`, `list_phases`, `add_phase`, `add_goal`, `mark_goal_met` (`met`
true or false). Every tool takes `workspace`. Since slice 6, `add_story`
proposes: the story goes in as `proposed`, with a required `reasoning`.
Ideas add `add_idea` (open at once, with optional `context`),
`list_ideas`, `get_idea`, `update_idea`, `move_idea`,
`refine_idea` and `drop_idea`. Search adds `search`. `list_branches` (2026-10-13) answers with a workspace's branches in merge order, the same JSON as `GET /v1/workspaces/{ws}/branches`. `list_workspaces` is the
one tool that takes no `workspace`: it answers with every workspace, the same
JSON as `GET /v1/workspaces`.

`batch` makes many board changes in one call: a list of `{tool, args}`,
1 to 100, each naming a tool that changes the board, with that tool's
arguments minus `workspace`, which the batch gives once. Each action runs
through its tool's own handler, so it is answered exactly as if called alone,
and runs as its own query. A failed action is answered with its reason and
doesn't stop the rest; nothing is undone. An action can name only items that
already exist, not one an earlier action in the batch adds. Reads can't be
batched. `POST /v1/workspaces/{ws}/batch` does the same over HTTP with
requests (`method`, a `path` relative to the workspace, `body`), answering
each with its route's own status and body.

### Per-project config and hook (slice 5)

The committed file `.backpack.json` holds `{ "workspace": "my-app", "url":
"https://backpack.<domain>" }`. The token comes from `BACKPACK_TOKEN` in the
environment. The hook, `deploy/hook.sh`, is `curl -fsS --max-time 3` against
`/status` as plain text. Without `.backpack.json` it prints nothing; on any
other failure it prints "Backpack unreachable — orientation skipped". It always
exits 0. It reads the config with `sed`, so it needs no `jq`.
`docs/ADOPTING.md` covers connecting a project: creating its workspace, the
MCP server (per person, or per team with `.mcp.json`, from an environment
variable or a secrets manager), the hook, and the section to paste into a
project's CLAUDE.md.

### Deployment

`docker-compose.yml` runs four services:
- `backpack`;
- `falkordb`, with its data volume;
- `typesense`, with its data volume and the API key from the environment;
- `cloudflared` (`cloudflare/cloudflared:2026.9.3`), under the `tunnel`
  profile: `make up` leaves it off, `make tunnel` starts it with
  `CLOUDFLARE_TOKEN`. The tunnel is token-managed; its public hostname routes
  to `http://backpack:8780` in the Cloudflare dashboard. `make tunnel` first
  runs `deploy/tunnel-guard.sh`, which refuses when the public address already
  answers and the tunnel isn't running on this Docker host: two copies on one
  tunnel would split requests between two boards.

The commands drive whichever Docker host `BACKPACK_DOCKER_CONTEXT` names (the
local one when unset). For a remote host reached over SSH, `make forward`
(`deploy/forward.sh`) forwards its ports to this machine's `127.0.0.1`.

Only the tunnel reaches `backpack` from outside. FalkorDB and Typesense are on the internal
network only.

### Gates and CI (slice 1)

`gofmt -l .`, `go vet ./...`, `go test -race ./...` and `go build ./...` in
GitHub Actions, with FalkorDB and Typesense as service containers so storage
tests run against the real thing.

### Layout

```
cmd/backpack/        main: serve, token create|revoke, reindex
internal/falkor/     connection and numbered migrations, applied at start-up
internal/auth/       tokens, the actor store, the bearer middleware
internal/health/     dependency checks
internal/board/      the operations and their rules (Cypher)
internal/search/     Typesense indexing and queries
internal/httpapi/    handlers
internal/mcpserver/  tool definitions
api/openapi.yaml
deploy/              Dockerfile, docker-compose.yml, hook, tunnel guard and forward scripts
```
