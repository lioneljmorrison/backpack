# Backpack

[![GitHub release](https://img.shields.io/github/v/release/lioneljmorrison/backpack?style=flat-square)](https://github.com/lioneljmorrison/backpack/releases/latest)
[![Tests](https://img.shields.io/github/actions/workflow/status/lioneljmorrison/backpack/ci.yml?branch=main&style=flat-square&label=tests)](https://github.com/lioneljmorrison/backpack/actions/workflows/ci.yml)
[![CodeQL](https://img.shields.io/github/actions/workflow/status/lioneljmorrison/backpack/codeql.yml?branch=main&style=flat-square&label=CodeQL)](https://github.com/lioneljmorrison/backpack/security/code-scanning)
[![Coverage threshold](https://img.shields.io/badge/coverage-%E2%89%A570%25-brightgreen?style=flat-square)](https://github.com/lioneljmorrison/backpack/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/lioneljmorrison/backpack?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/lioneljmorrison/backpack?style=flat-square&logo=go&logoColor=white)](https://go.dev/)

A Kanban board for agentic projects, built as a Go service.

Agent-driven projects often track progress in a document committed beside the
code. Every status change becomes a commit, and the document grows until the
current state is hard to find. Backpack replaces that document: agents ask it
"what's next" or "where are we" and get a short answer.

- Each project has its own **workspace**, with one backlog ranked in delivery
  order.
- Stories move (Proposed →) Backlog → In progress → In review → Merged →
  Released → Deployed, and each keeps an append-only log. A story an agent
  adds is Proposed until a person approves it.
- Dependencies are explicit, and a story always ranks below what it depends on.
- Agents can **search** stories, ideas and phases by keyword or by meaning,
  to check whether something is already covered before adding it.
- Agents use it through an **MCP server**, people through **web pages** at
  `/`, and scripts through the **HTTP API** (`api/openapi.yaml`). Every
  request needs a token; in a browser you paste it once to sign in.

The full design is in [docs/PLAN.md](docs/PLAN.md).

## Running it

You need Docker. The stack is Backpack, FalkorDB (the store) and Typesense (the
search index).

```bash
make up                  # build and start the stack on localhost
make tunnel              # the same, plus the Cloudflare tunnel (needs CLOUDFLARE_TOKEN)
make down                # stop (data volumes are kept)
make token ACTOR=<name>  # issue a token for a person or agent (shown once)
make reindex             # rebuild the search index from the board
make gates               # gofmt, vet, tests, build
```

The search index is rebuilt from the board at every start. The first start on
a new Typesense volume also downloads the small embedding model search uses
for matching by meaning. It runs inside the stack, so nothing tracked is sent
anywhere. If Typesense is down when the board changes, the change still saves
and the log says a reindex is needed; `make reindex`, or the next start,
catches search up.

Locally, Backpack answers on `http://127.0.0.1:8780`. To reach it from other
machines, `make tunnel` starts `cloudflared` with a token-managed Cloudflare
tunnel. In the Cloudflare dashboard, route the tunnel's public hostname to the
service **HTTP**, URL **`backpack:8780`**. That URL is the Backpack container's
name inside Docker. `localhost` won't work there, because inside the tunnel's
container it means the tunnel itself.

## Using the pages

Open Backpack's address in a browser (`http://127.0.0.1:8780` locally) and
paste a token from `make token ACTOR=<your name>`. The browser keeps it until
you sign out or the token is revoked. Each workspace has its board, with where
it stands above, and pages for each story, its phases, its ideas and search.
From the pages you can approve or decline proposed stories, re-rank the
backlog (drag a card, or use its arrows), move a story across the board, say
what it is doing, add to its log, add stories and ideas, and refine or drop
ideas. Every change is recorded under the token's actor, and the board's
rules apply exactly as they do for agents.

## Connecting a project

[docs/ADOPTING.md](docs/ADOPTING.md) is written for an AI agent in the
project. The agent creates the workspace, connects the MCP server, adds
`.backpack.json` and the session hook, and tells the project's agents how to
track. It asks the person only for the address, a few choices and a token,
and tells them how to get one. To set up a new project, point its agent at:

```
https://github.com/lioneljmorrison/backpack/blob/main/docs/ADOPTING.md
```
