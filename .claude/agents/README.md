# Agent roster

Model routing for this repository. Definitions live beside this file; the
harness picks them up by filename.

| Agent | Model | Use for |
|---|---|---|
| `planner` | Opus, high effort | Architecture, plans, anything touching more than one file. Cannot edit. |
| `executor` | Sonnet, medium effort | Building one already-scoped step against failing tests. |
| `researcher` | Haiku | Read-only lookups: where is X, how does Y work. Cannot edit. |

Built-in `Explore` (Haiku) covers broad fan-out codebase searches and needs no
definition here.

## When each is used

- **Plan before you build.** Any change touching more than one file, or any new
  endpoint, MCP tool, table or migration, goes through `planner` first and waits
  for explicit approval. See CLAUDE.md.
- **One step at a time.** `executor` takes a single bounded step from an
  approved plan, writes the failing test first, and runs the gates. It stops and
  reports rather than widening scope.
- **Cheap questions go to `researcher`.** Tracing behaviour or locating code
  does not need a frontier model.

Every agent is bound by `docs/ENGINEERING-STANDARD.md`.
