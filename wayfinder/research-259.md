# Wayfinder #259: Railway template mechanics

Scope: three questions from ticket #259. WebFetch used: 8 of 8 cap (5 in earlier pass, 3 in this pass).
Verified = read on a fetched page. Snippet = search result text only, not fetched. Not found = no source located.

## 1. Random secrets in a template (`${{secret()}}`)

**Answer:** Current docs document `secret()` and `randomInt()` as template variable functions. `secret(length?, alphabet?)` generates a random string, 32 characters by default. Hex and Base64 are made by choosing the alphabet and length; the example `${{secret(32, "abcdef0123456789")}}` is given as the equivalent of `openssl rand -hex 16`. `randomInt(min?, max?)` returns an integer, defaults 0 to 100. All template variable functions run when a template is deployed, and the result replaces the `${{ ... }}` in the variable.

- Source: https://docs.railway.com/templates/create
- Status: **Verified** on fetched page (signatures, 32-char default, 0-100 default, deploy-time execution, hex example).
- Not found on that page: default alphabet for `secret()`; whether each reference gets its own value or they share one; whether the generator is shown or reruns on fork or redeploy.
- Snippet only (not fetched): a 2023 changelog (https://railway.com/changelog/2023-07-28-template-variable-functions) describing per-deploy generation; a 2024 changelog saying a variable keeps its generator when an environment is forked (https://railway.com/changelog/2024-01-19-environment-creator-pb.md); Central Station threads reporting the value changes on page reload in the template editor, with a staff reply that the generator is meant for templates, so a deployed service keeps a fixed value (e.g. https://station.railway.com/questions/secret-generates-a-new-string-on-8523cc4d). Treat the reload behaviour as unconfirmed.
- Changed from ticket premise: `secret()` is no longer only in a changelog; it is in the current template docs.

## 2. Cross-service references and PostgreSQL variable names

**Answer:** A PostgreSQL service exposes `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, and `DATABASE_URL`, which other services can reference. Enabling public access adds `DATABASE_PUBLIC_URL` with the external connection string. The contents of `DATABASE_URL` (internal or public host, sslmode) are **not resolved**.

- Source (names): https://docs.railway.com/guides/postgresql
- Status (names): **Verified** on fetched page. The page lists the six variables without describing them.
- Source (`DATABASE_PUBLIC_URL`, databases private by default, `DATABASE_URL` auto-detected by libraries): https://docs.railway.com/databases/postgresql (fetched in earlier pass; verified).
- Not found on any fetched page: what `DATABASE_URL` contains (host, port, sslmode); what `PGHOST` contains; `DATABASE_PRIVATE_URL` (not in current guide).
- Conflicting snippets only (not fetched): docs.railway.app says `DATABASE_URL` is the public-internet string and `DATABASE_PRIVATE_URL` the in-project one; newer community posts say the reverse. Check the Postgres service's Variables tab before relying on either.
- Reference syntax: `${{Service.VAR}}` was already verified by the parent (https://docs.railway.com/variables/reference), so it was not re-fetched. Snippet only: service names are case-sensitive, e.g. `${{Postgres.DATABASE_URL}}`.
- Related, unresolved: TLS (see earlier pass notes in `<scratchpad>/research.md`). Not repeated here.

## 3. Storage bucket CORS from a template

**Answer:** Not documented as possible from a template. The only documented method is the AWS CLI `put-bucket-cors` command against `https://t3.storageapi.dev`, run by the operator after the bucket exists. The current docs give no dashboard instructions for CORS. Neither the template docs nor the bucket docs mention a template or config-as-code route for CORS. Treat it as a manual post-deploy step (CLI or script) until the platform documents otherwise.

- Source (bucket CORS): https://docs.railway.com/storage-buckets/uploading-serving (section "Configuring CORS to upload files from a browser")
- Status: **Verified** on fetched page. Command shape: `aws s3api put-bucket-cors --bucket <name> --endpoint-url https://t3.storageapi.dev --cors-configuration` with `CORSRules` (AllowedHeaders, AllowedMethods PUT/POST, AllowedOrigins, MaxAgeSeconds). Minor: the page uses `your_bucket_name` in the command and `your-bucket_name` in the note.
- Source (templates doc, no CORS or bucket mention): https://docs.railway.com/templates/create. Status: **Verified** on fetched page (absence).
- Snippet only: a Central Station answer says there is no CORS panel in the dashboard; a staff reply in another thread says CORS can be set; an older thread says custom CORS was not fully supported during beta. These conflict, and none was fetched. Dashboard support: **not found** in a current source.
- Template that sets CORS: **not found**. Public-buckets and Boxcar templates surfaced in search, neither documented as setting CORS.
