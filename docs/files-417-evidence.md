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

Validation on tree `e16c943acd1e13869946c48f04b34bd98284dafd`, based on
`main` at `66d4fba8cffca88cce718445b5e3b7e9a949a3e3`:

- `PLAYWRIGHT_PORT=58419 ./scripts/playwright-smoke.sh --grep Files`: all 10
  Files journeys passed, including four screenshot comparisons, system-theme
  response to light/dark OS preference, keyboard/Spanish labels, axe, upload
  failure/retry, no-JavaScript fallback, and viewer download.
- `A11Y_PORT=58418 ./scripts/accessibility-smoke.sh` passed earlier in English
  and Spanish, with zero axe violations on Files and all audited routes. Its
  keyboard/focus, 320px reflow, reduced motion, and progress accessibility-tree
  checks passed. It was not rerun after #477, which only changed Items test
  diagnostics; Files axe and keyboard journeys passed on the current tree.
- Four desktop/mobile light/dark Files screenshots were generated, compared,
  and visually reviewed at `scripts/playwright/files.spec.mjs-snapshots/`.
- Focused Items and Files handler tests passed: 16 tests. Files pruning and
  Clients enabled/omitted generated-profile regressions passed: 3 tests.
- The combined `Core,Identity,Agent,Files,Clients` profile was generated on
  this tree. Its lock, Files and Clients surfaces, and both capability flags
  were verified; generated `go test ./...` passed 1,048 tests across 36 packages.
  This was an isolated generated-app check, not a persisted combined-profile test.
- `npm run test:unit`: 33 passed. `npm run check:js`, `make templ-check`, and
  `make css-check` passed with no generated drift.
- The normal pre-push hook passed `make ci-local`: lint, browser unit tests,
  Go race tests, dependency verification, generation checks, templ/CSS checks,
  build, and secret scan.
- Current-head GitHub CI had 9 of 10 checks passed and `go` in progress when
  this evidence was updated.

The current tree passed the full local pre-push checks. A previous `make verify`
also passed on the earlier base; it was not rerun after the #477 Items test
diagnostic-only change. A persistent combined Files plus Clients profile test
is not yet part of the repository.

Synthetic browser progress validates uploader UI transitions; it does not
measure native network-byte reporting.
