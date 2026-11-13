---- MODULE SetupWizard ----
(*
First-run setup (internal/web/templates/setup.html) as a two-step wizard
wired by forms.js (enhanceWizard, wizardMove, the submit handler), posting to
setup in internal/web/handler.go.

- Step 1 "Your owner account": email, password (12+ characters), confirmation.
- Step 2 "Your first workspace": name and language.
- Next validates only the current step. Enter on step 1 acts as Next. The
  submit button shows only on the last step, and the submit handler validates
  every field again, jumping back to the step of the first invalid one.
- Without JavaScript, both steps show and the server validates.
*)
EXTENDS Naturals

VARIABLES
    step,      \* 1 | 2
    account,   \* step 1 fields valid
    workspace, \* step 2 fields valid
    result     \* "editing" | "created" | "serverError"

vars == <<step, account, workspace, result>>

Init == step = 1 /\ account = FALSE /\ workspace = FALSE /\ result = "editing"

Editing == result = "editing"

YouFillAccount(valid) ==
    /\ Editing /\ step = 1 /\ account # valid
    /\ account' = valid
    /\ UNCHANGED <<step, workspace, result>>

YouFillWorkspace(valid) ==
    /\ Editing /\ step = 2 /\ workspace # valid
    /\ workspace' = valid
    /\ UNCHANGED <<step, account, result>>

\* Next (or Enter on step 1): wizardMove moves only from a valid step.
YouPressNext ==
    /\ Editing /\ step = 1
    /\ step' = IF account THEN 2 ELSE 1
    /\ UNCHANGED <<account, workspace, result>>

YouPressBack ==
    /\ Editing /\ step = 2
    /\ step' = 1
    /\ UNCHANGED <<account, workspace, result>>

\* Create owner, only visible on the last step. The submit handler checks every
\* field; on an invalid one it shows the summary on that field's step.
YouPressCreate ==
    /\ Editing /\ step = 2
    /\ IF account /\ workspace
       THEN result' \in {"created", "serverError"} /\ UNCHANGED step
       ELSE /\ step' = IF account THEN 2 ELSE 1
            /\ UNCHANGED result
    /\ UNCHANGED <<account, workspace>>

\* The server rejected the setup (for example, the email is taken). The page
\* re-renders from step 1 with the error focused.
YouSeeServerError ==
    /\ result = "serverError"
    /\ result' = "editing"
    /\ step' = 1
    /\ account' = FALSE
    /\ UNCHANGED workspace

Done == result = "created" /\ UNCHANGED vars

Next ==
    \/ \E v \in BOOLEAN : YouFillAccount(v) \/ YouFillWorkspace(v)
    \/ YouPressNext \/ YouPressBack \/ YouPressCreate \/ YouSeeServerError \/ Done

Spec == Init /\ [][Next]_vars

\* ---- Rules ----

\* You reach the workspace step only with a valid account step.
NextOnlyWhenValid == [][(step = 1 /\ step' = 2) => account]_vars

\* The browser sends setup only when every field on every step is valid.
SubmitOnlyWhenAllValid == [][(result' \in {"created", "serverError"} /\ result = "editing") => (account /\ workspace)]_vars

\* Setup is sent only from the last step.
SubmitOnlyFromLastStep == [][(result' # result /\ result = "editing") => step = 2]_vars

====
