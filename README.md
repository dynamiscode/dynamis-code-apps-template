# Dynamis Code Apps Template

A resource-conscious Go modular monolith with server-rendered HTML, REST, MCP,
a REST-only CLI, SQLite by default, and an optional PostgreSQL deployment.

## Run with Docker

Requirements: Docker with Compose.

```sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE='My Workspace'
read -s BOOTSTRAP_ADMIN_PASSWORD; export BOOTSTRAP_ADMIN_PASSWORD
docker compose up --build -d
unset BOOTSTRAP_ADMIN_PASSWORD
```

Open <http://localhost:8080/login>. Data persists in the `app-data` volume.
The container listens on `0.0.0.0`, so a no-shell or remote deployment must set
`BOOTSTRAP_SETUP_TOKEN` in the platform environment before opening `/setup`, or
set all three admin variables for unattended bootstrap. See
[deployment](docs/deployment.md) for PostgreSQL, TLS, and production boundaries.

## Run from source

Requirements: Go 1.26 or newer. Go selects the recorded toolchain.

```sh
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/server
```

Create the first owner through environment bootstrap:

```sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE='My Workspace'
read -s BOOTSTRAP_ADMIN_PASSWORD; export BOOTSTRAP_ADMIN_PASSWORD
go run ./cmd/server
unset BOOTSTRAP_ADMIN_PASSWORD
```

With the default loopback address, an empty database also redirects
`/login` to `/setup`; no setup token is required for that local browser flow.

The CLI fallback is documented in [authentication](docs/authentication.md).

Health endpoints are `/health/live` and `/health/ready`; OpenAPI is
`/api/openapi.json`. Press `Ctrl-C` for graceful shutdown.

## Generate an application

Run from a verified release checkout and supply its immutable identity:

```sh
go run ./cmd/template-init \
  -output ../my-app -name 'My App' -module example.com/acme/my-app \
  -source https://github.com/OWNER/REPOSITORY \
  -commit 0123456789abcdef0123456789abcdef01234567
```

Generation refuses an existing output directory and writes `template.lock`.
It also replaces the output README with an application-specific handoff that
links the implemented setup, architecture, interfaces, operations, security,
contribution, release, and sample-feature guidance.
See [template lifecycle](docs/template-lifecycle.md) before updating a
generated application.

## Verify

```sh
make verify
npm ci && make accessibility-smoke
make docker-smoke
```

Use the [documentation router](docs/README.md) for architecture, configuration,
interfaces, operations, security, and contribution guidance.

Repository governance, dependency attribution, and the pending project-license
decision are documented in [governance](docs/governance.md).
