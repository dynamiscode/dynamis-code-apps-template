# Agent contract

This repository is the Dynamis Code Apps Go template. Keep changes bounded,
maintainable, and evidenced.

## Context order

1. `docs/README.md` routes task-specific reading.
2. Implemented contracts, migrations, and accepted records under
   `docs/decisions/` define behavior and architecture.
3. `docs/capabilities.md` records conformance evidence and deferred triggers.
4. Repository-local skills define recurring procedures; they do not override
   the sources above.

When sources conflict, stop and resolve the contract or decision first.

## Before changing anything

1. Inspect `git status` and preserve unrelated work.
2. Read the task route in `docs/README.md` and the files to change and call.
3. Reuse existing use cases, policies, middleware, and adapters.
4. Load one matching repository-local skill for a recurring workflow.
5. State the bounded outcome and verification before editing.
6. Use path-bounded or symbol-scoped searches (`git grep <pat> <dir>`, LSP symbol queries) rather than unbounded root keyword scans to keep context compact.

## Architecture invariants

- Keep a resource-conscious Go modular monolith with vertical feature slices
  and manual constructor injection.
- Keep business rules in domain or application code. Web, REST, and MCP call
  shared application use cases; the remote CLI calls REST only.
- MCP tools never access SQL or execute arbitrary shell commands.
- WebMCP is optional, browser-tab-bound, and separate from server MCP. Use
  feature detection, explicit non-secret schemas, visible controls, and
  ordinary form fallback; never expose hidden security fields or auto-submit
  mutations.
- Use server-rendered HTML, HTMX for targeted updates, and SSE for one-way
  realtime delivery. Add WebSockets only for measured bidirectional needs.
- Default to SQLite for one instance. PostgreSQL is required before multiple
  instances.
- Do not add an ORM, DI container, broker, cache, SPA runtime, protocol, or
  service until its trigger in `docs/capabilities.md` is accepted.
- Pass workspace scope explicitly. Deny authorization by default in shared
  application policy, never only in a handler.
- Model asynchronous queue, worker, and delivery lifecycles with pure outcome
  evaluators returning typed status enums; apply evaluated outcomes in persistence
  queries without distributing retry or exhaustion conditions across handler loops.

## Change obligations

- Make one reviewable change; no drive-by refactors.
- Update implementation, canonical contract, smallest regression test,
  capability evidence, changelog, and behavior docs together when applicable.
- When working in parallel agent worktrees or swarms, do not edit CHANGELOG.md
  directly on feature branches; changelog updates are aggregated at release or
  PR merge time.
- Follow the feature-surface checklist in [docs/development.md](docs/development.md)
  and record intentional omissions.
- Never claim planned behavior is implemented.
- Never log or commit credentials, authorization headers, session values,
  invitation values, connection strings, or signed URLs.
- Record an approved deviation under `docs/decisions/exceptions/` before
  violating an invariant. Expired exceptions are non-conforming.

## Verification ladder

Run applicable commands in order:

1. Focused tests.
2. `node --check internal/web/assets/*.js` for browser script changes.
3. `go test ./...`
4. make lint (runs fmt-check and go vet)
5. `go test -race ./...` for concurrent code or release readiness.
6. `go generate ./api` and confirm no generated drift.
7. SQLite and isolated PostgreSQL compatibility checks for data changes.
8. `make docker-smoke` for runtime or delivery changes.
9. `make verify` before release.

Report passed, failed, blocked, and not-applicable checks separately.

Keep this root contract short. Add nested `AGENTS.md` files only when a subtree
has genuinely different rules.
