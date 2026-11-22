---
name: verify
description: Drive the real Dynamis Code app and prove behavior - launch the Go server with a disposable SQLite database, exercise a feature the way a user does through real Chrome, and capture evidence. Use for any "verify this works", "prove it", or pre-handoff proof on this repo, and to reproduce a browser-suite failure outside the suite. Covers the shell (drawer/navbar), theme system, clients list screens, and file upload; deeper mechanism skills (async-feedback, form-drafts, file-upload-form, form-field-input, form-markup, form-wizard) extend it.
---

# Verify the app by driving it

Two ways to drive. The **suite** boots everything itself and is the default:
one command, isolated server + real Chrome, exit code is the verdict. The
**manual** boot is for ad-hoc drives a test does not cover, and for
reproducing a failure interactively.

## Launch

### Suite mode (default)

```
go test -tags browser -count=1 ./internal/browsertest/... -run '<TestName>'
```

This builds `./cmd/server` and fixtures, bootstraps a fresh SQLite owner
(`playwright@example.com` / `playwright-password-123`, workspace "UI
Baselines"), seeds a member, starts the server on a free port, waits for
`/health/ready`, and drives real Chrome via chromedp. Nothing to launch by
hand; the whole instance is torn down by the test run. Prerequisite:
`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` (or
`/opt/google/chrome/chrome` on Linux) must exist.

Whole-suite verdicts:

```
make browser-smoke          # go browser suite
make accessibility-smoke    # cmd/browsersmoke axe/keyboard/reflow runner
make docker-smoke           # containerized end-to-end via scripts/docker-smoke.sh
make test-unit              # alpine-components.js evaluator tests (node)
make verify                 # the repo ladder
```

### Manual mode

Mirrors `scripts/accessibility-smoke.sh` and the harness in
`internal/browsertest/harness_test.go`:

```
work="$(mktemp -d /tmp/dynamis-verify.XXXXXX)"
BOOTSTRAP_ADMIN_PASSWORD=verify-password-123 \
  DATABASE_DRIVER=sqlite SQLITE_PATH="$work/app.db" \
  go run ./cmd/bootstrap-admin -email verify@example.com -workspace Verify
go build -o "$work/server" ./cmd/server
MULTI_WORKSPACE_ENABLED=true HTTP_REQUESTS_PER_MINUTE=10000 \
  HTTP_AUTH_REQUESTS_PER_MINUTE=1000 DATABASE_DRIVER=sqlite \
  SQLITE_PATH="$work/app.db" HTTP_ADDRESS="127.0.0.1:18080" \
  "$work/server" >"$work/server.log" 2>&1 &
echo $! >"$work/pid"
```

Ready means usable, not answering: poll `curl -sf
http://127.0.0.1:18080/health/ready` until 200, then load `/login` once and
confirm the sign-in form renders. The bootstrap already ran migrations and
seeded the workspace, so ready == signed-in-ready.

Teardown: `kill "$(cat "$work/pid")"` then `rm -rf "$work"` (see Cleanup).

## Doctor

Run before any drive that looks off, and always before blaming the app:

```
curl -sf http://127.0.0.1:18080/health/ready && echo READY
curl -sf http://127.0.0.1:18080/health/live && echo LIVE
```

- READY + LIVE + `/login` shows the sign-in form: instance worth driving.
- Not ready: `cat "$work/server.log"` before retrying anything.
- Login 401/400 during a drive: CSRF cookie missing - GET the form first,
  POST with its `csrf` value (the suite scripts show the exact fields).
- Wrong page state: the workspace locale may have been switched by a
  previous test in the same DB; manual mode uses a fresh DB, so a stale
  locale in suite mode means test leakage, not an app bug.

## Drive

Stable handles, in priority order: `data-*` attributes
(`data-workspace-home`, `data-upload-progress`, `data-list-announce`),
ARIA labels and roles, IDs (`#file-upload`, `#client-display-name`),
route paths (`/workspaces/<id>/clients`). Never tab order or coordinates.

The suite selectors are the canon - reuse them instead of inventing
selectors. Copy the exact `-run` names from the feature map
(`features/README.md`) for the surface under test. For ad-hoc drives use
curl for REST and the seeded cookie flow, or a real browser pointed at the
manual instance for UI. The app's own pure evaluators are directly
drivable without a browser:

```
node --test internal/web/assets/*.test.mjs
```

## Evidence

Capture to `/tmp/verify-evidence/<UTC-timestamp>/` - created before the
drive, never inside the instance's temp dir, so teardown cannot eat it:

- exit code of the exact command run (`go test ... -run X; echo $?`),
- full output of that command (`| tee /tmp/verify-evidence/<ts>/run.log`),
- for browser drives: the HTML of the final state plus the action that
  produced it, not just a screenshot - in suite mode the failing-node HTML
  and server log are printed by the harness on failure; save them from the
  output,
- for curl drives: request + response bodies side by side,
- side effects: the SQLite rows touched (query `"$work/app.db"` with
  `sqlite3` only after teardown is safe) or files written under the
  instance's `STORAGE_LOCAL_PATH`.

Proof standard: the real user path only - never internal setters,
`go test` package internals, or test-only endpoints. Action + resulting
state, not just the final screen. No mocks except where production already
isolates the boundary (the OIDC fixture is such a boundary).

## Cleanup

Never kill by process name. Kill what you started:

```
kill "$(cat "$work/pid")"; rm -rf "$work"
```

Suite mode cleans itself up; if a suite run was interrupted, the leftover
processes are `server` binaries whose cwd is a `/tmp/...` dir - find them
by that dir, not by name. Evidence under `/tmp/verify-evidence/` is never
deleted by cleanup.

## Helpers

None beyond the repo's own commands - that is the point: every drive is a
`go test`, a `make` target, or the manual boot recipe above, all of which
this file shows verbatim.
