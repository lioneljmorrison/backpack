# Backpack — Plan

**Status:** Draft, awaiting approval
**Last updated:** 2026-10-10
**Integration branch:** `main`

Until Backpack is deployed and can track itself (slice 4), this file is its
tracker.

---

## The problem

**What's wrong.** Agent-driven projects commonly track their delivery in a
document kept in the code repository, which agents read at the start of every
session and update at the end of every task. Over time it grows to hundreds of
lines, its "where are we" paragraph becomes a chain of "before that…" history,
and the answer to "what is happening now" gets hard to find. A large share of
the project's commits end up doing nothing but updating it.

**Why it happens.** The document does six jobs at once:
- the current checkpoint;
- the in-flight marker;
- the phase roadmap;
- a session-by-session evidence log;
- lists of known defects;
- the source the session-start script reads.

Every one of those changes needs a commit. A document also can't enforce its
own rules, such as "only one thing in flight", or keep the current state short
while the history grows.

**How we'll fix it.** A small service, Backpack, holds this information for any
project. Agents and people read and update it over an HTTP API and an MCP
server, so tracking no longer lives in, or depends on, the code.

## What Backpack holds, and what it doesn't

**ClickUp stays the home of tasks.** Tasks are created there, move through
their statuses there and are closed there, exactly as today. Backpack refers to
a task by its ClickUp id. It never keeps its own copy of a task's status, which
is the surest way for the two to disagree.

Backpack holds what ClickUp doesn't:

| In-repo tracking document | In Backpack |
|---|---|
| Current checkpoint and next step | A short **checkpoint** per project. Setting a new one keeps the old ones in history, so the current one stays short. |
| Phases with exit conditions | **Phases**: title, exit condition, order, and whether the exit condition has been met. |
| `➡ CURRENT` and the work queue | **Work**: a ClickUp task id tied to a phase, a branch and, later, a pull request and its merge. Work is in flight from when it starts until its pull request merges. A merge is not "done": ClickUp's review → Closed stays the verification step. Backpack enforces the in-flight rule: one piece of work at a time unless it is marked as touching separate code, with the reason. |
| Evidence and update log | **Log entries**: date, branch, linked ClickUp task, and a written account of the change, its decisions and the tests run. |
| Git history of the tracker | **History**: every change is recorded and never edited or deleted. A correction is a new record. |
| The session-start script's text scraping | An **orientation** read: plain text, built for the start of a session, giving the checkpoint, what is in flight, phases at a glance and the latest log entries. |

## How it will be built

- **One small Go service**, one Docker container. Data lives in a single SQLite
  file on a Docker volume, which keeps backup down to copying one file.
- **Two ways in, one service:** an HTTP API (for scripts and the session hook)
  and an MCP server (for agents), both served by the same program and both
  calling the same code underneath.
- **Reached through a Cloudflare tunnel.** That makes it reachable from the
  internet, so **every request is authenticated** and the token lives in the
  environment, never in a repository. The only open route is a health check.
- **Any project.** Every record belongs to a project, named by a short slug
  (`my-app`, `backpack`, …). There are no user accounts or roles.
- **Each project repo keeps one small committed file** naming its Backpack
  project and address. It never holds a token.
- **The session-start hook** reads the orientation with a short timeout. If
  Backpack can't be reached, the session still starts, with a one-line notice.

## Delivery, in slices

Each slice is its own branch and pull request, reviewed and merged on its own.

1. **The empty service.** It runs, answers a health check, refuses
   unauthenticated requests, keeps its data in SQLite and runs in Docker. Tests
   and the build gates run on every pull request.
2. **Projects, phases, work, checkpoint, log and history over HTTP, plus the
   orientation read.** Usable locally from here; it takes over as the tracker
   once deployed in slice 4.
3. **The MCP server.** The same operations as tools agents can call: read the
   orientation, set the checkpoint, start work on a ClickUp task, attach the
   pull request, record that it merged, write a log entry, and list and update
   phases.
4. **Live behind the tunnel, with the reusable session hook.** The container
   runs on its Docker host with Cloudflare in front of it. A drop-in hook
   script and a short "how this project tracks" section can be copied into any
   project's CLAUDE.md. From here Backpack tracks itself and this file stops
   being the tracker.

**Out of scope, each needing its own approval:**
- Moving any existing project onto Backpack. Each move is its own task in that
  project: it starts fresh in Backpack, its old document stays as history, and
  its hook, CLAUDE.md and tracking rules are updated together.
- Any ClickUp integration (agents already reach ClickUp directly).
- A web interface.

## Open questions

**Needed before slice 1:** questions 1 and 2. **Needed before slice 2:**
questions 3–5.

1. **Authentication.** Options:
   - Backpack's own bearer token, which works anywhere, including for MCP
     clients and curl;
   - a Cloudflare Access service token in front;
   - both.
   **Proposed:** Backpack's token as the requirement, with Cloudflare Access as
   optional extra protection.
2. **Docker host and GitHub.** Which machine runs the container and
   `cloudflared`, and which hostname should it use? (GitHub repository:
   `lioneljmorrison/backpack`, settled 2026-10-10.)
3. **How is a merge recorded?** With an in-repo tracker, a status change ships
   in the same pull request as the code, so the tracker and the merge can't drift apart.
   Once tracking leaves the repository, something has to record that work
   merged. (Verification stays where it is: ClickUp's review → Closed.)
   - **Proposed default:** the agent makes an explicit call when the pull
     request merges, recording the pull request number and merge commit. Doing
     this is written into the project's CLAUDE.md as part of delivery.
   - **Alternatives:** a GitHub webhook that records it automatically on merge
     (needs a public webhook route and a GitHub secret), or a check at session
     start comparing tracked branches with git.
4. **Known defects.** In-repo trackers often list defects as well as ClickUp.
   Under "ClickUp owns tasks", defects become ClickUp tasks (for example tagged
   `bug`) and Backpack keeps no defect list. Confirm.
5. **Where does Backpack's own build get tracked until slice 4?** Either this
   file alone, or ClickUp tasks too. If ClickUp, which list?

---

## Technical appendix

### Stack (versions checked 2026-10-10)

- Go 1.26 (installed locally: `go1.26.0`).
- HTTP: standard library `net/http` with method-and-path routing. No framework.
- SQLite: `modernc.org/sqlite` v1.60.1, pure Go, so no cgo and a static binary
  in a distroless or scratch image. WAL mode, one writer.
- MCP: `github.com/modelcontextprotocol/go-sdk` v1.8.0 (official SDK),
  streamable-HTTP transport mounted at `/mcp` on the same server.
- Migrations: numbered `.sql` files embedded with `embed`, applied at start-up,
  with the version kept in a `schema_migrations` table. No migration library.

### Data model (slice 2)

| Table | Key columns |
|---|---|
| `projects` | `slug` (pk), `name`, `repo_url`, `integration_branch`, `clickup_list_id`, `created_at` |
| `phases` | `id`, `project_slug`, `position`, `title`, `exit_condition`, `met_at` (null until met) |
| `work` | `id`, `project_slug`, `phase_id`, `clickup_task_id`, `title_at_start` (a snapshot for readable history; ClickUp's title is authoritative), `branch`, `pr_url`, `started_at`, `merged_at`, `merge_commit`, `parallel_reason` |
| `checkpoints` | `id`, `project_slug`, `body`, `next`, `created_at`. The newest row is current. |
| `log_entries` | `id`, `project_slug`, `work_id` (nullable), `date`, `branch`, `body` (markdown) |
| `events` | `id`, `project_slug`, `at`, `actor`, `kind`, `payload` (JSON). Insert-only, enforced by triggers that reject UPDATE and DELETE. |

Every write goes through the store in one transaction with its event.

- **"In flight"** means `started_at` is set and `merged_at` is not. It is not
  a status field, because ClickUp keeps the status.
- **The in-flight rule:** starting work when other work is already in flight is
  refused unless `parallel_reason` is given.
- **`actor`** comes from the token's name, so the history says which agent or
  person made each change.

### HTTP API sketch (slice 2; the contract lives in `api/openapi.yaml`)

```
GET    /healthz                                   (unauthenticated)
GET    /v1/projects                               POST /v1/projects
GET    /v1/projects/{slug}/orientation            text/plain, for the hook
GET    /v1/projects/{slug}/phases                 POST, PATCH /phases/{id}
GET    /v1/projects/{slug}/work?inflight=true     POST /work (start)
PATCH  /v1/projects/{slug}/work/{id}              branch, pr_url
POST   /v1/projects/{slug}/work/{id}/merged       merge_commit, pr_url
GET    /v1/projects/{slug}/checkpoint             PUT  (sets a new one)
GET    /v1/projects/{slug}/log?limit=             POST
GET    /v1/projects/{slug}/events?since=
```

Auth: `Authorization: Bearer <token>`. Tokens are created with a CLI
subcommand (`backpack token create --name <actor>`), stored as SHA-256 hashes,
and shown once.

### MCP tools (slice 3)

`orientation`, `set_checkpoint`, `start_work`, `update_work`, `record_merge`,
`add_log_entry`, `list_log`, `list_phases`, `upsert_phase`, `mark_phase_met`.
Each is a thin wrapper over the same store call as its HTTP twin. Clients
connect with Claude Code's HTTP MCP config, passing the bearer token as a
header.

### Per-project config and hook (slice 4)

The committed file `.backpack.json` holds `{ "project": "my-app", "url":
"https://backpack.<domain>" }`. The token comes from `BACKPACK_TOKEN` in the
environment.

The hook is `curl -fsS --max-time 3` against the orientation route. On any
failure it prints "Backpack unreachable — orientation skipped" and exits 0.

### Deployment (slice 4)

`docker-compose.yml` runs two services: `backpack` (named volume at `/data`)
and `cloudflared` (tunnel token from the environment). Backpack is not
published on a host port; only the tunnel reaches it.

### Gates and CI (slice 1)

`gofmt -l .`, `go vet ./...`, `go test -race ./...` and `go build ./...` in a
GitHub Actions workflow on every pull request.

### Layout

```
cmd/backpack/        main: serve, token create, migrate
internal/store/      SQLite, migrations, the event log
internal/httpapi/    handlers
internal/mcpserver/  tool definitions
api/openapi.yaml
deploy/              Dockerfile, docker-compose.yml, hook script
```
