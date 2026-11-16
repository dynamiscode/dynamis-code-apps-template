# Web and Realtime Interface

Workspace paths carry the workspace short ID. A malformed, unknown, or non-member
ID returns `404`, the same as a missing workspace. A member without the required
permission gets `403`.

The browser interface is server-rendered and reaches the same application use
cases as REST. Sign in at `/login`, create or choose a workspace, then use the
workspace home and sidebar. `/workspaces/{workspaceId}` is the workspace home;
it highlights the first Item task and links to enabled workspace tools. Items is its resource surface and Settings is a nested route
group for members, invitations, API tokens, webhooks, SCIM provisioning, export, import, and audit history.
The Items page states its purpose, labels list controls as “Find and sort”, and
shows readable status labels in compact editor rows. Filters, sorting, pagination,
and ordinary form submissions remain shareable and usable without JavaScript.

Member and invitation pages explain role permissions, ownership transfer,
invited-email matching, and safe recovery for unavailable links.
Settings groups general and email setup, member access, API tokens, webhooks,
SCIM, exports, imports, and audit history into role-aware sidebar sections.
`/account`, `/notifications`, `/sessions`, and `/security` cover account and
security settings. Account pages share responsive panels, generated branding,
and the user's light, dark, or system theme. The authenticated shell uses
a top bar for brand, workspace switching, and account actions, plus a context-aware
sidebar whose contents follow the current workspace context. Every mutation validates
the session-bound CSRF token
and rechecks current workspace permission.

The templ dashboard shell accepts header and sidebar components. Its current
adapters preserve workspace home navigation until shared navigation migrates;
legacy templates remain for other routes. On desktop the workspace sidebar
spans the viewport height. On mobile its links remain visible and usable without
JavaScript.

Shared templ components cover page headings, labelled inputs and selects,
field errors, submit and link buttons, feedback, confirmations, captioned
tables, linked and responsive list cards, empty states, and next-page links. Workspace selection
and home use these components; other routes retain their existing templates
until their migrations. Callers supply translated text, field attributes,
table rows, equivalent mobile card contents, authorization, and CSRF-protected forms.
Use `listTable` with `listCards` for paired responsive lists; use `dataTable`
for standalone scrollable tables. A field error must be
associated with its input through `aria-describedby`. Destructive buttons use
the existing `data-confirm` enhancement; with JavaScript disabled the ordinary
form still submits, so workflows requiring explicit consent must validate a
required confirmation server-side. DaisyUI colors use the existing brand tokens.

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

- `/` lists memberships and creates workspaces in multi mode (`MULTI_WORKSPACE_ENABLED=true`). In single mode
  (`MULTI_WORKSPACE_ENABLED=false`) it redirects with `302` to the home of the
  oldest membership, or shows the no-memberships message when there is none;
  the create form is hidden and `POST /workspaces` returns 404. Workspace cards
  and the `/workspaces/{workspaceId}` route open the authenticated workspace
  home, which links to Items and Settings.
- `/workspaces/{workspaceId}/settings/general` reads workspace settings and lets authorized owners/admins
  change the workspace language fallback with CSRF protection.
- `/workspaces/{workspaceId}/settings/members` lists members in the shared
  responsive table/card view with per-user columns, email/role filters, stable
  sorting, query URLs, and optional cursor paging; the default view is complete.
  Authorized owners/admins change roles or remove members, and owners transfer
  ownership. The ordinary forms work without JavaScript.
- `/workspaces/{workspaceId}/settings/invitations` creates, resends, revokes, and shows
  copyable links. `/invitations/{secret}` accepts or registers an invitee with
  safe invalid, expired, revoked, duplicate, and wrong-email failures.
- `/workspaces/{workspaceId}/settings/email` lets owners/admins save the workspace invitation sender, either SMTP, Resend, or Postmark. Saving runs a provider check first (an SMTP login test, a read-only Resend domain check, or a read-only Postmark server and domain check) and stores nothing on failure. The Resend and Postmark pages show setup steps and the sender domain status from the last save. The password or API key is never shown again, and a blank value keeps the stored one for the same provider. Members cannot view or change it.
- `/workspaces/{workspaceId}/settings/tokens` manages current-user scoped tokens and
  shows a new secret once.
- `/workspaces/{workspaceId}/settings/webhooks` lists workspace webhook registrations,
  lets owners/admins create, rotate, and delete them, and shows redacted delivery
  history. Creation and rotation display the webhook secret once. When
  neither `ENCRYPTION_KEY` nor `WEBHOOK_ENCRYPTION_KEY` is set, managers see an actionable warning and
  create/rotate controls are disabled. Stale form submissions keep the user on
  this page with an inline error. Webhook IDs in paths and forms are short IDs;
  a malformed, integer, or unknown ID shows the not-found page for the history
  view and a "could not be found" message for rotate and delete.
- `/workspaces/{workspaceId}/settings/provisioning` is restricted to workspace
  owners and admins. It shows the workspace SCIM endpoint and setup instructions,
  creates or rotates the dedicated SCIM credential with its secret shown once,
  and revokes it through ordinary CSRF-protected forms. The page is no-store and
  has no WebMCP enhancement; SCIM Users and Groups remain REST/IdP-only.
- `/login`, `/setup`, password recovery, `/mfa`, and email verification share the generated app
  brand and compact responsive auth layout. Setup keeps required fields first
  and marks workspace language as deferrable. Sign-in, MFA, and setup remain
  ordinary CSRF-protected forms without JavaScript. Password-reset request,
  completion, unavailable, and validation states use the same templ layout and
  ordinary forms.
- `/sessions` shows current, active, expired, and revoked browser sessions with
  localized status and revocation feedback. CSRF-protected forms revoke any
  active session, including the current one; revoking the current session sends
  the browser to sign-in on its next request. The page shows a localized empty
  state and retains the ordinary form fallback. `/security` manages MFA factors
  and starts reauthenticated OIDC linking.
- `/account` edits profile preferences, changes a local password, requests email
  verification, and deletes the account after current-password reauthentication
  and an explicit confirmation checkbox. The server rejects deletion without
  confirmation and retains the owner-transfer restriction. `/settings/language`
  persists the account language. `/password-reset` provides localized generic
  request and single-use completion pages.
- `/notifications` lists recipient-scoped in-app records, shows unread state,
  marks them read, and updates the unread count through recipient-scoped SSE.
  It keeps a refresh link for ordinary navigation. Notification records are
  created through the shared identity service, and user plus workspace
  preferences are checked before storage.
- `/notifications/events` delivers recipient-scoped `notification.created` SSE
  events; an expired cursor triggers a page refresh to restore current records.
- `/workspaces/{workspaceId}/files` lists ready private workspace files with
  per-member column visibility, filename/MIME/size filters, stable sorting,
  keyset cursor pagination, and responsive table/card views; filter and sort
  controls use ordinary GET forms. The list and its download actions remain
  workspace-authorized. It also accepts an ordinary multipart upload form.
  File paths carry the file short
  ID; malformed, unknown, and integer-key IDs return not-found. Browsers with
  JavaScript send the same form in one request to that route through a client-side
  `FileUploader` state machine (IDLE, UPLOADING, SUCCESS, and ERROR phases). The
  button and file input stay locked while the request is in flight. A native
  progress control reports computable byte progress; a polite live region
  announces quarter milestones, success, and localized safe errors for an
  oversize file, a full workspace, or an invalid file. The form works without
  JavaScript. Uploads stream through the app for local and S3 storage; S3 downloads
  redirect to a short-lived private URL, and local downloads stream through the app.
- `/workspaces/{workspaceId}/settings/export` presents the authorized export screen;
  its `Download JSON` link downloads the export from
  `/workspaces/{workspaceId}/settings/export/download`.
- `/workspaces/{workspaceId}/settings/import` previews one bounded JSON export
  or strict UTF-8 `title,status` CSV before import. Preview validates every
  record, shows the count, up to five sample records, and up to ten actionable
  row errors without writing items. A valid preview offers an ordinary
  confirmation form; users select the same file again, and the server checks
  its digest before importing. Confirmation rechecks session CSRF, workspace
  permission, file limits, and item quota, then creates all rows atomically.
  Duplicate titles remain separate new items. Both steps work without
  JavaScript.
- `/workspaces/{workspaceId}/settings/audit` presents the authorized,
  read-only latest-100 audit history with redacted fields.
- `/share/{token}` presents a read-only Item projection containing only title
  and localized status. Invalid, expired, and revoked links share the same
  unavailable page. The Items page lets principals with `resources:write` create
  seven- or 30-day links and revoke active links with CSRF-protected ordinary
  forms. The controls state what link holders can see and explain revocation
  consequences without JavaScript. Its shared list view persists each member's
  title and status column
  visibility per workspace, filters by title and status, sorts by creation
  time, and carries query state through pagination and realtime refreshes. The
  same rows adapt from a table to cards on narrow screens; inline title/status
  editing and authorized row actions remain available when either data column
  is hidden.

Forms keep ordinary navigation as fallback. HTMX enhances item fragments only.
Secret-bearing responses use `no-store`; list pages never render session,
CSRF, invitation, API-token, SCIM credential, or webhook secrets. Webhook delivery history excludes
delivery payloads and shows only bounded status, attempt, HTTP status, timestamps,
and redacted error categories.

Public sharing uses `private, no-store`, `no-referrer`, and
`X-Robots-Tag: noindex, nofollow, noarchive`. The existing per-source HTTP
rate limit applies to public access. There is no public write, search, listing,
REST, CLI, MCP, or WebMCP sharing surface.

The MCP consent page shows the client name, requested item permissions, and
eligible workspace choices. Approve and deny remain ordinary CSRF-protected
form submissions; the page does not load WebMCP or submit automatically.

The workspace sidebar exposes `Home` above `Items` in the workspace context.
Home is active at `/workspaces/{workspaceId}`. Settings uses the nested
`/workspaces/{workspaceId}/settings` route and shows only its `Members & invitations`,
`API tokens`, `Webhooks`, `Export`, and `Import` sub-items. Owners and admins also see
`Provisioning (SCIM)` and `Audit history`. The Settings group is separated by
flexible space and anchored at the bottom in the workspace context. The members screen and
invitations screen retain local tabs behind the combined entry. In multi mode the Items page offers
`Back to Workspaces`, returning to the workspace selector; each Settings page offers
`Back to home`, returning to the current workspace home. The native workspace switcher and account menu use ordinary
`details` controls, so they work without JavaScript. In single mode the Items and Members screens show the workspace name as plain text
and omit the switcher list link and the top-bar `Workspaces` crumb; Items also omits `Back to Workspaces`. Tokens,
Provisioning, and Import omit the switcher list link and the top-bar crumb. The Account, Audit history,
and Export crumbs point to the default workspace home. On narrow screens the
workspace and settings sidebars become compact two-column navigation regions.
The workspace switcher panel stays within the viewport. Provisioning is
shown only to owners and admins. Item deletion is
explicitly permanent and asks for confirmation when the browser script is
available; ordinary form submission remains available without it.

Forms work with ordinary navigation. Vendored HTMX 4.0.1 enhances item forms
only by replacing `#item-list`; it contains no business rules. `app.js` keeps
HTMX 2.x non-swap behavior for `204`, `304`, `4xx`, and `5xx` responses so
error pages never replace the list. The exact upstream license is served from
`internal/web/assets/HTMX-LICENSE`.

## Form enhancements

Every page loads `internal/web/assets/forms.js` through the `ui-head` partial in
`navigation.html`. Pages work without it: forms post, the server validates, and
errors render as `role="alert"`. With scripting, forms gain these behaviors,
each driven by a template attribute:

| Pattern | Attribute or place | Behavior |
|---|---|---|
| Busy state and double-submit prevention | every form | A submit marks the form `aria-busy`, disables its buttons with "Saving…", and cancels a second submit. The file upload form keeps its own state machine. |
| Inline validation and error summary | every form | Fields are checked when left and on submit; invalid fields get a message and `aria-invalid`, and a focused summary links to each one. A full-page server error takes focus on load. |
| Forgiving format | `data-trim`, MFA codes | Text is trimmed. The server accepts authenticator codes with spaces or dashes and recovery codes in any case. |
| Input masks | `data-mask="digits"`, `data-mask="upper"` | Authenticator codes keep digits only; recovery codes become upper case. |
| Password strength and reveal | `data-strength`, every password | A meter scores 0 below 12 characters (the server minimum); a toggle shows or hides the value. `data-match` checks a confirmation. |
| Smart defaults | `data-default="timezone"`, token expiry | An empty timezone is filled from the browser; API tokens default to 90 days. |
| Dirty detection | `data-dirty-track`, `data-requires-change` | Save stays disabled until a field changes. |
| Unsaved-changes warning | `data-unsaved-warning` | Leaving a changed form asks first. |
| Draft persistence | `data-persist` | Unsent text is kept in `sessionStorage` (never passwords, hidden values, or files), restored after a rejected submit or a return visit, and discarded after a save. |
| Wizard | `data-wizard`, `fieldset data-step` | First-run setup shows one step at a time; Next validates the current step. |
| Progressive disclosure | `data-show-when="control=value"` | Email settings show only the chosen provider's section. |
| Optimistic UI | `data-optimistic="remove"` | A confirmed item delete hides the row at once and restores it on failure. |
| Autosave | `data-autosave` | Changing an item's status saves the row. |
| Toasts | `HX-Trigger: {"app:toast": …}` | Item saves, additions, deletions, and request failures show in a polite live region. |
| Lazy loading | `data-lazy-src` | The audit page renders first and fetches its table; without scripting, a link shows it. |
| Keyboard | skip link, Escape | The first Tab reaches "Skip to content"; Escape closes an open menu or the newest toast; focus returns to the same field after an htmx swap. |

A rejected item create keeps the typed title, and a reused create form answers
`409` with a message instead of `500`. The realtime item list waits while a row
is being edited and refreshes when focus leaves it.

## Browser journey and screenshot baselines

`make browser-smoke` starts a disposable Go server and uses pinned Playwright
with Chrome. It signs in, opens the workspace home, and creates the first Item.
The committed workspace-home screenshots cover desktop (1280×900) and mobile
(390×844) in light and dark system themes. CI installs Chrome and runs the same
journey and screenshot assertions.

The rules behind these behaviors, and how to check them, are in the
`item-create-submission`, `form-drafts`, `form-field-input`, `form-wizard`,
`async-feedback`, `form-markup`, and `file-upload-form` skills under `.agents/skills/`. `make ux-smoke`
drives them in headless Chrome against a fresh server; `docs/statecharts/` charts
the item create, item delete, draft, and setup flows.

## Theme

Light, dark, and system appearance. The signed-in user's `users.theme`
preference is canonical; `system` follows the OS `prefers-color-scheme`.
Saving the account profile also sets a `theme` cookie that mirrors the
preference, so anonymous and first-paint renders match it. Resolution order:
user preference, then the mirror cookie, then `system`. An invalid cookie falls
back to `system`, and a stale mirror is refreshed from the stored preference on
the next rendered page. The mirror holds no secret.

Every page root carries `data-theme`. `app.css` defines tokens for `light` and
`dark`; the `system` value applies the dark tokens only under
`prefers-color-scheme: dark`.

`common.product_name` supplies the browser brand name. Generated apps can
customize `--brand-accent`, `--brand-font-family`, `--brand-radius`, and
`--brand-space` in `internal/web/assets/app.css`; the existing user theme
preference controls light, dark, or system appearance.

The workspace home is the first page rendered with templ. `components.css` is
compiled from pinned Tailwind CSS and daisyUI packages and served from the
embedded asset tree. `make templ-check` and `make css-check` detect generated
file drift.

## Rendering migration inventory

Ticket #386 reconciles the route and template surface. `/` and `/workspaces/{workspaceId}`
render through `workspace.templ` and generated `workspace_templ.go`. The
inventory below names each route family's current renderer: `.templ` entries
are migrated, and named `html/template` views remain deferred. It records major
states while preserving escaping, HTMX fragment boundaries, ordinary-form
fallbacks, and route behavior during the incremental migration.

Shared navigation uses `navigation.templ`. The named definitions in
`navigation.html` bridge remaining `html/template` callers to the same escaped
components; remove each bridge when its final caller migrates. The workspace
shell composes those components directly.

| Navigation surface | Decision and evidence | Route owner |
|---|---|---|
| Script messages and skip link | Shared on authenticated, auth, public, and error pages; `TestLegacyNavigationBridge` checks escaping and message placeholders | #434; page migrations remain separate |
| Workspace context and breadcrumbs | Single mode omits workspace selection; multi mode retains the native switcher and ordinary links; `TestSingleModeHidesWorkspaceListNavigation` and `TestMultiModeKeepsWorkspaceListNavigation` | #410 and #434 |
| Workspace and settings navigation | Home, Items, Clients, Files and settings retain feature and role gates; `TestWebNavigationAndActionFeedback` checks current pages and settings | #434; resource routes remain separate |
| Member and invitation tabs | Both retain ordinary links and current-page state; `TestWebNavigationAndActionFeedback` | #434; member and invitation pages remain separate |
| Account menu | Native disclosure and CSRF-protected logout remain; `TestLegacyNavigationBridge` checks escaping | #434; account pages remain separate |

Empty, validation, pending and error states belong to the page routes above;
navigation remains present when those pages render it. Focused browser and
accessibility results and remaining limits are recorded in
[shared navigation migration evidence](capabilities.md#shared-navigation-migration-evidence-434-2026-11-16).

| Route family | Views and shared templates | Major states covered |
|---|---|---|
| `/`, `/workspaces/{workspaceId}` | `workspace.templ`, `components.templ` | Membership selection and creation, workspace home, unavailable workspace, setup-required and validation errors |
| `/setup`, `/login`, `/auth/oidc/{providerId}` and callback, `/password-reset[/{secret}]`, `/mfa`, `/account/verify-email/{secret}`, `/oauth/authorize` | `auth.templ`, `auth_templ.go`, `mfa.html`, `verification.html`, `mcp-consent.html`, `auth.html` (bridge for remaining legacy auth views) | Initial setup, form errors, sign-in and provider choice, OIDC callback failure, reset request/completion/unavailable/validation, MFA challenge, verification success/unavailable, OAuth approval/denial |
| Workspace items, clients, and custom fields | `items.html`, `item-list.html`, `list-view.html`, `clients.html`, `client.html`, `client-fields.html` | Empty/results states, filters and paging, create/edit/delete, validation, custom-field lifecycle, realtime refresh |
| Workspace members and invitations | `members.html`, `invitations.html`, `invitation.html` | Role changes, removal and ownership transfer, invitation create/resend/revoke, accepted and unavailable invitation |
| Workspace files | `files.html` | Empty/list states, filters and paging, upload progress/success/error, download authorization |
| `/sessions` | `sessions.templ`, `sessions_templ.go` | Current, active, expired and revoked status; revoke current or other active sessions; feedback and empty state |
| Account and workspace settings | `account.html`, `audit-history.html`, `email-settings.html`, `export.html`, `general-settings.html`, `import.html`, `language.html`, `notifications.html`, `provisioning.html`, `security.html`, `tokens.html`, `webhooks.html`, `navigation.html` | Settings validation, secret one-time display, redacted history, notification read/unread, export/import entry, integration setup and failure, responsive navigation |
| Public and unavailable pages | `public-share.html`, `error.html` | Share projection and invalid/expired/revoked links, not-found and request errors |

Corresponding route and handler tests cover server-rendered behavior.
`make ux-smoke` and `make accessibility-smoke` cover the browser paths they
enumerate; [the accessibility evidence](accessibility.md) records that
automated coverage. Workspace selection and workspace home use templ; the
listed legacy views are an intentional exception to the migration target, not
an exception to escaping, authorization, accessibility, localization, or
ordinary-form requirements.

## WebMCP progressive enhancement

WebMCP is optional, browser-only, and bound to the current tab's live web
session. The application uses only the imperative API and registers tools
after feature-detecting `document.modelContext`; browsers without that API
retain identical ordinary HTML navigation and form behavior. This surface does
not reuse bearer credentials, change server MCP scopes/tools/transport, or
replace server authorization. Server MCP remains persistent and authoritative.

Audit history is intentionally not an eligible WebMCP page: it has no
WebMCP marker or script and exposes no audit tool. The ordinary server-rendered
page remains the only browser surface.

Eligible pages load the local `app.js` and mark visible controls. The current
tool contract is:

| Tool | Input schema | Page preparation |
|---|---|---|
| `workspace-create-v1` | `{name}` | Fill workspace name and focus the control |
| `item-create-v1` | `{title}` | Fill item title and focus the control |
| `item-update-v1` | `{itemId,title,status}` | Fill visible title/status and focus Save |
| `item-delete-v1` | `{itemId}` | Focus Delete permanently |
| `member-role-update-v1` | `{memberId,role}` | Fill visible role and focus Change role |
| `member-remove-v1` | `{memberId}` | Focus Remove |
| `ownership-transfer-v1` | `{memberId}` | Focus Transfer ownership |
| `invitation-revoke-v1` | `{invitationId}` | Focus Revoke |
| `token-revoke-v1` | `{tokenId}` | Focus Revoke |
| `session-revoke-v1` | `{sessionId}` | Focus Revoke |
| `workspace-export-v1` | `{}` | Focus the authorized `Download JSON` link on the export screen |

Schemas are explicit, bounded, and versioned. Tools expose no passwords,
login/logout or reauthentication fields, OIDC state, invitation URLs or
secrets, token secrets, session or CSRF values, hidden form fields, operator
backup/restore/import/maintenance/audit controls, or export content. Invitation
creation/resend/acceptance/registration, token creation/secret display, SCIM
credential setup, and webhook management/secret display stay outside WebMCP.
The webhook page does not register browser-agent tools and retains ordinary
accessible forms.

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

`/workspaces/{workspaceId}/clients` lists authorized clients newest first by
default. It renders a semantic table above 38rem and cards at or below 38rem.
Display name and email are visible by default; common metadata and active custom
fields are available as columns. Column choices save per user, workspace, and
list; reset columns restores the defaults. Archived custom fields are hidden
without losing their saved choice, so restoring one restores its prior visibility.
The GET form accepts one sort and up to 20 distinct field filters, represented as
repeated `filter=field:operator:value` parameters, plus page size and archived
selector. Filters combine with AND, appear as removable chips, and remain visible
when their column is hidden. Sorting, filters, page size, and cursor stay in the
URL. Clear filters and reset columns are separate controls. Column and filter
forms work without JavaScript. Members, admins, and owners see the create form;
viewers see the list read-only. The create form stays open after validation
errors; active query state opens list options. The workspace shell uses the
generated product name and brand tokens. Header actions link to client creation
and custom-field management; results remain first, followed by create and list
options.
`/workspaces/{workspaceId}/clients/{clientId}` shows one client. Writers get an
edit form that carries the client's version; a stale version shows a retry
message. Writes are ordinary CSRF-protected POST forms that redirect after
success, with no HTMX or WebMCP enhancement. Every client and field form submits
without JavaScript. With scripting, a whitespace-only required name is stopped in
the browser with the error summary, and a choice option delete asks first. A
refused client edit or field definition keeps what was typed. Labels come from the locale catalog
and follow the workspace locale. The Clients navigation link appears only when
the Clients profile is composed. WebMCP is omitted for Clients in v1 (see
[ADR 0013](decisions/0013-clients-profile.md)).
The client page shows a value for each active custom field. Members and above
get one save form per field and a separate clear control; viewers see the values
read-only. Value writes are CSRF-protected POST forms that redirect to the client
page, and a stale or refused write re-renders it with a message.
Members and above see archive and unarchive buttons on the client page. Owners
and admins also see a confirmed permanent delete, which returns to the list. The
detail page displays the permanent-delete effects beside the control, including
when JavaScript is unavailable. The list form has an archived selector (active,
archived, or all), and the next-page link keeps it.
`/workspaces/{workspaceId}/client-fields` lists custom fields, archived ones
included. Owners and admins see the define form and the archive or restore
buttons; other roles see the list read-only. Writes are CSRF-protected POST
forms that redirect to the list. Labels are shown exactly as typed, escaped for
HTML. Permanent deletion is offered only for archived fields, with the effect on
all client values and choice options shown beside the control, including without
JavaScript. A refused label returns 422 and a reached limit returns 409, both on
the same page.

## Realtime contract

`GET /workspaces/{workspaceId}/items/events` is an authenticated SSE stream.
It sends redacted resource references, not item titles or credentials.

- `item.changed` carries a stable short event ID, schema version `1`, UTC occurrence
  time, workspace and item IDs, action, and item version.
- Events order by occurrence time then ID. Delivery is at least once; clients
  must tolerate duplicates and refresh the normal item list.
- Browsers reconnect with `Last-Event-ID`. Retained events after that ID replay
  in order. A missing, malformed, unknown, or expired cursor receives `resync`, requiring
  a normal list refresh before continuing.
- The database retains the latest 1,000 item events per workspace. Database
  polling makes delivery shared across instances once PostgreSQL is used.
- Defaults: one-second poll, 15-second heartbeat, five-minute connection
  lifetime, 100 connections per instance, and five per user per instance.
  Limit rejection returns `429` plus `Retry-After`; lifetime closure emits a
  `close` event and normal EventSource reconnection applies.

`GET /workspaces/{workspaceId}/clients/events` is the client list's stream. It
follows the same read permission, cursor, limit, heartbeat, and lifetime rules.
It sends `client.changed` with the client's short ID, schema version `1`, UTC
occurrence time, workspace ID, action (`created` or `updated`), and client
version. It carries no client fields. A missing, malformed, unknown, or expired
cursor receives `resync`. Viewers receive events, and a user outside the
workspace gets not found. The latest 1,000 client events per workspace are
retained, and older events fall under the same replay window as Items.
The list fragment and listener exist, but the client page does not load the
scripts needed to start its browser refresh. [#415](https://github.com/dynamiscode/dynamis-code-apps-template/issues/415)
tracks wiring it and protecting unsaved input; both stream and fragment must
continue to require `identity.ResourcesRead`.

`GET /notifications/events` uses the same stream limits, heartbeat, lifetime,
and database polling. Events carry the notification ID, type, title, body, and
UTC creation time only to the authenticated recipient. `Last-Event-ID` resumes
after the recipient's stored notification; no notification secret or other
user's record is exposed. The notification client explicitly closes its
EventSource on the server's lifetime `close` event and does not reopen it;
live updates stop until the page reloads. The refresh link remains available.

Permanent delete removes the live item and writes a redacted audit/change
event. Existing backups may retain deleted content until the expiry defined in
[data lifecycle](data-lifecycle.md); live deletion does not rewrite prior
backups.
