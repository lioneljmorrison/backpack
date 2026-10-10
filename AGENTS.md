# Agent team — model routing

This project uses a planner/executor split. Definitions live in `.claude/agents/`.
See `.claude/agents/README.md` for the roster and when each is used.

- **Planning / architecture / hard reasoning:** `planner` (Opus 5.5, high effort).
- **Implementation / mechanical execution against an approved plan:** `executor`
  (Sonnet 5.5, medium effort).
- **Read-only research / codebase mapping:** `researcher` (Haiku 5.5), or the
  built-in Explore agent for broad fan-out searches.

Pin a model only when the tier fits the role; otherwise inherit. The waste is not
using capable models — it is using them for work that never needed judgement.

Every agent is bound by `docs/ENGINEERING-STANDARD.md`. Two rules matter most:

- **Test first.** Behavior is introduced by a failing test, and a task is not
  done until the gates pass. Never modify a test just to make it pass.
- **One query per board action.** Every board change and its rule checks run as
  a single FalkorDB query, so a rule can never be half-applied.
