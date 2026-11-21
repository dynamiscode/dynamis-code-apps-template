---
name: daisyui-tailwind
description: How this repo styles the web UI - daisyUI components and Tailwind utilities on elements, tokens.css as the token layer, the system-theme rule (data-theme omitted for system), and what may live in app.css. Use when adding or changing any styling, class, or theme behavior, or when deciding where new CSS goes.
---

# daisyUI + Tailwind styles

## The pipeline

`internal/web/tailwind.css` is the source: Tailwind v4 with
`@plugin "daisyui"` (themes `light --default, dark --prefersdark`) and
`@source` scoped to the templ trees. Build:

```
npm run build:css   # -> internal/web/assets/components.css (committed)
```

Load order in `UiHead`: `tokens.css` (token layer) -> `components.css`
(daisyUI/Tailwind build) -> `app.css` (remainder) -> feature CSS
(`clients.css`). Never edit `components.css` by hand.

## The rules

1. **daisyUI component first.** A card is `card` (+
   `card-compact`/`card-body` as needed), a badge `badge badge-*`, a
   toast `toast toast-bottom toast-end` holding `alert` items, feedback
   `alert alert-error|warning|success|info`, buttons `btn btn-*`,
   the shell `drawer lg:drawer-open` + `navbar`, form actions keep
   `loading loading-spinner` while busy, file progress
   `progress progress-primary`.
2. **Tailwind utilities second** - on the element, in the component's
   `Class` prop: `flex`, `grid`, `gap-*`, `mx-auto w-full max-w-xl`,
   responsive `max-md:*`, arbitrary values like
   `grid-cols-[repeat(auto-fit,minmax(min(100%,15rem),1fr))]` when a
   daisyUI class does not exist.
3. **Tokens, not hex.** Colors come through `tokens.css` vars
   (`--canvas`, `--ink`, `--accent`, `--danger`, ...) or daisyUI semantic
   names (`bg-base-100`, `text-base-content`). No new hex literals in
   markup; brand values belong in `tokens.css`.
4. **Theme contract.** `data-theme` is `light`, `dark`, or omitted:
   the `system` preference renders NO `data-theme` attribute so daisyUI's
   `--prefersdark` follows `prefers-color-scheme` in step with the dark
   block in `tokens.css`. Never emit `data-theme="system"` - it is an
   unknown daisyUI theme and once rendered half-light cards under a dark
   OS. `main form .btn { min-height: 2.75rem; }` is the 44px touch
   contract the accessibility smoke asserts; do not remove.
5. **No new custom CSS without an audit row.** `app.css` holds only
   a11y states, JS-hook states, pseudo-content, and structural layout -
   every remaining rule is dispositioned in `daisyui-style-audit.md`
   ("Final custom-CSS map"). A new rule needs a row there with its
   reason, or utilities instead.

## Conventions kept from the adoption

- Marker classes stay next to daisyUI classes (`class="settings-panel
  card min-w-0 p-5"`) - tests and JS select by the marker.
- `sr-only` (not `visually-hidden`) for screen-reader-only text.
- Muted text is `text-base-content/60` or the `.eyebrow` rule - never a
  new opacity utility on body text without a contrast check.

## Checks

- `npm run build:css` after any `tailwind.css`/markup class change;
  commit the built `components.css`.
- `make css-check` (build is deterministic), `make lint`, then the
  browser suite subset touching your pages, then
  `make accessibility-smoke` for anything visual.
- A contrast change under dark OS matters: headless Chrome here defaults
  `prefers-color-scheme: dark` - test both.
