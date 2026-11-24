# Formal verification maintenance

Run `node scripts/check-formal-verification.mjs` to find covered files changed
since each skill's recorded sync commit. A warning requests review; it does not
prove drift or authorize updating the commit. Missing history is reported as
unknown. The check also validates explicit repository file paths in skill text.
Keep `verification/coverage.json` aligned with each skill when covered code
moves or new paths begin writing modeled state. Git compares tracked files;
the reviewer must also inspect untracked files and new writers.
Generated applications have profile-specific files and history, so the template
check skips them; establish a coverage manifest and sync baseline for the app.

For each check, record the following before advancing its sync commit:

- Prior sync commit and target commit; every covered change and any new writer.
- Baseline result and whether each change affects the modeled behavior.
- Model or harness changes, or why the existing abstraction still applies.
- Rerun result and a mutation that fails the relevant property, then restore it.
- Outcome: in sync, updated, bug, or blocked; counterexample and cause if failed.
- Bounds, assumptions, uncovered paths, and any checks not run.

Advance a sync commit only after the covered changes are reviewed and the check
passes against the target commit. Keep it unchanged for a blocked check. A
passing model alone is not evidence that it describes current code.
