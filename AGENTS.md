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

## Before changing anything

1. Inspect `git status` and preserve unrelated work.
2. Read the task route in `docs/README.md` and the files to change and call.
3. Reuse existing use cases, policies, middleware, and adapters.

## Architecture invariants

- Follow [architecture](docs/architecture.md) and [coding standards](CODING_STANDARDS.md).
- MCP tools never access SQL or execute arbitrary shell commands.
- Follow the [WebMCP contract](docs/web.md#webmcp-progressive-enhancement) when changing browser-agent behavior.
- Use server-rendered HTML, HTMX for targeted updates, and SSE for one-way
  realtime delivery. Add WebSockets only for measured bidirectional needs.
- Use Tailwind CSS and daisyUI for all page components, including prototypes.
- Do not add an ORM, DI container, broker, cache, SPA runtime, protocol, or
  service until its trigger in `docs/capabilities.md` is accepted.
- Pass workspace scope explicitly. Deny authorization by default in shared
  application policy, never only in a handler.

## Change obligations

- For test design, test removal, or CI changes, follow
  [test and CI practices](docs/development.md#test-and-ci-practices).

- For new browser journeys or meaningful interaction or visual changes, follow
  [UX review](docs/ux-review.md). Missing rendered-browser evidence blocks
  completion; automated checks alone do not establish usability.
- For browser work, use and extend the [local preview](cmd/preview/README.md)
  when synthetic states help; keep preview routes and fakes out of production.

- When working in parallel agent worktrees or swarms, do not edit CHANGELOG.md
  directly on feature branches; changelog updates are aggregated at release or
  PR merge time.
- Never log or commit credentials, authorization headers, session values,
  invitation values, connection strings, or signed URLs.
- Record an approved deviation under `docs/decisions/exceptions/` before
  violating an invariant. Expired exceptions are non-conforming.
- Write approval text in an exception record only when the user tells you to
  approve it in the current conversation. Never edit an existing approval line
  on your own.

Keep this root contract short. Add nested `AGENTS.md` files only when a subtree
has genuinely different rules.

## Agent skills

### Issue tracker

See `docs/agents/issue-tracker.md`.

### Triage labels

See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` at root (created lazily) and ADRs in `docs/decisions/`. See `docs/agents/domain.md`.
