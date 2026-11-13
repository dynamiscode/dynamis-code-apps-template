---- MODULE ItemDelete ----
(*
Deleting one item from /workspaces/{id}/items with optimistic UI.

Code this model follows:
- item-list.html: the delete button has data-confirm and
  data-optimistic="remove"; the row form posts with hx-post and
  hx-sync="this:drop".
- forms.js click handler (capture): window.confirm first; cancel stops
  everything. On OK, the row gets data-optimistic="hidden" (CSS hides it).
- forms.js htmx:finally:request: optimisticOutcome(status) shows the row again
  for any status outside 200..399 or no answer, and shows the
  "Something went wrong" toast when there is no answer below 500.
- handler.go changeItem: a stale version answers 412, re-rendered as 200 for
  htmx with the alert and the item still listed; success re-renders without
  the item and an "Item deleted." toast (HX-Trigger app:toast).
*)
EXTENDS Naturals

VARIABLES
    row,       \* what the list shows: "shown" | "hidden" | "gone"
    dialog,    \* the confirm dialog is open
    request,   \* "none" | "sent" | "answered"
    out,       \* the server's answer: "none" | "deleted" | "stale" | "failed"
    stored,    \* the item still exists on the server
    msg        \* "none" | "deletedToast" | "staleAlert" | "failedToast"

vars == <<row, dialog, request, out, stored, msg>>

Init == row = "shown" /\ dialog = FALSE /\ request = "none" /\ out = "none" /\ stored = TRUE /\ msg = "none"

YouPressDelete ==
    /\ row = "shown" /\ request = "none" /\ ~dialog
    /\ dialog' = TRUE
    /\ UNCHANGED <<row, request, out, stored, msg>>

YouCancel ==
    /\ dialog
    /\ dialog' = FALSE
    /\ UNCHANGED <<row, request, out, stored, msg>>

YouConfirm ==
    /\ dialog
    /\ dialog' = FALSE
    /\ row' = "hidden"
    /\ request' = "sent"
    /\ msg' = "none"
    /\ UNCHANGED <<out, stored>>

\* The server deletes it, finds a newer version (412), or fails (5xx/network).
ServerAnswers ==
    /\ request = "sent"
    /\ \E o \in {"deleted", "stale", "failed"} :
        /\ out' = o
        /\ stored' = IF o = "deleted" THEN FALSE ELSE stored
    /\ request' = "answered"
    /\ UNCHANGED <<row, dialog, msg>>

\* Someone else deleted it meanwhile (the server then answers 404, a 4xx).
OtherUserDeletes ==
    /\ stored /\ request = "sent"
    /\ stored' = FALSE
    /\ UNCHANGED <<row, dialog, request, out, msg>>

AnswerArrives ==
    /\ request = "answered"
    /\ request' = "none"
    /\ out' = "none"
    /\ CASE out = "deleted" -> row' = "gone" /\ msg' = "deletedToast"
         [] out = "stale"   -> row' = "shown" /\ msg' = "staleAlert"
         [] OTHER           -> row' = "shown" /\ msg' = "failedToast"
    /\ UNCHANGED <<dialog, stored>>

Done == row = "gone" /\ UNCHANGED vars

Next == YouPressDelete \/ YouCancel \/ YouConfirm \/ ServerAnswers \/ OtherUserDeletes \/ AnswerArrives \/ Done

Spec == Init /\ [][Next]_vars

\* ---- Rules ----

\* Nothing is sent without your OK in the confirm dialog.
ConfirmFirst == [][(request = "none" /\ request' = "sent") => dialog]_vars

\* The row is hidden only while the delete is on its way, or after it succeeded.
HiddenOnlyWhilePending == row = "hidden" => request # "none"

\* After an answer, the list matches the server, unless a message says why not.
ListMatchesServer == (request = "none" /\ row # "hidden") => (row = "gone" <=> ~stored) \/ msg \in {"staleAlert", "failedToast"}

\* A failure is never silent: the row is back and a message shows.
FailureVisible == (request = "none" /\ msg \in {"staleAlert", "failedToast"}) => row = "shown"

====
