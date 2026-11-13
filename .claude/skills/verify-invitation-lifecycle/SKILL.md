---
name: verify-invitation-lifecycle
description: Check the workspace invitation lifecycle (pending, accepted, revoked, expired) and its resend rules. Use when a change touches internal/identity/invitation.go (Status, ValidateInvitationTransition, CreateInvitation, ResendInvitation, RevokeInvitation, findInvitation, acceptLoadedInvitation, markInvitationExpired), the invitation routes, or the invitation tables in docs/data-lifecycle.md.
---

# Verify the invitation lifecycle

## The rules this checks

1. **Accepted is final.** An accepted invitation stays accepted and is never revoked. Harm if broken: a membership is granted twice, or an accepted invitation is later voided.
2. **Revoked is final.** Harm if broken: a revoked invitation becomes usable again.
3. **Accept only while pending.** An invitation is accepted only before its deadline, and not after it was revoked or accepted. Harm if broken: someone joins a workspace after its invitation expired or was revoked.
4. **Revoke only from pending.** An expired invitation cannot be revoked. Harm if broken: the expired state and the revoked state disagree about the same invitation.
5. **Expired leaves only by resend.** Only a resend can return an expired invitation to pending. Harm if broken: an expired invitation becomes usable without a new deadline.
6. **Expiry is marked only after the deadline.** Harm if broken: an invitation is marked expired early, and its user is blocked.

## What is modeled

- Model: `verification/invitations/Invitation.tla`, config `verification/invitations/Invitation.cfg`.
- Bounds: time from 0 to 4, lifetime 2 ticks. About 61 distinct states.
- Actions map to code: `Resend` is `ResendInvitation`, `Revoke` is `RevokeInvitation`, `Accept` is `acceptLoadedInvitation`, `MarkExpired` is `markInvitationExpired`, and `Tick` is time passing. `StatusOf` mirrors `Invitation.Status` (accepted, then revoked, then expired, then pending).
- Each action is one transaction. The model does not separate the read from the write inside each one. The real code reads the row and writes it in the same transaction.
- Abstracted away: secrets and their rotation, email binding, roles and memberships, and timing across processes.

## Coverage record

- Covered code: `internal/identity/invitation.go` (`Status`, `ValidateInvitationTransition`, `ResendInvitation`, `RevokeInvitation`, `findInvitation`, `acceptLoadedInvitation`, `markInvitationExpired`) and the tests in `internal/identity/service_test.go` (`TestInvitationLifecycle`, `TestInvitationStatus`, and the transition table).
- Sync commit: `11f139b483cbf15f0e47868c0f4f586e80995634`. The model was last confirmed against this commit.
- If covered code changed since the sync commit, rerun the checks and read the diff for `invitation.go` before trusting a pass. Move the sync commit forward only after the checks pass against that code and you confirm the model still matches it.

## Run the check

1. Run the model check:
   `verification/tools/run-tlc.sh verification/invitations Invitation`
   The pass line is `Model checking completed. No error has been found.`
2. Run the Go checks against the real code:
   `go test ./internal/identity/ -count=1`
   The whole identity package runs in about 3 seconds.
3. Both must pass. Changes to revocation, acceptance, or expiry also need the `verify-job-queue` check if they touch the queue.

## Read a failure

- **TLC prints a counterexample:** it is the shortest sequence of actions that breaks the named rule. Replay each action against `invitation.go`, one step for each action. If the code does the same thing, it is a real bug: report the rule and the steps, and do not weaken the rule or the model. If the code cannot take a step the model takes, fix the model, then rerun. To see a counterexample as a chart, use the `/statechart` skill by name.
- **A Go test fails:** `TestInvitationLifecycle` reports `expired invitation error = <nil>` when an expired invitation was accepted. That is rule 3 broken in the code.

## When a change is intended

If the rules change on purpose (a new resend rule, revocation of expired invitations, a new status):
1. Update the matching action in `Invitation.tla`, then the invariants and properties in `Invitation.cfg` if a rule changed.
2. Update the transition table in `service_test.go` and the Go tests that encode the old rule.
3. Update the coverage record above.
4. Rerun both checks. Move the sync commit only after they pass.

## Proof the check catches a break

Measured on this commit, in scratch copies:
- **Model: revoke allowed from any non-accepted state** (the validator's pending-only rule removed): the model fails `ExpiredOnlyLeavesByResend` with a 3-step counterexample: two ticks, then revoke of an expired invitation.
- **Model: accept without its status check and without its expiry guard**: the model fails `AcceptedWithinLifetime` (accept at the deadline) and `NoAcceptAndRevoke` (revoke then accept).
- **Go: all four accept gates removed** (the expired branch in `findInvitation`, the two validators in `findInvitation` and `acceptLoadedInvitation`, and the SQL expiry predicate): `TestInvitationLifecycle` fails with `expired invitation error = <nil>`.
- **Go: only the SQL expiry predicate removed**: no test fails, and the model's single guard covers it. This is expected defense in depth. The SQL predicate only matters if the row changes between the read and the write, and no test or atomic model reproduces that window. A change that moves the read out of the transaction must rerun this skill and must add a test for that window.
- **Model: only the accept expiry guard removed**: the model still passes, because the status check already implies the deadline has not passed. The two guards are redundant in the model, so remove both to test the rule.

## Not covered

- Races between two real database transactions that touch the same invitation. The model treats each operation as atomic.
- Secret rotation during accept: accept looks up the invitation by secret inside its transaction. A resend that rotates the secret at the same time is ordered by database isolation, not by this model.
- Email binding, roles, and membership creation (`workspace_members` and SCIM).
- Clock skew between application instances.
