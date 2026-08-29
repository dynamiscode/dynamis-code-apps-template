# Data lifecycle

The application stores ordinary personal data. Workspace access is denied by
default, transport uses TLS in production, and deployment storage and backups
must be encrypted at rest. Database backups contain credential hashes and are
sensitive even though plaintext secrets are never stored.

## Persisted-field catalog

`Export` means the authorized `dynamis-code.workspace/v1` workspace export. `Backup`
means the complete encrypted database backup described in
[operations](operations.md).

| Owner / table | Fields and class | Purpose | Retention, export, correction, deletion |
|---|---|---|---|
| database / `schema_migrations` | `version`, `applied_at` (internal) | Applied schema history | Installation lifetime; backup only; changed only by migrations. |
| identity / `users` | `id`, `created_at` (internal); `email`, `display_name`, nullable `locale`, `timezone`, `theme`, nullable `email_verified_at` (personal); `password_hash` (secret) | Account, local login, profile, and preferences | Account lifetime; workspace exports include member email but not profile or password data; authenticated profile correction is available; deletion removes the account after ownership transfer. Automatic locale is stored as NULL. |
| identity / `external_identities` | `id`, `user_id`, `provider_id`, `created_at` (internal); `issuer`, `subject`, `email` (personal) | Stable OIDC account binding | Account lifetime; excluded from export; linking is explicit. |
| identity / `instance_admins` | `user_id`, `created_at` (internal) | Separate installation administration | Assignment lifetime; excluded from workspace export; bootstrap-managed. |
| identity / `workspaces` | `id`, `created_at` (internal); `name` (personal user content); `locale` (`en`/`es`) | Tenant boundary and browser/email fallback | Workspace lifetime; all fields, including locale, export; workspace deletion/correction is not exposed. |
| identity / `workspace_members` | `workspace_id`, `user_id`, `role`, `created_at` (personal/internal) | Scoped authorization | Membership lifetime; workspace is implied by the export envelope and other fields export; role correction uses authorized membership changes; final-owner protection applies. |
| identity / `sessions` | `id`, `user_id`, `auth_method`, `oidc_provider_id`, timestamps (personal/internal); `secret_hash`, `csrf_hash` (secret) | Browser authentication and revocation | At most 10 active per user; expired sessions and revocations older than 30 days are pruned; excluded from export; users may list/revoke their sessions. |
| identity / `invitations` | IDs, `email`, `role`, timestamps (personal/internal); `secret_hash` (secret) | Bounded membership invitation lifecycle | Active until accepted, revoked, or expired; completed records older than 365 days are pruned; excluded from export; resend rotates the secret. |
| identity / `api_tokens` | IDs, `name`, `scopes`, timestamps (personal/internal); `secret_hash` (secret) | Scoped API/MCP credentials | Active until expiry/revocation; inactive records older than 365 days are pruned; excluded from export; scope changes and revocation are audited. |
| identity / `scim_tokens` | IDs, workspace, creator, timestamps (internal); `secret_hash` (secret) | Dedicated workspace SCIM bearer credentials | One active credential per workspace; secret is shown once; inactive records older than 365 days are pruned; excluded from export; creation, rotation, and revocation are audited. |
| identity / `scim_users`, `scim_groups` | Workspace mappings, external IDs, roles, active/version state, timestamps (internal) | Stable SCIM identity and role-group versions | Workspace lifetime; excluded from export; deactivation retains mappings for safe reactivation; owner mappings never appear in groups. |
| identity / `oidc_transactions` | `provider_id`, `redirect_uri`, timestamps (internal); all hash fields (secret) | Single-use OIDC state, PKCE, nonce, and browser binding | Ten-minute active lifetime; expired records are pruned; excluded from export; no correction. |
| identity / `email_verifications`, `password_resets` | user, email where applicable, timestamps (internal); token hash (secret) | Single-use account recovery and verification | Twenty-four-hour single-use lifetime; expired and consumed records are pruned; excluded from export; account deletion cascades. |
| identity / notification preference tables | user/workspace, notification type, enabled, updated timestamp (personal) | User and workspace in-app delivery preference | Account or membership lifetime; excluded from workspace export; account deletion and member removal clean up. |
| identity / `notifications` | IDs, recipient, optional workspace, type, title, body, timestamps (personal) | In-app notification inbox and SSE delivery | One-year retention; read state is user-correctable; excluded from workspace export; account/workspace deletion cascades. |
| identity / `audit_events` | IDs, types, actions, outcome, timestamps (internal); actor, workspace, request, source address, metadata (personal) | Security and administrative evidence | Append-only during normal operation; default 365-day retention; workspace events export; maintenance deletes expired events and records its own prune event. No product correction. |
| identity / `bootstrap_state` | `id`, `completed_at` (internal) | Enforce one-time first-owner bootstrap | Installation lifetime; backup only; never reset by application behavior. |
| items / `items` | IDs, status, version, timestamps (internal); nullable `created_by_user_id`, `title` (personal user content) | Sample feature | Until permanent deletion or future workspace deletion; all fields export; title/status correction uses conditional update; account deletion retains the item and clears its deleted creator reference. |
| items / `idempotency_records` | hashes, IDs, operation, result, timestamps (internal; hashes treated as secret) | Safe create replay | Exact 24-hour expiry; pruned after expiry; excluded from export; no correction. |
| items / `item_events` | IDs, type, version, time (internal) | SSE replay and resynchronization | Application keeps the newest 1,000 per workspace; maintenance also removes events older than 7 days; excluded from export; no correction. |
| sharing / `public_links` | IDs, workspace/item references, timestamps, expiry, revocation (internal); `token_hash` (secret) | Bounded read-only Item sharing without membership | Until expiry or revocation; expired links are pruned by maintenance and revoked links after 365 days; excluded from export; item/workspace deletion cascades; no correction. |
| webhooks / `webhooks` | IDs, workspace, name, endpoint URL, selected event names, timestamps (internal/configuration); encrypted secret (secret) | Workspace event delivery registration | Workspace lifetime or explicit deletion; excluded from export; secret rotates through authorized management and is never returned after creation. |
| webhooks / `webhook_deliveries` | IDs, webhook/event references, event type, bounded payload, attempt/status/timestamps, HTTP status, redacted error category (internal; payload personal) | Durable at-least-once item delivery and bounded delivery history | Pending rows remain until delivery settles; delivered/failed rows retained 365 days and pruned by maintenance; excluded from export; cascade on webhook deletion. |
| files / `files` | IDs, workspace/owner, object key, original name, detected MIME, size, SHA-256, status, timestamps (personal user content/internal) | Workspace-scoped private file metadata and object reconciliation | Workspace lifetime; metadata is included in backup but excluded from workspace export; owner deletion sets owner NULL; live object deletion/reconciliation is not exposed in this slice. Pending/failed rows remain quota-reserved for bounded operator reconciliation. |
| platform / `background_jobs` | IDs, workspace, handler kind, deduplication key, bounded payload, status/attempt/lease/timestamps, redacted error category (internal; payload personal) | Durable retry and lease ownership for bounded asynchronous handlers | Pending and leased rows remain until settlement; settled rows retained 365 days and pruned by maintenance; excluded from export; workspace deletion cascades. |

Every row is included in database backup until the operator's backup retention
expires. Production fixtures must never contain copied production data.

Workspace owners and admins receive their workspace audit history through the
authorized export. No public instance-wide audit endpoint exists. Only the
deployment operator may inspect instance events through restricted database
access. Normal application code inserts audit rows but never updates or
deletes them; the retention command is the sole deletion path.

Files are standalone workspace resources, not generic attachments. Local bytes
live under the configured storage path; S3 bytes live in the configured private
bucket/prefix. S3 URLs expire according to `STORAGE_SIGNED_URL_TTL`. The initial
allowlist rejects executable, HTML, SVG, archive, and mismatched
extension/signature content. Background scanning, orphan reconciliation,
durable deletion, and workspace deletion require the deferred Background Jobs
trigger; this slice leaves bounded metadata hooks and adds no worker.

## Resource and identity deletion

Items support `active` and `complete`; they do not support archive,
soft-delete, or product restore. Authorized `DELETE` requires the current ETag,
permanently removes the live row, emits a deletion event, and records an audit
event. The identifier is not reused. Recovery from backup is an operator
disaster-recovery action, not an item restore feature.

Account deletion is available after local-password reauthentication and is
refused for users who still own a workspace. Ownership transfer is required
first; deletion then cascades account credentials, memberships, external
identities, preferences, and notifications, and removes invitations created by
the account. The deletion audit event retains the user ID and safe metadata but
no credential or profile content. Items created by the account remain in their
workspace with `createdByUserId: null`. Workspace deletion remains unavailable.

Public links are not workspace memberships. A valid link returns only the Item
title and status; workspace names, emails, creator details, audit history,
internal IDs, and bearer tokens are excluded. Link creation and revocation
require `resources:write`; access outcomes are audited without the bearer
token. Links expire after seven days by default, at most 30 days, and never
have an unlimited lifetime. Item deletion invalidates links through the
foreign-key cascade.

## Portability

`GET /api/v1/workspaces/{workspaceId}/export` requires `workspace:export`,
checks workspace membership, and returns one synchronous JSON attachment. The
format includes version, export time, workspace (including its locale), members, items, audit events,
and an explicit exclusion list. Defaults cap it at 1,000 records and 4 MiB;
over-limit exports fail with `409 export-limit` and are audited. No server-side
download copy remains.

Import supports only item records through `POST /api/v1/workspaces/{workspaceId}/import`.
Authorized owners and admins with `workspace:update` may submit the versioned
workspace JSON export or strict UTF-8 `title,status` CSV. Source IDs,
timestamps, memberships, audit events, credentials, and unknown JSON fields are
ignored or rejected; imported items receive new IDs, the current actor as
creator, and current UTC timestamps. The configured import record and byte
limits apply, and the whole batch is validated and committed in one
transaction. Any validation, quota, or storage error rolls the batch back.
Successful and authorized rejected imports append workspace audit events;
invalid or over-limit input has a safe error without echoing file contents.
Membership, identity, and arbitrary-domain imports remain unsupported.
Deletion removes imported items through the normal permanent item deletion
path; database backups retain them until backup expiry. Import does not create
a restore or undelete workflow.
Import is intentionally REST-only; browser, remote CLI, MCP, and WebMCP
surfaces omit it because a bulk mutation needs explicit file selection and
operator review.

The workspace object in `dynamis-code.workspace/v1` includes its `locale` as
an additive field. Existing readers that ignore unknown fields remain
compatible; new exports always include `en` or `es`.

The workspace object in `dynamis-code.workspace/v1` includes its `locale` as
an additive field. Existing readers that ignore unknown fields remain
compatible; new exports always include `en` or `es`.

WebMCP may prepare the authorized browser export link under the Settings route
but never returns export
content, credentials, or secret data to a browser agent. It does not expose
operator backup, restore, import, maintenance, or audit administration. User
activation follows the normal authorization and audit path; redacted export
content remains bounded by the workspace export contract.

Webhook registrations and delivery history are not part of workspace export;
endpoint URLs and delivery payloads can contain integration or personal data,
and encrypted secrets are deployment-sensitive. Database backups containing
these rows remain sensitive.

Webhook delivery survives request disconnection through the background job
queue. No public long-running-operation resource, cancellation contract, or
browser/REST job administration exists; add those only when a product action
needs user-visible progress or cancellation beyond the bounded handler model.
