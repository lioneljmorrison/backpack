# Adopting Backpack in a project

This page sets a project up to track its delivery in
[Backpack](https://github.com/lioneljmorrison/backpack), a Kanban board for
agentic projects. Once it's set up, agents ask Backpack "where are we" and
"what's next" instead of keeping a tracking document beside the code.

It's written so an agent working in the project can follow it from start to
finish, with a person doing the steps that need access or a secret.

## Before you start

Ask the person for these, and don't guess them:

- **The Backpack address**, such as `https://backpack.example.com`.
- **The workspace slug**: the project's short name on the board, using
  lowercase letters, digits and single hyphens, up to 64 characters (for
  example `my-app`). Ask whether the workspace already exists.
- **Whether the project has a release stage**: a Released step between Merged
  and Deployed, for projects that ship merged work in releases. It's set once,
  when the workspace is created, and can't be changed later.
- **A token**, which comes later (step 2), and which the person keeps.

**Tokens are secrets.** Never write a token into a file the repository tracks,
a commit, a log entry, or a message. Never ask for one to be pasted into the
conversation. The person puts it in their own settings or environment.

In the examples, replace `my-app` with the workspace slug and
`https://backpack.example.com` with the Backpack address.

| Step | Who |
|---|---|
| 1. Create the workspace | the person |
| 2. Get a token | the person |
| 3. Connect the MCP server | the person |
| 4. Add `.backpack.json` | the agent |
| 5. Add the session hook (optional) | the agent, then the person |
| 6. Tell the agents how to track | the agent |
| 7. Check it works | either |

## 1. Create the workspace (the person)

Skip this if the workspace already exists. Each workspace is created once,
through the HTTP API, with any valid token:

```bash
curl -X POST https://backpack.example.com/v1/workspaces \
  -H "Authorization: Bearer $BACKPACK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug": "my-app", "name": "My app", "repo_url": "https://github.com/you/my-app", "has_release_stage": false}'
```

`slug` and `name` are required. `repo_url` is optional. `has_release_stage`
defaults to `false`. A `409` means the slug is already taken.

## 2. Get a token (the person)

Each person or agent gets their own token, so every change on the board records
who made it. Someone with access to the Backpack host runs, in the Backpack
repository:

```bash
make token ACTOR=<name>
```

The token is shown once. Keep it out of any file the repository tracks.

## 3. Connect the MCP server (the person)

Claude Code connects to Backpack's MCP server at `<address>/mcp`, sending the
token as `Authorization: Bearer <token>`. One server works for every project,
because every tool names its workspace. Choose **one** of these ways to set it
up.

### a. Just you: `claude mcp add`

Run once per machine. The server and the token are stored in your own Claude
Code settings (`~/.claude.json`), not in any repository, and work in every
project.

```bash
claude mcp add --transport http --scope user backpack https://backpack.example.com/mcp \
  --header "Authorization: Bearer <token>"
```

If the server is already added on this machine for another project, there's
nothing to do.

### b. A whole team: `.mcp.json` in the repository

A `.mcp.json` committed to the project gives everyone who opens it the server
automatically, so nobody has to run `claude mcp add`. The file is committed, so
it must **never contain a token**. It names an environment variable instead:

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

Each person then supplies `BACKPACK_TOKEN` in one of two ways:

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

### c. A whole team, fetching the token from a secrets manager on connect

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

### Things that apply to every way

- **Use one way per project.** A project `.mcp.json` entry named `backpack`
  overrides your own `claude mcp add` one. If the project entry can't get a
  token, it shows as failed, even though yours would have worked.
- The first time someone opens a project with a `.mcp.json`, Claude Code asks
  them to approve its servers.
- New MCP servers are picked up when Claude Code starts, so restart it after
  adding one.

## 4. Add `.backpack.json` (the agent)

Commit this at the project's root. It names the workspace and where Backpack
is, and holds no secret:

```json
{ "workspace": "my-app", "url": "https://backpack.example.com" }
```

## 5. Add the session hook (optional)

The hook prints "where are we" at the start of every Claude Code session.

The agent copies
[deploy/hook.sh](https://raw.githubusercontent.com/lioneljmorrison/backpack/main/deploy/hook.sh)
to `.claude/hooks/backpack.sh` in the project:

```bash
mkdir -p .claude/hooks
curl -fsSL https://raw.githubusercontent.com/lioneljmorrison/backpack/main/deploy/hook.sh \
  -o .claude/hooks/backpack.sh
```

Then the agent adds it to `.claude/settings.json`, merging it with anything
already there:

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
from `BACKPACK_TOKEN` in the environment. It doesn't use the MCP server's
settings, so with way 3a or 3c the person also sets `BACKPACK_TOKEN` in their
environment if they want the hook. It never fails a session:
- without `.backpack.json` it prints nothing;
- without a token, or if Backpack can't be reached within 3 seconds, it prints
  "Backpack unreachable — orientation skipped".

## 6. Tell the agents how to track (the agent)

Paste this into the project's `CLAUDE.md` (or `AGENTS.md`), with the workspace
slug filled in. If the project kept a tracking document, it can be retired:
add its open work to the board with `add_story`, and stop updating the
document.

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
- **Search first.** Before adding a story or an idea, ask `search` whether
  one already covers it, by keyword or by meaning. Closed and dropped ones
  come back too, with their state.
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
```

## 7. Check it works

- `claude mcp list`, or `/mcp` in a session, shows `backpack` as connected.
- The agent calls the `where_are_we` tool with the workspace slug. A new
  workspace answers with an empty board. An error naming the workspace means
  step 1 hasn't been done, or the slug in `.backpack.json` is different.
- With the hook, a new session starts with the same answer instead of
  "Backpack unreachable".

The person can check without Claude Code, too:

```bash
curl -H "Authorization: Bearer $BACKPACK_TOKEN" -H "Accept: text/plain" \
  https://backpack.example.com/v1/workspaces/my-app/status
```
