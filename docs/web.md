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

The web code is split into four packages under `internal/web`:

- `viewmodel`: the `PageData` view model and the list, form, and filter row types templates read.
- `components`: shared templ components (page headings, form fields, alerts, tables, list tables, list filters).
- `layouts`: the document head, the dashboard shell (`DashboardShell`), headers, navigation, and sidebars.
- `web` (the root): handlers, routes, and feature pages, which import the three packages above.

The templ dashboard shell accepts header and sidebar components. Workspace home
and Files compose the shared shell and navigation directly; adapters preserve
the same shell for legacy route templates during migration. On desktop the
workspace sidebar spans the viewport height. On mobile its links remain visible
and usable without JavaScript.

Shared templ components cover page headings, labelled inputs and selects,
field errors, submit and link buttons, feedback, confirmations, captioned
tables, linked and responsive list cards, empty states, and next-page links. Workspace selection,
home, and Files use these components; other routes retain their existing templates
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
  sorting, query URLs, and numbered page links; the default view is complete.
  The route template keeps role, removal, and ownership transfer in separate
  forms. Removal and transfer require separate explicit confirmations, including
  when JavaScript is disabled; removal explains that workspace access and shared
  resources stop. Shared identity policy authorizes role changes and removals
  for owners/admins, and ownership transfer for owners only. The page exposes
  non-secret WebMCP form markers while leaving submission to the user.
  Browser screenshots, no-JavaScript filtering, permissions, confirmations, and
  locale coverage live in [the members Playwright suite](../internal/browsertest/members_test.go).
- `/workspaces/{workspaceId}/settings/invitations` creates, resends, revokes, and shows
  copyable links. `/invitations/{secret}` registers an invitee or starts existing-account
  sign-in through a CSRF-protected form. The server keeps only hashed, browser-bound
  handoff state for up to ten minutes, then returns local, MFA, and OIDC sign-in to
  `/invitations/continue`; the raw invitation secret stays out of redirects and logs.
  Acceptance rechecks invitation status, matches the signed-in email, and consumes the
  handoff atomically with membership creation. Invitation pages use `private, no-store`,
  `no-referrer`, and `noindex` headers; forms work without JavaScript. Existing-account
  acceptance without JavaScript is covered in [the invitation Playwright suite](../internal/web/no_javascript_journeys_test.go).
- `/workspaces/{workspaceId}/settings/email` lets owners/admins save the workspace invitation sender, either SMTP, Resend, or Postmark. Saving runs a provider check first (an SMTP login test, a read-only Resend domain check, or a read-only Postmark server and domain check) and stores nothing on failure. The Resend and Postmark pages show setup steps and the sender domain status from the last save. The password or API key is never shown again, and a blank value keeps the stored one for the same provider. Members cannot view or change it.
- `/workspaces/{workspaceId}/settings/tokens` manages current-user scoped tokens with
  CSRF-protected create and revoke forms. Creation shows its secret once; the
  no-store response gives replacement guidance. The page supports empty and
  revoked-token states and the visible token-revoke WebMCP action.
- `/workspaces/{workspaceId}/settings/webhooks` lists workspace webhook registrations,
  lets owners/admins create, rotate, and delete them, and shows redacted delivery
  history. Creation and rotation display the webhook secret once. When
  neither `ENCRYPTION_KEY` nor `WEBHOOK_ENCRYPTION_KEY` is set, managers see an actionable warning and
  create/rotate controls are disabled. Stale form submissions keep the user on
  this page with an inline error. Delete requires a localized confirmation
  checkbox; the handler rejects unconfirmed submissions. Webhook IDs in paths and forms are short IDs;
  a malformed, integer, or unknown ID shows the not-found page for the history
  view and a "could not be found" message for rotate and delete.
  Browser and accessibility evidence, including the no-key guard, is recorded in
  [accessibility evidence](accessibility.md#workspace-administration-and-data-evidence-2026-11-17);
  lifecycle, permission, CSRF, and delivery-redaction behavior is covered by
  [webhook handler tests](../internal/web/webhooks_test.go).
- `/workspaces/{workspaceId}/settings/provisioning` is restricted to workspace
  owners and admins. It shows the workspace SCIM endpoint and setup instructions,
  creates or rotates the dedicated SCIM credential with its secret shown once,
  and revokes it through ordinary CSRF-protected forms with confirmation. The
  page is no-store and has no WebMCP enhancement; SCIM Users and Groups remain
  REST/IdP-only.
- `/login`, `/setup`, password recovery, `/mfa`, and email verification share the generated app
  brand and compact responsive auth layout. Setup keeps required fields first
  and marks workspace language as deferrable. Sign-in, setup, TOTP, and
  recovery-code flows retain ordinary CSRF-protected forms without JavaScript;
  passkey authentication stays an optional enhancement. Password-reset request,
  completion, unavailable, and validation states use the same templ layout and
  ordinary forms. MFA keeps passkey enhancement plus TOTP and one-use recovery
  code form fallbacks. Invalid MFA attempts show localized retry feedback;
  missing or expired challenges return to sign-in. Email verification shows
  localized success and invalid-or-expired states with ordinary account and
  sign-in links. These pages use `auth.templ` and generated `auth_templ.go`.
- `/sessions` shows current, active, expired, and revoked browser sessions with
  localized status and revocation feedback. CSRF-protected forms revoke any
  active session, including the current one; revoking the current session sends
  the browser to sign-in on its next request. The page shows a localized empty
  state and retains the ordinary form fallback. The browser test covers expired
  session rejection and status, plus active-session revocation:
  `internal/browsertest/account_test.go`. `/security` uses `security.templ` and
  `security_templ.go` with the shared account layout to manage MFA factors,
  recovery codes, passkey removal, and reauthenticated OIDC linking. Factor removal
  uses progressive confirmation; one-time enrollment secrets appear only in their active states.
- `/account` edits profile preferences, changes a local password, requests email
  verification, and deletes the account after current-password reauthentication
  and an explicit confirmation checkbox. The server rejects deletion without
  confirmation and retains the owner-transfer restriction. `/settings/language`
  persists the account language. `/password-reset` provides localized generic
  request and single-use completion pages. `internal/browsertest/account_test.go`
  covers account, language, security, and sessions across locales, themes, and
  viewport sizes, plus profile save, session revoke, and no-JavaScript deletion
  confirmation.
- `/notifications` lists recipient-scoped in-app records, shows unread state,
  marks them read, and updates the unread count through recipient-scoped SSE.
  Its initial SSE cursor is the newest row rendered, or empty when the inbox is
  empty. It keeps a refresh link for ordinary navigation. Notification records
  are created through the shared identity service, and user plus workspace
  preferences are checked before storage.
- `/notifications/events` delivers recipient-scoped `notification.created` SSE
  events, replaying after the page cursor or the browser's last delivered event;
  an expired cursor triggers a page refresh to restore current records.
- `/workspaces/{workspaceId}/files` lists ready private workspace files with
  per-member column visibility, filename/MIME/size filters, stable sorting,
  numbered page links, and responsive table/card views; filter and sort
  controls use ordinary GET forms. `files.templ` composes the shared workspace
  shell, navigation, and DaisyUI list/form components. The list and download
  actions require workspace read permission. Only principals with
  `resources:write` see the CSRF-protected multipart upload form; read-only
  viewers can list and download files without upload controls. JavaScript sends
  the same form in one request through `FileUploader` (IDLE, UPLOADING, SUCCESS,
  and ERROR phases). The button and file input stay locked during upload. A
  native progress control reports computable byte progress; a polite live region
  announces quarter milestones, success, and localized safe errors for oversize,
  workspace-limit, or invalid-file failures. The ordinary form works without
  JavaScript. File paths carry short IDs; malformed, unknown, and integer-key
  IDs return not-found. Uploads stream through the app for local and S3 storage;
  S3 downloads redirect to a short-lived private URL with no-store headers, and
  local downloads stream through the app. No HTMX fragment is used on Files.
- `/workspaces/{workspaceId}/settings/export` renders the authorized export
  screen with templ; its ordinary `Download JSON` link downloads the export
  from `/workspaces/{workspaceId}/settings/export/download`. The download is
  synchronous, bounded, workspace-authorized, and sent with `no-store`.
- `/workspaces/{workspaceId}/settings/import` previews one bounded JSON export
  or strict UTF-8 `title,status` CSV before import. Preview validates every
  record, shows the count, up to five sample records, and up to ten actionable
  row errors without writing items. A valid preview offers an ordinary
  confirmation form; users select the same file again, and the server checks
  its digest before importing. Confirmation rechecks session CSRF, workspace
  permission, file limits, and item quota, then creates all rows atomically.
  Duplicate titles remain separate new items. Both steps work without
  JavaScript and render through templ with shared form, table, and alert
  components. Errors report safe categories and bounded row diagnostics without
  echoing file contents. Ordinary-form browser coverage for preview, no-write
  behavior, confirmation, and export is in
  [the workspace administration Playwright suite](../internal/browsertest/admin_test.go).
- `/workspaces/{workspaceId}/settings/audit` presents the authorized,
  read-only latest-100 audit history with redacted fields. Its permissions,
  empty/success states, and browser accessibility are covered by
  [audit handler tests](../internal/web/handler_test.go) and
  [accessibility evidence](accessibility.md#workspace-administration-and-data-evidence-2026-11-17).
- `/share/{token}` renders through `public_pages.templ` as a localized public
  card with shared alert components. It presents a read-only Item projection containing only title
  and localized status. Invalid, expired, and revoked links share the same
  unavailable page. The Items page lets principals with `resources:write`
  create seven- or 30-day links and revoke active links with CSRF-protected
  forms. Create returns the targeted list fragment; revoke uses ordinary
  redirect navigation. Share forms opt out of HTMX history snapshots so a
  one-time URL is not cached. The controls state what link holders can see and
  explain revocation consequences without JavaScript. Its shared list view
  persists each member's title and status column visibility per workspace,
  filters by title and status, sorts by creation time, and carries query state
  through pagination and realtime refreshes. The
  same rows adapt from a table to cards on narrow screens; inline title/status
  editing and authorized row actions remain available when either data column
  is hidden.

Forms keep ordinary navigation as fallback. HTMX enhances Items item fragments
and the Clients realtime list fragment.
Secret-bearing responses use `no-store`; list pages never render session,
CSRF, invitation, API-token, SCIM credential, or webhook secrets. Webhook delivery history excludes
delivery payloads and shows only bounded status, attempt, HTTP status, timestamps,
and redacted error categories.

Public sharing uses `private, no-store`, `no-referrer`, and
`X-Robots-Tag: noindex, nofollow, noarchive`. The existing per-source HTTP
rate limit applies to public access. There is no public write, search, listing,
REST, CLI, MCP, or WebMCP sharing surface.

`/oauth/authorize` renders consent and generic errors through `public_pages.templ`.
The consent page shows the client name, requested item permissions, and eligible
workspace choices in the shared card layout. Approve and deny remain ordinary
CSRF-protected form submissions; the page does not load WebMCP or submit
automatically. Invalid requests and unavailable workspace permissions use the
localized error page. These pages use `no-store` and are excluded from preload
and history caching.

The workspace sidebar exposes `Home` above `Items` in the workspace context.
Home is active at `/workspaces/{workspaceId}`. Settings uses the nested
`/workspaces/{workspaceId}/settings` route and groups `General` under Setup,
`Members & invitations`, `API tokens`, and `Webhooks` under Access, and `Export`
under Activity. Owners and admins also see `Email` under Setup, `Provisioning
(SCIM)` under Access, and `Import` and `Audit history` under Activity. The
Settings group is separated by flexible space and anchored at the bottom in
the workspace context. The members screen and invitations screen retain local
tabs behind the combined entry. In multi mode the Items page offers
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

Forms work with ordinary navigation. Vendored HTMX 4.0.1 enhances Items item
forms and the Clients realtime list refresh; it contains no business rules. `app.js` keeps
HTMX 2.x non-swap behavior for `204`, `304`, `4xx`, and `5xx` responses so
error pages never replace the list. The exact upstream license is served from
`internal/web/assets/HTMX-LICENSE`.

## Form enhancements

Every page runs the form enhancement from `internal/web/assets/alpine-components.js` (registered Alpine components on the `@alpinejs/csp` build) through the `pageBehaviors` mount on each shell body. Pages work without it: forms post, the server validates, and
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
`async-feedback`, `form-markup`, and `file-upload-form` skills under `.agents/skills/`. `make browser-smoke`
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
inventory below names each route family's current renderer and major states.
The current route inventory and named calls to `render()` map the known page
routes to generated templ views. `TestRenderCallSitesUseTemplSwitchCases`
parses production Go files and verifies every literal `h.render` call name has a
matching case in `Handler.render`'s switch; non-literal names fail the check.
This checks call-site names against dispatch cases, but does not execute each
view or prove that its renderer produces output. An unknown view name panics.
The legacy HTML files, the `html/template` fallback and the navigation bridge
were removed. See [issue #401 migration evidence](capabilities.md#setup-and-authentication-templ-migration-401) and
[issue #432 verification evidence](capabilities.md#rendering-assets-and-generated-app-verification-432).

Shared navigation uses `layouts/navigation.templ` directly. The workspace shell
composes those components without a bridge.

| Navigation surface | Decision and evidence | Route owner |
|---|---|---|
| Script messages and skip link | Shared on authenticated, auth, public, and error pages; `TestNavigationComponentsEscapeInput` checks escaping and message placeholders | #434; page migrations remain separate |
| Workspace context and breadcrumbs | Single mode omits workspace selection; multi mode retains the native switcher and ordinary links; `TestSingleModeHidesWorkspaceListNavigation` and `TestMultiModeKeepsWorkspaceListNavigation` | #410 and #434 |
| Workspace and settings navigation | Home, Items, Clients, Files and settings retain feature and role gates; `TestWebNavigationAndActionFeedback` checks current pages and settings | #434; resource routes remain separate |
| Member and invitation tabs | Both retain ordinary links and current-page state; `TestWebNavigationAndActionFeedback` | #434; member and invitation pages remain separate |
| Account menu | Native disclosure and CSRF-protected logout remain; `TestNavigationComponentsEscapeInput` checks escaping | #434; account pages use templ |

Empty, validation, pending and error states belong to the page routes above;
navigation remains present when those pages render it. Focused browser and
accessibility results and remaining limits are recorded in
[shared navigation migration evidence](capabilities.md#shared-navigation-migration-evidence-434-2026-11-16).

| Route family | Views and shared templates | Major states covered |
|---|---|---|
| `/`, `/workspaces/{workspaceId}` | `workspace.templ`, `workspace_templ.go`, `layouts/navigation.templ`, `components/components.templ` | Membership selection and creation, workspace home, unavailable workspace, setup-required and validation errors; native workspace switcher and shared templ navigation; CSRF-protected creation and ordinary-form fallback |
| `/setup`, `/login`, `/auth/oidc/{providerId}` and callback, `/password-reset[/{secret}]`, `/mfa`, `/account/verify-email/{secret}`, `/oauth/authorize` | `auth.templ`, `auth_templ.go`, `mfa.js`, `public_pages.templ` | Setup completion and validation, sign-in and provider choice, OIDC callback success/failure and denied workspace access, reset request/completion/unavailable/validation, MFA challenge/retry/expired redirect, verification success/invalid-or-expired, OAuth approval/denial; all setup/auth states dispatch through templ |
| `/workspaces/{workspaceId}/items` | `items.templ`, `items_templ.go`, `assets/items.js` | Empty/results states, filters, sorting and paging, shareable query state, column preferences, create/edit/delete, validation, sharing, read-only viewer and denied writes, realtime refresh with dirty-row and standalone form preservation, native query history; ordinary form fallback |
| Workspace clients and custom fields | `clients.templ` (list and detail), `client_fields.templ`, `components/components.templ`, `workspace.templ`, `layouts/navigation.templ` | Empty/results states, filters and paging, create/edit/delete, validation, custom-field and option lifecycle, realtime refresh; no client-family `html/template` views remain |
| Workspace members | `members.templ`, generated `members_templ.go`, shared navigation/components | Role changes, removal and ownership transfer; responsive table/cards, filters, sorting, paging, validation, permission denial, and ordinary-form confirmations |
| `/workspaces/{workspaceId}/settings/invitations`, `/invitations/{secret}`, `/invitations/{secret}/sign-in`, `/invitations/continue` | `invitations.templ`, `invitations_templ.go`, `workspace.templ`, `layouts/navigation.templ`, `components/components.templ` | Create/resend/revoke with explicit revoke confirmation; registration, account-matched acceptance, sign-in return through MFA/OIDC, invalid/expired/unavailable states; ordinary form fallback |
| `/workspaces/{workspaceId}/settings/audit` | `audit.templ`, `audit_templ.go`, `components/components.templ`, `components_templ.go` | Scoped redacted history, lazy table fragment, empty state, ordinary `?all=1` fallback, permission denial and expired-session redirect |
| `/sessions` | `sessions.templ`, `sessions_templ.go` | Current, active, expired and revoked status; revoke current or other active sessions; feedback and empty state |
| `/security` | `security.templ`, `security_templ.go`, shared account shell and form components | MFA enrollment/removal, recovery codes, passkey removal, linked identity with reauthentication, validation and failure feedback |
| `/account`, `/settings/language` | `account.templ`, `account_templ.go`, `layouts/account_header.templ`, `components/components.templ` | Profile, theme, language, notifications, email verification, password change, reauthenticated account deletion, saved/validation/delivery failure; ordinary CSRF-protected forms and password exclusion from drafts |
| Workspace files | `files.templ`, `workspace.templ`, `layouts/navigation.templ`, `components/components.templ` | Empty/results, filters, sort, saved columns and paging; read-only viewer list/download; write-authorized multipart upload progress/success/validation/failure with no-JavaScript fallback; authorized downloads |
| `/workspaces/{workspaceId}/settings/general` | `settings_pages.templ`, `settings_pages_templ.go`, `workspace.templ`, `layouts/navigation.templ`, `components/components.templ` | Workspace locale and notification preferences, empty and saved states, invalid input and permission handling |
| `/workspaces/{workspaceId}/settings/email` | `settings_pages.templ`, `settings_pages_templ.go`, `workspace.templ`, `layouts/navigation.templ`, `components/components.templ` | Unconfigured/configured provider, saved state, validation, provider verification failure, redacted credentials and permission handling |
| `/notifications`, `/notifications/events` | `notifications.templ`, `notifications_templ.go` | Recipient-scoped list and unread count, empty state, CSRF-protected mark-read, initial cursor and SSE replay after reconnect; verified for #407 |
| `/security` | `security.templ`, `security_templ.go`, `components/components.templ`, `layouts/navigation.templ` | Factor enrollment, removal and recovery, provider linking, no-provider state, validation/failure alerts, one-time secret display; ordinary form fallback |
| `/workspaces/{workspaceId}/settings/webhooks` and `/workspaces/{workspaceId}/settings/webhooks/{webhookId}/deliveries` | `webhooks.templ`, `webhooks_templ.go`, `components/components.templ`, `layouts/navigation.templ` | Permission-denied and unavailable states, one-time secret creation/rotation, required localized delete confirmation, no-store redacted delivery history, keyboard/accessibility, mobile light/dark themes, no-JS confirmation and ordinary form fallback |
| `/account`, `/settings/language` | `account.templ`, `account_templ.go`, `components/components.templ`, `layouts/navigation.templ` | Profile, theme, language, notifications, email verification, password change, reauthenticated account deletion, saved/validation/delivery failure; ordinary CSRF-protected forms and password exclusion from drafts; verified for #407 |
| Workspace access and integration settings | `credential_settings.templ`, `credential_settings_templ.go`, `security.templ`, `security_templ.go`, `layouts/navigation.templ` | Settings validation, secret one-time display, integration setup and failure, responsive navigation |
| `/workspaces/{workspaceId}/settings/export`, `/workspaces/{workspaceId}/settings/export/download`, `/workspaces/{workspaceId}/settings/import` | `settings_pages.templ`, `settings_pages_templ.go`, `workspace.templ`, `layouts/navigation.templ`, `components/components.templ` | authorized no-store export page and JSON download; import file selection, bounded preview, safe validation feedback, explicit multipart confirmation, and atomic create-only result; ordinary form fallback |
| `/share/{token}` | `public_pages.templ`, `public_pages_templ.go`, `components/components.templ` | Localized title/status-only share projection, invalid/expired/revoked unavailable state, no-store and referrer/robots headers |
| `/oauth/authorize` and generic errors | `public_pages.templ`, `public_pages_templ.go`, `auth.templ`, `components/components.templ` | Localized OAuth consent and generic errors; ordinary CSRF-protected approve/deny forms; invalid request and unavailable permission errors; no-store and preload/history-cache exclusion. `mcp_consent.go` and generic error dispatch call generated templ views; no `html/template` view remains for these routes |

Corresponding route and handler tests cover server-rendered behavior, including
public-share projection and unavailable states, consent approval and denial,
and generic OAuth errors.
`make browser-smoke` and `make accessibility-smoke` cover the browser paths they
enumerate; [the accessibility evidence](accessibility.md) records that
automated coverage. Issue #432 ran the `ui` and `components` Playwright suites;
these cover representative route rendering and reusable responsive components,
not every route family or all browser states. Workspace selection, workspace home, audit history, API token
and SCIM settings, webhook registry, and delivery history use templ. Issue #400
migrates and verifies the workspace route family (selection, creation, home,
switching, and shared navigation). The shared `render()` switch names each view
explicitly; an unrecognized name panics.
General and
email settings use `settings_pages.templ`; #406 route-family verification is
recorded in [workspace settings migration evidence](capabilities.md#workspace-settings-template-migration-evidence-406).
The AST test
covers production `h.render` call sites; it does not parse route handlers or
prove that every documented route inventory row is exercised. This does not
change escaping,
authorization, accessibility, localization, or ordinary-form requirements.

The issue #454 reconciliation maps each route family and major state to its
automated evidence, committed screenshot samples, and remaining unverified
states in the next section.

## Browser journey coverage reconciliation

Review date: 2026-11-18. Issue #431's route and navigation runs passed 113
Playwright checks (2 notification tests skipped by spec conditions) across
`ui.spec.mjs`, `clients-list.spec.mjs`, `files.spec.mjs`, `members.spec.mjs`,
`invitations.spec.mjs`, `settings.spec.mjs`, `notifications.spec.mjs`, and
`account.spec.mjs`.
It verified Home, Items, Clients, and Files within the shared shell, tested
Items back/forward query history, workspace Files navigation, role-sensitive
access, mobile layouts, keyboard focus, and representative no-JavaScript flows.
Only Items browser history was explicitly asserted; direct-link and permission
coverage is representative, not exhaustive for all route states. Two notification
checks were skipped under their fixture conditions. See [issue #431 evidence](capabilities.md#route-and-navigation-journey-verification-431-2026-11-18).

Verification for issue #433 ran on 2026-11-18. Full Chrome journey suite: 146
passed, 2 conditionally skipped; all committed screenshot assertions matched.
Automated axe reported zero violations for its enumerated English and Spanish
route scans. Keyboard/focus, reduced-motion, no-JavaScript, and request-failure
journeys passed where covered by the suite. See [dated verification evidence](capabilities.md#browser-verification-for-templ-route-migration-433-2026-11-18).

Review date: 2026-11-18. This inventory connects the route families and major
states above to automated browser or route checks. Browser checks run in pinned
Chrome against disposable SQLite fixtures. Route tests establish server
responses and policy; they do not establish every browser state listed beside
them. Paths below name evidence in the template repository; generated apps do
not include these Playwright specs or source tests.

| Route family and named states | Automated evidence in the template | Screenshot baselines | Remaining unverified states |
|---|---|---|---|
| `/setup`, `/login`, OIDC provider and callback; setup validation, login failure, OIDC callback failure/success and denied workspace | `internal/browsertest/ui_test.go`; `internal/web/auth_journeys_test.go`; `internal/web/oidc_test.go` | None committed for auth routes | No browser screenshot comparison for setup, login, or OIDC. MFA, email verification, and password-reset token success are not exercised as complete browser journeys. |
| `/`, `/workspaces/{workspaceId}`; workspace selection, creation, home, unavailable/setup-required | `internal/browsertest/ui_test.go`; `internal/web/workspace_mode_test.go` | `internal/browsertest/ui_test.go` | Unavailable-workspace and setup-required error views have route coverage, not screenshot baselines. |
| `/workspaces/{workspaceId}/items`; empty/results, filters, sorting, paging, create/edit/delete, validation, sharing, denied writes, refresh and query history | `internal/browsertest/ui_test.go`; `internal/web/items_list_test.go`; `internal/web/item_create_model_test.go` | `internal/browsertest/ui_test.go` | Dedicated browser screenshots cover the representative list state; every mutation and permission state is not captured. |
| Workspace clients and custom fields; list, validation, CRUD, custom-field lifecycle, refresh and draft preservation | `internal/browsertest/clients_list_test.go`; focused client route tests under `internal/web/` | `internal/browsertest/clients_list_test.go` | Client detail and custom-field pages have behavior tests but no screenshot baseline. |
| Workspace members; list, role change, removal, ownership transfer, denied actions | `internal/browsertest/members_test.go`; `internal/web/members_list_test.go`; `internal/web/members_role_test.go` | `internal/browsertest/members_test.go` | Full confirmation and denial state coverage is behavioral; screenshots represent the list. |
| Invitations and acceptance; create/resend/revoke, existing-account sign-in, invalid/unavailable guidance | `internal/web/no_javascript_journeys_test.go`; `internal/web/invitation_guidance_test.go`; `internal/web/management_test.go` | None committed | Browser journey covers acceptance without JavaScript; management and invalid/expired states have route tests, not screenshots. |
| `/workspaces/{workspaceId}/settings/audit`; scoped history, lazy fragment, empty, permission denial and expired session | `cmd/browsersmoke/accessibility.go`; `internal/web/handler_test.go` | None committed | Audit table fragment and denial states have route checks; no browser screenshot baseline. |
| `/sessions`; current/active/expired/revoked and revoke actions | `cmd/browsersmoke/accessibility.go`; `internal/web/handler_test.go` | None committed | No dedicated Playwright session lifecycle journey or screenshot baseline. |
| Workspace files; list/filter/sort, upload progress/success/validation/failure/retry, viewer download and no-JavaScript fallback | `internal/browsertest/files_test.go`; `internal/web/files_test.go` | `internal/browsertest/files_test.go` | Upload states are exercised behaviorally, not captured as separate screenshots. |
| General and email settings; save, validation, provider verification failure, read-only permission | `internal/browsertest/settings_test.go`; `internal/web/email_settings_test.go` | `internal/browsertest/settings_test.go` (email mobile also has Linux baselines) | Failure and permission variants are behavior checks, not screenshot baselines. |
| Account, language, security, tokens, webhooks, SCIM; validation, one-time secret, integration failure and permissions | `internal/browsertest/settings_test.go`; `cmd/browsersmoke/accessibility.go`; `internal/web/webhooks_test.go`; `internal/web/handler_test.go` | Token and provisioning settings have desktop/mobile light/dark snapshots; no baselines for account, language, security, or webhooks | One-time-secret and integration failure states have no screenshot comparisons. |
| `/notifications`, `/notifications/events`; empty/list, mark-read, reconnect/replay, Spanish no-JavaScript navigation | `internal/browsertest/admin_test.go`; `internal/web/notifications_test.go` | `internal/browsertest/admin_test.go` | Populated list and replay states have behavior checks, not screenshot baselines. |
| Export and import; authorized download, preview, invalid input, confirmation and atomic result | `cmd/browsersmoke/accessibility.go`; `internal/web/management_test.go` | None committed | Browser accessibility smoke checks import page and preview. Export download and import failure/result states rely on route and application tests; no Playwright journey or screenshots. |
| Public share, OAuth consent, not-found and request errors | `cmd/browsersmoke/accessibility.go`; `internal/web/handler_test.go`; `internal/web/mcp_consent_test.go` | None committed | Public share and consent are axe-scanned; no screenshot baselines. Generic request errors have route checks only. These routes use `public_pages.templ`; authentication route names also dispatch to templ views through the shared renderer. |

### Accessibility limits

`make accessibility-smoke` runs axe scans for English and Spanish on the routes
enumerated in `cmd/browsersmoke/accessibility.go`. The runner also checks selected
keyboard order, focus, accessible names, reduced motion, 320 CSS-pixel
overflow, and import/upload contracts. Focused Playwright specs add route-
specific axe and keyboard checks. These checks cannot establish full WCAG
conformance, screen-reader behavior across assistive technologies, 200% zoom,
or every state and locale/theme combination. The only recorded screen-reader
walkthrough is the historical VoiceOver observation in
`docs/accessibility.md`; it is not a repeatable automated check.

Screenshot assertions intentionally sample routes rather than every state and
combination. Captures disable animation. Reduced-motion behavior is checked
separately in the journey suite. `make browser-smoke` is the baseline refresh
gate; update committed images only when the rendered change is intentional and
review actual/diff artifacts before accepting them.

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

Test with `make webmcp-smoke`. The runner checks ordinary fallback in Chrome, the only
browser `cmd/browsersmoke/webmcp.go` drives. It runs native tool names, schema/redaction,
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
when their column is hidden. Sorting, filters, page size, and page stay in the
URL. Clear filters and reset columns are separate controls. Column and filter
forms work without JavaScript. Members, admins, and owners see the create form;
viewers see the list read-only. The create form stays open after validation
errors; active query state opens list options. The workspace shell uses the
generated product name and brand tokens. Header actions link to client creation
and custom-field management; results remain first, followed by create and list
options.

The Clients list page loads local HTMX and `app.js`. Its `client.changed` and
`resync` stream events refresh the list fragment while preserving current URL
state. Refresh waits while a list form is dirty, submitting, or focused. The
stream and list require `ResourcesRead`; without JavaScript, links and ordinary
forms continue to work.
`/workspaces/{workspaceId}/clients/{clientId}` renders with templ and shows one client. Writers get an
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
`/workspaces/{workspaceId}/client-fields` renders with templ and lists custom fields, archived ones
included. Owners and admins see the define form and the archive or restore
buttons; other roles see the list read-only. Writes are CSRF-protected POST
forms that redirect to the list. Labels are shown exactly as typed, escaped for
HTML. Permanent deletion is offered only for archived fields, with the effect on
all client values and choice options shown beside the control, including without
JavaScript. A refused label returns 422 and a reached limit returns 409, both on
the same page. The route uses generated `client_fields.templ` and shared
DaisyUI form, alert, and confirmation components. Choice fields expose option
add and delete forms to owners and admins; deleting an option warns that client
values using it will be cleared. All actions retain ordinary POST fallback.

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
The client page loads the local HTMX and `app.js` assets to start browser refresh;
the listener preserves URL state and defers replacement while forms are focused,
dirty, or submitting. The list fragment and stream both require
`identity.ResourcesRead`.

`GET /notifications/events` uses the same stream limits, heartbeat, lifetime,
and database polling. Events carry the notification ID, type, title, body, and
UTC creation time only to the authenticated recipient. The page gives
`EventSource` an `after` query cursor matching its newest rendered row; an empty
value replays rows created after an empty page. `Last-Event-ID` takes precedence
and resumes after the recipient's last delivered notification. The lifetime
`close` event ends the response; native `EventSource` reconnects with its last
event ID, or the original page cursor if no event arrived. An invalid cursor
triggers `resync`. No notification secret or other user's record is exposed.
The refresh link remains available.

Permanent delete removes the live item and writes a redacted audit/change
event. Existing backups may retain deleted content until the expiry defined in
[data lifecycle](data-lifecycle.md); live deletion does not rewrite prior
backups.
