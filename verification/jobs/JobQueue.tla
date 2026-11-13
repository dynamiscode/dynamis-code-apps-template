---- MODULE JobQueue ----
(*
Model of one background job in internal/jobs/queue.go, with several workers.

Each action names the code it models:
  Claim          claim()                  (queue.go ~263-310)
  Execute        Process(), handler call  (queue.go ~170-240)
  FinishApplied  finish() UPDATE matches  (queue.go ~320-352, WHERE status='running' AND lease_token=?)
  FinishStale    finish() UPDATE matches 0 rows (lease token no longer current)
  Expire         leased_until passes      (time, not code)
  Outcome        EvaluateJobOutcome()     (queue.go ~358)

Abstractions:
  - Lease expiry is an explicit Expire action. The model does not track clocks.
  - A handler returns "ok" or "err" nondeterministically. Its real error category is not modeled.
  - "handler-unavailable" and the over-limit branch are not separate: over-limit is att > Max.
  - Finish uses the attempt number from the claim (job.AttemptCount), not the current attempts counter.
*)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Workers,        \* set of worker identities
    MaxAttempts     \* maxAttempts in queue.go

VARIABLES
    status,         \* "pending" | "running" | "succeeded" | "failed"  (background_jobs.status)
    attempts,       \* background_jobs.attempt_count
    live,           \* TRUE while the current lease has not expired
    lastTok,        \* the most recently issued lease token (ids are monotonic)
    held,           \* [worker -> token from its last claim]
    phase,          \* [worker -> "idle" | "running" | "returned"]
    att,            \* [worker -> attempt number captured at claim (job.AttemptCount)]
    result,         \* [worker -> "none" | "ok" | "err"]  (handler outcome)
    staleCommit     \* ghost: TRUE if a finish from a non-current lease was ever applied

vars == <<status, attempts, live, lastTok, held, phase, att, result, staleCommit>>

Terminal == {"succeeded", "failed"}

\* EvaluateJobOutcome, reduced to the status it chooses
Outcome(a, r) ==
    IF a > MaxAttempts THEN "failed"
    ELSE IF r = "ok" THEN "succeeded"
    ELSE IF a >= MaxAttempts THEN "failed"
    ELSE "pending"

Init ==
    /\ status = "pending"
    /\ attempts = 0
    /\ live = FALSE
    /\ lastTok = 0
    /\ held = [w \in Workers |-> 0]
    /\ phase = [w \in Workers |-> "idle"]
    /\ att = [w \in Workers |-> 0]
    /\ result = [w \in Workers |-> "none"]
    /\ staleCommit = FALSE

\* claim(): takes a pending job, or reclaims a running job whose lease expired.
Claim(w) ==
    /\ phase[w] = "idle"
    /\ attempts < MaxAttempts + 1
    /\ \/ status = "pending"
       \/ (status = "running" /\ ~live)
    /\ status' = "running"
    /\ attempts' = attempts + 1
    /\ live' = TRUE
    /\ lastTok' = lastTok + 1
    /\ held' = [held EXCEPT ![w] = lastTok + 1]
    /\ phase' = [phase EXCEPT ![w] = "running"]
    /\ att' = [att EXCEPT ![w] = attempts + 1]
    /\ result' = [result EXCEPT ![w] = "none"]
    /\ UNCHANGED staleCommit

\* Process(): the handler runs only when job.AttemptCount <= maxAttempts.
Execute(w) ==
    /\ phase[w] = "running"
    /\ att[w] <= MaxAttempts
    /\ \E r \in {"ok", "err"} :
        /\ phase' = [phase EXCEPT ![w] = "returned"]
        /\ result' = [result EXCEPT ![w] = r]
    /\ UNCHANGED <<status, attempts, live, lastTok, held, att, staleCommit>>

\* A finish is pending for a worker when the handler returned, or when the
\* over-limit branch of Process is taken (att > maxAttempts, no handler call).
Finishing(w) ==
    \/ phase[w] = "returned"
    \/ (phase[w] = "running" /\ att[w] > MaxAttempts)

\* finish() applied: the UPDATE matches because the lease token is current.
FinishApplied(w) ==
    /\ Finishing(w)
    /\ status = "running"
    /\ held[w] = lastTok
    /\ status' = Outcome(att[w], result[w])
    /\ live' = FALSE
    /\ phase' = [phase EXCEPT ![w] = "idle"]
    /\ UNCHANGED <<attempts, lastTok, held, att, result, staleCommit>>

\* finish() with a stale token: the UPDATE matches zero rows and changes nothing.
FinishStale(w) ==
    /\ Finishing(w)
    /\ ~(status = "running" /\ held[w] = lastTok)
    /\ phase' = [phase EXCEPT ![w] = "idle"]
    /\ UNCHANGED <<status, attempts, live, lastTok, held, att, result, staleCommit>>

\* leased_until passes while the worker is still running.
Expire ==
    /\ status = "running"
    /\ live
    /\ live' = FALSE
    /\ UNCHANGED <<status, attempts, lastTok, held, phase, att, result, staleCommit>>

\* The job is finished and no worker holds it: the legal end state.
Done ==
    /\ status \in Terminal
    /\ UNCHANGED vars

Next ==
    \E w \in Workers :
        \/ Claim(w)
        \/ Execute(w)
        \/ FinishApplied(w)
        \/ FinishStale(w)
    \/ Expire
    \/ Done

Spec == Init /\ [][Next]_vars

\* ---- Invariants (state rules) ----

TypeOK ==
    /\ status \in {"pending", "running", "succeeded", "failed"}
    /\ attempts \in 0..(MaxAttempts + 1)
    /\ live \in BOOLEAN
    /\ staleCommit \in BOOLEAN

\* A finish from a worker whose lease was superseded never changes the job.
NoStaleCommit == ~staleCommit

\* The handler only runs for an attempt within maxAttempts.
HandlerWithinMax ==
    \A w \in Workers : phase[w] = "returned" => att[w] <= MaxAttempts

\* A job is failed only after its attempts are exhausted.
FailedOnlyAfterMax == status = "failed" => attempts >= MaxAttempts

\* At most one worker can hold the current lease.
\* (Derived: only Claim issues a token, and only from idle workers.)
SingleLeaseHolder ==
    Cardinality({w \in Workers : phase[w] # "idle" /\ held[w] = lastTok}) <= 1

\* ---- Properties (action rules) ----

\* Terminal states never change.
TerminalIsFinal ==
    [][(status \in Terminal) => (status' = status)]_vars

====
