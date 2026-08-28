# Web and Realtime Interface

The browser interface is server-rendered and reaches the same application use
cases as REST. Sign in at `/login`, create or choose a workspace, then use the
workspace home and sidebar. `/workspaces/{workspaceId}` is the workspace home;
Items is its resource surface and Settings is a nested route
group for members, invitations, API tokens, and export.
`/account`, `/notifications`, `/sessions`, and `/security` cover account and
security settings. The authenticated shell uses
a top bar for brand, workspace switching, and account actions, plus a context-aware
sidebar whose contents follow the current workspace context. Every mutation validates
the session-bound CSRF token
and rechecks current workspace permission.

## Localization

Browser catalogs are embedded, Git-reviewed JSON under `internal/i18n/locales`
and currently support English (`en`) and Spanish (`es`). The browser resolves
locale with this precedence:

| Surface | Precedence |
|---|---|
| Authenticated global page | user preference, explicit `locale` cookie, `Accept-Language`, `en` |
| Workspace page | user preference, explicit `locale` cookie, workspace locale, `Accept-Language`, `en` |
| Invitation page | explicit `locale` cookie, invitation workspace locale, `Accept-Language`, `en` |
| Invitation email | workspace locale |

`GET /language?locale=en|es&return_to=...` sets the safe local explicit
locale cookie. `/settings/language` persists the account preference; Automatic
clears both the preference and cookie. Workspace owners and admins edit the
workspace fallback at `/workspaces/{workspaceId}/settings/general`. That
fallback controls invitation email language and browser fallback when no user
or explicit cookie preference exists. New and existing workspaces default to
English unless selected otherwise.

Every document response emits `Content-Language` and its root `lang` matches
the resolved locale. User content, identifiers, API contracts, WebMCP names,
and stored UTC values remain unchanged. REST, CLI, and MCP errors remain
stable and English for machine clients.

Baseline browser surfaces:

- `/` lists memberships and creates workspaces. Workspace cards and the
  `/workspaces/{workspaceId}` route open the authenticated workspace home, which
  links to Items and Settings.
- `/workspaces/{workspaceId}/settings/general` reads workspace settings and lets authorized owners/admins
  change the workspace language fallback with CSRF protection.
- `/workspaces/{workspaceId}/settings/members` lists members; authorized owners/admins
  change roles or remove members, and owners transfer ownership.
- `/workspaces/{workspaceId}/settings/invitations` creates, resends, revokes, and shows
  copyable links. `/invitations/{secret}` accepts or registers an invitee with
  safe invalid, expired, revoked, duplicate, and wrong-email failures.
- `/workspaces/{workspaceId}/settings/tokens` manages current-user scoped tokens and
  shows a new secret once.
- `/sessions` lists metadata and revokes sessions; `/security` starts
  reauthenticated OIDC linking.
- `/account` edits profile preferences, changes a local password, requests email
  verification, and deletes the account after reauthentication. `/password-reset`
  provides generic request and single-use completion pages.
- `/notifications` lists recipient-scoped in-app records and marks them read;
  `/notifications/events` delivers recipient-scoped `notification.created` SSE
  events. Notification records are created only through the shared identity
  service, and user plus workspace preferences are checked before storage.
- `/workspaces/{workspaceId}/settings/export` presents the authorized export screen;
  its `Download JSON` link downloads the export from
  `/workspaces/{workspaceId}/settings/export/download`.
- `/share/{token}` presents a read-only Item projection containing only title
  and status. The Items page lets principals with `resources:write` create
  seven- or 30-day links and revoke active links with CSRF-protected ordinary
  forms.

Forms keep ordinary navigation as fallback. HTMX enhances item fragments only.
Secret-bearing responses use `no-store`; list pages never render session,
CSRF, invitation, or token secrets.

Public sharing uses `private, no-store`, `no-referrer`, and
`X-Robots-Tag: noindex, nofollow, noarchive`. The existing per-source HTTP
rate limit applies to public access. There is no public write, search, listing,
REST, CLI, MCP, or WebMCP sharing surface.

The workspace sidebar exposes `Home` above `Items` in the workspace context.
Home is active at `/workspaces/{workspaceId}`. Settings uses the nested
`/workspaces/{workspaceId}/settings` route and shows only its `Members & invitations`,
`API tokens`, and `Export` sub-items. The Settings group is separated by flexible
space and anchored at the bottom in the workspace context. The members screen and
invitations screen retain local tabs behind the combined entry. The Items page offers
`Back to Workspaces`, returning to the workspace selector; each Settings page offers
`Back to home`, returning to the current workspace home. The native workspace switcher and account menu use ordinary
`details` controls, so they work without JavaScript. On narrow screens the
settings sidebar becomes a compact stacked navigation region. Item deletion is
explicitly permanent and asks for confirmation when the browser script is
available; ordinary form submission remains available without it.

Forms work with ordinary navigation. Vendored HTMX 2.0.4 enhances item forms
only by replacing `#item-list`; it contains no business rules. The exact
upstream license is served from `internal/web/assets/HTMX-LICENSE`.

## WebMCP progressive enhancement

WebMCP is optional, browser-only, and bound to the current tab's live web
session. The application uses only the imperative API and registers tools
after feature-detecting `document.modelContext`; browsers without that API
retain identical ordinary HTML navigation and form behavior. This surface does
not reuse bearer credentials, change server MCP scopes/tools/transport, or
replace server authorization. Server MCP remains persistent and authoritative.

Eligible pages load the local `app.js` and mark visible controls. The current
tool contract is:

| Tool | Input schema | Page preparation |
|---|---|---|
| `workspace-create-v1` | `{name}` | Fill workspace name and focus the control |
| `item-create-v1` | `{title}` | Fill item title and focus the control |
| `item-update-v1` | `{itemId,title,status}` | Fill visible title/status and focus Save |
| `item-delete-v1` | `{itemId}` | Focus Delete permanently |
| `member-role-update-v1` | `{userId,role}` | Fill visible role and focus Change role |
| `member-remove-v1` | `{userId}` | Focus Remove |
| `ownership-transfer-v1` | `{userId}` | Focus Transfer ownership |
| `invitation-revoke-v1` | `{invitationId}` | Focus Revoke |
| `token-revoke-v1` | `{tokenId}` | Focus Revoke |
| `session-revoke-v1` | `{sessionId}` | Focus Revoke |
| `workspace-export-v1` | `{}` | Focus the authorized `Download JSON` link on the export screen |

Schemas are explicit, bounded, and versioned. Tools expose no passwords,
login/logout or reauthentication fields, OIDC state, invitation URLs or
secrets, token secrets, session or CSRF values, hidden form fields, operator
backup/restore/import/maintenance/audit controls, or export content. Invitation
creation/resend/acceptance/registration and token creation/secret display stay
outside WebMCP.

Preparation never calls `submit()` or `requestSubmit()`, including for role,
ownership, removal, revocation, and delete tools. The user completes the
existing visible control through its normal CSRF-protected flow. Results are
safe statuses only. The `tools=(self)` Permissions Policy and
`Origin-Agent-Cluster: ?1` response headers provide the browser security
boundary; same-origin page state and existing authorization remain required.

Test with `make webmcp-smoke`. The runner checks ordinary fallback on every
configured Selenium browser. It runs native tool names, schema/redaction,
focus, and non-submission assertions only when `document.modelContext` exists;
missing WebMCP support is a passing fallback result. Debug registration with
the browser's `document.modelContext.getTools()` inspection without copying
session values or hidden inputs into logs.

## Realtime contract

`GET /workspaces/{workspaceId}/items/events` is an authenticated SSE stream.
It sends redacted resource references, not item titles or credentials.

- `item.changed` carries a stable event ID, schema version `1`, UTC occurrence
  time, workspace and item IDs, action, and item version.
- Events order by occurrence time then ID. Delivery is at least once; clients
  must tolerate duplicates and refresh the normal item list.
- Browsers reconnect with `Last-Event-ID`. Retained events after that ID replay
  in order. A missing, unknown, or expired cursor receives `resync`, requiring
  a normal list refresh before continuing.
- The database retains the latest 1,000 item events per workspace. Database
  polling makes delivery shared across instances once PostgreSQL is used.
- Defaults: one-second poll, 15-second heartbeat, five-minute connection
  lifetime, 100 connections per instance, and five per user per instance.
  Limit rejection returns `429` plus `Retry-After`; lifetime closure emits a
  `close` event and normal EventSource reconnection applies.

`GET /notifications/events` uses the same stream limits, heartbeat, lifetime,
and database polling. Events carry the notification ID, type, title, body, and
UTC creation time only to the authenticated recipient. `Last-Event-ID` resumes
after the recipient's stored notification; no notification secret or other
user's record is exposed.

Permanent delete removes the live item and writes a redacted audit/change
event. Existing backups may retain deleted content until the expiry defined in
[data lifecycle](data-lifecycle.md); live deletion does not rewrite prior
backups.
