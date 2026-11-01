# Upgrading a project that uses Backpack

This page is for an AI agent, such as Claude Code, working in a project that
already tracks its delivery in [Backpack](https://github.com/lioneljmorrison/backpack).
It is organised by version. Work through every section from the version the
project is on up to the current one, in order. For a new project, use
[ADOPTING.md](ADOPTING.md) instead.

You do each step yourself. Ask the person only where a step says to, and never
change their own settings files without telling them what you changed.

## Upgrading to 0.5.0

The server now offers a smaller, stable set of tools. The rarer changes are no
longer tools of their own: they run through `batch` by name, and `actions`
describes them. These left the tool list: `add_branch`, `update_branch`,
`remove_branch`, `merge_branch`, `cut_release`, `add_phase`, `update_phase`,
`add_goal`, `update_goal`, `mark_goal_met`, `refine_idea`, `drop_idea`,
`update_idea`, `move_idea`, `move_story`, `remove_dependency` and
`mark_reviewed`. A call to one of them directly is now refused as an unknown
tool. The arguments, answers and refusals are unchanged.

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
  `deny` rule for a removed tool with one for `mcp__backpack__batch`, and add
  `mcp__backpack__actions`, which only reads.
- Allowing `batch` allows every change it can run. A person who allowed only
  some of the rarer changes needs to know this, so tell them before you widen
  a rule, and ask whether to allow `batch`, to ask each time, or to leave it.
- A `deny` rule for a removed tool no longer blocks it, because `batch` now
  runs it. Tell the person, and ask before you replace it.

### 3. Find other places that name a removed tool

Search the project for the names above: its docs, skills, agent definitions,
scripts and hook files (for example `grep -rnE "add_branch|cut_release|add_phase|refine_idea" .`,
extended with every name above). List what you find to the person, with the file and line, and ask
before you change any of it. The change is the same each time: say that the
action runs through `batch` (`{"tool": "cut_release", "args": {...}}`), and
that `actions` gives its arguments.

### 4. Check it works

After the person restarts Claude Code (MCP servers load at startup):

- call `actions` with no name: it answers one line for each rarer action;
- call `actions` with a name, such as `cut_release`: it answers that action's
  arguments;
- call `batch` with one action that changes nothing, such as `mark_reviewed`
  on a story id that does not exist (`S-0`): `results[0]` has `ok` false and a
  reason that names the story. That shows `batch` reaches the rarer actions.

If `actions` is not there, the server is older than 0.5.0: tell the person.
