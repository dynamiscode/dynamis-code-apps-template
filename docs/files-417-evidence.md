# Files UI migration evidence

Issue: [#417](https://github.com/dynamiscode/dynamis-code-apps-template/issues/417)

## Routes and behavior

- `GET /workspaces/{workspaceId}/files` renders the Files page with the shared
  templ dashboard shell, DaisyUI form controls, and responsive table/card list.
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

Pending final branch verification. Browser journeys, screenshot comparison,
keyboard paths, and the full locale/theme/state matrix still need the serialized
browser-test window.
