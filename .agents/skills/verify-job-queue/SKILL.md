---
name: verify-job-queue
description: Check the background job queue's lease, retry, and exhaustion rules. Use when a change touches internal/jobs/queue.go (claim, finish, Process, EvaluateJobOutcome, RetryDelay, the lease SQL), changes how a job is enqueued or retried, or changes a caller that relies on job retries or exhaustion callbacks, such as webhook delivery.
---

# Verify the job queue

## The rules this checks

1. **A stale worker cannot commit.** A worker whose lease expired and was reclaimed must not change the job. Harm if broken: a job is marked `succeeded` or `failed` while another worker still runs it.
2. **A terminal job stays terminal.** Once `succeeded` or `failed`, no step changes it. Harm if broken: a finished job runs again.
3. **The handler runs only within max attempts.** Harm if broken: a job runs past its retry budget.
4. **A job fails only after its attempts are exhausted.** Harm if broken: a job gives up early or never gives up.
5. **At most one worker holds the current lease.** Harm if broken: two workers run the same attempt.

Rule 1 is the one the model found to be most at risk. Rule 1 is also what `TestStaleLeaseFinishCannotOverwriteReclaimedJob` checks in Go.

## What is modeled

- Model: `verification/jobs/JobQueue.tla`, with config `verification/jobs/JobQueue.cfg`.
- Bounds: 2 workers, `MaxAttempts = 2`. About 143 distinct states.
- Each action names the code it models: `Claim` is `claim()`, `Execute` is the handler call in `Process()`, `FinishApplied` and `FinishStale` are `finish()`'s UPDATE with and without a matching `lease_token`, `Expire` is `leased_until` passing, and `Outcome` is `EvaluateJobOutcome`.
- Abstracted away: real clocks (lease expiry is an explicit `Expire` step), the error categories a handler returns (only ok or err), and the `handler-unavailable` branch.

## Coverage record

- Covered code: `internal/jobs/queue.go` (`claim`, `finish`, `Process`, `EvaluateJobOutcome`) and `internal/jobs/queue_lease_test.go`.
- Sync commit: `11f139b483cbf15f0e47868c0f4f586e80995634`. The model and test were last confirmed against this commit.
- If the covered code has changed since the sync commit, this check needs a fresh look. Run the steps below and read what changed in `queue.go` before trusting a pass. Move the sync commit forward only after you confirm the model still describes the code at the new commit.

## Run the check

1. Run the model check:
   `verification/tools/run-tlc.sh verification/jobs JobQueue`
   The pass line is `Model checking completed. No error has been found.` The first run downloads the pinned TLA+ tools (checksum-verified) into `~/.cache/dynamis-tlc`. Java 11 or later must be on PATH.
2. Run the Go checks against the real code:
   `go test ./internal/jobs/ -run 'TestStaleLeaseFinish|TestQueue|TestEvaluateJobOutcome' -count=1`
3. Both must pass. If either fails, go to "Read a failure".

## Read a failure

- **TLC prints a counterexample:** it is the shortest sequence of actions that breaks the named rule. Replay each action against `queue.go`, one step for each action. If the code does the same thing, it is a real bug: report it with the rule and the steps, and do not weaken the rule or the model. If the code cannot do a step the model takes, the model is wrong: fix the model to match the code, then rerun. To see a counterexample as a chart, use the `/statechart` skill by name.
- **The Go test fails:** the test prints the job state it found. For `TestStaleLeaseFinishCannotOverwriteReclaimedJob`, `status=succeeded` with another worker's lease means rule 1 is broken in the code.

## When a change is intended

If a change is meant to alter the rules (a new status, another retry policy, a different lease rule):
1. Update the matching action in `JobQueue.tla` and the matching `Outcome` branch to the new rule.
2. Update the invariant list in `JobQueue.cfg` if a rule changed.
3. Update the Go test expectations if they encode the old rule.
4. Update the coverage record above.
5. Rerun both checks. Move the sync commit only after the checks pass against the changed code.

## Proof the check catches a break

Measured on this commit:
- **Model break:** in `FinishApplied`, drop the `held[w] = lastTok` guard and record `staleCommit`. TLC fails `NoStaleCommit` with a 5-step counterexample: worker 1 claims, executes ok, its lease expires, worker 2 reclaims, worker 1 finishes and the job becomes `succeeded` while worker 2 holds it.
- **Code break:** in `internal/jobs/queue.go` `finish`, remove `AND lease_token = ?` and its argument. The existing `TestQueue*` tests still pass, so they do not cover this. `TestStaleLeaseFinishCannotOverwriteReclaimedJob` fails with `status=succeeded` while the lease belongs to the reclaiming worker.

## Not covered

- Real timing: lease length, `RetryDelay` values, and clock skew between workers.
- Handler idempotency: the model is at-least-once, and it does not check what a handler does twice.
- Postgres behavior: the Go test runs on SQLite. Run `internal/jobs/postgres_test.go` as well when the SQL changes.
- Liveness: the model does not prove that every job eventually finishes. It checks only safety.
