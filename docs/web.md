# Web and Realtime Interface

Workspace paths carry the workspace short ID. A malformed, unknown, or non-member
ID returns `404`, the same as a missing workspace. A member without the required
permission gets `403`.

The browser interface is server-rendered and reaches the same application use
cases as REST. Sign in at `/login`, create or choose a workspace, then use the
workspace home and sidebar. `/workspaces/{workspaceId}` is the workspace home;
it links to enabled workspace tools. Settings groups members, invitations, API tokens, webhooks, SCIM provisioning, export, and audit history.

Owners and admins manage the public intake link and consent text from Clients.
Visitors use `/intake/{token}` without signing in; the form follows their browser
language (English or Spanish) and returns a generic confirmation. A disabled or
unknown link, or a workspace without an open stage, is unavailable.
Accepted submissions create account notifications for owners and admins and
queue staff email. Both link to the new client without including submitted data.
Client details show a possible-duplicate warning when an intake submission
matches another client in the workspace by email or phone.

Catalog profile: `/workspaces/{workspaceId}/catalog` lists active entries and their unconverted price options for workspace resource readers, with an option to include archived entries. Owners and admins can create, edit, archive, unarchive, or delete unused entries and add, edit, or remove fixed or manual prices; members and viewers see no write controls. Fixed amounts are non-negative decimals, while manual options have no preset amount. Creation requires an initial price; removing the last price is rejected. New rows can only be added to active entries. To repair a legacy price-less entry, show archived entries, unarchive it if necessary, add its actual price, and archive it again if appropriate; no prices are invented. Entries reported in use cannot be deleted and should be archived, and booked entries are protected; sale linkage remains deferred. Catalog entries and prices are scoped to their workspace. Workspace home and workspace sidebar link to the enabled catalog. Catalog navigation was previously omitted from the sidebar because feature availability was not populated consistently across pages; keep the shared page data feature flag populated and mark the catalog page current.

When creating a catalog entry, an entered amount creates a fixed price; an empty amount creates a manual price. Currency and other price options are under advanced settings.

Workspace currency settings are available independently of the Catalog profile.
Resource readers can view enabled currencies; owners and admins can enable a
currency from the static list, disable currencies, and choose one default.
Search and selection work without JavaScript. Existing amounts retain their
currency and precision.

Catalog profile: the resource schedule shows bookings in the workspace time zone. Writers can select a catalog entry and start time; readers see the schedule without booking controls. Invalid, overlapping, and out-of-hours requests retain the draft and show an error.

Transactions administrators can open Manage transactions from the transactions menu, then choose
payment methods or expense categories. Both settings pages use the workspace
settings sidebar and link back to the transactions. Owners and admins can add or rename
methods and enable or disable them; at least one method must remain enabled.
They can add, rename, and archive expense categories. Disabled methods and
archived categories remain visible for historical entries. All settings forms
work without JavaScript and retain invalid drafts with feedback.

Transaction entries require a short description separate from the optional note.
Historical entries without one show a localized link to their details. Detail
dialogs preserve the list context, with ordinary navigation when JavaScript is
unavailable. Rejected submissions and client-picker requests retain the draft.
Owners and administrators can edit individual fields on active entries; each
save checks the entry revision and records an audit event. Conflicts retain the
draft for explicit retry. Currency changes preserve the exact numeric amount,
never exchange it, and reject precision loss.

Member and invitation pages explain role permissions, ownership transfer,
invited-email matching, and safe recovery for unavailable links.
Settings groups general and email setup, member access, API tokens, webhooks,
SCIM, exports, and audit history into role-aware sidebar sections.
For workspace resource readers, workspace home shows overdue and due-today action counts (UTC), up to five individual actions in each category, and paginated counts of work in open stages, including archived stages. Preview links open the engagement; view-all links open the corresponding action queue filter. Counts link to the queue or stage-filtered engagement list. These links work without JavaScript.
The Clients list links to the workspace engagement list. Owners and administrators
can open stage and label settings from the engagement list's settings menu.
The engagement list supports ordinary GET search, stage and assignee filters,
sortable headers, and bounded pagination; links open each engagement or its client.
Search matches literal substrings in engagement and client names, folds ASCII
letters only, and compares non-ASCII characters as typed on SQLite and PostgreSQL.
Writers can select engagements and confirm an atomic bulk move to an available
stage; the page-wide action is available without JavaScript. Workspace resource
readers can use the list without JavaScript. Writers can create an engagement
from the list by choosing a client, name, stage, and notes.
The client picker supports search and pagination without JavaScript. On the stage board,
writers can move an engagement using its labeled form; targets exclude archived
and current stages. A successful move returns to the selected board kind, including
without JavaScript. Readers do not see move controls.
Owners and admins can add, reorder,
and archive stages; workspace readers see their order and archive state. Archiving
requires explicit confirmation, explains that existing work stays in the stage,
and prevents moves into it. The forms work without JavaScript.
The approved stage-settings redesign groups stages in open, won, and lost order,
including empty drop targets. Active stages can be dragged within a group or
between groups; archived stages appear through the status filter and cannot be
dragged. Keyboard and no-JavaScript controls provide the same operations. A
cross-group move requires a server-rendered confirmation showing the stage, its
old and new types, and the count of all engagements in that stage. The server
rejects a stale stage snapshot or engagement count without applying the move.
The change preserves engagement and history rows. See the
[approved workflow decisions](engagement-workflow-decisions.md).
On a client detail page, full-width engagement tiles show the stage and link to each
engagement. Writers can open creation with that client preselected, with a
full-page fallback when JavaScript is unavailable. Engagement details combine
stage moves and action history in one paginated activity feed, newest first.
Entries include the actor and localized UTC time; unavailable identities are
labeled. Stage moves show archived stage names. Action events include the
recorded outcome, text, due date, and owner. Client details show the existing
authored timeline. On wide screens, a draggable divider adjusts the activity
column while preserving a minimum width for each column. Clicking the divider
opens a slider; arrow keys, Home, and End also resize the columns. On narrow
screens, activity follows the main content. Without JavaScript, the columns use
their default widths.
Writers can edit the engagement's quote and next action; readers see their summaries.
Invalid quote and next-action submissions
retain entered values and associate determinable field errors with their controls.
Unrecognized selection values appear as unavailable. Cancel returns to the saved
detail. Successful detail edits show localized confirmation after redirect, including without JavaScript. Writers can create an engagement with notes and move it to an
unarchived stage.
Detail pages show saved values until editing begins. Writers can edit engagement
titles, client name, email, phone, and custom text values in place. Client
contact fields save independently with a version check; a conflict retains the
draft and requires an explicit retry. Engagement stage and assignee, and
independently saved client choice fields, remain visible as selectors; saving
requires an explicit action. Related engagement, client, and catalog fields
use grouped editors. Ordinary forms remain available without JavaScript, and
rejected edits retain their drafts.
Notes support GitHub Flavored Markdown. On engagement details, writers see an
editable notes composer with explicit save and unsaved-change feedback; readers
see rendered notes. Writers can preview without saving, and the text area remains
usable without JavaScript. Notes preserve
whitespace and accept at most 5,000 Unicode code points. Rendering disables raw
HTML and unsafe links. Notes edits require workspace write permission, verify the
client relationship, and produce an audit event.
Creation requires a selected unarchived open stage. When none exists, administrators
can open stage setup; other writers are directed to an administrator. Client deletion warns that its
engagements and their history are permanently deleted too.
Archiving a client requires confirmation and can be cancelled without JavaScript.
Client creation keeps optional contact details under Advanced and warns before
discarding a dirty dialog. Timeline correction search and paging preserve the
composer draft without placing its text in the URL.
Owners and admins can set or reset the workspace work item label on the stages
page. The sidebar, engagement list, and client pages use that workspace's label;
the localized default is engagement.
Readers see next actions on client details; writers can set, change, or clear
them. Stale edit or clear submissions are rejected with conflict feedback, the
current action, and the submitted draft. Writers can review both and explicitly
retry against the observed current version. The Clients list links to the shared action queue. Its default view shows
open-stage actions due today or earlier (UTC); readers can filter by overdue,
today, upcoming, undated, completed, and owner, and navigate pages without
JavaScript. Upcoming, undated, and completed views include closed stages.
Writers can reschedule or complete an active action, or reopen a completed action,
from the queue without JavaScript. Rescheduling changes only the optional due date.
A stale reschedule or invalid date preserves the submitted date on a recovery page.
When an action remains, the page shows its current text and date and offers an
explicit retry against the observed version, even outside the original filter. If
no action remains, it shows the submitted date without a retry. Other stale actions
or occupied active slots produce conflict feedback; readers do not see these controls.
Readers see quote status, amount, and sent date on client details; writers can
update them. The status persists across stage changes.
Readers see the assignee on client details; writers can assign, change, or clear
the assignment using workspace members.
Readers see whether a signed agreement is recorded and its optional file short
ID. Writers can update both; the referenced file must belong to the workspace.
On the engagement detail page, writers with independent Files read access can
choose a ready file by name and short ID. Choices are paginated; page controls
preserve the unsaved checkbox, manual ID, and selected file without saving. The optional manual short-ID field
remains available, including without Files read access or JavaScript. A chosen
file takes precedence over the manual field.
`/account`, `/notifications`, `/settings/language`, `/sessions`, and `/security`
cover account and security settings. They share the authenticated drawer layout
and account navigation, distinct from workspace and settings navigation. Passing
no navigation previously removed the drawer and made account pages feel separate;
keep each account destination in the shared navigation and mark its current page.
Account pages share responsive panels and the user's light, dark, or system
theme. The top-bar theme menu saves only the theme with CSRF protection; using
the full profile update for this control would risk overwriting other preferences.
The menu returns to the originating path, validated as a local redirect; relying
on Referer previously returned users to workspace selection because the browser
suppresses it. Query parameters are not retained. Theme menu forms must stretch each button
across its full visible row: daisyUI's grid form layout previously left only
the text clickable. Dropdowns and modal boxes share a strong token border:
the workspace switcher previously blended into the identical dark sidebar surface. Account and workspace headers use the same top-bar component with their
own context labels; separate header markup previously drifted in wrapping and
alignment. Shared page preparation supplies the signed-in profile to all render
paths; loading it only in the legacy renderer previously made direct-rendered
pages show a question-mark avatar. The drawer overlay must be block-level: an inline overlay adds a
line box and creates a scrollbar even when navigation fits. Preserve scrolling
when the menu exceeds the viewport. Sidebar menus use neutral links and a
soft accent current-page state: the global link color previously overrode
daisyUI's active text against a dark background. Scope navigation state colors
to the sidebar and use existing light/dark tokens. The authenticated shell uses a top bar for brand, workspace
switching, and account actions, plus context-aware sidebars. Every mutation validates
the session-bound CSRF token
and rechecks current workspace permission.

The web code is split into five packages under `internal/web`:

- `viewmodel`: the `PageData` view model and the list, form, and filter row types templates read.
- `components/atoms`: one-element controls (inputs, selects, buttons, alerts, badges, dropdowns, tables, progress).
- `components/molecules`: composed controls (submit/confirm/link buttons, labelled text fields, search, headings, empty states).
- `components/organisms` and `components/templates`: feature regions (headers, navigation, list screens, filters) and the document shells (`UiHead`, the drawer workspace shell).
- `web` (the root): handlers, routes, and feature pages, which import the packages above.

Component conventions are recorded in the repository skills
(`templ-components`, `daisyui-tailwind`, `alpinejs-app`,
`htmx-patterns`).

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
Use linked-card grids for navigation choices. Keep short editors visible rather
than hidden in disclosures. Space cards, fields, and actions; let actions wrap.
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
  home, which links to enabled workspace tools and Settings.
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
- `/workspaces/{workspaceId}/settings/audit` presents the authorized,
  read-only latest-100 audit history with redacted fields. Its permissions,
  empty/success states, and browser accessibility are covered by
  [audit handler tests](../internal/web/handler_test.go) and
  [accessibility evidence](accessibility.md#workspace-administration-and-data-evidence-2026-11-17).
Forms keep ordinary navigation as fallback. HTMX enhances the Clients realtime list fragment.
Secret-bearing responses use `no-store`; list pages never render session,
CSRF, invitation, API-token, SCIM credential, or webhook secrets. Webhook delivery history excludes
delivery payloads and shows only bounded status, attempt, HTTP status, timestamps,
and redacted error categories.

`/oauth/authorize` renders consent and generic errors through `public_pages.templ`.
The consent page shows the client name, requested resource permissions, and eligible
workspace choices in the shared card layout. Approve and deny remain ordinary
CSRF-protected form submissions; the page does not load WebMCP or submit
automatically. Invalid requests and unavailable workspace permissions use the
localized error page. These pages use `no-store` and are excluded from preload
and history caching.

The workspace sidebar links Home, enabled resource screens, and role-gated Settings. Navigation and forms retain ordinary HTML fallback. HTMX enhances the Clients realtime list refresh without business rules. The realtime wiring preserves non-swap behavior for `204`, `304`, `4xx`, and `5xx` responses.

## Form enhancements

Every page runs the form enhancement from `internal/web/assets/alpine-components.js` (registered Alpine components on the `@alpinejs/csp` build) through the `pageBehaviors` mount on each shell body. Pages work without it: forms post, the server validates, and
errors render as `role="alert"`. With scripting, forms gain these behaviors,
each driven by a template attribute:

| Pattern | Attribute or place | Behavior |
|---|---|---|
| Busy state and double-submit prevention | every form | A submit marks the form `aria-busy`, disables its buttons with "Saving…" plus a daisyUI `loading` spinner, and cancels a second submit. The file upload form keeps its own state machine. |
| Inline validation and error summary | every form | Fields are checked when left and on submit; invalid fields get a message and `aria-invalid`, and a focused summary links to each one. A full-page server error takes focus on load. |
| Forgiving format | `data-trim`, MFA codes | Text is trimmed. The server accepts authenticator codes with spaces or dashes and recovery codes in any case. |
| Input masks | `data-mask="digits"`, `data-mask="upper"` | Authenticator codes keep digits only; recovery codes become upper case. |
| Password strength and reveal | `data-strength`, every password | New passwords must contain 12–1024 UTF-8 bytes. The meter scores 0 outside that range; a toggle shows or hides the value. `data-match` checks a confirmation. Without JavaScript, the server validates submissions. |
| Smart defaults | `data-default="timezone"`, token expiry | An empty timezone is filled from the browser; API tokens default to 90 days. |
| Dirty detection | `data-dirty-track`, `data-requires-change` | Save stays disabled until a field changes. |
| Unsaved-changes warning | `data-unsaved-warning` | Leaving a changed form asks first. |
| Draft persistence | `data-persist` | Unsent text is kept in `sessionStorage` (never passwords, hidden values, or files), restored after a rejected submit or a return visit, and discarded after a save. |
| Wizard | `data-wizard`, `fieldset data-step` | First-run setup shows one step at a time; Next validates the current step. |
| Progressive disclosure | `data-show-when="control=value"` | Email settings show only the chosen provider's section. |
| Lazy loading | `data-lazy-src` | The audit page renders first and fetches its table; without scripting, a link shows it. |
| Keyboard | skip link, Escape | The first Tab reaches "Skip to content"; Escape closes an open menu or the newest toast; focus returns to the same field after an htmx swap. |

## Browser journey and screenshot baselines

`make browser-smoke` runs real Chrome against a disposable server. The `form-drafts`, `form-field-input`, `form-wizard`, `async-feedback`, `form-markup`, and `file-upload-form` skills describe focused checks.

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

Shared navigation uses `components/organisms/navigation.templ` directly. The workspace shell
composes those components without a bridge.

`actionattrs.Scanned` buffers each wrapped component's complete output. Client,
member, and file list results no longer use the open `ListScreen` and
`ListResults` slots; the streaming regression exercises 1,500 rows. Saved-view
and column-menu panels also render direct children instead of buffering entire
panels; their streaming and original-output tests are
`TestListViewSwitcherStreamsPanel` and `TestListColumnsMenuStreamsPanel`. This is
not a global memory bound: open slots in atoms (including table rows,
infinite-scroll rows, dropdown panels, modal contents, tabs, drawers, and
resizable panels) and empty-state CTAs still buffer each supplied component.
Those components have no enforced output-size limit. Table and infinite-scroll
scan rows individually rather than buffering the entire row collection; neither
has a production caller outside the infinite-scroll table wrapper. The
dashboard shell selects a statically declared header and navigation from a
typed navigation kind. The workspace switcher renders each item directly;
`TestWorkspaceSwitcherStreamsItems` checks 1,500 items through the rendered
member page. The legacy list components still
buffer their supplied slots if invoked; the action checker rejects their direct
use in production Go and templ source. The typed check also rejects new
buffered slots reachable from an enabled list entry point.

List page-size, saved-view, column, filter, and row-action menus no longer
pass arbitrary components to the generic dropdown API. The page-size and
row-action menus iterate their typed data directly. Open panels render
statically named templates or compiler-generated template children.
Tests compare native and enhanced output with the former dropdowns.

Generic attribute spreads on non-action elements use
`actionattrs.ScalarAttributes`. It accepts native scalar values and preserves
their existing keys, class names and rendering order. Component values, scripts,
callbacks, other unknown types, and `srcdoc` are rejected before the spread is
emitted. Validation walks the attribute map once without buffering output or
imposing a rendered list-size limit. Action elements still use the stricter
action attribute allowlist.

| Navigation surface | Decision and evidence | Route owner |
|---|---|---|
| Script messages and skip link | Shared on authenticated, auth, public, and error pages; `TestNavigationComponentsEscapeInput` checks escaping and message placeholders | #434; page migrations remain separate |
| Workspace context and breadcrumbs | Single mode omits workspace selection; multi mode retains the native switcher and ordinary links; `TestSingleModeHidesWorkspaceListNavigation` and `TestMultiModeKeepsWorkspaceListNavigation` | #410 and #434 |
| Workspace and settings navigation | Home, Clients, Files and settings retain feature and role gates; `TestWebNavigationAndActionFeedback` checks current pages and settings | #434; resource routes remain separate |
| Member and invitation tabs | Both retain ordinary links and current-page state; `TestWebNavigationAndActionFeedback` | #434; member and invitation pages remain separate |
| Account menu | Native disclosure and CSRF-protected logout remain; `TestNavigationComponentsEscapeInput` checks escaping | #434; account pages use templ |

Empty, validation, pending and error states belong to the page routes above;
navigation remains present when those pages render it. Focused browser and
accessibility results and remaining limits are recorded in
[shared navigation migration evidence](capabilities.md#shared-navigation-migration-evidence-434-2026-11-16).

| Route family | Views and shared templates | Major states covered |
|---|---|---|
| `/`, `/workspaces/{workspaceId}` | `workspace.templ`, `workspace_templ.go`, `components/organisms/navigation.templ`, `components/molecules/` | Membership selection and creation, workspace home, unavailable workspace, setup-required and validation errors; native workspace switcher and shared templ navigation; CSRF-protected creation and ordinary-form fallback |
| `/setup`, `/login`, `/auth/oidc/{providerId}` and callback, `/password-reset[/{secret}]`, `/mfa`, `/account/verify-email/{secret}`, `/oauth/authorize` | `auth.templ`, `auth_templ.go`, `assets/alpine-components.js`, `public_pages.templ` | Setup completion and validation, sign-in and provider choice, OIDC callback success/failure and denied workspace access, reset request/completion/unavailable/validation, MFA challenge/retry/expired redirect, verification success/invalid-or-expired, OAuth approval/denial; all setup/auth states dispatch through templ |
| Workspace clients and custom fields | `clients.templ` (list and detail), `client_fields.templ`, `components/molecules/`, `workspace.templ`, `components/organisms/navigation.templ` | Empty/results states, filters and paging, create/edit/delete, validation, custom-field and option lifecycle, realtime refresh; no client-family `html/template` views remain |
| Workspace members | `members.templ`, generated `members_templ.go`, shared navigation/components | Role changes, removal and ownership transfer; responsive table/cards, filters, sorting, paging, validation, permission denial, and ordinary-form confirmations |
| `/workspaces/{workspaceId}/settings/invitations`, `/invitations/{secret}`, `/invitations/{secret}/sign-in`, `/invitations/continue` | `invitations.templ`, `invitations_templ.go`, `workspace.templ`, `components/organisms/navigation.templ`, `components/molecules/` | Create/resend/revoke with explicit revoke confirmation; registration, account-matched acceptance, sign-in return through MFA/OIDC, invalid/expired/unavailable states; ordinary form fallback |
| `/workspaces/{workspaceId}/settings/audit` | `audit.templ`, `audit_templ.go`, `components/molecules/` | Scoped redacted history, lazy table fragment, empty state, ordinary `?all=1` fallback, permission denial and expired-session redirect |
| `/sessions` | `sessions.templ`, `sessions_templ.go` | Current, active, expired and revoked status; revoke current or other active sessions; feedback and empty state |
| `/security` | `security.templ`, `security_templ.go`, shared account shell and form components | MFA enrollment/removal, recovery codes, passkey removal, linked identity with reauthentication, validation and failure feedback |
| `/account`, `/settings/language` | `account.templ`, `account_templ.go`, `components/organisms/account_header.templ`, `components/molecules/` | Profile, theme, language, notifications, email verification, password change, reauthenticated account deletion, saved/validation/delivery failure; ordinary CSRF-protected forms and password exclusion from drafts |
| Workspace files | `files.templ`, `workspace.templ`, `components/organisms/navigation.templ`, `components/molecules/` | Empty/results, filters, sort, saved columns and paging; read-only viewer list/download; write-authorized multipart upload progress/success/validation/failure with no-JavaScript fallback; authorized downloads |
| `/workspaces/{workspaceId}/settings/general` | `settings_pages.templ`, `settings_pages_templ.go`, `workspace.templ`, `components/organisms/navigation.templ`, `components/molecules/` | Workspace locale and notification preferences, empty and saved states, invalid input and permission handling |
| `/workspaces/{workspaceId}/settings/email` | `settings_pages.templ`, `settings_pages_templ.go`, `workspace.templ`, `components/organisms/navigation.templ`, `components/molecules/` | Unconfigured/configured provider, saved state, validation, provider verification failure, redacted credentials and permission handling |
| `/notifications`, `/notifications/events` | `notifications.templ`, `notifications_templ.go` | Recipient-scoped list and unread count, empty state, CSRF-protected mark-read, initial cursor and SSE replay after reconnect; verified for #407 |
| `/security` | `security.templ`, `security_templ.go`, `components/molecules/`, `components/organisms/navigation.templ` | Factor enrollment, removal and recovery, provider linking, no-provider state, validation/failure alerts, one-time secret display; ordinary form fallback |
| `/workspaces/{workspaceId}/settings/webhooks` and `/workspaces/{workspaceId}/settings/webhooks/{webhookId}/deliveries` | `webhooks.templ`, `webhooks_templ.go`, `components/molecules/`, `components/organisms/navigation.templ` | Permission-denied and unavailable states, one-time secret creation/rotation, required localized delete confirmation, no-store redacted delivery history, keyboard/accessibility, mobile light/dark themes, no-JS confirmation and ordinary form fallback |
| `/account`, `/settings/language` | `account.templ`, `account_templ.go`, `components/molecules/`, `components/organisms/navigation.templ` | Profile, theme, language, notifications, email verification, password change, reauthenticated account deletion, saved/validation/delivery failure; ordinary CSRF-protected forms and password exclusion from drafts; verified for #407 |
| Workspace access and integration settings | `credential_settings.templ`, `credential_settings_templ.go`, `security.templ`, `security_templ.go`, `components/organisms/navigation.templ` | Settings validation, secret one-time display, integration setup and failure, responsive navigation |
| `/workspaces/{workspaceId}/settings/export`, `/workspaces/{workspaceId}/settings/export/download` | `settings_pages.templ`, `components/molecules/` | Authorized no-store export page and JSON download; ordinary link fallback |
| `/oauth/authorize` and generic errors | `public_pages.templ`, `public_pages_templ.go`, `auth.templ`, `components/molecules/` | Localized OAuth consent and generic errors; ordinary CSRF-protected approve/deny forms; invalid request and unavailable permission errors; no-store and preload/history-cache exclusion. `mcp_consent.go` and generic error dispatch call generated templ views; no `html/template` view remains for these routes |

Corresponding route and handler tests cover server-rendered behavior, including
consent approval and denial,
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

Historical evidence below describes routes available at the time. Items, Item sharing, and Item import were subsequently retired; these observations do not describe current coverage.


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

Eligible pages load the WebMCP registration and mark visible controls. The current
tool contract is:

| Tool | Input schema | Page preparation |
|---|---|---|
| `workspace-create-v1` | `{name}` | Fill workspace name and focus the control |
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

The Clients list page loads local HTMX. Its `client.changed` and
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
The client page shows its manual timeline in chronological order, including author,
channel, and correction references. Writers can add entries through a localized,
CSRF-protected ordinary form; viewers see the timeline without the form. Rejected
entries retain the draft. Timeline displays 50 entries per page; select a page to correct an entry on that page. Timeline reads and writes use client permissions.
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

## Transactions expense categories

The Transactions page lets owners and admins create, rename and archive expense
categories. Staff may select an active category on a money-out entry; the
category is optional. History shows the current name after rename or archival.

Transactions readers can filter entries by client, payment method, category,
direction, and inclusive dates. The client page shows paginated transactions history
only to transactions readers; viewers see no money panel.

## Realtime contract

`GET /workspaces/{workspaceId}/clients/events` is the client list's stream. It uses bounded read permission, cursor, connection, heartbeat, and lifetime rules.
It sends `client.changed` with the client's short ID, schema version `1`, UTC
occurrence time, workspace ID, action (`created` or `updated`), and client
version. It carries no client fields. A missing, malformed, unknown, or expired
cursor receives `resync`. Viewers receive events, and a user outside the
workspace gets not found. The latest 1,000 client events per workspace are
retained; maintenance removes events older than seven days.
The client page loads the local HTMX asset to start browser refresh;
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
