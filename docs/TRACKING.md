# Tracking a project with Backpack

How any project hands its tracking to Backpack: a token, the MCP server, two
files and a hook to add, and a section to paste into its `CLAUDE.md`. Replace `my-app` with the project's
workspace slug and `https://backpack.example.com` with the Backpack address.

## Setup

**1. `.backpack.json`** (committed) names the workspace and where Backpack is:

```json
{ "workspace": "my-app", "url": "https://backpack.example.com" }
```

**2. A token** per person or agent. Someone with access to the Backpack host
issues it with `make token ACTOR=<name>`. It is shown once.

**3. The MCP server**, added to Claude Code once per machine. One server
serves every project, because every tool takes the workspace slug:

```bash
claude mcp add --transport http --scope user backpack https://backpack.example.com/mcp \
  --header "Authorization: Bearer <token>"
```

The token is kept in your user config, never in the repository. Don't also
add a project `.mcp.json` entry named `backpack`: a project entry overrides
the user one.

**4. The session hook.** Copy [`deploy/hook.sh`](../deploy/hook.sh) to
`.claude/hooks/backpack.sh` and add it to `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/backpack.sh\"" }] }
    ]
  }
}
```

The hook reads the token from `BACKPACK_TOKEN` in the environment. Every
session then starts with "where are we". If Backpack can't be reached, the
hook says so in one line and the session carries on.

## The section to paste into CLAUDE.md

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
- **New work** goes in with `add_story`, placed with `before` or `after`, and
  its dependencies with `add_dependency`. Backpack refuses an order that breaks
  a dependency; read its reason and place the story again.
```
