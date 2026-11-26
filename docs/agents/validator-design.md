# Validator design

Before implementing a validator or migrating callers to it, record:

- Accepted inputs and the intended output.
- Rejected inputs and the observable error behavior.
- Resource limits, including behavior when a limit is reached.
- The scope the validator proves and any explicitly reviewed trust boundaries.

Exercise those boundaries with runnable adversarial cases before migrating
callers. Include valid inputs that resemble rejected inputs, so the check does
not silently narrow supported behavior.

If two fixes fail the same gate, reproduce and test their shared premise before
another patch. Record the result, compare bounded designs, and select one that
meets the contract. Do not treat a passing happy path as proof of unsupported
inputs or unresolved bypasses.
