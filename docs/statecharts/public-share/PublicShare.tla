---- MODULE PublicShare ----
(*
An owner shares one item through a read-only link, and a visitor opens it
(docs/decisions/0005-public-sharing.md). Links expire, can be revoked, and show only the
item title and status. The owner and the visitor are two regions.
*)
EXTENDS Naturals

VARIABLES
    link,     \* "none" | "active" | "revoked" | "expired"
    seen      \* what the visitor last saw: "nothing" | "item" | "notFound"

vars == <<link, seen>>

Init ==
    /\ link = "none"
    /\ seen = "nothing"

Create ==
    /\ link = "none"
    /\ link' = "active"
    /\ UNCHANGED seen

Revoke ==
    /\ link = "active"
    /\ link' = "revoked"
    /\ UNCHANGED seen

Expire ==
    /\ link = "active"
    /\ link' = "expired"
    /\ UNCHANGED seen

\* The visitor opens the link. Only an active link shows the item.
Visit ==
    /\ seen' = IF link = "active" THEN "item" ELSE "notFound"
    /\ UNCHANGED link

Done ==
    /\ link \in {"revoked", "expired"}
    /\ seen # "nothing"
    /\ UNCHANGED vars

Next == Create \/ Revoke \/ Expire \/ Visit \/ Done

Spec == Init /\ [][Next]_vars

TypeOK ==
    /\ link \in {"none", "active", "revoked", "expired"}
    /\ seen \in {"nothing", "item", "notFound"}

\* A new visit shows the item only while the link is active.
\* A page the visitor already has open may stay on screen after a revoke.
ItemOnlyWhileActive == [][(seen' = "item" /\ seen' # seen) => (link' = "active")]_vars

====
