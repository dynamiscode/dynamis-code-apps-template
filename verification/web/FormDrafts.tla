---- MODULE FormDrafts ----
(*
One full-page form with a stored draft (data-persist), dirty tracking
(data-dirty-track), and the unsaved-changes warning (data-unsaved-warning),
as wired by internal/web/assets/forms.js. Example: the invitation form in
internal/web/templates/invitations.html, whose server re-render does not echo
the typed email.

Code this model follows (forms.js):
- input handler: updateDirty, then writeDraft(form, false), which is skipped
  while the form is submitting.
- submit handler: writeDraft(form, true), then mark the form submitting.
- applyDraft on page load: draftAction(stored, pageHasError) picks
  "discard" (submitted and no error) or "restore" (otherwise).
- beforeunload: warn when a data-unsaved-warning form is dirty and not
  submitting.
- The Discard button removes the draft and resets the fields.
sessionStorage is per tab, so the draft survives leaving and coming back.
*)
EXTENDS Naturals

VARIABLES
    page,       \* "form" | "away"
    box,        \* what the field shows: "" or "typed"
    stored,     \* the draft: "none" | "draft" | "submitted"
    submitting, \* a submit is on its way
    msg,        \* what the form page shows: "none" | "saved" | "error"
    server,     \* what the server stored: "" or "typed"
    warned,     \* the browser asked "leave the page?" on the last exit
    unwarnedLoss \* the user left with unsent text and no warning and no draft

vars == <<page, box, stored, submitting, msg, server, warned, unwarnedLoss>>

Init ==
    /\ page = "form" /\ box = "" /\ stored = "none" /\ submitting = FALSE
    /\ msg = "none" /\ server = "" /\ warned = FALSE /\ unwarnedLoss = FALSE

Dirty == box # ""

YouType ==
    /\ page = "form" /\ ~submitting /\ box = ""
    /\ box' = "typed"
    /\ stored' = "draft"
    /\ msg' = "none"
    /\ UNCHANGED <<page, submitting, server, warned, unwarnedLoss>>

YouClear ==
    /\ page = "form" /\ ~submitting /\ box = "typed"
    /\ box' = ""
    /\ stored' = "none"
    /\ msg' = "none"  \* the rules below are about the page right after an answer
    /\ UNCHANGED <<page, submitting, server, warned, unwarnedLoss>>

YouSubmit ==
    /\ page = "form" /\ ~submitting /\ box = "typed"
    /\ submitting' = TRUE
    /\ stored' = "submitted"
    /\ UNCHANGED <<page, box, msg, server, warned, unwarnedLoss>>

\* While the request is out, the browser can fire input or change on the old
\* form (seen in Chrome when htmx swaps it out). writeDraft skips a submitting
\* form, so the draft stays marked submitted.
EventWhileSubmitting ==
    /\ submitting
    /\ stored' = IF submitting THEN stored ELSE "draft"
    /\ UNCHANGED <<page, box, submitting, msg, server, warned, unwarnedLoss>>

\* A new page loads; the server's field is empty, then applyDraft runs.
Load(hasError) ==
    LET action == IF stored = "none" THEN "none"
                  ELSE IF stored = "submitted" /\ ~hasError THEN "discard"
                  ELSE "restore"
    IN  /\ page' = "form"
        /\ submitting' = FALSE
        /\ box' = IF action = "restore" THEN "typed" ELSE ""
        /\ stored' = IF action = "restore" THEN "draft" ELSE "none"

ServerSaves ==
    /\ submitting
    /\ server' = "typed"
    /\ msg' = "saved"
    /\ Load(FALSE)
    /\ UNCHANGED <<warned, unwarnedLoss>>

ServerRejects ==
    /\ submitting
    /\ msg' = "error"
    /\ Load(TRUE)
    /\ UNCHANGED <<server, warned, unwarnedLoss>>

YouLeave ==
    /\ page = "form" /\ ~submitting
    /\ page' = "away"
    /\ warned' = Dirty
    /\ unwarnedLoss' = (unwarnedLoss \/ (Dirty /\ ~warned' /\ stored = "none"))
    /\ UNCHANGED <<box, stored, submitting, msg, server>>

YouComeBack ==
    /\ page = "away"
    /\ msg' = "none"
    /\ Load(FALSE)
    /\ UNCHANGED <<server, warned, unwarnedLoss>>

YouDiscard ==
    /\ page = "form" /\ ~submitting /\ stored = "draft"
    /\ stored' = "none"
    /\ box' = ""
    /\ msg' = "none"
    /\ UNCHANGED <<page, submitting, server, warned, unwarnedLoss>>

Next == YouType \/ YouClear \/ YouSubmit \/ EventWhileSubmitting \/ ServerSaves \/ ServerRejects \/ YouLeave \/ YouComeBack \/ YouDiscard

Spec == Init /\ [][Next]_vars

\* ---- Rules ----

\* After the server rejects a submit, the field shows what you typed.
KeptOnReject == msg = "error" => box = "typed"

\* After the server saves, the old draft is gone and is never restored.
NoStaleDraft == msg = "saved" => (stored = "none" /\ box = "")

\* Coming back to the form shows unsent text, unless it was saved or discarded.
DraftSurvivesLeaving == (page = "form" /\ ~submitting /\ stored = "draft") => box = "typed"

\* Leaving with unsaved text always warns.
NoUnwarnedLoss == ~unwarnedLoss

\* The browser warns only when there is something unsaved.
WarnOnlyWhenDirty == (page = "away" /\ warned) => box = "typed"

====
