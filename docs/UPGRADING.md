# Upgrading a project that uses Backpack

This page is for an AI agent, such as Claude Code, working in a project that
already tracks its delivery in [Backpack](https://github.com/lioneljmorrison/backpack).
It is organised by version. Work through every section from the version the
project is on up to the current one, in order. For a new project, use
[ADOPTING.md](ADOPTING.md) instead.

You do each step yourself. Ask the person only where a step says to, and never
change their own settings files without telling them what you changed.

## Upgrading to 0.5.0

The server now offers a smaller, stable set of 13 tools. The rarer changes are
no longer tools of their own: they run through `batch` by name, and the rarer
reads run through `actions`, which also describes them all. These left the
tool list: `add_branch`, `update_branch`,
`remove_branch`, `merge_branch`, `cut_release`, `add_phase`, `update_phase`,
`add_goal`, `update_goal`, `mark_goal_met`, `refine_idea`, `drop_idea`,
`update_idea`, `move_idea`, `move_story`, `remove_dependency` and
`mark_reviewed` (changes, run with `batch`), and `list_branches`,
`list_releases`, `get_release`, `read_log`, `list_phases`, `list_ideas` and
`get_idea` (reads, run with `actions`). A call to one of them directly is now refused as an unknown
tool. The arguments, answers and refusals are unchanged.

`add_bug_fix` is new, and also on request (run with `batch`; `actions` gives
its arguments). It adds a bug with a clear fix to the backlog without waiting
for approval, if every check holds, and is refused otherwise. `add_story` is
unchanged and still proposes.

`get_context` is a new read, also on request (`actions` with `name` and
`args`, `id` and the workspace). It answers summaries of a story's
neighbourhood in one call: phase and goals, source idea, dependencies and
dependents (finished ones too) with their recent decisions, where it was
found, what was found while building it, and open bugs. The HTTP route is
`GET /v1/workspaces/{ws}/stories/{id}/context`.

`list_events` is a new read, also on request (`actions` with `name` and
`args`, the workspace included). It answers the latest 20 things that happened
on the board, however old, oldest first, one short line each: decisions,
corrections, merges, holds, closes, releases, and stories and ideas added. It
takes one optional `before` or `after` (a position such as `L-41`) or `since`
(an RFC 3339 time or a date) to page back or catch up. The HTTP route is
`GET /v1/workspaces/{ws}/events`, with the same query parameters.

`search` takes a new optional `story` (the HTTP route a `story` query
parameter). With it, search looks near that story first: its dependencies and
dependents, where it was found, what it found, its nearby bugs, its idea and
its phase with the phase's other stories. If nothing matches there, it
searches the whole workspace. The answer gains `scope` (`story` or
`workspace`) only when `story` was given; without it nothing changes. An
unknown story is refused.

To run a read, call `actions` with `name` and `args`, the workspace included:
`{"name": "list_ideas", "args": {"workspace": "my-app"}}`. A name alone
describes the action instead of running it. `actions` never runs a change, so
it stays read-only; `batch` still refuses reads.

### 1. Swap the tracking section

Replace the "How this project tracks" section in the project's `CLAUDE.md`
(or `AGENTS.md`) with the current one in [ADOPTING.md](ADOPTING.md), step 7,
filling in the workspace slug. If the section was changed on purpose, keep
those changes and bring the rest in line with the current text.

### 2. Rewrite tool permission rules

Claude Code's permission rules name tools as `mcp__backpack__<name>`. Look for
rules naming a tool from the list above, in `.claude/settings.json` and
`.claude/settings.local.json` (the project's), and `~/.claude/settings.json`
(the person's). `.claude/settings.local.json` may hold a token, so read only
its `permissions` entries and never print the rest.

- A rule for a removed name no longer matches anything. Replace an `allow` or
  `deny` rule for a removed change with one for `mcp__backpack__batch`. Replace
  one for a removed read with one for `mcp__backpack__actions`, which only
  reads (add it if the project allows reads).
- Allowing `batch` allows every change it can run. A person who allowed only
  some of the rarer changes needs to know this, so tell them before you widen
  a rule, and ask whether to allow `batch`, to ask each time, or to leave it.
- A `deny` rule for a removed change no longer blocks it, because `batch` now
  runs it. (A `deny` for `actions` blocks the lists and the reads, not the
  changes.) Tell the person, and ask before you replace it.

### 3. Find other places that name a removed tool

Search the project for the names above: its docs, skills, agent definitions,
scripts and hook files (for example `grep -rnE "add_branch|cut_release|add_phase|list_ideas|read_log" .`,
extended with every name above). List what you find to the person, with the file and line, and ask
before you change any of it. The change is the same each time: say that the
change runs through `batch` (`{"tool": "cut_release", "args": {...}}`), a read
through `actions` (`{"name": "list_ideas", "args": {"workspace": "..."}}`), and
`actions` gives the arguments of either.

### 4. Check it works

After the person restarts Claude Code (MCP servers load at startup):

- call `actions` with no name: it answers one line for each rarer action;
- call `actions` with a name, such as `cut_release`: it answers that action's
  arguments;
- call `actions` with `name` `list_phases` and `args` with your workspace: it
  answers the phases;
- call `batch` with one action that changes nothing, such as `mark_reviewed`
  on a story id that does not exist (`S-0`): `results[0]` has `ok` false and a
  reason that names the story. That shows `batch` reaches the rarer actions.

If `actions` is not there, the server is older than 0.5.0: tell the person.

### 5. Several repositories (optional)

A workspace can hold several repositories. A project that stays with one
repository needs nothing: the board answers as before, and `repo` is never
required.

- **The hook.** To see only one repository's next stories at session start,
  download the hook again (the same command as in
  [ADOPTING.md](ADOPTING.md) step 6, which overwrites the old copy) and add
  `"repo": "<name>"` to `.backpack.json`. Without `repo`, or with the old hook,
  the session shows the whole workspace's next stories.
- **Tracking section.** Bring in the current one from ADOPTING.md step 7: it
  tells agents to pass `repo` from `.backpack.json` to `whats_next`,
  `add_story` and `add_bug_fix`.
- **A second repository in an existing workspace.** The existing repository is
  already there, named after the workspace slug (see it with `list_repos`
  through `actions`). Add the new one with `add_repo` (through `batch`), shape
  its branches with `add_branch` and `repo`, then give each repository's
  `.backpack.json` a `"repo"`. Details: "Several repositories" in
  [ADOPTING.md](ADOPTING.md) step 3. From then on a story, branch action,
  merge or release that names no repository is refused, so agents must pass
  `repo`.
- **Joining workspaces** that already exist separately into one is done by the
  person running the board (`make join`); if it was done for yours, see "Your
  workspaces were joined" below. Renaming or removing a repository is not
  available yet.

## Your workspaces were joined

Follow this only when the person tells you the project's workspace was joined
into another (`make join`), or when a request answers `<old> was joined into
<new>; use workspace <new> with repo <r>` (404). The work was copied to the new
workspace; the old workspace refuses every request and drops out of
`list_workspaces`, and its data is kept.

1. **Point the project at the new workspace.** In the project's
   `.backpack.json` set `"workspace"` to the new workspace's slug and add
   `"repo"` set to the old workspace's slug. The old workspace's repository
   keeps its name, and the refusal names it. If the refusal lists several
   (`r1 or r2`), this project is the one that holds that repository: use its
   name. Leave `"url"` as it is.
2. **Several repositories now apply.** The new workspace holds the repositories
   of every workspace that was joined, so a story, branch action, merge or
   release that names no repository is refused. Bring in the tracking section
   from [ADOPTING.md](ADOPTING.md) step 7 and the hook from step 6, as in
   "Several repositories (optional)" above, so that `repo` from
   `.backpack.json` is passed to `whats_next`, `add_story` and `add_bug_fix`.
3. **Tokens are unchanged.** Tokens belong to people and agents, not to a
   workspace; the existing ones work on the new workspace. Do not ask for a new
   one.
4. **Ids changed.** In the new workspace a story, idea, goal, release and log
   entry may have a new id. Each keeps its old id and workspace as `old_id` and
   `old_workspace`, and each story's log ends with a note saying where it came
   from. Text is copied exactly, so descriptions and plans that name an id such
   as `S-3` still name the old one. Branch names recorded on stories are
   unchanged. Check what the project's own files (a plan, a commit message, a
   pull request description) say about ids before relying on them.
5. **Search is ready.** The join rebuilds the new workspace's search index and
   drops the old workspaces' indexes, so searching the new workspace works at
   once and finds the join notes. Only if the person tells you the join printed
   `search: run make reindex` (Typesense was down) does search wait until they
   run it.
6. **Check it works.** `whats_next` with the `repo` answers from the new
   workspace.
