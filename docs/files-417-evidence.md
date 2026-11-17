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
  desktop/mobile light/dark screenshots, and system-theme response to OS
  preference changes. Its upload progress journey emits
  a synthetic 50% `ProgressEvent` from a wrapped XHR and captures the uploader's
  progress listener instead of registering it for native events. It verifies UI
  progress, pending locks, failure recovery, and server retry deterministically;
  it does not measure actual browser network-byte progress.

## Verification

Passed:

- `PLAYWRIGHT_PORT=58417 ./scripts/playwright-smoke.sh --grep Files`: all 10
  Files journeys passed, including four screenshot comparisons, system-theme
  response to light/dark OS preference, keyboard/Spanish labels, axe, upload
  failure/retry, no-JavaScript fallback, and viewer download.
- `A11Y_PORT=58418 ./scripts/accessibility-smoke.sh`: English and Spanish runs reported
  zero axe violations on Files and all audited routes. Keyboard/focus, 320px
  reflow, reduced motion, and progress accessibility-tree checks passed.
- Four desktop/mobile light/dark Files screenshots were generated and visually
  reviewed at `scripts/playwright/files.spec.mjs-snapshots/`.
- Generated-profile regressions passed for Files pruning and Clients enabled
  and omitted profiles; the Files test confirms its route, evidence, browser
  spec, and screenshot baselines are pruned.
- `go test ./internal/web -run '^(TestFile|TestFiles)' -count=1`: 14 passed.
  `npm run test:unit`: 33 passed; JavaScript and Playwright syntax checks passed.
- `make templ-check` and `make css-check` passed with no generated drift.
- `make verify` passed: Go tests, race tests, lint, dependency/workflow checks,
  API generation, templ/CSS drift, builds, and generated-app template smoke.
  This run preceded local integration of refreshed #422 head `7c87ad9`.

Pending: refreshed #422 CI, combined Files plus Clients generated-profile
coverage from #415, and final validation after dependency ancestry integration.
Synthetic browser progress validates uploader UI transitions; it does not
measure native network-byte reporting.
