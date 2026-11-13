---
name: verify-file-upload
description: Check the private file lifecycle (pending, ready, failed), its quota reservation, and the rule that a ready file has its stored content. Use when a change touches internal/files/service.go (reserve, Initiate, Upload, PutContent, Complete, completeDB, transitionFile, markFailed, CleanupStaleFiles, validTransitions), the file HTTP routes in internal/httpapi/files.go, or the presigned upload path.
---

# Verify file upload

## The rules this checks

1. **Quota holds.** Pending, ready, and failed files together never exceed the workspace byte limit. Harm if broken: a workspace stores more than its limit, and the limit stops protecting storage.
2. **A ready file has matching content.** A file becomes ready only when its stored object exists and its size equals the size it reserved. Harm if broken: a user downloads a file that is missing or differs from its record.
3. **Ready is final, and failed never becomes ready.** Harm if broken: a file that failed validation is later accepted, or a ready file changes.

## What is modeled

- Model: `verification/files/FileUpload.tla`, config `verification/files/FileUpload.cfg`.
- Bounds: two files, workspace limit 3 bytes, object limit 2 bytes. About 176 distinct states.
- Actions map to code: `Reserve` is `reserve()` (quota check, then INSERT as pending), `PutObject` and `PutWrongSize` are the stored object (validated by `PutContent`, or a client-chosen size on the presigned path), `CompleteAccepts` and `CompleteRejects` are `Complete` and `completeDB`, `Abort` is `markFailed` during `PutContent`, and `Cleanup` is `CleanupStaleFiles`.
- Each action is treated as one serializable transaction, matching the `SERIALIZABLE` transactions in `service.go`.
- Abstracted away: MIME types, owners, signed URLs, transient store errors (they leave a file pending, which the model already allows), and real timing for cleanup.

## Coverage record

- Covered code: `internal/files/service.go` (`reserve`, `completeDB`, `Complete`, `PutContent`, `transitionFile`, `markFailed`, `CleanupStaleFiles`) and the tests listed below.
- Sync commit: `11f139b483cbf15f0e47868c0f4f586e80995634`. The model was last confirmed against this commit.
- If covered code changed since the sync commit, rerun the checks and read the diff before trusting a pass. Move the sync commit forward only after the checks pass against that code and you confirm the model still matches it.

## Run the check

1. Run the model check:
   `verification/tools/run-tlc.sh verification/files FileUpload`
   The pass line is `Model checking completed. No error has been found.`
2. Run the Go checks against the real code:
   `go test ./internal/files/ -run 'TestLocalFileLifecycleAndValidation|TestWorkspaceQuotaAndIsolation|TestPresignFailureReleasesPendingQuota|TestFileStateTransitions|TestQuotaCalculationCleanupStaleAndFailedFiles' -count=1`
3. Both must pass. For the Postgres path, set `POSTGRES_TEST_URL` and run `TestPostgresFileStateTransitions` too.

## Read a failure

- **TLC prints a counterexample:** it is the shortest sequence of actions that breaks the named rule. Replay each action against `service.go`, one step for each action. If the code can do the same thing, it is a real bug: report the rule and the steps, and do not weaken the rule or the model. If the code cannot take a step the model takes, the model is wrong: fix the model, then rerun. To see a counterexample as a chart, use the `/statechart` skill by name.
- **A Go test fails:** the test names the failed expectation. For example, `failed object quota error = <nil>, want ErrLimit` means the reserve quota check is gone.

## When a change is intended

If the rules change on purpose (a new file state, a different quota rule, a new way to complete an upload):
1. Update the matching action in `FileUpload.tla`, then the invariants in `FileUpload.cfg` if a rule changed.
2. Update any Go test expectations that encode the old rule.
3. Update the coverage record above.
4. Rerun both checks. Move the sync commit only after they pass.

## Proof the check catches a break

Measured on this commit, in scratch copies:
- **Reserve quota removed** (`service.go` reserve, `used > s.maxWorkspaceBytes-size`): the model fails `QuotaHolds` with a 2-step counterexample (two reservations of 2 bytes against a limit of 3). `TestLocalFileLifecycleAndValidation` and `TestQuotaCalculationCleanupStaleAndFailedFiles` both fail.
- **Object size check removed** (`Complete`, `object.Size != file.Size`): the model fails `ReadyHasMatchingContent` (a presigned object of a different size is completed). `TestLocalFileLifecycleAndValidation` fails.
- **Completion quota check removed** (`completeDB`, the `used > …` check after the `id <> ?` sum): no test fails, and the model still passes `QuotaHolds` when that check is removed. This is expected: completion keeps the reserved size, and the size stays equal to the reservation, so the check cannot trigger. Treat it as redundant defense, not a gap. If a future change lets completion change the size, this check becomes the only quota guard, and this skill must be rerun.

## Not covered

- Concurrency beyond the model's two files, and the real `SERIALIZABLE` retry behavior on Postgres.
- Storage failures (S3 or local disk) beyond the "pending stays pending" behavior.
- Real cleanup timing (`CleanupStaleFiles` cutoff) and the file HTTP routes.
