# Adopting Backpack in a project

This page is for an AI agent, such as Claude Code, working in a project that
should track its delivery in [Backpack](https://github.com/lioneljmorrison/backpack),
a Kanban board for agentic projects. Once it's set up, agents ask Backpack
"where are we" and "what's next" instead of keeping a tracking document beside
the code.

Follow the steps in order. (For a project that already uses Backpack, see
[UPGRADING.md](UPGRADING.md).) You do every step yourself. You need the person
only for what they alone can give you: the Backpack address, a token, a few
choices, and a restart of Claude Code at the end.

## Rules for the token

The token is a secret that lets anything holding it change the board.

- Use it only as `$BACKPACK_TOKEN` inside commands, so it is never printed.
- Never ask the person to paste it into the conversation. If they do anyway,
  don't repeat it, and tell them to have it reissued.
- Never write it into a file the repository tracks, a commit, a log entry or
  a message.
- Don't run commands that print it, such as `echo $BACKPACK_TOKEN`, `env`,
  `curl -v`, or anything that shows the MCP server's headers.
- Don't open files that may hold it: `.claude/settings.local.json`, `.env`
  files, or `~/.claude.json`.
- Before anything is committed, make sure `.gitignore` covers
  `.claude/settings.local.json` and any `.env` file. Add the lines if it
  doesn't. You can do this without opening those files.

## 1. Gather what you need

Check whether a token is in the environment, without printing it:

```bash
[ -n "$BACKPACK_TOKEN" ] && echo "token: set" || echo "token: missing"
```

**If the token is missing,** stop and give the person these steps, then wait:

> Backpack needs a token for this project, set as `BACKPACK_TOKEN`.
>
> 1. **Get a token.** If you run the Backpack server, run this in its
>    repository on that host: `make token ACTOR=<your-name>`. Otherwise ask
>    your Backpack admin for one. Each person or agent gets their own token,
>    so the board records who made each change. It is shown only once.
> 2. **Make it available to Claude Code.** Choose one:
>    - **This project only:** put it in `.claude/settings.local.json` in this
>      project, which is your own settings file here and stays out of git.
>      Create the file if it doesn't exist, or add the `env` entry to it:
>      ```json
>      { "env": { "BACKPACK_TOKEN": "<token>" } }
>      ```
>    - **Every project:** add `export BACKPACK_TOKEN=<token>` to your shell
>      profile (`~/.bashrc`, `~/.zshrc`).
>    - **A `.env` file:** Claude Code doesn't read `.env` files by itself.
>      Put `BACKPACK_TOKEN=<token>` in one that git ignores, and load it
>      before starting Claude Code: `set -a; . ./.env; set +a; claude`.
>    - **A secrets manager:** start Claude Code (or VS Code) through it, for
>      example `<secrets-cli> run -- claude`.
> 3. **Restart Claude Code** (in VS Code, restart VS Code itself) so it
>    picks the token up, then ask me to carry on with the Backpack setup.
>
> Please don't paste the token into this conversation.

Also ask the person, without guessing:

- **The Backpack address**, such as `https://backpack.example.com`. If the
  project already has a `.backpack.json`, it's the `url` there.
- **The workspace slug**: the project's short name on the board, using
  lowercase letters, digits and single hyphens, up to 64 characters. Suggest
  one from the repository's name (for example `my-app`) and let them confirm.
- **Whether a team shares this project**, which decides how the MCP server is
  set up in step 4.

In the commands below, replace `my-app` with the workspace slug and
`https://backpack.example.com` with the address.

## 2. Check the token works

```bash
curl -fsS -H "Authorization: Bearer $BACKPACK_TOKEN" https://backpack.example.com/v1/whoami
```

It answers with the name the token was issued to. A `401` means the token is
wrong or revoked, so give the person the steps in step 1 again. If the address
can't be reached, check it with them.

## 3. Set up the workspace and its branches

Skip this if the workspace already exists, branches included. To check, run:

```bash
curl -fsS -H "Authorization: Bearer $BACKPACK_TOKEN" -H "Accept: text/plain" \
  https://backpack.example.com/v1/workspaces/my-app/status
```

A `404` means it doesn't exist yet. A workspace's **branches** are the places
a story can be merged into, in order, ending in the **release branch**. They
can be changed at any time, so nothing here is final. Work them out in this
order, and don't create anything until the person has approved the proposal.
Nothing pending is stored in Backpack: the proposal lives in this conversation.

### a. Read how the repository flows

Find out how code reaches the release branch today, with the repository's own
tools. For a Git repository, for example:

```bash
git branch -r                                  # the branches that exist
gh repo view --json defaultBranchRef           # the default branch
gh api repos/{owner}/{repo}/branches --jq '.[] | {name, protected}'
gh pr list --state merged --limit 30 --json baseRefName,headRefName
```

The last one shows which branches pull requests merge into. Note the default
branch (usually the release branch), any integration branch that pull requests
target before it, any staging branch between them, and which branches are
protected. Read the project's contributing notes too, if it has them. Don't
guess what you can't see: ask.

### b. Propose the branches

Tell the person, in plain words, what you found and what you suggest:

- the branches, in merge order (for example `dev` merges into `main`, or
  `dev` merges into `staging`, which merges into `main`);
- the **release branch**, the one nothing merges into;
- any branch to show under another name on the web board (an `alias`, such as
  "Production" for `main`).

A project with a single branch is fine: `main` alone is the whole line.

### c. Get the person's answer

Wait for their approval or changes, and revise the proposal until they agree.
Don't apply anything before then.

### d. Apply it

Create the workspace with the seed closest to the approved proposal. The
`has_release_stage` setting only seeds the branches, once: `false` gives `main`
alone, `true` gives `dev` merging into `main`. After that it means nothing.

```bash
curl -fsS -X POST https://backpack.example.com/v1/workspaces \
  -H "Authorization: Bearer $BACKPACK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug": "my-app", "name": "My app", "repo_url": "https://github.com/you/my-app", "has_release_stage": false}'
```

`slug` and `name` are required. `repo_url` is optional. A `409` means the slug
is already taken, so check with the person whether that workspace is this
project's.

Then bring the branches to the approved shape. These are MCP actions: you run
each through `batch` (`actions` names its arguments), and they work after
step 4 and a restart. Until then the same changes are HTTP calls
(`POST /v1/workspaces/{ws}/branches`, `PATCH` and `DELETE`
`/v1/workspaces/{ws}/branches/{name}`, in `api/openapi.yaml`). If the
tools aren't there yet, carry on with step 4 and finish this after the
restart. Check the result with `actions`, naming `list_branches` and
giving `args` of `{"workspace": "my-app"}`.

For example, to add `staging`: `batch` with `actions` of
`[{"tool": "add_branch", "args": {"name": "staging", "into": "main"}}]`.

- **Rename** a seeded branch to the repository's name (`master`, `develop`):
  `update_branch` with `branch` (its current name) and `name`. No story moves.
- **Add** a branch: `add_branch` with `name`, `into` (the existing branch it
  merges into; required, because the release branch is already there) and an
  optional `alias`. For `staging` between `dev` and `main`, add `staging` into
  `main`, then re-point `dev` with `update_branch` and `into` set to
  `staging`.
- **Show a column under another name** on the web board: `update_branch` with
  `alias` (an empty one clears it). The API, tools and logs keep the branch
  name.
- **Remove** a seeded branch the project doesn't use (for example `dev`):
  `remove_branch` with `branch`. It is refused while a story is merged into it
  or a branch merges into it (re-point that branch first), and the release
  branch and the only starting branch are never removed.

While two or more branches both merge into the release branch (`dev` and
`staging` into `main`), a story from review is merged into one of them by
naming it (`into`).

## 4. Connect the MCP server

Claude Code reaches Backpack's tools through its MCP server at
`<address>/mcp`, sending the token as `Authorization: Bearer <token>`. One
server works for every project, because every tool names its workspace.

First check whether it's already connected, from another project on this
machine:

```bash
claude mcp list
```

If `backpack` is listed, skip to step 5. Otherwise use **one** of these ways.
Don't use both: a project `.mcp.json` entry named `backpack` overrides the
person's own one, even when the project entry can't get a token.

### a. Just this person: `claude mcp add`

This is the default when no team shares the project. It stores the server in
the person's own Claude Code settings (`~/.claude.json`), outside any
repository, and it works in every project. The shell fills in the token, and
its output is discarded so that it can't be shown:

```bash
claude mcp add --transport http --scope user backpack https://backpack.example.com/mcp \
  --header "Authorization: Bearer $BACKPACK_TOKEN" > /dev/null
```

### b. A whole team: `.mcp.json` in the repository

Commit this as `.mcp.json` at the project's root. If the file already exists,
add the `backpack` entry to it. It names the variable, never the token, and
everyone who opens the project gets the server once their own
`BACKPACK_TOKEN` is set:

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

The first time someone opens the project, Claude Code asks them to approve
its servers.

A team that keeps the token in a secrets manager can fetch it at each
connection instead, with `headersHelper`. Claude Code runs this command and
uses the JSON it prints as the request headers:

```json
"headersHelper": "printf '{\"Authorization\": \"Bearer %s\"}' \"$(<secrets-cli> read backpack-token)\""
```

The command runs in the project's directory and gives up after 10 seconds.
Claude Code removes variables whose names contain `TOKEN`, `KEY`, `SECRET`,
`PASSWORD` or `AUTH` from its environment. If the secrets manager signs in
with one of those, it must get its sign-in another way.

## 5. Add `.backpack.json`

Commit this at the project's root. It names the workspace and where Backpack
is, and holds no secret:

```json
{ "workspace": "my-app", "url": "https://backpack.example.com" }
```

## 6. Add the session hook

This step is optional, but recommended. The hook prints "where are we" at the
start of every session, so each session starts oriented. Download it into the
project:

```bash
mkdir -p .claude/hooks
curl -fsSL https://raw.githubusercontent.com/lioneljmorrison/backpack/main/deploy/hook.sh \
  -o .claude/hooks/backpack.sh
```

Then add it to `.claude/settings.json`, merging it with anything already
there:

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
from `BACKPACK_TOKEN`. It never fails a session:
- without `.backpack.json` it prints nothing;
- without a token, or if Backpack can't be reached within 3 seconds, it prints
  "Backpack unreachable — orientation skipped".

## 7. Tell the project's agents how to track

Add this section to the project's `CLAUDE.md` (or `AGENTS.md`), with the
workspace slug filled in.

If the project kept a tracking document, propose retiring it. Its open work
goes onto the board with `add_story`, and the document stops being updated.

```markdown
## How this project tracks

Delivery is tracked in Backpack, workspace `my-app` (see `.backpack.json`),
through the `backpack` MCP tools. There is no tracking document, and tracking
is never committed. `actions` lists the rarer actions; a rarer change runs
through `batch`.

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
  clear fix: `add_bug_fix`), then `add_dependency`. Rough thoughts go in with
  `add_idea` and a `context`; `refine_idea` or `drop_idea` closes them.
- **Many changes at once** go in one `batch`; read every answer.
```

## 8. Finish

Commit what you added: `.backpack.json`, `CLAUDE.md`, and, if you used them,
`.mcp.json` and the hook files. Follow the project's own branch and pull
request rules.

Then tell the person what you set up, and ask them to **restart Claude Code**,
so that the `backpack` tools appear. MCP servers are loaded when it starts.
After the restart:

- `claude mcp list`, or `/mcp` in a session, shows `backpack` as connected;
- the `where_are_we` tool answers for the workspace, in plain text (a new one
  has an empty
  board);
- with the hook, the session starts with the same answer instead of
  "Backpack unreachable".
