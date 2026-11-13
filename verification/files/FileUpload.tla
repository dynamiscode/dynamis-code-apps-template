---- MODULE FileUpload ----
(*
Model of the private file lifecycle in internal/files/service.go, for one
workspace with two files that compete for quota.

Each action names the code it models:
  Reserve         reserve()                         (service.go ~344-380): quota check, then INSERT as pending
  PutObject       PutContent() store.Put            (service.go ~155-195): validated bytes stored under the reserved size
  PutWrongSize    presigned PUT of different bytes  (Initiate path, client-controlled object): stored size differs
  CompleteAccepts Complete()/PutContent() + completeDB() accept path (service.go ~195-243, ~388-441)
  CompleteRejects Complete()/PutContent() markFailed paths (object size or content mismatch)
  Abort           PutContent() markFailed before the object is stored (content invalid)
  Cleanup         CleanupStaleFiles()               (service.go ~529): deletes pending and failed rows, releasing quota

Abstractions:
  - Only the sizes matter. Detected MIME, owners, and the signed URL are not modeled.
  - Each action is one serializable transaction (reserve, completeDB) or one check.
    Real code uses SERIALIZABLE transactions (service.go lines 344-441).
  - A transient store error leaves the file pending. The model does not add it, because it only adds stuttering.
  - Cleanup is allowed at any time. The real rule is "updated before cutoff".
*)
EXTENDS Naturals

CONSTANTS
    MaxWorkspace,   \* maxWorkspaceBytes
    MaxObject       \* maxObjectBytes

\* File ids are 1 and 2. Explicit sums keep the model small.
Files == {1, 2}

VARIABLES
    status,         \* [file -> "none" | "pending" | "ready" | "failed"]
    size,           \* [file -> reserved size (files.size)]
    obj             \* [file -> stored object size, 0 when no object exists]

vars == <<status, size, obj>>

\* Quota contribution: pending, ready, and failed files reserve quota.
Contrib(f) == IF status[f] \in {"pending", "ready", "failed"} THEN size[f] ELSE 0
Used == Contrib(1) + Contrib(2)
UsedExcept(f) == Used - Contrib(f)

Init ==
    /\ status = [f \in Files |-> "none"]
    /\ size = [f \in Files |-> 0]
    /\ obj = [f \in Files |-> 0]

\* reserve(): the check is used > maxWorkspaceBytes - size, so it passes when used + size <= max.
Reserve(f, s) ==
    /\ status[f] = "none"
    /\ s \in 1..MaxObject
    /\ Used + s <= MaxWorkspace
    /\ status' = [status EXCEPT ![f] = "pending"]
    /\ size' = [size EXCEPT ![f] = s]
    /\ UNCHANGED obj

PutObject(f) ==
    /\ status[f] = "pending"
    /\ obj[f] = 0
    /\ obj' = [obj EXCEPT ![f] = size[f]]
    /\ UNCHANGED <<status, size>>

PutWrongSize(f, n) ==
    /\ status[f] = "pending"
    /\ obj[f] = 0
    /\ n \in 1..MaxObject
    /\ n # size[f]
    /\ obj' = [obj EXCEPT ![f] = n]
    /\ UNCHANGED <<status, size>>

CompleteAccepts(f) ==
    /\ status[f] = "pending"
    /\ obj[f] >= 1
    /\ obj[f] <= MaxObject
    /\ obj[f] = size[f]
    /\ UsedExcept(f) + obj[f] <= MaxWorkspace
    /\ status' = [status EXCEPT ![f] = "ready"]
    /\ UNCHANGED <<size, obj>>

CompleteRejects(f) ==
    /\ status[f] = "pending"
    /\ obj[f] >= 1
    /\ (obj[f] # size[f] \/ obj[f] > MaxObject)
    /\ status' = [status EXCEPT ![f] = "failed"]
    /\ UNCHANGED <<size, obj>>

Abort(f) ==
    /\ status[f] = "pending"
    /\ status' = [status EXCEPT ![f] = "failed"]
    /\ UNCHANGED <<size, obj>>

Cleanup(f) ==
    /\ status[f] \in {"pending", "failed"}
    /\ status' = [status EXCEPT ![f] = "none"]
    /\ size' = [size EXCEPT ![f] = 0]
    /\ obj' = [obj EXCEPT ![f] = 0]

Done ==
    /\ \A f \in Files : status[f] \in {"none", "ready", "failed"}
    /\ UNCHANGED vars

Next ==
    \E f \in Files :
        \/ \E s \in 1..MaxObject : Reserve(f, s)
        \/ PutObject(f)
        \/ \E n \in 1..MaxObject : PutWrongSize(f, n)
        \/ CompleteAccepts(f)
        \/ CompleteRejects(f)
        \/ Abort(f)
        \/ Cleanup(f)
    \/ Done

Spec == Init /\ [][Next]_vars

\* ---- Invariants (state rules) ----

TypeOK ==
    /\ \A f \in Files : status[f] \in {"none", "pending", "ready", "failed"}
    /\ \A f \in Files : size[f] \in 0..MaxObject
    /\ \A f \in Files : obj[f] \in 0..MaxObject

\* Pending, ready, and failed files together never exceed the workspace quota.
QuotaHolds == Used <= MaxWorkspace

\* A ready file has stored content of exactly its reserved size.
ReadyHasMatchingContent ==
    \A f \in Files : status[f] = "ready" => (obj[f] >= 1 /\ obj[f] = size[f])

\* ---- Properties (action rules) ----

\* Ready is final. A failed file can only be cleaned up, never become ready.
ReadyAndFailedAreFinal ==
    [][\A f \in Files :
        /\ (status[f] = "ready" => status'[f] = "ready")
        /\ (status[f] = "failed" => status'[f] \in {"failed", "none"})]_vars

====
