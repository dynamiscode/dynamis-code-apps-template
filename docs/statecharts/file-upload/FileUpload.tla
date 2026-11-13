---- MODULE FileUpload ----
(*
A person uploads a private file with the browser uploader (internal/web/assets/app.js,
PresignedUploader phases IDLE, INITIATING, UPLOADING, COMPLETING, SUCCESS, ERROR; server
side in internal/files/service.go). A failed attempt can leave a pending server file that
holds quota until cleanup.

phase follows PresignedUploader. failedAt records which step failed last.
held counts pending server files for this workspace. stored is TRUE once the bytes reached
storage for the latest attempt. ready counts files the server has finished.
*)
EXTENDS Naturals

CONSTANTS MaxTries   \* attempts the chart explores (the uploader itself has no limit)

VARIABLES
    phase,    \* "idle" | "initiating" | "uploading" | "completing" | "success" | "error"
    failedAt, \* "none" | "initiate" | "put" | "complete"
    tries,    \* attempts started
    held,     \* pending server files
    stored,   \* bytes reached storage for the latest attempt
    ready     \* ready server files

vars == <<phase, failedAt, tries, held, stored, ready>>

Init ==
    /\ phase = "idle"
    /\ failedAt = "none"
    /\ tries = 0
    /\ held = 0
    /\ stored = FALSE
    /\ ready = 0

\* The uploader has no retry limit. CONSTRAINT Bounded keeps the state space finite.
Submit ==
    /\ phase \in {"idle", "error"}
    /\ phase' = "initiating"
    /\ failedAt' = "none"
    /\ tries' = tries + 1
    /\ stored' = FALSE
    /\ UNCHANGED <<held, ready>>

InitiateFails ==
    /\ phase = "initiating"
    /\ phase' = "error"
    /\ failedAt' = "initiate"
    /\ UNCHANGED <<tries, held, stored, ready>>

\* The server reserves a pending file and returns an upload URL.
InitiateOk ==
    /\ phase = "initiating"
    /\ phase' = "uploading"
    /\ held' = held + 1
    /\ UNCHANGED <<failedAt, tries, stored, ready>>

PutFails ==
    /\ phase = "uploading"
    /\ phase' = "error"
    /\ failedAt' = "put"
    /\ UNCHANGED <<tries, held, stored, ready>>

PutOk ==
    /\ phase = "uploading"
    /\ phase' = "completing"
    /\ stored' = TRUE
    /\ UNCHANGED <<failedAt, tries, held, ready>>

CompleteFails ==
    /\ phase = "completing"
    /\ phase' = "error"
    /\ failedAt' = "complete"
    /\ UNCHANGED <<tries, held, stored, ready>>

\* The server finishes the file: pending becomes ready.
CompleteOk ==
    /\ phase = "completing"
    /\ phase' = "success"
    /\ held' = held - 1
    /\ ready' = ready + 1
    /\ UNCHANGED <<failedAt, tries, stored>>

Done ==
    /\ phase = "success"
    /\ UNCHANGED vars

Next ==
    \/ Submit \/ InitiateFails \/ InitiateOk \/ PutFails \/ PutOk
    \/ CompleteFails \/ CompleteOk \/ Done

Spec == Init /\ [][Next]_vars

\* Bounded: the explored states stop at MaxTries attempts (the chart's CONSTRAINT).
Bounded == tries <= MaxTries

TypeOK ==
    /\ phase \in {"idle", "initiating", "uploading", "completing", "success", "error"}
    /\ failedAt \in {"none", "initiate", "put", "complete"}
    /\ tries \in Nat
    /\ held \in Nat
    /\ ready \in 0..1
    /\ stored \in BOOLEAN

\* The page reports success only when the server file is ready.
SuccessOnlyWhenReady == phase = "success" => ready = 1

\* A ready file always has its bytes in storage.
ReadyOnlyAfterStored == ready = 1 => stored

\* Every pending server file belongs to an attempt that was started.
PendingWithinAttempts == held <= tries

====
