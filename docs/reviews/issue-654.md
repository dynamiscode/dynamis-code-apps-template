# Issue 654: clients and engagement action roles

Base: `aa927e2`, with issue 654 changes in this commit. Synthetic data only.

## Inventory

Client list and creation, list search/filter/columns/views/bulk actions, client detail, timeline and custom fields; engagement list, board, detail, stages, action queue and reschedule recovery. Shared navigation, breadcrumbs and list row links use existing bounded styles. The current base and `origin/main` were checked; list and board are present on the current base.

Secondary actions and prominent navigation use outlined daisyUI buttons; inline record links retain compact boundaries to preserve wrapping. Archive-stage uses the danger role and retains its required confirmation checkbox. Native elements, destinations and permission conditions remain unchanged.

## Real-app review

Owner, disposable SQLite instance, Chrome/Playwright. Created a client, added an open stage, created an engagement, and navigated to its detail. Client empty and populated states and engagement detail rendered. Keyboard Tab reached the skip link. At the narrow viewport, document width and scroll width both measured 375 CSS pixels. Evidence uses synthetic records:

- [Client detail](issue-654/clients654-detail.png)
- [Engagement detail, desktop](issue-654/clients654-engagement-desktop.png)
- [Engagement detail, narrow dark preference](issue-654/clients654-engagement-mobile.png)

Three passes by the implementer, not independent: journey navigation and creation worked; primary and secondary actions remain distinct; native controls and keyboard entry remain available. Automated client/engagement browser tests and accessibility smoke supplement this review.

## Remaining review gaps

Rendered review is incomplete: error recovery, read-only role, populated board and queue, custom-field editing, actual dark theme selection, and keyboard operation through every changed control were not manually driven. Desktop and mobile screenshots alone do not establish usability or accessibility conformance. Review is blocked until these states are exercised and findings resolved.
