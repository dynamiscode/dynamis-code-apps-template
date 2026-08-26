# Dynamis Code Apps Template

A resource-conscious Go modular monolith with server-rendered HTML, REST, MCP,
a REST-only CLI, SQLite by default, and an optional PostgreSQL deployment.

## Run with Docker

Requirements: Docker with Compose.

```sh
docker compose up --build -d
read -s BOOTSTRAP_PASSWORD; export BOOTSTRAP_PASSWORD
docker compose exec -e BOOTSTRAP_PASSWORD app /bootstrap-admin \
  -email owner@example.com -workspace 'My Workspace'
unset BOOTSTRAP_PASSWORD
```

Open <http://localhost:8080/login>. Data persists in the `app-data` volume.
See [deployment](docs/deployment.md) for PostgreSQL, TLS, and production
boundaries.

## Run from source

Requirements: Go 1.26 or newer. Go selects the recorded toolchain.

```sh
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/server
```

Create the first owner in another terminal:

```sh
read -s BOOTSTRAP_PASSWORD; export BOOTSTRAP_PASSWORD
go run ./cmd/bootstrap-admin \
  -email owner@example.com -workspace 'My Workspace'
unset BOOTSTRAP_PASSWORD
```

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
