# Operations

## Health, telemetry, and limits

`/health/live` checks only process life. `/health/ready` performs a database
ping with a 2-second default timeout. Shutdown stops accepting traffic, drains
HTTP work for up to 10 seconds, flushes telemetry, and closes the database.

Logs are structured and correlate `request_id` with the W3C `trace_id`.
Configure an optional OTLP/HTTP base endpoint with
`OTEL_EXPORTER_OTLP_ENDPOINT`; the application sends to `/v1/traces` and
`/v1/metrics`. Empty endpoint keeps instrumentation active without a backend.
Telemetry records no paths, query values, bodies, credentials, or user data.

Metrics:

- `http.server.request.count` and `http.server.request.duration` by method and status;
- `http.client.request.count` and `http.client.request.duration` by method and status;
- `auth.failure.count` for HTTP 401 responses;
- `database.health.check.count` and `.duration` by health result;
- `realtime.stream.active` and `realtime.stream.rejected.count`;
- `resource.limit.rejected.count` by bounded resource type.

Default limits are 100 concurrent ordinary requests, 120 requests per source
per minute, 10 authentication attempts per source per minute, 1 MiB request
bodies, 100 SSE streams per instance and 5 per user, 10 active sessions per
user, 10,000 items per workspace, 1,000 export/import records, and 4 MiB per
export/import payload (the HTTP body limit may be lower).
SQLite uses one database connection; PostgreSQL defaults to four open and two
idle. OIDC calls use a 10-second client timeout. Limits return bounded 409 or
429 errors with remediation or `Retry-After` as applicable.

The current application has no background delivery or long-running product
operations, so it has no worker metric or operation queue.

Run retention at least daily:

```sh
go run ./cmd/maintain
```

It removes expired transient records, old inactive credentials, expired audit
history, and stale realtime replay in one transaction, then appends a safe
summary audit event. `AUDIT_RETENTION` defaults to 365 days.

## Webhook delivery

Webhook delivery uses a database-backed outbox written with each item mutation.
The application runs one bounded in-process delivery loop per instance; it
polls pending rows, sends signed requests with a 10-second timeout, and retries
up to five attempts with 1s, 2s, 4s, and 8s delays. A 2xx response settles a
delivery; other responses and network failures are recorded as redacted
categories. Delivery history is available through the workspace REST endpoint
and is retained for 365 days after settlement. Consumers must deduplicate by
`Webhook-Id`.

The loop is intentionally not a shared queue. Run PostgreSQL before multiple
application instances and replace the loop with a shared job ownership model
when delivery volume or replica count requires it.

## Backup and restore

Recovery owner: deployment operator. Production targets: hourly backups,
RPO 1 hour, RTO 4 hours, hourly copies retained 24 hours, daily copies retained
30 days, and an isolated restore verification every 30 days. Alert when the
newest successful backup exceeds 1 hour or restore verification exceeds 30
days. Protect backup files and manifests as sensitive data.

SQLite uses `VACUUM INTO` for a consistent snapshot:

```sh
DATABASE_DRIVER=sqlite SQLITE_PATH=data/app.db \
  go run ./cmd/dbtool backup -file backups/app.db
DATABASE_DRIVER=sqlite SQLITE_PATH=data/app.db \
  go run ./cmd/dbtool restore -file backups/app.db \
  -target data/restored.db -max-age 25h
```

Restore refuses an existing target. Verify the restored file, stop the
application, replace the configured database through a recoverable operator
move, then start and check readiness and smoke behavior.

PostgreSQL uses `pg_dump` custom format and `pg_restore`; both tools must match
the server's supported major version. Credentials enter child processes only
through `PG*` environment values, never arguments:

```sh
DATABASE_DRIVER=postgres DATABASE_URL="$DATABASE_URL" \
  go run ./cmd/dbtool backup -file backups/app.dump
DATABASE_DRIVER=postgres DATABASE_URL="$EMPTY_RESTORE_DATABASE_URL" \
  go run ./cmd/dbtool restore -file backups/app.dump -max-age 25h
```

PostgreSQL restore refuses a non-empty public schema. Both paths verify the
SHA-256 manifest, reject corrupt or over-age evidence, restore into isolation,
and verify migration history. A backup is valid only after the isolated
restore test passes.

Database backups do not contain plaintext application secrets. Deployment
secrets (`DATABASE_URL`, OIDC client secret, OTLP headers, TLS keys) remain in
the deployment secret store. Back up that store separately, test recovery,
rotate access, and treat loss of an unrecoverable encryption root as permanent
loss of encrypted data.

## Upgrades and alerts

Upgrades are stop-the-world: create and verify a backup, stop the one SQLite
instance or all PostgreSQL application instances, start one new version to run
forward-only migrations, then check readiness and smoke paths before restoring
traffic. PostgreSQL migrations take a transaction advisory lock. All migration
steps and history writes share one transaction; interruption rolls them back.
Binary rollback is safe only when the old binary accepts the new schema.
Otherwise restore the pre-upgrade backup. Future breaking schema work must use
expand-contract or publish its explicit outage and recovery procedure.

Measured 2026-08-25 on Apple M1 Pro, macOS 26.5, Go 1.27.0, SQLite: idle RSS
33.9 MiB and 0% CPU. A 5,000-request readiness run at concurrency 16 completed
in 0.749 seconds with zero failures (6,673 requests/second, 2.398 ms mean),
observed 61.5 MiB RSS and 63.1% of one CPU. SQLite used one connection and the
HTTP-only load used zero streams.

Alert targets:

- readiness failure for 2 consecutive checks or 1 minute;
- 5xx rate above 1% or p95 request latency above 500 ms for 5 minutes;
- RSS above 205 MiB, CPU above 80% of one core, or database pool saturation for 5 minutes;
- active streams above 80, any sustained stream/request/resource rejection, or authentication failures above 20 per minute for 5 minutes;
- backup/restore ages above the targets, certificate expiry below 14 days, or failed migration/security controls immediately.

## Troubleshooting

- Readiness `503`: inspect safe structured logs, validate configuration, and
  verify database reachability before routing traffic. Liveness may remain
  healthy while the required database is unavailable.
- SQLite lock or durability errors: confirm only one application instance uses
  the file and that `/data` is persistent local storage with UID/GID 65532
  write access.
- Migration failure: stop rollout, preserve logs, and verify the current
  database and backup before retrying. Restore the pre-upgrade backup when the
  old binary cannot read the new schema.
- Rejected requests or streams: compare the machine-readable error and
  `Retry-After` with configured request, rate, session, item, export, and SSE
  limits before raising capacity.
