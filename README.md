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
make gates               # gofmt, vet, tests, build
```

Locally, Backpack answers on `http://127.0.0.1:8780`. To reach it from other
machines, `make tunnel` starts `cloudflared` with a token-managed Cloudflare
tunnel. In the Cloudflare dashboard, route the tunnel's public hostname to the
service **HTTP**, URL **`backpack:8780`**. That URL is the Backpack container's
name inside Docker. `localhost` won't work there, because inside the tunnel's
container it means the tunnel itself.

## Connecting a project

A project needs four things:
- a `.backpack.json`;
- a token;
- the MCP server added to Claude Code;
- optionally, the session hook.

In the examples, replace `my-app` with the project's workspace slug and
`https://backpack.example.com` with your Backpack address.

### 1. `.backpack.json`

Commit this at the project's root. It names the workspace and where Backpack is:

```json
{ "workspace": "my-app", "url": "https://backpack.example.com" }
```

### 2. A token

Each person or agent gets their own token, so every change on the board records
who made it. Someone with access to the Backpack host runs:

```bash
make token ACTOR=<name>
```

The token is shown once. Keep it out of any file the repository tracks.

### 3. The MCP server

Claude Code connects to Backpack's MCP server at `<your address>/mcp`, sending
the token as `Authorization: Bearer <token>`. One server works for every
project, because every tool names its workspace. Choose **one** of these ways
to set it up:

#### a. Just you: `claude mcp add`

Run once per machine. The server and your token are stored in your own Claude
Code settings (`~/.claude.json`), not in any repository, and work in every
project.

```bash
claude mcp add --transport http --scope user backpack https://backpack.example.com/mcp \
  --header "Authorization: Bearer <token>"
```

#### b. A whole team: `.mcp.json` in the repository

A `.mcp.json` committed to the project gives everyone who opens it the server
automatically, so nobody has to run `claude mcp add`. Copy
[.mcp.json.example](.mcp.json.example) to `.mcp.json` and set the address.

The file is committed, so it must **never contain a token**. It names an
environment variable instead:

```json
{
  "mcpServers": {
    "backpack": {
      "type": "http",
      "url": "https://backpack.example.com/mcp",
      "headers": { "Authorization": "Bearer ${BACKPACK_TOKEN}" }
    }
  }
}
```

Each person then supplies `BACKPACK_TOKEN`, either way:

- **As an environment variable.** Add `export BACKPACK_TOKEN=<token>` to your
  shell profile, then restart Claude Code. In VS Code, restart VS Code itself.
  Claude Code reads the variable when it starts. If it's missing, the server
  shows as failed, and `claude mcp list` warns `Missing environment variables:
  BACKPACK_TOKEN`.
- **Injected by a secrets manager at launch.** Most secrets managers can start
  a program with secrets set as environment variables. Start Claude Code, or VS
  Code, through it:

  ```bash
  <secrets-cli> run -- claude
  <secrets-cli> run -- code .
  ```

#### c. A whole team, fetching the token from a secrets manager on connect

Instead of a fixed header, `.mcp.json` can name a command (`headersHelper`).
Claude Code runs the command each time it connects, and uses the JSON it prints
as the request headers. With a secrets manager, nothing needs to be in the
environment or launched specially. A rotated token is picked up on the next
connection, and if Backpack refuses a token, Claude Code reruns the command and
retries once.

```json
{
  "mcpServers": {
    "backpack": {
      "type": "http",
      "url": "https://backpack.example.com/mcp",
      "headersHelper": "printf '{\"Authorization\": \"Bearer %s\"}' \"$(<secrets-cli> read backpack-token)\""
    }
  }
}
```

Replace `<secrets-cli> read backpack-token` with your secrets manager's command
that prints the token. Things to know:

- The command runs in the project's directory, and gives up after 10 seconds.
- It runs only after the person has trusted the project in Claude Code.
- Claude Code removes variables whose names contain `TOKEN`, `KEY`, `SECRET`,
  `PASSWORD` or `AUTH` from the command's environment. If your secrets manager
  signs in with such a variable rather than a stored session, run it through a
  script that gets its sign-in another way.

#### Things that apply to every way

- **Use one way per project.** A project `.mcp.json` entry named `backpack`
  overrides your own `claude mcp add` one. If the project entry can't get a
  token, it shows as failed, even though yours would have worked.
- The first time someone opens a project with a `.mcp.json`, Claude Code asks
  them to approve its servers.
- To check: `claude mcp list`, or `/mcp` in a session, should show `backpack`
  as connected.

### 4. The session hook (optional)

The hook prints "where are we" at the start of every Claude Code session. Copy
[deploy/hook.sh](deploy/hook.sh) to `.claude/hooks/backpack.sh` and add it to
`.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/backpack.sh\"" }] }
    ]
  }
}
```

The hook reads the workspace and address from `.backpack.json`, and the token
from `BACKPACK_TOKEN` in the environment. It does not use the MCP server's
settings. It never fails a session:
- without `.backpack.json` it prints nothing;
- without a token, or if Backpack can't be reached within 3 seconds, it prints
  "Backpack unreachable — orientation skipped".

### 5. Tell the agents how to track

Paste this into the project's `CLAUDE.md`:

```markdown
## How this project tracks

Delivery is tracked in Backpack, workspace `my-app` (see `.backpack.json`),
through the `backpack` MCP tools. There is no tracking document, and tracking
is never committed.

- **Orientation.** The session starts with "where are we". Ask `whats_next`
  for the next story and `get_story` for its detail. Don't guess the next task.
- **Record each move as it happens**, with `set_status`:
  - start work: `in_progress`, with the task branch;
  - open the pull request: `in_review`, with its link;
  - merged: `merged`, with the merge commit;
  - then `released` and `deployed` as they happen.
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
```
