# Backpack

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
- Agents use it through an **MCP server**; a UI can use the **HTTP API**
  (`api/openapi.yaml`). Every request needs a token.

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

## Connecting a project

[docs/ADOPTING.md](docs/ADOPTING.md) sets a project up to track its work in
Backpack: creating its workspace, a token, the MCP server, `.backpack.json`,
the optional session hook, and the block that tells its agents how to track.
It's written so an agent in that project can follow it, with a person doing
the steps that need a secret. To set up a new project, point its agent at:

```
https://github.com/lioneljmorrison/backpack/blob/main/docs/ADOPTING.md
```
