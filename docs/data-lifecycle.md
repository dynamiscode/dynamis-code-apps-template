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
| identity / `users` | `id`, `created_at` (internal); `email`, nullable `locale` preference (personal); `password_hash` (secret) | Account, local login, and browser language preference | Account lifetime; locale preference and member email exports, password hash never exports; email/locale correction and account deletion need a future authenticated workflow. Automatic locale is stored as NULL. |
| identity / `external_identities` | `id`, `user_id`, `provider_id`, `created_at` (internal); `issuer`, `subject`, `email` (personal) | Stable OIDC account binding | Account lifetime; excluded from export; linking is explicit; account deletion is not exposed. |
| identity / `instance_admins` | `user_id`, `created_at` (internal) | Separate installation administration | Assignment lifetime; excluded from workspace export; bootstrap-managed. |
| identity / `workspaces` | `id`, `created_at` (internal); `name` (personal user content); `locale` (`en`/`es`) | Tenant boundary and browser/email fallback | Workspace lifetime; all fields, including locale, export; workspace deletion/correction is not exposed. |
| identity / `workspace_members` | `workspace_id`, `user_id`, `role`, `created_at` (personal/internal) | Scoped authorization | Membership lifetime; workspace is implied by the export envelope and other fields export; role correction uses authorized membership changes; final-owner protection applies. |
| identity / `sessions` | `id`, `user_id`, `auth_method`, `oidc_provider_id`, timestamps (personal/internal); `secret_hash`, `csrf_hash` (secret) | Browser authentication and revocation | At most 10 active per user; expired sessions and revocations older than 30 days are pruned; excluded from export; users may list/revoke their sessions. |
| identity / `invitations` | IDs, `email`, `role`, timestamps (personal/internal); `secret_hash` (secret) | Bounded membership invitation lifecycle | Active until accepted, revoked, or expired; completed records older than 365 days are pruned; excluded from export; resend rotates the secret. |
| identity / `api_tokens` | IDs, `name`, `scopes`, timestamps (personal/internal); `secret_hash` (secret) | Scoped API/MCP credentials | Active until expiry/revocation; inactive records older than 365 days are pruned; excluded from export; scope changes and revocation are audited. |
| identity / `oidc_transactions` | `provider_id`, `redirect_uri`, timestamps (internal); all hash fields (secret) | Single-use OIDC state, PKCE, nonce, and browser binding | Ten-minute active lifetime; expired records are pruned; excluded from export; no correction. |
| identity / `audit_events` | IDs, types, actions, outcome, timestamps (internal); actor, workspace, request, source address, metadata (personal) | Security and administrative evidence | Append-only during normal operation; default 365-day retention; workspace events export; maintenance deletes expired events and records its own prune event. No product correction. |
| identity / `bootstrap_state` | `id`, `completed_at` (internal) | Enforce one-time first-owner bootstrap | Installation lifetime; backup only; never reset by application behavior. |
| items / `items` | IDs, status, version, timestamps (internal); `title` (personal user content) | Sample feature | Until permanent deletion or future workspace deletion; all fields export; title/status correction uses conditional update. |
| items / `idempotency_records` | hashes, IDs, operation, result, timestamps (internal; hashes treated as secret) | Safe create replay | Exact 24-hour expiry; pruned after expiry; excluded from export; no correction. |
| items / `item_events` | IDs, type, version, time (internal) | SSE replay and resynchronization | Application keeps the newest 1,000 per workspace; maintenance also removes events older than 7 days; excluded from export; no correction. |
| webhooks / `webhooks` | IDs, workspace, name, endpoint URL, selected event names, timestamps (internal/configuration); encrypted secret (secret) | Workspace event delivery registration | Workspace lifetime or explicit deletion; excluded from export; secret rotates through authorized management and is never returned after creation. |
| webhooks / `webhook_deliveries` | IDs, webhook/event references, event type, bounded payload, attempt/status/timestamps, HTTP status, redacted error category (internal; payload personal) | Durable at-least-once item delivery and bounded delivery history | Pending rows remain until delivery settles; delivered/failed rows retained 365 days and pruned by maintenance; excluded from export; cascade on webhook deletion. |

Every row is included in database backup until the operator's backup retention
expires. Production fixtures must never contain copied production data.

Workspace owners and admins receive their workspace audit history through the
authorized export. No public instance-wide audit endpoint exists. Only the
deployment operator may inspect instance events through restricted database
access. Normal application code inserts audit rows but never updates or
deletes them; the retention command is the sole deletion path.

## Resource and identity deletion

Items support `active` and `complete`; they do not support archive,
soft-delete, or product restore. Authorized `DELETE` requires the current ETag,
permanently removes the live row, emits a deletion event, and records an audit
event. The identifier is not reused. Recovery from backup is an operator
disaster-recovery action, not an item restore feature.

Account and workspace deletion are intentionally unavailable. Although
foreign keys define cascades, a product workflow still needs reauthentication,
last-owner handling, retained-audit policy, external-identity consequences,
and bounded cascading behavior. Add that workflow only when the product needs
it; large deletion must then use an authorized long-running operation.

## Portability

`GET /api/v1/workspaces/{workspaceId}/export` requires `workspace:export`,
checks workspace membership, and returns one synchronous JSON attachment. The
format includes version, export time, workspace (including its locale), members, items, audit events,
and an explicit exclusion list. Defaults cap it at 1,000 records and 4 MiB;
over-limit exports fail with `409 export-limit` and are audited. No server-side
download copy remains.

Import is unsupported. Importing memberships and stable identities safely
requires explicit identifier mapping, duplicate policy, authority ceilings,
and partial-result semantics. Adding cloud/self-hosted migration is its trigger;
input must then be treated as untrusted and must never elevate authority.

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

No current product action survives request disconnection or exceeds the
ordinary request contract: item operations are small and exports reject their
bounds. A long-running-operation resource and job system therefore do not
exist. Add both only when measured work needs retry, resumption, cancellation,
or duration beyond the request timeout.
