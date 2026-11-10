# Development

## Setup

```sh
make setup
set -a; . ./.env; set +a
go run ./cmd/server
```

Running `make setup` configures `.githooks` (enforcing `gofmt` on staged Go files, plus `go vet` and generate drift checks) and installs dependencies. Refresh only the hook path with `make setup-hooks`.

Use Go 1.26 or newer; the module records the development toolchain. Use an
isolated PostgreSQL database through `POSTGRES_TEST_URL` (for example,
`postgres://postgres@127.0.0.1:5432/dynamis_code_test?sslmode=disable`) for
PostgreSQL compatibility tests across schema migrations, leasing locks, and
queue claim transactions.

## Change workflow

1. Read `AGENTS.md`, inspect `git status`, and follow the route in
   [docs/README.md](README.md).
2. Trace the affected use case, policy, repository, adapters, contracts, and
   tests. Keep workspace scope explicit.
3. Make one bounded change. Update OpenAPI, migrations, behavior docs,
   changelog, and capability evidence only when affected.
4. Run focused checks, then the applicable verification ladder.

When working in parallel agent worktrees or swarms, do not edit CHANGELOG.md
directly on feature branches; changelog updates are aggregated at release or PR
merge time.

Ordinary features stay vertical: application behavior first, transport
adapters second. Reuse shared authorization, Problem Details, audit,
telemetry, and rendering code. Add no optional capability until its trigger in
[capabilities](capabilities.md) is accepted and recorded in a decision.

Localization changes keep embedded English and Spanish catalogs in exact key
parity, use named interpolation only for trusted catalog messages, preserve
escaped user content, and test browser precedence separately from stable
English REST/CLI/MCP contracts. Add plural support only when the first
count-dependent product message requires it.

## Feature completeness

For every feature, record its shared application behavior, authorization
boundary, and applicable surfaces before implementation:

- browser: routes, templates, CSRF, accessible controls, and ordinary-form
  fallback;
- browser-agent (WebMCP): decide whether assistance helps, define one-purpose
  bounded schemas, exclude secrets and hidden security fields, preserve
  ordinary-form fallback, and test supported and unsupported browsers;
- REST/OpenAPI: bearer authorization, generated contract, limits, errors, and
  response redaction;
- CLI/MCP: only when automation needs the capability, using shared use cases;
- realtime: only when clients need change delivery, with scope and reconnect
  semantics;
- operations/data: persistence, migrations, export, retention, deletion, and
  recovery when durable data is involved;
- state lifecycle: entities with multi-step lifecycles define typed status enums,
  pure computed `Status(now)` methods, and `ValidateTransition` functions
  matching the [architecture](architecture.md#domain-state-machines) pattern.

Mark each omitted surface with a reason in the phase brief or capability
ledger. A feature is complete only when every applicable surface has code,
focused tests, canonical documentation, and linked capability evidence.

## Agent environment and hook resilience

When running coding agents or AI orchestrators (e.g. Antigravity, Orca, Claude Code)
configured with lifecycle hooks (`PreInvocation`, `PreToolUse`, `PostToolUse`),
hook scripts must adhere to strict runtime boundaries:

1. **Non-blocking stdin**: Stdin reading should never wait indefinitely or employ
   long select timeouts. If stdin has no immediate data or EOF, hooks must continue
   immediately.
2. **Sub-second execution**: The agent CLI enforces strict timeouts on tool hooks
   (typically 2-5 seconds). Synchronous hooks that block on network requests or
   heavy computations will be terminated with `signal: killed`, failing tool calls.
3. **Graceful degradation**: Inability to contact telemetry or spooling endpoints
   must fall back cleanly without emitting errors or aborting tool execution.

Verify hook script timing and non-blocking behavior using:
```sh
./scripts/check-agent-hooks.sh /path/to/hook-script
```

## Checks

```sh
make lint
make verify
npm ci && make accessibility-smoke
make webmcp-smoke
make docker-smoke
```

`make verify` checks formatting, tests, vet, race behavior, module checks,
workflow syntax and action pins, generated OpenAPI drift, command builds, and
generation of a clean application. Run `node --check internal/web/assets/*.js`
when modifying browser scripts. Run `make secret-check`, `make vuln-check`,
and `make fuzz-smoke` for the source security gates, plus the full suite with
`POSTGRES_TEST_URL` for data or release changes. CI also runs CodeQL for Go and
JavaScript/TypeScript.

### Verification checklist

Prior to PR creation, verify changes with the following checklist:
- Run `make lint` (runs `fmt-check` and `go vet`) so mechanical formatting failures are caught before pushing.
- Run focused tests and `go test ./...` across affected packages.
- Run `make verify` before submitting changes for review.
- Run `npm ci && make accessibility-smoke`, `make webmcp-smoke`, and `make docker-smoke` when interface or container delivery are affected.
- Run security gates (`make secret-check`, `make vuln-check`, `make fuzz-smoke`) and database tests against PostgreSQL when applicable.

## Replace the sample feature

The item feature is the executable reference slice. Replace or remove it only
as one reviewed vertical change after the replacement passes browser, REST,
CLI, MCP, SSE, and WebMCP fallback smoke. Update routes, shared use cases,
OpenAPI, migrations, tests, navigation, docs, and capability evidence
together; do not leave a partially disconnected sample.

Contribution and security-reporting rules live in
[CONTRIBUTING.md](../CONTRIBUTING.md) and [SECURITY.md](../SECURITY.md).
