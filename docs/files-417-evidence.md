# Files UI migration evidence

Issue: [#417](https://github.com/dynamiscode/dynamis-code-apps-template/issues/417)

## Routes and behavior

- `GET /workspaces/{workspaceId}/files` renders the Files page with the shared
  templ dashboard shell, DaisyUI form controls, and responsive table/card list.
- The Files route copies `Handler.filesEnabled` into the shared navigation
  data. Optional feature navigation uses this always-present capability flag,
  so generated profiles do not inspect pruned handler fields or templates.
- Query filters, sort links, saved columns, and cursor pagination remain ordinary
  GET links and CSRF-protected POST forms.
- `POST /workspaces/{workspaceId}/files` keeps one CSRF-protected multipart
  request for both JavaScript and native form submission. `FileUploader` keeps
  byte progress, pending control locks, localized feedback, retry, and redirect
  after success.
- `GET /workspaces/{workspaceId}/files/{fileId}/content` remains an ordinary
  authorized download link. Local storage streams content; S3 redirects to a
  short-lived signed URL with `private, no-store`. The page does not render or
  persist signed URLs.
- No HTMX behavior is added. This follows the Files decision in
  [the HTMX 4 review](htmx-review-436.md): keep native navigation and upload
  progress until an equivalent enhancement is proven.

## Regression evidence

- `TestFilesPageRendersTemplShellAndAccessibleEmptyState` checks the shared
  shell, empty state, responsive list variants, no-store response, locale, and
  absence of HTMX attributes or signed URL markers.
- `TestFilesPageRendersOrdinaryMultipartUploadForm` checks the native multipart
  form, CSRF field, and absence of presigned-upload fields.
- `TestFilesListUsesSharedPresentationAndSavedColumns` checks filters, sorting,
  saved columns, and cursor navigation.
- Existing route tests cover upload success, localized validation failures,
  viewer upload denial, and authorized, unauthorized, and missing downloads.
- `internal/web/assets/upload-states.test.mjs` exercises progress, locking,
  duplicate-submit prevention, success, failures, and retry.
- `scripts/playwright/files.spec.mjs` checks Files with axe, keyboard and
  Spanish labels, no-JavaScript upload/filter/error handling, viewer downloads,
  and desktop/mobile light/dark screenshots. Its upload progress journey emits
  a synthetic 50% `ProgressEvent` from a wrapped XHR and captures the uploader's
  progress listener instead of registering it for native events. It verifies UI
  progress, pending locks, failure recovery, and server retry deterministically;
  it does not measure actual browser network-byte progress.

## Verification

Passed:

- `PLAYWRIGHT_PORT=58117 make browser-smoke`: 23 Playwright journeys passed,
  including Files screenshots, keyboard/Spanish labels, axe checks, upload
  failure and retry, no-JavaScript fallback, and viewer download.
- `A11Y_PORT=58118 make accessibility-smoke`: English and Spanish runs reported
  zero axe violations on Files and all audited routes. Keyboard/focus, 320px
  reflow, reduced motion, and progress accessibility-tree checks passed.
- Four desktop/mobile light/dark Files screenshots were generated and visually
  reviewed at `scripts/playwright/files.spec.mjs-snapshots/`.
- `go test ./cmd/template-init -run
  '^TestGenerateWithoutFilesPrunesFilesTemplAndBuilds$' -count=1` passed.
  The regression checks that Files routes, evidence, browser spec, and
  screenshot baselines are pruned. Generated no-Files application build passed.
- Files omitted, Core+Identity omitted, Clients enabled, and Clients omitted
  generated-profile tests passed after integrating the stable feature flags.
- Focused Files route, upload-state, templ drift, Playwright syntax, and diff
  checks passed.

Pending: combined Files plus Clients generated-profile coverage from #415, then
full `make test`, race suite, and `make verify` after final base integration.
Synthetic browser progress validates uploader UI transitions; it does not
measure native network-byte reporting.
