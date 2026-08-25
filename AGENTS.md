# Agent contract

This repository is preparing, then implementing, the Dynamis Code Apps template.
Keep work within the active phase in `PLAN.md`.

## Context order

- `STANDARDS.md` defines required behavior while generation is in progress.
- Accepted records under `docs/decisions/` choose implementations within the
  standard.
- `PLAN.md` defines construction order and status.
- The active file under `docs/implementation/` defines phase deliverables and
  evidence.
- `docs/README.md` routes task-specific reading.
- Repository-local skills define recurring procedures; they do not override
  the sources above.

When sources conflict, stop. Resolve the standard, decision, or exception
before coding around it.

## Before changing anything

1. Inspect `git status` and preserve unrelated work.
2. Read `docs/README.md`, the active phase brief, and only the linked standard
   sections relevant to the task.
3. Inspect the files to change and their callers. Reuse existing use cases,
   policies, middleware, and adapters.
4. Load one matching repository-local skill when the task is recurring.
5. State the bounded outcome and its verification before editing.

## Architecture invariants

- Build a resource-conscious Go modular monolith using vertical feature
  slices and manual constructor injection.
- Keep business rules in domain or application code. Web, REST, and MCP call
  the same application use cases.
- The remote CLI calls REST only. MCP tools never access SQL or execute
  arbitrary shell commands.
- WebMCP is optional, browser-tab-bound, and separate from server MCP. Use
  feature detection, explicit non-secret schemas, visible controls, and
  ordinary form fallback; never expose hidden security fields or auto-submit
  mutations.
- Use server-rendered HTML, HTMX for targeted updates, and SSE for one-way
  realtime delivery. Add WebSockets only for measured bidirectional needs.
- Default to SQLite for one application instance. Keep migrations portable to
  PostgreSQL, which is required before multiple instances.
- Prefer the Go standard library, native platform features, and selected
  dependencies. Do not add an ORM, DI container, broker, cache, SPA runtime,
  protocol, or service without its documented trigger.
- Pass workspace scope explicitly. Deny authorization by default and enforce
  permissions in shared application policy, never only in a handler.

## Change obligations

- Keep each change inside one plan item. No drive-by refactors.
- Update implementation, canonical contract, smallest regression test,
  capability evidence, changelog, and implemented-behavior docs together.
- Follow the feature-surface checklist in [docs/development.md](docs/development.md)
  and record intentional omissions.
- Never claim planned behavior is implemented.
- Never log or commit credentials, authorization headers, session values,
  invitation values, connection strings, or signed URLs.
- Record an approved deviation under `docs/decisions/exceptions/` before
  violating a standard. Expired exceptions are non-conforming.
- Mark a phase complete only after its evidence is linked from
  `docs/capabilities.md` and every required command has been inspected.

## Verification ladder

Run only commands whose prerequisites exist, in this order:

1. Focused tests for the changed behavior.
2. `go test ./...`
3. `go vet ./...`
4. `go test -race ./...` for concurrent code or before phase completion.
5. `go generate ./api` and a clean generated diff after OpenAPI generation
   exists.
6. SQLite and PostgreSQL migration/repository compatibility checks after both
   adapters exist.
7. `make docker-smoke` after container and smoke targets exist.
8. The release and conformance checks required by the active phase.

Report passed, failed, blocked, and not-yet-applicable checks separately.

## Instruction scope

Keep this root contract short. Add nested `AGENTS.md` files only when a
subtree has genuinely different rules. Do not add host-specific instruction
adapters without a demonstrated loading need.
