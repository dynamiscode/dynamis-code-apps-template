# File upload

Private workspace files: pending -> uploading -> ready/failed lifecycle
with a quota reservation, byte progress on a daisyUI `progress` bar,
retry after failure, and viewer/download without upload controls.

## Sub-features

- Upload form: `form.file-upload-form`, `input[type=file]` with
  `#file-upload`, controls disabled while a request is in flight.
- Progress: `progress[data-upload-progress]` with phase messages
  localized (`data-status-uploading` etc.).
- Retry: failure shows the retry affordance; a retry resumes without
  double-submitting.
- Viewer: lists ready files, download links, no upload controls.
- Server contract: pending/ready/failed transitions and "a ready file
  has stored content" - see the `verify-file-upload` skill.

## How to get to it (user POV)

Workspace home -> Files card.

## Driving it with the go browser suite

```
go test -tags browser -count=1 ./internal/browsertest/... \
  -run 'TestFiles'
```

covers layouts (light/dark, desktop/mobile), OS-theme following, keyboard +
Spanish labels, axe, upload progress recovery, and the viewer. Unit-level
state machine without Chrome:

```
node --test internal/web/assets/upload-states.test.mjs
```

Deep contract: `file-upload-form` skill (browser side) and
`verify-file-upload` skill (server side).

## Gotchas

- `openFiles` signs in, opens the workspace home card, then navigates
  directly to the files URL - on mobile viewports the sidebar link is
  inside the closed drawer and cannot be clicked.
- The progress element keeps `hidden` until a transfer starts; drive by
  `data-upload-progress`, never by visibility of the bar.
- Files land under the instance's `STORAGE_LOCAL_PATH`; assert the stored
  bytes only via the download route, not the temp dir path.
