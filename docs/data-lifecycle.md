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
| identity / `workspace_members` | `id`, `workspace_id`, `user_id`, `role`, `created_at` (personal/internal) | Scoped authorization | Membership lifetime; workspace is implied by the export envelope and other fields export; role correction uses authorized membership changes; final-owner protection applies. |
| identity / `sessions` | `id`, `user_id`, `auth_method`, `oidc_provider_id`, timestamps (personal/internal); `secret_hash`, `csrf_hash` (secret) | Browser authentication and revocation | At most 10 active per user; expired sessions and revocations older than 30 days are pruned; excluded from export; users may list/revoke their sessions. |
| identity / `invitations` | IDs, `email`, `role`, timestamps (personal/internal); `secret_hash` (secret) | Bounded membership invitation lifecycle | Active until accepted, revoked, or expired; completed records older than 365 days are pruned; excluded from export; resend rotates the secret. |
| identity / `invitation_return_handoffs` | `invitation_id`, lifecycle timestamps (internal); `token_hash`, `binding_hash` (secret) | Short-lived browser-bound return after invitation sign-in | At most ten minutes and capped at invitation expiry; expired rows are pruned and resend/revoke delete active rows; excluded from export; invitation deletion cascades. Raw invitation secrets are never stored. |
| identity / `api_tokens` | IDs, `name`, `scopes`, timestamps (personal/internal); `secret_hash` (secret) | Scoped API/MCP credentials | Active until expiry/revocation; inactive records older than 365 days are pruned; excluded from export; scope changes and revocation are audited. |
| identity / `oauth_clients`, `oauth_grants`, `oauth_tokens` | IDs, client name, redirect URIs, scopes, auth level, timestamps (personal/internal); `secret_hash` (secret) | Built-in MCP OAuth clients, consent grants, and single-use codes, access tokens, and rotating refresh tokens | Grant lifetime until revocation or account deletion; codes last 60 seconds, access tokens 15 minutes, refresh tokens 30 days; expired-record pruning is follow-up work; excluded from export; account deletion cascades grants and tokens; consent is revocable through `/oauth/revoke`. |
| identity / `scim_tokens` | IDs, workspace, creator, timestamps (internal); `secret_hash` (secret) | Dedicated workspace SCIM bearer credentials | One active credential per workspace; secret is shown once; inactive records older than 365 days are pruned; excluded from export; creation, rotation, and revocation are audited. |
| identity / `scim_users`, `scim_groups` | Workspace mappings, external IDs, roles, active/version state, timestamps (internal) | Stable SCIM identity and role-group versions | Workspace lifetime; excluded from export; deactivation retains mappings for safe reactivation; owner mappings never appear in groups. |
| identity / `oidc_transactions` | `provider_id`, `redirect_uri`, timestamps (internal); all hash fields (secret) | Single-use OIDC state, PKCE, nonce, and browser binding | Ten-minute active lifetime; expired records are pruned; excluded from export; no correction. |
| identity / `mfa_challenges` | user, purpose, authentication method, attempt and lifecycle timestamps (internal); token hash, WebAuthn state, encrypted enrollment secret (secret) | Single-use MFA login and factor-enrollment challenges | Five-minute active lifetime; expired and consumed records are pruned after 30 days; excluded from export; no correction. |
| identity / `email_verifications`, `password_resets` | user, email where applicable, timestamps (internal); token hash (secret) | Single-use account recovery and verification | Twenty-four-hour single-use lifetime; expired and consumed records are pruned; excluded from export; account deletion cascades. |
| identity / notification preference tables | user/workspace, notification type, enabled, updated timestamp (personal) | User and workspace in-app delivery preference | Account or membership lifetime; excluded from workspace export; account deletion and member removal clean up. |
| lists / `list_column_preferences` | workspace, user, list and column keys, visibility (personal) | Explicit deviations from each list's declared column defaults | Account or workspace lifetime; excluded from workspace export; account/workspace deletion cascades; permanent Client custom-field deletion removes that field's overrides in the field deletion transaction. |
| identity / `notifications` | IDs, recipient, optional workspace, type, title, body, timestamps (personal) | In-app notification inbox and SSE delivery | One-year retention; read state is user-correctable; excluded from workspace export; account/workspace deletion cascades. |
| identity / `audit_events` | IDs, types, actions, outcome, timestamps (internal); actor, workspace, request, source address, metadata (personal) | Security and administrative evidence | Append-only during normal operation; default 365-day retention; workspace events export and the redacted latest-100 browser projection are available to owners/admins; maintenance deletes expired events and records its own prune event. No product correction. |
| identity / `bootstrap_state` | `id`, `completed_at` (internal) | Enforce one-time first-owner bootstrap | Installation lifetime; backup only; never reset by application behavior. |
| platform / `idempotency_records` | hashes, IDs, operation, result, timestamps (internal; hashes treated as secret) | Retained legacy create replay records | Exact 24-hour expiry; pruned after expiry; excluded from export; no correction. |
| webhooks / `webhooks` | IDs, workspace, name, endpoint URL, selected event names, timestamps (internal/configuration); encrypted secret (secret) and its key version (internal) | Workspace event delivery registration | Workspace lifetime or explicit deletion; excluded from export; secret rotates through authorized management and is never returned after creation. |
| email / `workspace_email_settings` | workspace, provider, sender address and name, host, port, username, sender domain status (personal/configuration); encrypted password, Resend API key, or Postmark Server API token (secret) and Postmark Account API token (secret), each with its key version (internal) | Workspace-scoped invitation delivery through its own SMTP or Resend account | Workspace lifetime; replaced on save; never returned after save; excluded from export; cascades on workspace deletion. |
| intake / `public_intake_links` | workspace, consent text (configuration); token hash (secret) | Workspace public intake link and consent | Workspace lifetime; consent is editable and the token can be rotated or disabled; excluded from workspace export, included in backups; workspace deletion cascades. |
| webhooks / `webhook_deliveries` | IDs, webhook/event references, event type, bounded payload, attempt/status/timestamps, HTTP status, redacted error category (internal; payload personal) | Durable at-least-once Client event delivery and bounded delivery history | Pending rows remain until delivery settles; delivered/failed rows retained 365 days and pruned by maintenance; excluded from export; cascade on webhook deletion. |
| webhooks / `webhook_events` | Integer key (short ID for `eventId` and `Webhook-Id`), workspace, event type, creation time (internal) | Shared event identity for one Client change across its deliveries | Removed by maintenance once no delivery references it (after the delivery history window); excluded from export; cascade on workspace deletion. |
| catalog / `catalog_entries` and `catalog_prices` | workspace, name, optional duration, archive marker; price mode, decimal amount, currency, variant and optional eligibility (internal/user content) | Active workspace catalog | Workspace lifetime or deletion of an unused entry; workspace and entry deletion cascade. Included in database backups but not workspace export. Owners and admins can edit entries and edit or remove price rows. |
| clients / `client_timeline_entries` | workspace, client, author, type, channel, correction, timestamp (internal/personal); text (personal user content) | Append-only client history | Client or workspace lifetime; client/workspace deletion cascades; author account deletion anonymizes author and preserves entries and corrections. Excluded from workspace export, included in database backups. |
| engagement / `engagement_stages` | workspace, position, kind, archive state (internal); name (user content) | Workspace-owned ordered stages | Workspace lifetime; workspace deletion cascades; included in database backups, excluded from workspace export. Stages can be reordered or archived; this slice does not expose correction or deletion. |
| engagement / `engagement_stage_history` | workspace, engagement, actor, stages, timestamp (personal/internal) | Append-only history of stage moves | Engagement or workspace lifetime; deletion cascades; actor ID remains after account deletion. Included in backups, excluded from workspace export. Audit events for the same moves follow ordinary audit retention. |
| engagement / `engagements` | workspace, client, stage, assignee, agreement file key (internal); notes, quote status, amount, sent date and agreement flag (user content) | Work associated with one client | Client or workspace lifetime; client deletion removes engagements and workspace deletion cascades; included in backups, excluded from workspace export. Assignees must be members when assigned; membership removal or account deletion clears the reference. A referenced file cannot be deleted until the reference is cleared. |
| engagement / `engagement_next_actions` | workspace, engagement, owner (internal); text and due date (user content) | Follow-up for an engagement | Engagement lifetime; workspace deletion cascades; included in backups, excluded from workspace export. Writers can replace or clear the action. The owner must be a member when assigned; membership removal or account deletion clears the owner reference. |
| files / `files` | Integer key (public as the file short ID), workspace/owner, object key, original name, detected MIME, size, SHA-256, status, timestamps (personal user content/internal) | Workspace-scoped private file metadata and object reconciliation | Workspace lifetime; metadata is included in backup but excluded from workspace export; owner deletion sets owner NULL; live object deletion/reconciliation is not exposed in this slice. Valid transitions are Pending -> Ready and Pending -> Failed; Pending and Failed rows remain quota-reserved until cleaned up by operator reconciliation or `CleanupStaleFiles`. |
| platform / `background_jobs` | IDs, workspace, handler kind, deduplication key, bounded payload, status/attempt/lease/timestamps, redacted error category (internal; payload personal) | Durable retry and lease ownership for bounded asynchronous handlers | Pending and leased rows remain until settlement; settled rows retained 365 days and pruned by maintenance; excluded from export; workspace deletion cascades. |

Every row is included in database backup until the operator's backup retention
expires. Production fixtures must never contain copied production data.

Workspace owners and admins can view the latest 100 events for their workspace
at Settings → Audit history, using the existing `workspace:export` permission.
The browser projection omits metadata, workspace and target IDs, request IDs,
and source addresses. Older workspace events remain available through the
authorized bounded export. Members, viewers, and cross-workspace requests are
denied. No public instance-wide audit endpoint exists. Only the deployment
operator may inspect instance events through restricted database access.
Normal application code inserts audit rows but never updates or deletes them;
the retention command is the sole deletion path.

Files are standalone workspace resources, not generic attachments. Local bytes
live under the configured storage path; S3 bytes live in the configured private
bucket/prefix. S3 URLs expire according to `STORAGE_SIGNED_URL_TTL`. The initial
allowlist rejects executable, HTML, SVG, archive, and mismatched
extension/signature content. Background scanning, orphan reconciliation,
durable deletion, and workspace deletion require the deferred Background Jobs
trigger; this slice leaves bounded metadata hooks and adds no worker.

## Resource and identity deletion

Account deletion is available after local-password reauthentication and is
refused for users who still own a workspace. Ownership transfer is required
first; deletion then cascades account credentials, memberships, external
identities, preferences, and notifications, and removes invitations created by
the account. The deletion audit event retains the user ID and safe metadata but
no credential or profile content. File metadata remains in its workspace with `ownerUserId: null`. Workspace deletion remains unavailable.

## Portability

`GET /api/v1/workspaces/{workspaceId}/export` requires `workspace:export`
and workspace membership. The synchronous JSON attachment uses
`dynamis-code.workspace/v1` and includes export time, workspace, members,
audit events, and an explicit exclusion list. When the Clients profile is
enabled it also includes `clients`, `clientFields`, and
`clientFieldOptions`; archived clients and fields are included. Custom values
are keyed by field short ID. Defaults limit the export to 1,000 records and
4 MiB. Over-limit exports fail with `409 export-limit`; no server-side
download copy remains.

Items are no longer included, and the former Items-only import routes are
removed. Workspace export is not an operator backup or restore mechanism.
WebMCP can prepare the authorized browser export link, but cannot return the
export content or submit a mutation.

Webhook registrations and delivery history are not part of workspace export;
endpoint URLs and delivery payloads can contain integration or personal data,
and encrypted secrets are deployment-sensitive. Database backups containing
these rows remain sensitive.

Webhook delivery survives request disconnection through the background job
queue. No public long-running-operation resource, cancellation contract, or
browser/REST job administration exists; add those only when a product action
needs user-visible progress or cancellation beyond the bounded handler model.
