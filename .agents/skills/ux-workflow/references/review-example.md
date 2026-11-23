# Worked review: follow-up queue

This is an illustrative source-level example, not a browser test result or
proof that a redesign works.

## Task and observation

A staff member needs to find and complete overdue work for a client with two
engagements. A due list displays only action text and date; its link opens the
client page, where each engagement exposes all editing forms. Source shows no
specific engagement target or identifying heading. It does not establish the
actual layout, keyboard behavior, or user completion rate.

## Findings and candidate

1. **Identity:** two similar actions can be hard to distinguish. Candidate: show
   client and engagement identity alongside action and due status, linking to
   the relevant work item. This needs an available identifier and permission.
2. **Task hierarchy:** repeated edit forms compete with follow-up. Candidate:
   show summary and next action first; offer focused editing. Compare inline and
   dedicated-detail flows rather than treating either as universally correct.
3. **Recovery:** generic validation may not identify the affected field.
   Candidate: preserve attempted input and show actionable, associated errors.

## Evidence required before declaring success

Seed two similar engagements using disposable data. Starting from the normal
entry point, find the correct item, open it, attempt an invalid update, recover,
complete the task, and return. Capture actions and observed results plus desktop
and narrow screenshots. Repeat the relevant role, keyboard, and no-JavaScript
paths. If the feature or test environment lacks an action, record the blocker;
do not claim it passed or invent behavior.

For actual user research, give participants a realistic goal without naming
the target control, observe rather than coach, and record limitations and
consent. See [GOV.UK moderated usability testing](https://www.gov.uk/service-manual/user-research/using-moderated-usability-testing).
