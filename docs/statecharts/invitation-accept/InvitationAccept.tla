---- MODULE InvitationAccept ----
(*
A person opens a workspace invitation link and joins (docs/authentication.md, "Invitations
and API tokens"; internal/identity/invitation.go). The invitation and the person's page are
two regions that change independently.

Modeled: the invitation status (pending, revoked, expired, accepted), the page the person
sees, and whether the person's email matches the invitation. A new local account is not
modeled.
*)
EXTENDS Naturals

VARIABLES
    inv,      \* "pending" | "revoked" | "expired" | "accepted"
    page,     \* "notStarted" | "linkOpened" | "signIn" | "matchingEmail" | "wrongEmail" | "joined" | "unusable"
    matched   \* TRUE when the signed-in person's email matches the invitation (fixed per run)

vars == <<inv, page, matched>>

Init ==
    /\ inv = "pending"
    /\ page = "notStarted"
    /\ matched \in BOOLEAN

OpenLink ==
    /\ page = "notStarted"
    /\ page' = "linkOpened"
    /\ UNCHANGED <<inv, matched>>

\* Only a pending invitation leads to sign-in. Otherwise the page says it is unusable.
Route ==
    /\ page = "linkOpened"
    /\ page' = IF inv = "pending" THEN "signIn" ELSE "unusable"
    /\ UNCHANGED <<inv, matched>>

SignIn ==
    /\ page = "signIn"
    /\ page' = IF matched THEN "matchingEmail" ELSE "wrongEmail"
    /\ UNCHANGED <<inv, matched>>

SwitchAccount ==
    /\ page = "wrongEmail"
    /\ page' = "signIn"
    /\ UNCHANGED <<inv, matched>>

Accept ==
    /\ page = "matchingEmail"
    /\ inv = "pending"
    /\ inv' = "accepted"
    /\ page' = "joined"
    /\ UNCHANGED matched

\* The invitation became unusable while the person was on the page.
Refused ==
    /\ page = "matchingEmail"
    /\ inv # "pending"
    /\ page' = "unusable"
    /\ UNCHANGED <<inv, matched>>

Revoke ==
    /\ inv = "pending"
    /\ inv' = "revoked"
    /\ UNCHANGED <<page, matched>>

Expire ==
    /\ inv = "pending"
    /\ inv' = "expired"
    /\ UNCHANGED <<page, matched>>

Done ==
    /\ page \in {"joined", "unusable"}
    /\ UNCHANGED vars

Next ==
    \/ OpenLink \/ Route \/ SignIn \/ SwitchAccount \/ Accept \/ Refused \/ Revoke \/ Expire \/ Done

Spec == Init /\ [][Next]_vars

TypeOK ==
    /\ inv \in {"pending", "revoked", "expired", "accepted"}
    /\ page \in {"notStarted", "linkOpened", "signIn", "matchingEmail", "wrongEmail", "joined", "unusable"}

\* A person joins only when the invitation was pending and their email matched.
JoinsOnlyWithPendingInvitation == page = "joined" => inv = "accepted"

\* Once accepted, an invitation stays accepted.
AcceptedIsFinal == [][inv = "accepted" => inv' = "accepted"]_vars

====
