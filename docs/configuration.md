# Configuration

Configuration is deployment-owned and loaded from environment
variables at startup. Invalid values stop startup before application work
begins. Changes require restart.

| Variable | Type | Default | Required | Secret |
|---|---|---|---|---|
| `DATABASE_DRIVER` | `sqlite` or `postgres` | `sqlite` | No | No |
| `SQLITE_PATH` | non-empty path | `data/app.db` | SQLite only | No |
| `DATABASE_URL` | PostgreSQL URL | none | PostgreSQL only | Yes |
| `DATABASE_MAX_OPEN_CONNS` | integer 1-64 | `4` | No | No |
| `DATABASE_MAX_IDLE_CONNS` | integer 1-64, not above open limit | `2` | No | No |
| `BOOTSTRAP_ADMIN_EMAIL` | email | none | With other admin variables | No |
| `BOOTSTRAP_ADMIN_WORKSPACE` | workspace name, 1-120 characters | none | With other admin variables | No |
| `BOOTSTRAP_ADMIN_PASSWORD` | password, 12-1024 characters | none | With other admin variables | Yes |
| `BOOTSTRAP_SETUP_TOKEN` | non-empty setup secret | none | No | Yes |
| `OIDC_ENABLED` | boolean | `false` | No | No |
| `OIDC_PROVIDER_ID` | lowercase stable identifier | none | OIDC only | No |
| `OIDC_PROVIDER_NAME` | display label, 1-80 characters | none | OIDC only | No |
| `OIDC_ISSUER_URL` | public HTTPS issuer URL | none | OIDC only | No |
| `OIDC_CLIENT_ID` | provider client ID | none | OIDC only | No |
| `OIDC_CLIENT_SECRET` | provider client secret | none | OIDC only | Yes |
| `OIDC_REDIRECT_URL` | exact HTTPS or loopback-HTTP callback | none | OIDC only | No |
| `APP_PUBLIC_URL` | HTTPS or loopback HTTP base URL | none | SMTP only | No |
| `SMTP_HOST` | SMTP hostname | none | Optional invitation email | No |
| `SMTP_PORT` | integer 1-65535 | `587` | SMTP only | No |
| `SMTP_USERNAME` | SMTP username | none | With SMTP password | No |
| `SMTP_PASSWORD` | SMTP password | none | With SMTP username | Yes |
| `SMTP_FROM` | sender address | none | SMTP only | No |
| `HTTP_ADDRESS` | listen address | `127.0.0.1:8080` | No | No |
| `HTTP_SECURE` | boolean | `false` | No | No |
| `HTTP_READ_HEADER_TIMEOUT` | duration | `10s` | No | No |
| `HTTP_REQUEST_TIMEOUT` | duration | `30s` | No | No |
| `HTTP_SHUTDOWN_TIMEOUT` | duration | `10s` | No | No |
| `HTTP_READINESS_TIMEOUT` | duration | `2s` | No | No |
| `HTTP_MAX_HEADER_BYTES` | bytes, 8192-1048576 | `32768` | No | No |
| `HTTP_MAX_BODY_BYTES` | bytes, 1024-16777216 | `1048576` | No | No |
| `HTTP_DEFAULT_PAGE_SIZE` | integer, within max | `50` | No | No |
| `HTTP_MAX_PAGE_SIZE` | integer 1-100 | `100` | No | No |
| `HTTP_REQUESTS_PER_MINUTE` | integer 1-10000 | `120` | No | No |
| `HTTP_AUTH_REQUESTS_PER_MINUTE` | integer, within ordinary limit | `10` | No | No |
| `HTTP_SSE_POLL_INTERVAL` | duration, 100ms-30s | `1s` | No | No |
| `HTTP_SSE_HEARTBEAT_INTERVAL` | duration, 1s-1m | `15s` | No | No |
| `HTTP_SSE_MAX_LIFETIME` | duration, 1m-1h | `5m` | No | No |
| `HTTP_SSE_MAX_CONNECTIONS` | integer 1-10000 | `100` | No | No |
| `HTTP_SSE_MAX_CONNECTIONS_PER_USER` | integer, within instance limit | `5` | No | No |
| `HTTP_MAX_CONCURRENT_REQUESTS` | integer 1-10000 | `100` | No | No |
| `MCP_ALLOWED_ORIGINS` | comma-separated exact HTTP origins | none | Browser MCP only | No |
| `ITEMS_MAX_PER_WORKSPACE` | integer 1-1000000 | `10000` | No | No |
| `EXPORT_MAX_RECORDS` | integer 1-10000 | `1000` | No | No |
| `EXPORT_MAX_BYTES` | bytes 65536-4194304 | `4194304` | No | No |
| `AUDIT_RETENTION` | duration 30 days-10 years | `8760h` | No | No |
| `OTEL_SERVICE_NAME` | string 1-255 | `dynamis-code-apps-template` | No | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | HTTPS or loopback HTTP base URL | none | No | No |
| `OTEL_EXPORTER_OTLP_HEADERS` | OTLP header list | none | Authenticated exporter only | Yes |
| `OTEL_METRIC_EXPORT_INTERVAL` | duration 5s-5m | `30s` | No | No |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | duration 1s-1m | `10s` | No | No |

SQLite always uses one open and one idle connection regardless of pool
variables because one instance owns the file.

The three `BOOTSTRAP_ADMIN_*` variables enable unattended first-owner
bootstrap only when all are non-empty. A partial set fails startup; an empty
set leaves browser or CLI bootstrap available. `BOOTSTRAP_SETUP_TOKEN` enables
the protected browser form for an unbootstrapped database. After bootstrap,
these values do not overwrite database records.

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

## OIDC

OIDC needs no provider variables while disabled. When enabled, every provider
variable above is required. Startup validates the values, performs discovery,
and fails without returning the client secret when configuration or discovery
is invalid. Provider changes require deployment restart.

Discovery and token requests reject non-public destinations. Use an accepted
exception before enabling an identity provider on a private network.

## Invitation email

SMTP is disabled when `SMTP_HOST` is empty. Any partial SMTP configuration fails
startup. Enabling it requires `APP_PUBLIC_URL`, `SMTP_FROM`, and matching
username/password values; port defaults to 587. Delivery negotiates STARTTLS
before optional authentication. Credentials are not logged or placed in
invitation URLs. Invitation rows commit before delivery, so a mail failure
leaves the invitation valid and the user receives its copyable link.

## HTTP

Set `HTTP_SECURE=true` only when clients reach the application over HTTPS; it
enables secure cookies and HSTS. A reverse proxy may terminate TLS, but must
preserve the configured limits. Rate limits are per source address and reset
on process restart.

SSE connection limits are per application instance. Event replay state is in
the database; multi-instance deployment therefore requires PostgreSQL but no
broker for this bounded sample stream.

Requests to `/mcp` that include `Origin` are accepted only when the exact
origin appears in `MCP_ALLOWED_ORIGINS`. Non-browser clients omit `Origin`.

OTLP exporter authentication headers use the standard
`OTEL_EXPORTER_OTLP_HEADERS` deployment secret. The endpoint is a base URL;
the application appends `/v1/traces` and `/v1/metrics`.
