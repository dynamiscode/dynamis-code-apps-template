# Deployment

## Railway one-click deploy

Two Railway templates deploy the app with no required inputs. The README has
a "Deploy on Railway" button for each.

- **Minimal:** the app plus one volume mounted at `/data`. SQLite and local
  file storage live on the volume. Run one instance only.
- **Full:** the app, a PostgreSQL service, and a Railway bucket. `DATABASE_URL`
  is referenced from the PostgreSQL service. The app reads the bucket's
  S3-compatible values with `STORAGE_DRIVER=s3`. Uploads stream through the
  application, so the bucket needs no CORS rule for uploads.

Both templates generate `BOOTSTRAP_SETUP_TOKEN`. After the deploy finishes,
open the generated public domain at `/setup`, paste the token from the app
service's Variables tab, and create the first admin. Back up the PostgreSQL
service and the bucket. The templates do not enforce backups.

## SQLite: one instance

```sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE='My Workspace'
export BOOTSTRAP_ADMIN_WORKSPACE_LOCALE=en
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
  -email owner@example.com -workspace 'My Workspace' -workspace-locale en
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

## Cloudflare R2 object storage

Use R2 only with the optional Files profile. Metadata stays in the database and
object bytes go to one private bucket. Variables are described in
[configuration](configuration.md#files).

1. Create a bucket and leave public access off. Do not attach an `r2.dev` URL
   or a custom public domain to it.
2. Create an R2 API token limited to that bucket with object read and write
   permission. Store its access key ID and secret access key in the deployment
   secret store.
3. Set these variables at runtime. Do not bake them into the image:

   ```sh
   STORAGE_DRIVER=s3
   STORAGE_S3_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com
   STORAGE_S3_REGION=auto
   STORAGE_S3_BUCKET=<bucket-name>
   AWS_ACCESS_KEY_ID=<token-access-key-id>
   AWS_SECRET_ACCESS_KEY=<token-secret-access-key>
   ```

4. Verify the endpoint from a source checkout with a dedicated test bucket. The
   suite writes and deletes objects under a random `conformance-` prefix. Enter
   the secret without putting it in shell history:

   ```sh
   read -s AWS_SECRET_ACCESS_KEY; export AWS_SECRET_ACCESS_KEY
   export AWS_ACCESS_KEY_ID=<token-access-key-id>
   S3_CONFORMANCE_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com \
   S3_CONFORMANCE_BUCKET=<test-bucket-name> S3_CONFORMANCE_REGION=auto \
     go test ./internal/files/ -run TestS3ConformanceConfiguredEndpoint -count=1 -v
   unset AWS_SECRET_ACCESS_KEY
   ```

   `S3_CONFORMANCE_FORCE_PATH_STYLE=true` selects path-style addressing for
   endpoints that need it. The test is skipped unless the endpoint, bucket, and
   `AWS_ACCESS_KEY_ID` are all set. Record the result as a dated section in the
   object-storage conformance evidence in [capabilities](capabilities.md).

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
