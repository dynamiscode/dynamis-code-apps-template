---
name: async-feedback
description: Check feedback for work that finishes later - busy buttons while a form submits, optimistic delete that hides a row at once and brings it back on failure, toasts for saves and failures, autosave of an item's status, the lazily loaded audit table, and keyboard focus after swaps (skip link, Escape, focus restore) - with a TLA+ model, exhaustive tests, and browser checks. Use when a change touches setBusy, optimisticOutcome, toast, loadLazy, the htmx event handlers or the keyboard handler in internal/web/assets/forms.js, data-optimistic, data-autosave or data-lazy-src in internal/web/templates/, itemToastKey or asciiJSON in internal/web/handler.go, or auditHistoryPage in internal/web/management.go.
---

# Async feedback

## The rules

1. **ConfirmFirst.** A delete is sent only after OK in the confirm dialog.
2. **HiddenOnlyWhilePending.** An optimistically hidden row stays hidden only while its delete is on its way or after it succeeded. Harm: an item looks deleted but is not.
3. **FailureVisible / ListMatchesServer.** On a failure (5xx, network) or a stale version (412), the row comes back and a toast or alert says why; otherwise the list matches the server.
4. **Busy while sending.** A submitting form marks `aria-busy`, disables its buttons and shows "Saving"; it is restored when the request ends or the page comes back from the back/forward cache. (The item create form is checked in depth by `item-create-submission`.)
5. **Toasts are polite and dismissible.** Saves (`HX-Trigger: app:toast`, ASCII-escaped by `asciiJSON`) and failures show in one `role="status"` region; they close on their button, on Escape, or after 6 seconds.
6. **Autosave** of an item's status sends the row as soon as the select changes. **Lazy loading:** the audit page renders its shell first and fetches the table; without JavaScript, a link shows it; a failed fetch says so and keeps the link.
7. **Keyboard:** the first Tab reaches the skip link, Escape closes an open menu and returns focus to it, and after an htmx swap focus returns to the same field or to the new error.

## What is modeled

- Model: `verification/web/ItemDelete.tla`, config `verification/web/ItemDelete.cfg`; 18 distinct states. Actions → code: `YouPressDelete`/`YouCancel`/`YouConfirm` (forms.js capture click handler, `window.confirm`, `data-optimistic="hidden"`), `ServerAnswers` and `OtherUserDeletes` (`changeItem`), `AnswerArrives` (htmx swap, or `htmx:finally:request` → `optimisticOutcome`).
- `optimisticOutcome` is tested for every status 100–599 and missing answers in `forms.test.mjs`.
- Browser checks (`scripts/ux-patterns.mjs`, tagged `async-feedback`): busy Add button, add toast, autosave keeps the status, delete hides the row before the answer, lazy audit table, Escape closes the account menu; `scripts/accessibility.mjs` checks the skip link is the first Tab stop.
- Left out: toast timing, the network-failure toast in a browser (covered by `optimisticOutcome` and the model), realtime refresh (see `form-drafts`).

## Coverage record

- Covered code: `internal/web/assets/forms.js` (`setBusy`, `optimisticOutcome`, `toast`, `loadLazy`, click/keydown/pageshow handlers, `htmx:before:request`, `htmx:finally:request`, `htmx:after:swap`, `app:toast`), `internal/web/templates/item-list.html` (delete and status controls), `internal/web/templates/audit-history.html` (`audit-table`), `internal/web/handler.go` (`itemToastKey`, `asciiJSON`, `render` HX-Trigger), `internal/web/management.go` (`auditHistoryPage` lazy and fragment paths), `internal/web/templates/navigation.html` (`ui-head`, `skip-link`).
- Go check: `TestWebAuditHistoryIsScopedRedactedAndNotWebMCP` covers the lazy shell, the fragment, and the no-JavaScript `?all=1` page.
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/async-feedback/SKILL.md`). Diff the covered files against it before trusting a pass, then update this file.

## Run the check

1. `verification/tools/run-tlc.sh verification/web ItemDelete` → `Model checking completed. No error has been found.`
2. `node --test internal/web/assets/forms.test.mjs` and `go test ./internal/web/ -count=1` → pass.
3. `make ux-smoke` and `make accessibility-smoke` → pass.
4. Read a counterexample or redraw `docs/statecharts/item-delete/chart/chart.mmd` with the `/statechart` skill and `docs/statecharts/item-delete/view.js`.

## Read a failure

- **TLC** `HiddenOnlyWhilePending`: a path where a row stays hidden after its request ended; check the `htmx:finally:request` handler and `optimisticOutcome`.
- **forms.test.mjs `optimistic removal`** prints the status that kept a row hidden.
- **ux smoke `async-feedback: ...`** names the pattern whose wiring changed.

## Proof the check catches a break

Measured on scratch changes, then reverted: `optimisticOutcome` always "removed" failed `forms.test.mjs`; the model leaving the row hidden after a failure failed `HiddenOnlyWhilePending`; forms.js without `setBusy` failed the ux smoke.
