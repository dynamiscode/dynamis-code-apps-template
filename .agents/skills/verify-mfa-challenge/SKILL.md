---
name: verify-mfa-challenge
description: Formally verify the MFA sign-in challenge lifecycle - a challenge is active, consumed, expired, or locked, only an active challenge accepts a code, the fifth wrong code locks it, and a locked or expired challenge only ever starts over. Use when a change touches evaluateChallengeState, ChallengeActive/Consumed/Expired/Locked, maxMFAAttempts or mfaChallengeLifetime in internal/identity/mfa.go, loadChallenge, reserveMFAAttempt, consumeChallenge, or docs/statecharts/sign-in-mfa/SignInMfa.tla.
---

# MFA challenge lifecycle

## The rules

- A session is granted only from a challenge that still had attempts left
  (`SignedInWithinAttempts`): a locked challenge must never sign anyone in.
- A locked or expired challenge never accepts a code again; the only way
  forward is starting over with a fresh sign-in
  (`LockedOrExpiredOnlyStartOver`).

Harm if broken: an attacker guessing codes against a locked challenge, or a
replayed expired challenge, signs in as the user.

## Coverage record

- Covered code: `internal/identity/mfa.go` (`evaluateChallengeState`,
  `loadChallenge`, `reserveMFAAttempt`, `consumeChallenge`, the
  `maxMFAAttempts`/`mfaChallengeLifetime` constants) and
  `internal/identity/mfa_test.go` (`TestEvaluateChallengeState`).
- Model: `docs/statecharts/sign-in-mfa/SignInMfa.tla`, config
  `SignInMfa.cfg` (17 distinct states, complete graph).
- Sync commit: `14a5d42b3c5d96a7e72759e6c20c13a3e91b370f`. The model and test were last confirmed
  against this commit.
- If the covered code has changed since the sync commit, this check needs a
  fresh look. Run the steps below and read what changed in `mfa.go` before
  trusting a pass. Move the sync commit forward only after you confirm the
  model still describes the code at the new commit.

## Run the check

1. Run the model check:
   `sh verification/tools/run-tlc.sh docs/statecharts/sign-in-mfa SignInMfa`
   The pass line is `Model checking completed. No error has been found.`
   The first run downloads the pinned TLA+ tools (checksum-verified) into
   `~/.cache/dynamis-tlc`. Java 11 or later must be on PATH.
2. Run the code-level guard of the same ordering:
   `go test ./internal/identity/ -run TestEvaluateChallengeState`
   The pass line is `ok example.com/dynamis-code/apps-template/internal/identity`.

Both must pass. The model checks every interleaving of attempts, expiry,
and sign-in; the Go test checks the evaluator's exact branch order on the
boundary cases (consumed before locked, locked at and beyond 5 attempts,
expired only while attempts remain).

## Reading a failure

- **TLC prints a counterexample:** it is the shortest sequence of sign-in
  steps that breaks the named rule. Replay each action against `mfa.go`,
  one step for each action (`SignIn`, `WrongCode`, `FifthWrongCode`,
  `RightCode`, `TimeOut`, `StartOver` map to challenge creation, a failed
  verify incrementing `attempts` via `reserveMFAAttempt`, the lock at
  `maxMFAAttempts`, the session grant guarded by `loadChallenge`'s
  `ChallengeActive` check, expiry against `expiresAt`, and a fresh sign-in).
  If the code does the same thing, it is a real bug: report it with the rule
  and the steps, and do not weaken the rule or the model. If the code cannot
  do a step the model takes, the model is wrong: fix the model to match the
  code, then rerun. To read a counterexample as a chart, use the
  `/statechart` skill by name.
- **The Go test fails:** the evaluator's branch order no longer matches the
  table. Read the `got`/`want` pair - it names the state and the input case.
  A real bug if the code change was unintended; an intended rule change if
  you meant to alter the order (see below).

## When a change is intended

If a change is meant to alter the rules (another attempt limit, a different
challenge lifetime, a new state):
1. Update the matching action or constant in `SignInMfa.tla`
   (`MaxTries` in `SignInMfa.cfg` follows `maxMFAAttempts`).
2. Update the invariant list in `SignInMfa.cfg` if a rule changed.
3. Update the `TestEvaluateChallengeState` cases if they encode the old rule.
4. Update the coverage record above.
5. Rerun both checks. Move the sync commit only after the checks pass
   against the changed code.

## Proof the check catches a break

Measured on this commit:
- **Model break:** in `SignInMfa.tla`, let `RightCode` also fire from
  `phase = "locked"`. TLC fails `SignedInWithinAttempts` with a
  counterexample: sign in, enter five wrong codes (the fifth locks the
  challenge), then enter the right code against the locked challenge and
  get signed in.
- **Code break:** in `internal/identity/mfa.go`
  `evaluateChallengeState`, change the lock guard to
  `attempts >= maxMFAAttempts + 100`. `TestEvaluateChallengeState` fails
  with `evaluateChallengeState() = active, want locked` for the
  at-limit and over-limit cases; the challenge stays accepting.

## Not covered

- Recovery codes and their single-use consumption: guarded by
  `TestMFAEnrollmentLoginRecoveryAndReplay`, not by this model.
- Passkey/WebAuthn ceremonies: the model treats any accepted factor as
  `RightCode`; the webauthn protocol checks are outside it.
- Real timing: the 5-minute lifetime is an explicit `TimeOut` action, not a
  clock; clock skew between the challenge store and the verifier is out of
  scope.
- Concurrency: `reserveMFAAttempt` and `consumeChallenge` are single-row
  conditional UPDATEs; their interleaving is not modeled.
- The lockout's user-visible reset path beyond starting over (rate limits,
  audit events) is not modeled.
