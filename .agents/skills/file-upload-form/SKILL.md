---
name: file-upload-form
description: Check the browser side of the file upload form (FileUploader in internal/web/assets/app.js) - controls disabled by state, a status message per phase, double-submit prevention, error recovery and retry - by running the real class through every outcome of its one multipart request, with and without an extra press. Use when a change touches FileUploader, its phases or messages, the data-file-upload form in internal/web/templates/files.html, the forms.js early return for that form, or the multipart upload route it posts to.
---

# File upload form

## The rules

Checked after every step of every run:
1. **Controls follow the state.** The submit button and file input are disabled exactly in UPLOADING and SUCCESS. Harm if broken: a second upload starts, or the form stays locked after a failure.
2. **One press, one upload.** A press while the request is in flight sends nothing. Harm: duplicate files that hold quota.
3. **Every phase says what is happening.** The status text is non-empty and visible in every phase but IDLE, and only ERROR has the error style.
4. **Success leaves once, failure stays.** Success calls `location.assign(form.action)` exactly once; a failure never navigates.
5. **Retry works.** After any failure, the next press starts a fresh upload.

## Why exhaustive tests, not a model

The real class runs under Node with a fake `fetch` that the test releases on demand, so it checks the code itself. The space is small: the one POST ends {ok, HTTP error, network error}, with or without an extra press while it is in flight (6 runs), plus the no-file press.

forms.js returns early for `form[data-file-upload]`, before its own busy state, so `FileUploader` is the only owner of these controls. If that early return goes, forms.js keeps the form marked submitting after an upload error and blocks rule 5.

## Coverage record

- Covered code: `internal/web/assets/app.js` (`FileUploader`: `isInFlight`, `transition`, `handleSubmit`, `extractErrorMessage`), the `data-file-upload` early return in the forms.js submit handler.
- Check: `internal/web/assets/upload-states.test.mjs`.
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/file-upload-form/SKILL.md`). If `app.js` changed since, read the diff and confirm the outcome list and rules still describe it before trusting a pass, then update this file.

## Run the check

`node --test internal/web/assets/upload-states.test.mjs` (also part of `make test-unit`). Pass: `fail 0`. Under one second.

## Read a failure

Each test name is the outcome and the extra-press mask, for example `http extra-press-mask=1` (the POST fails, with one extra press while it is in flight). The message names the rule: `button disabled in ERROR` is rule 1, `extra press during request 0 sent a request` is rule 2, `retry did not start a new upload` is rule 5. If a new phase or request was added on purpose, extend `UPLOAD` or `IN_FLIGHT` in the test first.

## Proof the check catches a break

Measured on scratch changes, then reverted:
- `if (this.isInFlight()) return;` removed from `handleSubmit`: fails with `extra press during request 0 sent a request`.
- `this.submitButton.disabled = false` in `transition`: fails with `button disabled in UPLOADING`.

## Not covered

The server side of the upload (see the file upload model in `verification/files/`), real network timing, and a press after SUCCESS before navigation completes (the controls stay disabled, so it is blocked).
