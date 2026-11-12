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
| `MULTI_WORKSPACE_ENABLED` | boolean; `false` is single workspace mode, `true` is multi | `false` | No | No |
| `BOOTSTRAP_ADMIN_EMAIL` | email | none | With other admin variables | No |
| `BOOTSTRAP_ADMIN_WORKSPACE` | workspace name, 1-120 characters | none | With other admin variables | No |
| `BOOTSTRAP_ADMIN_PASSWORD` | password, 12-1024 characters | none | With other admin variables | Yes |
| `BOOTSTRAP_ADMIN_WORKSPACE_LOCALE` | `en` or `es` | `en` | No | No |
| `BOOTSTRAP_SETUP_TOKEN` | non-empty setup secret for remote browser setup | none | No | Yes |
| `OIDC_ENABLED` | boolean | `false` | No | No |
| `OIDC_PROVIDER_ID` | lowercase stable identifier | none | OIDC only | No |
| `OIDC_PROVIDER_NAME` | display label, 1-80 characters | none | OIDC only | No |
| `OIDC_ISSUER_URL` | public HTTPS issuer URL | none | OIDC only | No |
| `OIDC_CLIENT_ID` | provider client ID | none | OIDC only | No |
| `OIDC_CLIENT_SECRET` | provider client secret | none | OIDC only | Yes |
| `OIDC_REDIRECT_URL` | exact HTTPS or loopback-HTTP callback | none | OIDC only | No |
| `MFA_ENABLED` | boolean | `false` | No | No |
| `MFA_ENCRYPTION_KEY` | exactly 32 random bytes as hex or base64 | none | MFA only | Yes |
| `MFA_REQUIRE_FOR_ADMINS` | boolean | `false` | No | No |
| `WEBAUTHN_RP_ID` | relying-party domain | `localhost` | MFA only | No |
| `WEBAUTHN_RP_ORIGIN` | exact HTTPS or loopback-HTTP origin | `http://localhost:8080` | MFA only | No |
| `WEBAUTHN_RP_DISPLAY_NAME` | display name, 1-80 characters | `Dynamis Code` | MFA only | No |
| `APP_PUBLIC_URL` | HTTPS or loopback HTTP base URL | none | SMTP or Cloudflare Email only | No |
| `SMTP_HOST` | SMTP hostname | none | Optional email delivery | No |
| `SMTP_PORT` | integer 1-65535 | `587` | SMTP only | No |
| `SMTP_USERNAME` | SMTP username | none | With SMTP password | No |
| `SMTP_PASSWORD` | SMTP password | none | With SMTP username | Yes |
| `SMTP_FROM` | sender address | none | SMTP only | No |
| `CLOUDFLARE_EMAIL_ACCOUNT_ID` | Cloudflare account ID | none | Optional email delivery | No |
| `CLOUDFLARE_EMAIL_API_TOKEN` | API token with Email Sending permission | none | With Cloudflare account ID | Yes |
| `CLOUDFLARE_EMAIL_FROM` | verified sender address | none | Cloudflare Email only | No |
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
| `MCP_OAUTH_ENABLED` | boolean | `false` | Requires origin-only `APP_PUBLIC_URL`; conflicts with `MCP_OAUTH_AUTHORIZATION_SERVER` | No |
| `MCP_OAUTH_AUTHORIZATION_SERVER` | HTTPS or loopback HTTP issuer URL without credentials, query, or fragment | none (discovery off) | Requires origin-only `APP_PUBLIC_URL` | No |
| `ITEMS_MAX_PER_WORKSPACE` | integer 1-1000000 | `10000` | No | No |
| `EXPORT_MAX_RECORDS` | integer 1-10000 | `1000` | No | No |
| `EXPORT_MAX_BYTES` | bytes 65536-4194304 | `4194304` | No | No |
| `IMPORT_MAX_RECORDS` | integer 1-10000 | `1000` | No | No |
| `IMPORT_MAX_BYTES` | bytes 65536-4194304 | `4194304` | No | No |
| `AUDIT_RETENTION` | duration 30 days-10 years | `8760h` | No | No |
| `STORAGE_DRIVER` | `local` or `s3` | `local` | No | No |
| `STORAGE_LOCAL_PATH` | non-empty path | `data/files` | Local only | No |
| `STORAGE_S3_ENDPOINT` | HTTPS or loopback HTTP URL | none | No | No |
| `STORAGE_S3_REGION` | non-empty region | `us-east-1` | S3 only | No |
| `STORAGE_S3_BUCKET` | bucket name | none | S3 only | No |
| `STORAGE_S3_PREFIX` | safe object prefix | none | No | No |
| `STORAGE_S3_FORCE_PATH_STYLE` | boolean | `false` | No | No |
| `STORAGE_MAX_OBJECT_BYTES` | bytes 1-1073741824 | `16777216` | No | No |
| `STORAGE_MAX_WORKSPACE_BYTES` | bytes, at least object limit | `1073741824` | No | No |
| `STORAGE_SIGNED_URL_TTL` | duration 1m-15m | `5m` | No | No |
| `ENCRYPTION_KEY` | exactly 32 random bytes as hex or base64 | none | Webhook create/rotation (primary general key) | Yes |
| `WEBHOOK_ENCRYPTION_KEY` | exactly 32 random bytes as hex or base64 | none | Webhook fallback when `ENCRYPTION_KEY` is unset; still opens webhook secrets sealed before `ENCRYPTION_KEY` was set | Yes |
| `OTEL_SERVICE_NAME` | string 1-255 | `dynamis-code-apps-template` | No | No |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | HTTPS or loopback HTTP base URL | none | No | No |
| `OTEL_EXPORTER_OTLP_HEADERS` | OTLP header list | none | Authenticated exporter only | Yes |
| `OTEL_METRIC_EXPORT_INTERVAL` | duration 5s-5m | `30s` | No | No |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | duration 1s-1m | `10s` | No | No |

SQLite always uses one open and one idle connection regardless of pool
variables because one instance owns the file.

`MULTI_WORKSPACE_ENABLED` is parsed at startup and stored as the instance
workspace mode. It defaults to `false`, which selects single workspace mode;
`true` selects multi workspace mode. In single mode the browser root (`/`)
redirects a signed-in user to the home of their oldest workspace membership,
hides the workspace creation form, and `POST /workspaces` returns 404; the
first workspace comes from `cmd/bootstrap-admin`. In multi mode `/` lists
memberships and creates workspaces. The default change applies to existing
deployments: those that create workspaces in the browser must set
`MULTI_WORKSPACE_ENABLED=true` before upgrading.

The three `BOOTSTRAP_ADMIN_*` variables enable unattended first-owner
bootstrap only when all are non-empty. A partial set fails startup; an empty
set leaves loopback browser or CLI bootstrap available. `BOOTSTRAP_SETUP_TOKEN`
enables the protected browser form for remote requests; local loopback browser
setup does not need it. After bootstrap, these values do not overwrite database
records. The workspace locale controls invitation email language and browser
fallback.

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

## MFA and passkeys

MFA is disabled by default. Enable it with `MFA_ENABLED=true` and inject a
deployment-owned 32-byte `MFA_ENCRYPTION_KEY`. TOTP secrets are encrypted at
rest and recovery codes are stored only as hashes. Passkeys use the configured
RP ID, exact origin, and display name. Defaults are safe for local loopback
development; production deployments must set HTTPS RP values.
The RP ID must match the origin host or be a domain suffix of it.

`MFA_REQUIRE_FOR_ADMINS=true` requires an enrolled factor for owners and
administrators. Members and viewers remain optional. Factor changes require
fresh authentication; passkey removal revokes active sessions. MFA challenge
and factor data are never logged or written to audit metadata.

## Invitation email

Email is disabled when neither `SMTP_HOST` nor `CLOUDFLARE_EMAIL_ACCOUNT_ID` is
set. Any partial configuration fails startup, and configuring both providers
fails startup.

SMTP requires `APP_PUBLIC_URL`, `SMTP_FROM`, and matching username/password
values; port defaults to 587. Delivery negotiates STARTTLS before optional
authentication.

Workspace administrators can set their own invitation sender on the Email
settings page (`/workspaces/{workspaceId}/settings/email`), either SMTP or
Resend. A workspace with saved settings sends its invitations through them;
workspaces without them use the instance provider above. Saving runs a provider
check first and stores nothing if it fails: an SMTP login test, or for Resend a
read-only `GET /domains` call that sends nothing. The Resend key needs Full access
so the check can read domains. The workspace password or Resend key is sealed
with `ENCRYPTION_KEY`, so that key must be set before it can be saved, and it is
never shown again. Account email (verification and password reset) always uses
only the instance provider.

Cloudflare Email Sending requires `APP_PUBLIC_URL`, `CLOUDFLARE_EMAIL_ACCOUNT_ID`,
`CLOUDFLARE_EMAIL_API_TOKEN`, and `CLOUDFLARE_EMAIL_FROM`. The sender domain
must be verified in Cloudflare before any message is delivered. Delivery calls
the Email Sending REST API with a bearer token and times out after 10 seconds.
Provider error bodies are not logged or returned.

Credentials are not logged or placed in invitation URLs. Invitation rows commit
before delivery, so a mail failure leaves the invitation valid and the user
receives its copyable link.

## Webhooks

Webhook registration is available without a key, but creation and rotation
fail safely until `ENCRYPTION_KEY` (or `WEBHOOK_ENCRYPTION_KEY` as a fallback)
is configured. New secrets are sealed with `ENCRYPTION_KEY` and record key
version 2. Secrets sealed with `WEBHOOK_ENCRYPTION_KEY` record version 1 and
keep decrypting while that variable is set, so keep it configured after
switching until no version 1 rows remain. Keep keys in the deployment secret
store and back them up separately from the database; losing a key makes its
secrets undecryptable. The app starts without either key; only webhook secret
saving is disabled. Delivery accepts HTTPS endpoints
and local loopback HTTP for development, and rejects private resolved addresses.

## Files

The optional Files profile stores metadata in SQLite or PostgreSQL and object
bytes in the local filesystem by default. S3 mode uses the AWS SDK standard
credential chain and one S3-compatible endpoint configuration for AWS S3, R2,
MinIO, Wasabi, Backblaze, and similar services. Credentials are never logged.
Objects remain private; S3 clients receive short-lived presigned PUT/GET URLs,
while local objects stream through the application. The object and workspace
limits default to 16 MiB and 1 GiB. File routes use their separate object
limit; do not raise `HTTP_MAX_BODY_BYTES` globally. Browser S3 uploads also
require bucket CORS permitting PUT from the application origin and the signed
upload headers returned by the initiation response. For a Cloudflare R2 setup
walkthrough, including the CORS rule and the conformance check, see
[deployment](deployment.md#cloudflare-r2-object-storage).

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

`MCP_OAUTH_AUTHORIZATION_SERVER` enables the optional RFC 9728 metadata and
`WWW-Authenticate` hints described in [MCP](mcp.md#protected-resource-discovery).
It does not accept OAuth access tokens; bearer API tokens remain the only
`/mcp` credentials.

OTLP exporter authentication headers use the standard
`OTEL_EXPORTER_OTLP_HEADERS` deployment secret. The endpoint is a base URL;
the application appends `/v1/traces` and `/v1/metrics`.
