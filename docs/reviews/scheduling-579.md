# Scheduling resources UX review

Revision `1810fc9`; disposable SQLite workspace with synthetic owner and viewer
accounts. The later generator-only change does not affect the reviewed UI.
Screenshots and accessibility snapshots were captured in the conversation's
browser tool output; no credentials or session values are part of this record.

## Journey and design

An owner enters from the workspace home, creates or edits a resource, sets the
shared time zone, and returns to the list. Members and viewers can read the
same information without mutation controls. Invalid input should remain visible
with a recoverable error. The workspace member selector avoids recalling user
IDs; unlinked resources remain available.

Inline disclosure forms were chosen over separate edit pages because each edit
has only a name and an optional member, and a list makes similar resources easy
to compare. The trade-off is that many open forms can add visual length.
Ordinary POST forms provide the non-JavaScript path.

## Browser observations

- Desktop owner, 1728 × 997: workspace home linked to resources; the empty
  state appeared. Creating and editing a synthetic room and saving
  `America/Bogota` returned to the list with the changed values visible.
- Initial desktop review found full-width action buttons and inline labels
  made forms harder to scan. The forms were constrained, labels stacked, and
  buttons sized to their text; the revised desktop layout was inspected.
- Desktop owner: an invalid time zone preserved the submitted value and showed
  an error. The subsequent change made the message specific and the error
  focusable; those changes have automated web coverage.
- Viewer, desktop and 390 × 844: resource and time zone were visible; create,
  edit, and time-zone mutation controls were absent.
- Owner, 390 × 844: forms and resource card fit the viewport. Keyboard Tab
  reached the time-zone field with a visible focus indicator. Created a room
  with Tab and Return, then edited its name. An invalid zone submitted by
  keyboard retained the value and focused the error; correcting it to
  `America/Bogota` returned successfully to the list.
- Owner, 390 × 844: three resources, including a 101-character name, remained
  within their cards; the long name wrapped and its edit control stayed visible
  without horizontal clipping.

These are separate journey, interaction/visual, and accessibility passes by
the same team, not independent reviews. Automated service, REST, and web tests
cover authorization and workspace isolation. The PostgreSQL tracer covers the
migration and resource operations.

Browser screenshots are available only in the conversation tool output, not
as repository artifacts.
