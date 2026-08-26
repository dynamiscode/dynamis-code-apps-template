# Deployment

## SQLite: one instance

```sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE='My Workspace'
read -s BOOTSTRAP_ADMIN_PASSWORD; export BOOTSTRAP_ADMIN_PASSWORD
docker compose up --build -d
unset BOOTSTRAP_ADMIN_PASSWORD
```

The container listens on `0.0.0.0`, so browser setup for Docker, Coolify, and
other remote deployments requires `BOOTSTRAP_SETUP_TOKEN`. Set it instead of
the three admin variables, deploy, and open `/setup`. This is the recommended
path for platforms that provide environment configuration but no shell access.
Source runs using the default loopback address can open `/setup` without a
token. The CLI fallback is:

```sh
read -s BOOTSTRAP_ADMIN_PASSWORD; export BOOTSTRAP_ADMIN_PASSWORD
docker compose exec app /bootstrap-admin \
  -email owner@example.com -workspace 'My Workspace'
unset BOOTSTRAP_ADMIN_PASSWORD
```

The non-root application listens on port 8080 and stores SQLite data in the
`app-data` volume. Override the host port with `APP_PORT`. Never place SQLite
on ephemeral or shared network storage, and never run more than one
application instance against its file.

## PostgreSQL: multiple instances

For local validation, set a non-production password outside version control:

```sh
POSTGRES_PASSWORD='local-only-password' \
  docker compose -f compose.yaml -f compose.postgres.yaml up --build -d
```

Production PostgreSQL must be external, backed up, TLS-protected, and injected
through `DATABASE_URL`. Multiple application instances also require external
traffic distribution. SSE polling shares database state; connections remain
instance-local.

## Runtime contract

- Container user: UID/GID 65532; writable path: `/data`; port: `8080`.
- Liveness: `/health/live`; readiness: `/health/ready`.
- Send SIGTERM for the configured graceful drain, 10 seconds by default.
- Terminate public TLS before the application and set `HTTP_SECURE=true` so
  HSTS and Secure cookies are enabled.
- Inject secrets at runtime. Do not bake `.env`, database URLs, OIDC secrets,
  OTLP headers, TLS keys, or backups into the image.
- Persist and protect database and backup storage before accepting durable
  data.

The release image uses pinned build and runtime bases and records version,
commit, creation time, and source OCI labels. Start with 0.25 vCPU and 256 MiB
RAM, then tune from the measurements and alert thresholds in
[operations](operations.md).

Run `make docker-smoke` after image or deployment changes. Backup, restore,
upgrade, and alert procedures are in [operations](operations.md).
