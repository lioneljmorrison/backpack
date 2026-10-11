# Engineering Standard: TDD, YAGNI, and Small Solutions

**Status:** Mandatory for new code and behavior changes
**Applies to:** the Backpack service (HTTP API, MCP server, FalkorDB and Typesense
storage, migrations)
and its supporting scripts, including the session hook other projects run

There is deliberately no "update the tracking document in the same branch"
rule: Backpack exists to replace that document.

---

## 1. Required Development Loop

All production behavior must be developed test-first:

1. Write the smallest test that describes one required behavior or boundary.
2. Run it and confirm that it fails for the expected reason.
3. Write the smallest clear implementation that makes it pass.
4. Run the relevant suite and confirm that it passes.
5. Refactor only when the tests remain green.

A test written after the implementation does not, by itself, satisfy this TDD
requirement. Bug fixes begin with a test that reproduces the defect.

Exceptions are limited to non-executable documentation, generated artifacts, and
disposable investigation spikes. Spike code must not enter production.

---

## 2. The board's rules

Backpack is the record people and agents act on to decide what to do next. A
board that can be silently rewritten or left half-changed is worse than none.

- **FalkorDB is the source of truth. Typesense is only a search index** built
  from it, and can always be rebuilt from FalkorDB. Nothing is ever read from
  Typesense to make a board decision.
- **Every board action is one FalkorDB query.** FalkorDB rolls back a failed
  query completely, but has no transactions that span several queries. An
  action and its rule checks (ordering, dependencies, status flow) therefore
  run as a single query, so a rule can never be half-applied.
- **The ordering rule always holds.** A story ranks below everything it depends
  on. Dependencies point only to the same or an earlier phase, never across
  workspaces, and never in a cycle.
- **Logs are append-only.** Story log entries are never edited or deleted. A
  mistake is fixed by a correction entry that points to the one it corrects.
  Every status change writes a log entry.
- **Every change names its actor:** the person or agent whose token made it.
- **Every request is authenticated.** The service is reachable from the
  internet through the tunnel; there is no unauthenticated route except health.
- **Secrets come from the environment.** Never in code, tests, fixtures or any
  file the repository tracks.

---

## 3. YAGNI

Implement only behavior required by an accepted test, the current API contract,
or the current plan.

Do not add speculative:

- Endpoints, MCP tools or fields with no present consumer.
- User accounts, roles or permissions. There is one person today; keep every
  change attributed to an actor so users can be added later, and build no more.
- Integrations with outside task trackers.
- Storage abstractions over FalkorDB or Typesense before a second backend
  exists.
- Configuration switches without a current operational requirement.

---

## 4. Smallest Clear Implementation

Prefer the standard library. Add a dependency only when it replaces substantial,
error-prone code, and record why in the commit message.

Prefer a one-line or one-expression solution when it is complete, obvious and
testable. "One-liner" is a simplicity constraint, not a line-count target. Do
not compress code when doing so would hide a rule, reduce error context, create
dense boolean expressions or make a test harder to read.

---

## 5. Test Shape

Tests are part of the specification and should remain direct and readable.

- One behavior or boundary per test.
- Descriptive names that state the rule and expected outcome.
- Table-driven tests for equivalent input/output cases.
- Storage tests run against real FalkorDB and Typesense — never a mock of the
  store. They skip unless `BACKPACK_TEST_FALKORDB` (and, for search,
  `BACKPACK_TEST_TYPESENSE`) is set; CI sets both, using service containers.
- Each storage test uses its own uniquely named graph and collection, and drops
  them afterwards.
- HTTP handlers are tested through `net/http/httptest`; MCP tools through an
  in-process MCP client.
- Fixtures are invented. Never copy another project's tracked content into one.

Avoid tests that merely reproduce implementation structure or assert private
helper calls.

---

## 6. Definition of Done for Code

A code change is complete only when:

- The new or changed behavior was introduced by a failing test.
- The implementation is the smallest clear solution that passes it.
- All gates pass (§7.3).
- No speculative behavior or unused abstraction was added.
- Every new HTTP endpoint is in the API contract with handler tests; every new
  MCP tool has a test calling it.
- Every change to indexes, constraints or the search schema is a numbered
  migration that applies cleanly to an empty database and to the previous
  version.
- The plan or Backpack's own record is updated.

---

## 7. Task, Context, and Branch Hygiene

One branch represents one bounded task. Start each new task in a fresh AI
session.

Before implementation:

1. Confirm the task has one clear outcome and definition of done.
2. Start from `dev` and update it safely.
3. Inspect the worktree and preserve unrelated or user-owned changes.
4. Create a new task branch before changing code.
5. Write the first failing test before production implementation.

Do not combine unrelated fixes, cleanup, speculative refactoring, or future
features in the same branch.

### 7.1 Branch names

```text
<type>/<YYYYMMDD>-<short-name>
```

- `type` is exactly `bug` or `feature`.
- `YYYYMMDD` is the branch-creation date.
- `short-name` is concise, descriptive kebab-case describing the outcome.

`dev` (integration) and `main` (released) are the only exceptions to the
pattern. Neither is committed to directly.

### 7.2 Delivery sequence

1. Update local `dev` from `origin/dev` and create the task branch from it.
2. Develop through red-green-refactor TDD and YAGNI.
3. Run the complete gate set before handoff.
4. Review the diff for unrelated files, secrets and accidental data.
5. Commit the complete task with its tests and documentation.
6. Push the branch and open a pull request against `dev` with test evidence
   and known limitations.
7. Merge once checks and review pass, then delete the task branch.
8. To release, open a pull request from `dev` into `main`.

Do not claim a task is tested when a required check was skipped or failed.

### 7.3 Gates

| Gate | Must |
|---|---|
| `gofmt -l .` | print nothing |
| `go vet ./...` | pass |
| `go test -race ./...` | pass |
| `go build ./...` | pass |

### 7.4 Commit boundaries

- Commits must belong only to the branch's task.
- Include tests in the same commit series as the behavior they specify.
- Never commit secrets, database files, environment files or tracked content
  from other projects.
- Do not rewrite shared branch history without explicit approval.

### 7.5 AI handoff checkpoint

Before ending a task session, record enough in the branch, the plan (or
Backpack itself, once it tracks this repo) and the pull request to continue
without the chat transcript: objective and acceptance criteria, decisions and
their why, files and contracts changed, tests run and results, and known
limitations or follow-ups.
