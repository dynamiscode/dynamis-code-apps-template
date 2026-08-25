# Configuration

Phase 01 configuration is deployment-owned and loaded from environment
variables at startup. Invalid values stop startup before application work
begins. Changes require restart.

| Variable | Type | Default | Required | Secret |
|---|---|---|---|---|
| `DATABASE_DRIVER` | `sqlite` or `postgres` | `sqlite` | No | No |
| `SQLITE_PATH` | non-empty path | `data/app.db` | SQLite only | No |
| `DATABASE_URL` | PostgreSQL URL | none | PostgreSQL only | Yes |
| `DATABASE_MAX_OPEN_CONNS` | integer 1-64 | `4` | No | No |
| `DATABASE_MAX_IDLE_CONNS` | integer 1-64, not above open limit | `2` | No | No |

SQLite always uses one open and one idle connection regardless of pool
variables because one instance owns the file.

## SQLite

```sh
DATABASE_DRIVER=sqlite \
SQLITE_PATH=data/app.db \
go run ./cmd/server
```

## PostgreSQL

```sh
DATABASE_DRIVER=postgres \
DATABASE_URL='postgres://user:password@localhost:5432/app?sslmode=require' \
go run ./cmd/server
```

Inject `DATABASE_URL` through deployment secret handling. Do not commit it,
print it, or store it in ordinary configuration. Startup validation and URL
parse errors name the variable without returning its value.

`.env.example` contains safe local defaults. Environment files containing real
credentials must remain outside version control.
