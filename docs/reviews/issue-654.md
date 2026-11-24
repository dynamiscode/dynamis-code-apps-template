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

## Follow-up review

Base `ff38f57` plus the follow-up in this commit, disposable SQLite and real Chrome/Playwright, synthetic records. On a populated engagement list, a viewer saw client and engagement links with their correct destinations but no create action. Selecting **Dark** in the theme menu set `data-theme="dark"`. Focusing the Stage board link showed a solid focus outline; Enter opened the populated board. Submitting a whitespace-only stage name produced HTTP 422 and the message “Enter a valid name and kind.” Entering a valid name afterward created the stage.

- [Viewer engagement list](issue-654/654-viewer.png)
- [Viewer board in selected dark theme](issue-654/654-board-dark.png)
- [Stage validation error](issue-654/654-error.png)

The viewer was provisioned in the disposable fixture; no production data was used. Screenshots precede restoration of the existing `link` class on record anchors to retain its wrapping rules. These passes were not independent. Remaining gaps: populated queue, custom-field editing, and keyboard operation through every changed control were not manually driven. Screenshots and automated checks alone do not establish usability or accessibility conformance.
