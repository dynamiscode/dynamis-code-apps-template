# Items lifecycle

The sample feature slice: create an item (idempotent, one per press), edit
inline, share with a public link, autosave status, optimistic delete, toast
feedback, and the realtime list refresh.

## Sub-features

- Create: `#item-list > form.create` with `idempotency_key`; busy button
  shows "Saving..." plus a daisyUI `loading` spinner; double press makes
  one item.
- Edit: inline `.item-edit-form` grid; status select autosaves
  (`data-autosave`).
- Delete: optimistic hide (`data-optimistic="hidden"`), restored on
  failure with a toast.
- Share: scoped public link, revoke, public page renders without auth.
- Feedback: toasts in `.toasts` region, closeable by button/Escape/timeout.

## How to get to it (user POV)

Workspace home -> Items card, or the sidebar Items link.

## Driving it with the go browser suite

```
go test -tags browser -count=1 ./internal/browsertest/... \
  -run 'TestFormUXPatterns|TestItemsSharingCreatesAndRevokesPublicLink'
```

`TestFormUXPatterns` walks the whole journey in one tab: wizard, items
create double-press, validation summary, drafts. Pure evaluator checks run
without Chrome:

```
node --test internal/web/assets/items.test.mjs internal/web/assets/forms.test.mjs
```

The deep contract (TLA+ model, replay test) lives in the
`item-create-submission` skill - use it when the create form itself changes.

## Gotchas

- The toast container must keep the `toasts` marker class alongside the
  daisyUI `toast` classes; the suite selects `.toasts .toast`.
- The wizard buttons are `btn`-based now; the old
  `button:not(.secondary)` selector was replaced with
  `button:not(.btn-ghost)` - keep the back button ghost.
- Share rows render the `.success` alert only after the POST returns;
  wait for `role="status"`, don't poll a class.
