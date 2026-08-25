# Dynamis Code Apps Template

Status: Phase 04 web foundation. Identity, REST, accessible server-rendered
items, HTMX fragments, and scoped SSE work; MCP and remote CLI remain pending.

The repository implements a reusable Go web-application template with a small
default footprint and explicit paths to larger deployments. The temporary
[generation standard](STANDARDS.md) is authoritative until the template passes
its final conformance gate.

## Run the current foundation

Requirements: Go 1.26 or newer. Go selects the recorded Go 1.27 toolchain.

```sh
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/server
```

The process validates configuration, creates `data/app.db`, applies embedded
migrations, and listens on `:8080`. Check `/health/live`, `/health/ready`, or
the contract at `/api/openapi.json`. Press `Ctrl-C` for graceful shutdown.

Create the first owner with the one-time command documented in
[authentication](docs/authentication.md). It requires an explicit password and
creates the user, workspace, and owner membership atomically.

```sh
go test ./...
go vet ./...
go test -race ./...
```

## Start here

- Maintainers and coding agents: read [AGENTS.md](AGENTS.md), then
  [PLAN.md](PLAN.md).
- Find task-specific context through [docs/README.md](docs/README.md).
- Review implemented boundaries in
  [docs/architecture.md](docs/architecture.md) and configuration in
  [docs/configuration.md](docs/configuration.md).
- Review identity behavior in
  [docs/authentication.md](docs/authentication.md).
- Review HTTP and REST behavior in [docs/api.md](docs/api.md).
- Review browser and realtime behavior in [docs/web.md](docs/web.md) and the
  dated [accessibility evidence](docs/accessibility.md).
- Track planned and deferred capabilities in
  [docs/capabilities.md](docs/capabilities.md).
- Review accepted technology choices in
  [docs/decisions/0001-go-modular-monolith.md](docs/decisions/0001-go-modular-monolith.md).

Phase 07 replaces this foundation guide with the verified application and
container quick start.
