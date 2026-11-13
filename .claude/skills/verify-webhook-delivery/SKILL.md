---
name: verify-webhook-delivery
description: Check webhook delivery outcomes and the delivery row's terminal states. Use when a change touches internal/webhooks/service.go (evaluateDeliveryOutcome, deliverOne, deliveryFailure, HandleExhaustedJob, the webhook_deliveries UPDATE statements), changes retry or exhaustion for webhooks, or changes what counts as a successful delivery.
---

# Verify webhook delivery

## The rules this checks

1. **Delivered only on a 2xx with no error.** A 1xx, 3xx, 4xx, or 5xx response, or any response with an error reason, must never mark a delivery delivered. Harm if broken: a receiver that did not accept the event is recorded as delivered, and the event is never retried.
2. **Failed only at the attempt limit.** A retryable failure before the limit gets a pending retry at `now + RetryDelay(attempt)`. Harm if broken: retries stop early, or a delivery retries past its budget.
3. **Every outcome carries its status code and error.** Harm if broken: the delivery log shows the wrong reason for a failure.
4. **A terminal delivery stays terminal.** Once `delivered` or `failed`, a later write must not change it. Every delivery UPDATE is guarded by `status = 'pending'`. Harm if broken: a delivered event is reopened and sent again.

Rules 1 to 3 are checked for every input in a bounded range by `TestDeliveryOutcomeRulesHoldForEveryInput`. Rule 4 is checked on the real database by `TestDeliveredRowIsNotOverwrittenByLaterFailure`.

## Why this is not a TLA+ model

`evaluateDeliveryOutcome` is a pure function. Its whole input space is small enough to enumerate. The lease and retry interleavings that could change a delivery's state are covered by `verify-job-queue`. So the check here is an exhaustive sweep of the real function, not a model of it. Go has no verifier that proves this function for all inputs, so the sweep covers the stated range exactly.

## What is covered

- Function: `evaluateDeliveryOutcome` in `internal/webhooks/service.go`, with its attempt and max-attempt clamps.
- Input range checked: attempt from -1 to 7, max attempts from 0 to 5, status code from -1 to 600 plus 999, and six reasons (including empty).
- Database writes: `deliveryFailure` and the success and exhaustion UPDATEs in `service.go`.
- Tests: `internal/webhooks/delivery_rules_test.go`.

Not covered: the HTTP response read and signature, the SSRF-safe dialer, and Postgres-specific SQL. The existing `TestEvaluateDeliveryOutcome` and `TestItemEventsAreSignedEncryptedAndDeliveredOnce` remain part of the suite.

## Coverage record

- Covered code: `internal/webhooks/service.go` (`evaluateDeliveryOutcome`, `deliveryFailure`, the delivered and exhausted UPDATEs) and `internal/webhooks/delivery_rules_test.go`.
- Sync commit: `11f139b483cbf15f0e47868c0f4f586e80995634`. The tests were last confirmed against this commit.
- If covered code changed since the sync commit, rerun the checks and read the diff for `service.go` before trusting a pass. Move the sync commit forward only after the checks pass against that code, and after you confirm the rules above still describe the intended behavior.

## Run the check

1. `go test ./internal/webhooks/ -run 'TestDeliveryOutcomeRules|TestDeliveredRowIsNotOverwritten|TestEvaluateDeliveryOutcome' -count=1`
2. Expect `ok`. The sweep runs in about 10 ms.
3. Run `verify-job-queue` too whenever the job lease or retry path changed.

## Read a failure

The failure message names the input: `attempt=… max=… code=… reason=…`, then what the function returned.
- A sweep failure at `code=300` (or any code outside 200–299) with `Status:delivered`: the success range was widened. Restore `statusCode >= 200 && statusCode < 300`, unless the product has decided to accept 3xx. That decision needs an ADR in `docs/decisions/`, not a silent change.
- A sweep failure with `want failed at max`: the attempt limit comparison changed. Check `attempt >= maxAttempts`.
- `TestDeliveredRowIsNotOverwrittenByLaterFailure` failing with `changed to "pending"`: a delivery UPDATE lost its `status = 'pending'` guard.

## When a change is intended

If the rules change on purpose (for example, a new success range or a new terminal state):
1. Update the oracle in `TestDeliveryOutcomeRulesHoldForEveryInput` to the new rule, and say in the commit which rule changed.
2. Update the coverage record above.
3. Rerun the checks. Move the sync commit only after they pass.

## Proof the check catches a break

Measured on this commit, in a scratch copy:
- **Guard break:** remove `AND status = 'pending'` from the failure UPDATE in `deliveryFailure`. `TestDeliveredRowIsNotOverwrittenByLaterFailure` fails with `delivered row changed to "pending"`.
- **Range break:** change `statusCode < 300` to `statusCode < 400` in `evaluateDeliveryOutcome`. `TestDeliveryOutcomeRulesHoldForEveryInput` fails at `code=300`. The existing `TestEvaluateDeliveryOutcome` and the other webhook tests still pass under this break, so the sweep is what catches it.
