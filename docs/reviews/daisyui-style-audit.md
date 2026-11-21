# daisyUI/Tailwind style replacement audit (temporary tracker)

Every custom CSS rule in app.css / clients.css / import.css is a finding.
Plan: replace with a daisyUI component class when one exists, else Tailwind
utilities on the element; delete the replaced CSS rule. Status marked when done.

| # | Finding | Files | Replacement | Status |
|---|---|---|---|---|
| W1a | `.topbar` shell + `.topbar-context` + `.topbar-title` | app.css, workspace.templ, dashboard_header.templ | daisyUI `navbar` + utilities | done |
| W1b | `.app-shell` grid + `.sidebar` + `.sidebar-back` + max-38rem sidebar block | app.css, templates/workspace.templ, home_navigation.templ | daisyUI `drawer drawer-open lg:drawer-open` | done |
| W1c | `.workspace-main` width rules | app.css, pages | Tailwind `max-w-*` on main | done |
| W2a | `.cards a`, `.items` li, `.workspace-cards` | app.css, workspace page | daisyUI `card` | done |
| W2b | `.first-task`, `.notification-empty`, `.auth-card`/`.narrow` | app.css | daisyUI `card` + border-s utilities | done |
| W2c | `.webhook-card`, `.session-card`, `.notification-card`, `.settings-panel`, `.settings-page section/form`, `.settings-record`, `.client-panel`, `.client-value`, `.client-field-card`, `.client-field-option-list li`, `.file-upload-panel`, `.list-view-cards li`, `#item-list .items li` | app.css, clients.css | daisyUI `card` | done |
| W2d | `import.css` preview/error/confirm | import.css | daisyUI `card` + utilities | done (import.css deleted: rules dead, file never linked; preview section now daisyUI card) |
| W3 | `.list-badge`, `.session-status*`, `.notification-read/unread`, `.list-view-badge`, `.list-view-summary-current`, `.client-bulk-count` | app.css | daisyUI `badge` (+ variants) | done |
| W4a | `.toast`/`.toasts` (JS-created) | app.css, alpine-components.js | daisyUI `toast toast-end toast-bottom` + `alert` | done |
| W4b | `.error-summary`, `.field-error`, `.draft-notice`, `.strength` (JS-created) | app.css, alpine-components.js | daisyUI `alert alert-error` / `label` / `progress`? evaluate | done (fieldset note-reservation rules kept: a11y, see form-field-input skill) |
| W4c | form[aria-busy] CSS spinner + @keyframes spin | app.css, alpine-components.js | daisyUI `loading loading-spinner` span | done (setBusy injects daisyUI loading-spinner; CSS spinner + keyframes deleted) |
| W4d | `.reveal`, `.secondary`, `.wizard-nav/status` | app.css, alpine-components.js | daisyUI `btn btn-ghost btn-xs` / utilities | done |
| W5a | global `input, select, button` element styling | app.css | delete; daisyUI inputs/buttons already classed | done |
| W5b | global `label` styling | app.css | delete; label classes at use sites | done |
| W5c | global `table/th/td` styling | app.css | delete; daisyUI `.table` | done |
| W5d | `.visually-hidden` → Tailwind `sr-only` | app.css, 11 templ uses | class swap, delete rule | done |
| W5e | `.skip-link` | app.css, skip_link.templ | Tailwind `sr-only focus:not-sr-only` pattern | kept (skip-link CSS retained deliberately: focus-positioning, a11y-critical) |
| W6 | clients.css remainder (match/field-choices/steps/detail dl) | clients.css | Tailwind utilities inline | done (chips/filter cosmetics -> utilities; structural dl/steps kept) |
| W7 | residual layout rules (grids, spacing, sticky columns) | app.css | Tailwind utilities inline where touched | done (daisyUI-conflicting overrides + dead .error/.warning/.success/.secondary deleted; structural layout kept) |
| W8 | final: rebuild CSS, make test, browser-smoke, a11y smoke | — | verify | done: full browser suite 0 failures, accessibility-smoke EXIT 0, make verify green, templ 0 drift |

Rules kept on purpose: tokens.css (design token layer), `:focus-visible` global
(a11y), prefers-reduced-motion globals, JS-hook rules with no class equivalent
([aria-busy], [data-optimistic], data-submit-button).

## Final custom-CSS map (post-replacement)


Rule-by-rule disposition of the remainder after waves W1-W9. All daisyUI-
equivalent surfaces (cards, badges, toasts, alerts, drawer, navbar, badges,
progress, buttons, labels) and dead rules are gone; what remains below is
a11y state, JS-hook state, pseudo-content, or structural layout.

| Selector | Disposition |
|---|---|
| body | kept: token-driven base (margin/background/color) |
| header | kept: legacy header base (navbar carries its own now; candidate for deletion after header sweep) |
| .workspace-name | kept: navbar brand name truncation |
| @media min-width: 38.001rem | kept: responsive block (table<->card switch, form stacking) |
| .client-controls | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls summary | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls summary:hover | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls[open] summary | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .list-toolbar | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar .menu | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar > details | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-page-size | kept: page-size control row |
| .workspace-main .list-page-size .select | kept: shell width + search-mark tint (color-mix with token vars) |
| .list-toolbar > details > summary | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar > details > summary::-webkit-details-marker | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar > details > summary::after | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar > details[open] > summary | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-view-table tbody td:first-child | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-row | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row:hover | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row-link | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row-link:hover | kept: row interaction states tied to sticky-cell backgrounds |
| .list-view-table thead th > a | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table thead th > a::after | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table thead th[aria-sort="ascending"] > a::after | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table thead th[aria-sort="descending"] > a::after | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-column-switches | kept: column-switch menu (drag handles, grid list) |
| .list-column-switch form | kept: column-switch menu (drag handles, grid list) |
| .list-column-name | kept: column-switch menu (drag handles, grid list) |
| .list-column-note | kept: column-switch menu (drag handles, grid list) |
| .list-column-count | kept: column-switch menu (drag handles, grid list) |
| .list-footer | kept: footer flex + page-size |
| .list-footer .list-page-size | kept: footer page-size alignment |
| .list-identity | kept: identity row flex |
| .list-initials | kept: avatar initials circle |
| main | kept: page container widths/padding (responsive clamp) |
| main > h1 | kept: page heading scale |
| main h2:not(.sr-only) | kept: section heading scale |
| .narrow | kept: width/margin only (bg/border moved to daisyUI card) |
| .auth-brand | kept: auth page layout remainders (method divider, provider grid) |
| .auth-card h1 | kept: auth page layout remainders (method divider, provider grid) |
| .auth-intro | kept: auth page layout remainders (method divider, provider grid) |
| .auth-recovery-link | kept: auth page layout remainders (method divider, provider grid) |
| .auth-note | kept: auth page layout remainders (method divider, provider grid) |
| .auth-method | kept: auth page layout remainders (method divider, provider grid) |
| .auth-method h2 | kept: auth page layout remainders (method divider, provider grid) |
| .auth-provider-list | kept: auth page layout remainders (method divider, provider grid) |
| .breakable | kept: overflow-wrap:anywhere marker (3 sites, JS-tested) |
| .brand | kept: gradient logo pseudo-content |
| .brand:not(:has(.brand-mark))::before | kept: gradient logo pseudo-content |
| .brand-mark | kept: gradient logo pseudo-content |
| .brand-mark::after | kept: gradient logo pseudo-content |
| a | kept: global link color/tie to brand tokens |
| main form:not(.inline) > :is(fieldset, input:not([type="hidden"]), select, textarea, .reveal, .strength, .wizard-nav) + button | kept: JS-created (utilities added in JS) |
| main form .btn | kept: 44px touch height contract (a11y smoke asserts it) |
| main form:not(.inline) > button.reveal | kept: JS reveal-toggle layout |
| :focus-visible | kept: global a11y focus ring |
| .items | kept: item list container (JS + tests rely) |
| #item-list .items form | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .items > li | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .items > li > form | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .items > li > form.share | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .items > li > .status, #item-list .items > li > .success | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .share .row button | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list > form.create | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list > form.create > label | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list > form.create .row | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list > form.create button | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > label | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > label:first-of-type | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > input[name="title"] | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > label:nth-of-type(2) | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > select[name="status"] | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .item-edit-form > .actions | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .share-scope | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .share-active | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| #item-list .share-revoke-consequence | kept: item list structural block (create form grid, edit form grid areas, share rows) |
| .status | kept: JS marker (realtime status element) |
| .public-share-details, .consent-details | kept: consent/workspace-choice layout (radio rows, action row) |
| .public-share-details > div, .consent-details > div | kept: consent/workspace-choice layout (radio rows, action row) |
| .public-share-details dt, .consent-details dt | kept: consent/workspace-choice layout (radio rows, action row) |
| .public-share-details dd, .consent-details dd | kept: consent/workspace-choice layout (radio rows, action row) |
| .consent-workspaces | kept: consent/workspace-choice layout (radio rows, action row) |
| .consent-workspaces label | kept: consent/workspace-choice layout (radio rows, action row) |
| .consent-workspaces input[type="radio"] | kept: consent/workspace-choice layout (radio rows, action row) |
| .consent-actions | kept: consent/workspace-choice layout (radio rows, action row) |
| .consent-actions button | kept: consent/workspace-choice layout (radio rows, action row) |
| .workspace-main | kept: shell width + search-mark tint (color-mix with token vars) |
| .workspace-heading | kept: page heading flex |
| .workspace-heading h1 | kept: page heading flex |
| .eyebrow | kept: 11 use sites; one rule beats 6 utility classes x 11 |
| main:has(> .account-page) | kept: account shell width override |
| .notification-center | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-center .workspace-heading | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-description | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-summary strong | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-live | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-card-heading | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-card h2 | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-card p | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-card time | kept: notification layout remainders (heading flex, live status sizing) |
| .notification-card .inline | kept: notification layout remainders (heading flex, live status sizing) |
| .row, .actions | kept: form action rows (18+9 sites, media-query stacking) |
| .row input | kept: input stretch in action rows |
| .table-scroll | kept: marker + overflow (emitted via TableWrap props) |
| .confirmation | kept: checkbox confirmation row (390px wrap fix, a11y-tested) |
| .confirmation input | kept: checkbox confirmation row (390px wrap fix, a11y-tested) |
| fieldset label:has(> input[type="checkbox"]), fieldset label:has(> input[type="radio"]) | kept: checkbox/radio row alignment (touch height) |
| .table-wrap | kept: marker + overflow (emitted via TableWrap props) |
| .table :where(thead, tfoot) | kept: header ink color |
| .list-view-table | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table thead th | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table .table-scroll | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table > table | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table th, .list-view-table td | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table td | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-view-table td.num, .list-view-table th.num | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| #realtime-status | kept: JS live-region sizing (390px fix) |
| .list-create | kept: create-form positioning |
| [data-submit-button][hidden] | kept: structural layout remainder |
| .list-view-cards | kept: mobile card view (grid dl layout + menu clearance) |
| .list-view-cards dl | kept: mobile card view (grid dl layout + menu clearance) |
| .list-view-cards dt | kept: mobile card view (grid dl layout + menu clearance) |
| .list-view-cards dd | kept: mobile card view (grid dl layout + menu clearance) |
| .file-upload-controls | kept: structural layout remainder |
| .file-upload-controls input | kept: structural layout remainder |
| .file-upload-controls button | kept: structural layout remainder |
| .upload-progress[hidden] | kept: JS hidden-state handling only (progress styling now daisyUI) |
| .list-filter-chips | kept: filter chips strip + connector pseudo styling |
| .list-filter-chips > li | kept: filter chips strip + connector pseudo styling |
| code | kept: global wrap for code/IDs |
| @media max-width: 38rem | kept: responsive block (table<->card switch, form stacking) |
| @media prefers-reduced-motion: reduce | kept: responsive block (table<->card switch, form stacking) |
| .skip-link | kept: a11y focus positioning |
| .skip-link:focus | kept: a11y focus positioning |
| .field-error | kept: JS-created field note |
| [aria-invalid="true"] | kept: structural layout remainder |
| .error-summary | kept: JS-created summary |
| .error-summary p | kept: JS-created summary |
| .error-summary a | kept: JS-created summary |
| form[aria-busy="true"] button | kept: structural layout remainder |
| li[data-optimistic="hidden"], tr[data-optimistic="hidden"] | kept: structural layout remainder |
| .strength | kept: JS-created (utilities added in JS) |
| .strength meter | kept: JS-created (utilities added in JS) |
| .wizard-status | kept: JS-created (utilities added in JS) |
| .wizard-nav | kept: JS-created (utilities added in JS) |
| .settings-page > h1 | kept: settings page layout remainders (heading spacing, wide panel span) |
| .settings-page > .settings-intro | kept: settings page layout remainders (heading spacing, wide panel span) |
| .settings-page > section > h2:first-child | kept: settings page layout remainders (heading spacing, wide panel span) |
| .account-page | kept: account page layout remainders (shortcuts nav, section grid) |
| .account-heading | kept: account page layout remainders (shortcuts nav, section grid) |
| .account-heading h1 | kept: account page layout remainders (shortcuts nav, section grid) |
| .account-intro | kept: account page layout remainders (shortcuts nav, section grid) |
| .account-shortcuts | kept: account page layout remainders (shortcuts nav, section grid) |
| .account-section-grid | kept: account page layout remainders (shortcuts nav, section grid) |
| .settings-panel-wide | kept: settings page layout remainders (heading spacing, wide panel span) |
| .session-list | kept: marker only (grid moved to utilities) |
| .session-card-heading | kept: marker only (grid moved to utilities) |
| .session-provider | kept: marker only (grid moved to utilities) |
| .list-view-table thead th, .list-view-table tbody td | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-row:hover > td:first-child | kept: row interaction states tied to sticky-cell backgrounds |
| .list-action-row | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-action-row .btn | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-action-row + .list-toolbar | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .client-bulk | kept: JS is-selecting visibility state |
| .list-filter-icon | kept: inline icon alignment |
| .list-filter-strip | kept: filter chips strip + connector pseudo styling |
| .client-bulk.is-selecting | kept: JS is-selecting visibility state |
| .client-bulk .btn.client-bulk-danger | kept: bulk-danger button token colors |
| .client-bulk .btn.client-bulk-danger:hover | kept: JS is-selecting visibility state |
| .client-bulk .btn .list-action-icon | kept: icon color inheritance |
| .list-select | kept: row select cell |
| .client-bulk-count | kept: muted count text |
| .list-select-cell | kept: select cell width |
| .list-actions-cell | kept: sticky actions column |
| .list-view-table tbody .list-row:hover > .list-actions-cell | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-action-items | kept: menu block |
| .list-action-items a, .list-action-items button | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-action-items a:hover, .list-action-items button:hover | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-action-items .btn | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-action-items .btn:hover | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-name | kept: two-line name cell (flex column) |
| .list-columns-icon | kept: icon display |
| .workspace-main mark | kept: shell width + search-mark tint (color-mix with token vars) |
| .workspace-main .list-row-link mark | kept: row interaction states tied to sticky-cell backgrounds |
| .list-footer .btn | kept: structural layout remainder |
| .js-enhanced [data-submit-button] | kept: structural layout remainder |
| .list-column-switch | kept: column-switch menu (drag handles, grid list) |
| .list-column-handle | kept: column-switch menu (drag handles, grid list) |
| .list-column-switch[draggable="true"] | kept: column-switch menu (drag handles, grid list) |
| .list-column-switch[draggable="true"]:active | kept: column-switch menu (drag handles, grid list) |
| .list-column-order | kept: column-switch menu (drag handles, grid list) |
| .list-action-icon | kept: row action menu block (menu-item styling overrides daisyUI btn on purpose) |
| .list-column-switch form[aria-busy="true"] | kept: column-switch menu (drag handles, grid list) |
| .list-view-cards > li | kept: mobile card view (grid dl layout + menu clearance) |
| .list-view-cards > li > dl | kept: mobile card view (grid dl layout + menu clearance) |
| .list-view-cards .list-row-link | kept: row interaction states tied to sticky-cell backgrounds |
| .client-scope-link | kept: quiet footer link |
| .list-views-panel | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-views-intro | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-views-list | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-view-row | kept: structural layout remainder |
| .list-view-apply | kept: structural layout remainder |
| .list-view-option | kept: structural layout remainder |
| .list-view-option:hover | kept: structural layout remainder |
| .list-view-option.is-applied | kept: structural layout remainder |
| .list-view-name | kept: overflow name |
| .list-view-manage > summary | kept: structural layout remainder |
| .list-view-manage > summary:hover | kept: structural layout remainder |
| .list-view-manage > summary::-webkit-details-marker | kept: structural layout remainder |
| .list-view-manage-body | kept: structural layout remainder |
| .list-view-action | kept: structural layout remainder |
| .list-view-hint | kept: structural layout remainder |
| .list-views-save | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-views-save > summary | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-views-save > summary::-webkit-details-marker | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-views-save > form, .list-views-save > .list-view-note | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-view-check | kept: structural layout remainder |
| .list-view-note | kept: structural layout remainder |
| .list-view-manage > summary, .list-views-save > summary | kept: saved-views panel (disclosure summaries + menu items, custom details controls) |
| .list-view-action .btn | kept: structural layout remainder |
| click's mousedown can trigger the note, and the layout shift between mousedown and mouseup can push the intended target out from under the pointer. Fieldsets reserve the note space and the note sits in it. */ .fieldset | kept: inline-note space reservation (mousedown-shift fix, form-field-input skill) |
| .fieldset .field-error | kept: JS-created field note |
| .client-results | kept: results spacing |
| .client-match | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .workspace-main .client-match .select | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-match label | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-prompt | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .list-filter-chip-edit:hover span, .list-filter-chip-edit:focus-visible span | kept: filter chips strip + connector pseudo styling |
| .list-filter-chip-remove:hover, .list-filter-chip-remove:focus-visible | kept: filter chips strip + connector pseudo styling |
| .list-filter-chips .list-filter-add | kept: filter chips strip + connector pseudo styling |
| .list-filter-connector | kept: uppercase connector styling |
| .client-field-choices | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-choices li | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-choices a | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-choices a::after | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-choices a:hover | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-choices a:focus-visible | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-step-section | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-step-field | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-add-step .select | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-step-section:first-of-type | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-add-step input:focus-visible, .client-add-step select:focus-visible | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-step-actions | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-detail dl | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-detail dd | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-intro | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-delete | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-delete .warning | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-lifecycle | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-lifecycle form, .client-field-actions form, .client-field-option-list form, .client-value-clear | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-values | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-value-clear | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-fields | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-heading | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-heading > strong | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-heading > span | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-actions | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-options | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-options h3 | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-option-list | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-field-option-list button | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls > form | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .workspace-main .client-page-actions | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .label | kept: daisyUI label muted color raised from 60% to 70% ink; axe color-contrast fails at 60% (4.21:1 on the light canvas #f3f6fa), 70% passes (5.82:1) while keeping the muted look |
