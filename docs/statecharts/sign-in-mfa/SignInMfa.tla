---- MODULE SignInMfa ----
(*
Browser sign-in when a second factor is enrolled (docs/authentication.md, "MFA and passkeys").
A login challenge is active, consumed (a factor was accepted), expired (5 minutes), or locked
(5 failed attempts). Only an active challenge accepts a code.
*)
EXTENDS Naturals

CONSTANTS MaxTries   \* failed attempts before the challenge locks (the product uses 5)

VARIABLES
    phase,           \* "signedOut" | "challenge" | "signedIn" | "expired" | "locked"
    tries            \* failed code attempts on the current challenge

vars == <<phase, tries>>

Init ==
    /\ phase = "signedOut"
    /\ tries = 0

SignIn ==
    /\ phase = "signedOut"
    /\ phase' = "challenge"
    /\ tries' = 0

WrongCode ==
    /\ phase = "challenge"
    /\ tries + 1 < MaxTries
    /\ tries' = tries + 1
    /\ UNCHANGED phase

FifthWrongCode ==
    /\ phase = "challenge"
    /\ tries + 1 = MaxTries
    /\ phase' = "locked"
    /\ tries' = MaxTries

RightCode ==
    /\ phase = "challenge"
    /\ phase' = "signedIn"
    /\ UNCHANGED tries

TimeOut ==
    /\ phase = "challenge"
    /\ phase' = "expired"
    /\ UNCHANGED tries

StartOver ==
    /\ phase \in {"locked", "expired"}
    /\ phase' = "signedOut"
    /\ tries' = 0

Done ==
    /\ phase = "signedIn"
    /\ UNCHANGED vars

Next == SignIn \/ WrongCode \/ FifthWrongCode \/ RightCode \/ TimeOut \/ StartOver \/ Done

Spec == Init /\ [][Next]_vars

TypeOK ==
    /\ phase \in {"signedOut", "challenge", "signedIn", "expired", "locked"}
    /\ tries \in 0..MaxTries

\* A session is granted only from a challenge that still had attempts left.
SignedInWithinAttempts == phase = "signedIn" => tries < MaxTries

\* A locked or expired challenge never accepts a code again. It only starts over.
LockedOrExpiredOnlyStartOver ==
    [][(phase \in {"locked", "expired"}) => (phase' \in {"locked", "expired", "signedOut"})]_vars

====
