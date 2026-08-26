# Deployment

Phase 06 runtime deployment is one Go process plus persistent SQLite storage,
or one or more stateless processes using PostgreSQL. Container artifacts land
in Phase 07.

Build and run:

```sh
go build -trimpath -o bin/server ./cmd/server
HTTP_ADDRESS=0.0.0.0:8080 SQLITE_PATH=/var/lib/dynamis_code/app.db ./bin/server
```

Persist `/var/lib/dynamis_code`; never place SQLite on ephemeral or shared network
storage. SQLite permits one application instance. Multiple instances require
PostgreSQL plus external traffic distribution; SSE delivery remains
instance-local until a measured need triggers shared delivery.

Expose port 8080 directly or through external TLS termination. Set
`HTTP_SECURE=true` when clients use HTTPS so HSTS and Secure cookies apply.
Production traffic and PostgreSQL connections must use TLS. Grant the process
write access only to its data and backup paths, run it as a non-root user, and
inject secrets outside the binary and filesystem image.

Use `/health/live` for process health and `/health/ready` for traffic routing.
Send SIGTERM for the documented 10-second graceful drain. Start with 0.25 vCPU
and 256 MiB RAM; measured evidence and alert limits are in
[operations](operations.md). Configure backups before accepting durable data.
