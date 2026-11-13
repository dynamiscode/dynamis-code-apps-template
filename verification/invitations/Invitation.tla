---- MODULE Invitation ----
(*
Model of one workspace invitation in internal/identity/invitation.go.

Each action names the code it models:
  Tick          time passes                                   (clock, not code)
  Resend        ResendInvitation (~165-265): UPDATE sets a new expires_at,
                clears expired_at, from pending or expired
  Revoke        RevokeInvitation (~265-350): from pending only; UPDATE guarded by accepted/expired/revoked IS NULL
  Accept        acceptLoadedInvitation (~599-645): from pending, guarded by expires_at > now
  MarkExpired   markInvitationExpired (~687-705): sets expired_at only once expires_at <= now

Status mirrors Invitation.Status(now) (~25-38): accepted, then revoked, then expired, then pending.

Abstractions:
  - Time is an integer clock from 0 to MaxT. Lifetime is Life ticks.
  - Secrets, email, role, and workspace membership are not modeled.
  - Each action is one database transaction. Real code uses one transaction per operation.
*)
EXTENDS Naturals

CONSTANTS
    MaxT,       \* time bound for the model
    Life        \* invitation lifetime in ticks

VARIABLES
    now,        \* current time
    expires,    \* expires_at
    acc,        \* accepted_at is set
    rev,        \* revoked_at is set
    exp,        \* expired_at is set
    accAt       \* time of acceptance (ghost; meaningful only when acc is TRUE)

vars == <<now, expires, acc, rev, exp, accAt>>

\* Invitation.Status(now): accepted, revoked, expired, pending, in that precedence
StatusOf(a, r, e, t, x) ==
    IF a THEN "accepted"
    ELSE IF r THEN "revoked"
    ELSE IF e \/ t >= x THEN "expired"
    ELSE "pending"

Status == StatusOf(acc, rev, exp, now, expires)

Init ==
    /\ now = 0
    /\ expires = Life
    /\ acc = FALSE
    /\ rev = FALSE
    /\ exp = FALSE
    /\ accAt = 0

Tick ==
    /\ now < MaxT
    /\ now' = now + 1
    /\ UNCHANGED <<expires, acc, rev, exp, accAt>>

\* ValidateInvitationTransition: pending -> pending and expired -> pending
Resend ==
    /\ Status \in {"pending", "expired"}
    /\ expires' = now + Life
    /\ exp' = FALSE
    /\ UNCHANGED <<now, acc, rev, accAt>>

\* ValidateInvitationTransition: pending -> revoked only
Revoke ==
    /\ Status = "pending"
    /\ rev' = TRUE
    /\ UNCHANGED <<now, expires, acc, exp, accAt>>

\* ValidateInvitationTransition: pending -> accepted only, and the guard expires_at > now
Accept ==
    /\ Status = "pending"
    /\ expires > now
    /\ acc' = TRUE
    /\ accAt' = now
    /\ UNCHANGED <<now, expires, rev, exp>>

\* ValidateInvitationTransition: pending -> expired, and the guard expires_at <= now
MarkExpired ==
    /\ ~exp
    /\ ~acc /\ ~rev
    /\ expires <= now
    /\ exp' = TRUE
    /\ UNCHANGED <<now, expires, acc, rev, accAt>>

Done ==
    /\ now = MaxT
    /\ UNCHANGED vars

Next ==
    \/ Tick
    \/ Resend
    \/ Revoke
    \/ Accept
    \/ MarkExpired
    \/ Done

Spec == Init /\ [][Next]_vars

\* ---- Invariants (state rules) ----

TypeOK ==
    /\ now \in 0..MaxT
    /\ expires \in 0..(MaxT + Life)
    /\ acc \in BOOLEAN /\ rev \in BOOLEAN /\ exp \in BOOLEAN

\* An invitation is never both accepted and revoked.
NoAcceptAndRevoke == ~(acc /\ rev)

\* An accepted invitation was accepted before its deadline.
AcceptedWithinLifetime == acc => accAt < expires

\* expired_at is set only after the deadline has passed.
ExpiredFlagOnlyAfterDeadline == exp => now >= expires

\* ---- Properties (action rules) ----

\* Accepted is final: it stays accepted and is never revoked.
AcceptedIsFinal == [][acc => (acc' /\ ~rev')]_vars

\* Revoked is final.
RevokedIsFinal == [][rev => rev']_vars

\* Expired moves only to pending, and only through Resend.
ExpiredOnlyLeavesByResend ==
    [][(StatusOf(acc, rev, exp, now, expires) = "expired")
        => (StatusOf(acc', rev', exp', now', expires') \in {"expired", "pending"})]_vars

====
