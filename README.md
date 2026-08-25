# Dynamis Code Apps Template

Status: Phase 02 identity foundation. Database startup, authentication,
workspace authorization, and credential lifecycles work; HTTP, web, REST, MCP,
and remote CLI surfaces remain unimplemented.

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
migrations, then waits for shutdown. Press `Ctrl-C` to stop. No HTTP port
exists before Phase 03.

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
- Track planned and deferred capabilities in
  [docs/capabilities.md](docs/capabilities.md).
- Review accepted technology choices in
  [docs/decisions/0001-go-modular-monolith.md](docs/decisions/0001-go-modular-monolith.md).

Phase 07 replaces this foundation guide with the verified application and
container quick start.
