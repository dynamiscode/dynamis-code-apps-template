# Custom CSS disposition

Prefer daisyUI components and Tailwind utilities. The rules below are retained
for accessibility, JavaScript state, pseudo-content, structural layout, or
other documented reasons. Update this map when adding or changing custom CSS.

The `tokens.css` design token layer, global `:focus-visible` and
`prefers-reduced-motion` rules, and JavaScript state hooks such as
`[aria-busy]`, `[data-optimistic]`, and `data-submit-button` are intentional.

| Selector | Disposition |
|---|---|
| body | kept: token-driven base (margin/background/color) |
| header | kept: legacy header base (navbar carries its own now; candidate for deletion after header sweep) |
| .workspace-name | kept: navbar brand name truncation |
| .sidebar-navigation .menu | kept: 44px targets and keyboard focus, plus text colors overriding global link styles while daisyUI supplies menu hover and active surfaces |
| .dropdown-content, .modal-box | kept: shared token border distinguishes floating surfaces from matching dark backgrounds |
| @media min-width: 38.001rem | kept: responsive block (table<->card switch, form stacking) |
| .client-controls | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls summary | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls summary:hover | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .client-controls[open] summary | kept: clients feature layout (dl grids, step sections, option lists; utilities would sprawl across three files for no gain) |
| .list-toolbar | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar .menu | kept: toolbar structural block (dropdown summaries are custom details controls) |
| .list-toolbar > details and #list-filter-add > details | kept: toolbar disclosure positioning, including the out-of-band filter wrapper |
| .list-page-size | kept: page-size control row |
| .workspace-main .list-page-size .select | kept: shell width + search-mark tint (color-mix with token vars) |
| .list-toolbar disclosure summaries and states | kept: consistent trigger sizing, surface, marker, and chevron for direct and wrapped toolbar menus |
| .list-view-table tbody td:first-child | kept: table layout block (sticky first column/actions cell, nowrap/ellipsis sizing, header height) |
| .list-row | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row:hover | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row-link | kept: row interaction states tied to sticky-cell backgrounds |
| .list-row-link:hover | kept: row interaction states tied to sticky-cell backgrounds |
| .list-view-table thead th:has(> a) | kept: remove cell padding so sortable links cover the full header |
| .list-view-table thead th > a | kept: table layout block (full-cell sort target and right-aligned indicator) |
| .list-view-table thead th > a:hover | kept: full-cell sort hover affordance |
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
| .workspace-main :is(a.link, p > a:not(.btn, .detail-back)) | kept: inline navigation boundaries clone across wrapped text without changing anchor layout; quiet back links remain borderless |
| .list-page-size-options a, .workspace-main a.btn-ghost | kept: shared list and workspace navigation retain visible boundaries |
| main form:not(.inline) > :is(fieldset, input:not([type="hidden"]), select, textarea, .reveal-box, .strength, .wizard-nav) + button | kept: JS-created (utilities added in JS) |
| main form .btn | kept: 44px touch height contract (a11y smoke asserts it) |
| .auth-card .input | kept: iOS zoom guard - 1rem font so Safari does not zoom on focus (auth forms only; rest of app pending measurement) |
| html | kept: root background so the browser chrome (iOS Safari URL bar) tints with --canvas, not system white |
| .reveal-box, .reveal-box .input, .reveal-box .reveal | kept: JS reveal-toggle overlay - eye inside the password field, absolute at inline-start, height = input height (36px, WCAG 2.5.8 AA pass; raise --size for 44px if a11y smoke demands) |
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
| .list-actions-cell | kept: sticky actions column and row-matching surface |
| .list-view-table tbody td.list-actions-cell | kept: visible menu overflow and full-cell trigger |
| .list-actions-cell:has(details[open]), .list-view-cards > li:has(> details[open]) | kept: open menu stacking above adjacent rows |
| .list-actions-cell > details.dropdown | kept: anchor menu to the action cell |
| .list-actions-cell > details > summary.list-row-menu-button | kept: borderless full-cell keyboard-accessible trigger |
| .list-actions-cell > details > .dropdown-content | kept: right-aligned fallback without JavaScript |
| .list-view-cards .list-row-menu-button | kept: borderless mobile row trigger |
| .list-view-table tbody .list-row:hover > .list-actions-cell | kept: sticky cell matches row hover |
| .list-action-items | kept: row-matching menu surface and layout |
| .list-view-cards .list-action-items | kept: menu surface matches mobile cards |
| .dropdown-content > .menu | kept: action lists fill their dropdown panel |
| dropdown menu action links, buttons, and .list-view-option | kept: shared full-width unboxed touch targets and hover/focus states; excludes form inputs and submission controls outside action lists |
| dropdown menu actions with aria-current | kept: selected workspace and theme state |
| .list-action-items .list-action-item.is-danger | kept: destructive action color and hover/focus state |
| .list-name | kept: two-line name cell (flex column) |
| .list-columns-icon | kept: icon display |
| .workspace-main mark | kept: shell width + search-mark tint (color-mix with token vars) |
| .workspace-main .list-row-link mark | kept: row interaction states tied to sticky-cell backgrounds |
| .list-footer .btn | kept: structural layout remainder |
| .js-enhanced [data-submit-button] | kept: structural layout remainder |
| .detail-split, .detail-main, .detail-sidebar, .detail-values, .detail-engagement-meta, [data-detail-divider], [data-detail-range] | kept: progressive split layout, draggable divider and range popup, with pane-width-responsive detail content |
| .transaction-entry-form, .transaction-detail-values | kept: matching responsive label/value rows and quiet creation controls; native inputs remain visible and focusable without JavaScript |
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
| .list-view-option.is-applied, .list-views-list .list-view-option[aria-current="true"] | kept: selected client and engagement view state |
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
| .fieldset | kept: reserve inline-note space so a note triggered on mousedown cannot shift the intended target before mouseup (form-field-input skill) |
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
| a:not(.btn) | kept: global link color tied to accent; anchors that are daisyUI buttons keep their component color (unlayered CSS beats @layer, so the old plain-a rule painted btn text invisible on primary fills) |
| .status-line | kept: app status paragraphs; renamed from .status because daisyUI ships that name as an indicator-dot component |
| details.dropdown:not([open]) > .dropdown-content | kept: WebKit closed-panel guard (::details-content unsupported there paints closed menus) |
| tokens.css `--border` → `--border-color` | renamed: the app token was a color, daisyUI's `--border` is a border *width*. Same `:root` name meant `border: var(--border) solid …` resolved to `border: #ccd3dd solid`, which is invalid, so every daisyUI control lost its outline (0px). Checkbox, radio, toggle, and any control drawn from that variable were invisible. |
| `.input, .select, .textarea, .checkbox, .radio, .toggle { --input-color: var(--border-color) }` | kept: daisyUI defaults each control's `--input-color` to base-content at 20%, a different grey from the app border. One declaration gives fields, selects, textareas, and selector controls the same outline in both themes. |
| `.checkbox:checked, .checkbox:indeterminate, .radio:checked` | kept: daisyUI fills `:checked` from `--input-color` and leaves it unset, so a checked box was an unfilled outline with a base-content tick. Accent fill plus on-accent glyph, one rule for every selector control. |
| `.engagement-markdown` and descendants | kept: server-rendered Markdown has no utility classes; scoped rules restore headings, lists, links, tables, and code overflow after Tailwind's reset. |
