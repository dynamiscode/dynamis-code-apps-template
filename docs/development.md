# Development

## Setup

```sh
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/server
```

Use Go 1.26 or newer; the module records the development toolchain. Use an
isolated PostgreSQL database through `POSTGRES_TEST_URL` for PostgreSQL tests.

## Change workflow

1. Read `AGENTS.md`, inspect `git status`, and follow the route in
   [docs/README.md](README.md).
2. Trace the affected use case, policy, repository, adapters, contracts, and
   tests. Keep workspace scope explicit.
3. Make one bounded change. Update OpenAPI, migrations, behavior docs,
   changelog, and capability evidence only when affected.
4. Run focused checks, then the applicable verification ladder.

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
  recovery when durable data is involved.

Mark each omitted surface with a reason in the phase brief or capability
ledger. A feature is complete only when every applicable surface has code,
focused tests, canonical documentation, and linked capability evidence.

## Checks

```sh
make verify
npm ci && make accessibility-smoke
make webmcp-smoke
make docker-smoke
```

`make verify` checks formatting, tests, vet, race behavior, module checks,
workflow syntax and action pins, generated OpenAPI drift, command builds, and
generation of a clean application. Run `make secret-check`, `make vuln-check`,
and `make fuzz-smoke` for the source security gates, plus the full suite with
`POSTGRES_TEST_URL` for data or release changes. CI also runs CodeQL for Go and
JavaScript/TypeScript.

## Replace the sample feature

The item feature is the executable reference slice. Replace or remove it only
as one reviewed vertical change after the replacement passes browser, REST,
CLI, MCP, SSE, and WebMCP fallback smoke. Update routes, shared use cases,
OpenAPI, migrations, tests, navigation, docs, and capability evidence
together; do not leave a partially disconnected sample.

Contribution and security-reporting rules live in
[CONTRIBUTING.md](../CONTRIBUTING.md) and [SECURITY.md](../SECURITY.md).
