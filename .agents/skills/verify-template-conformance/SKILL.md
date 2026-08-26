---
name: verify-template-conformance
description: Verify a Dynamis Code implementation phase, conformance profile, release, or final standards handoff read-only and report passed, failed, blocked, exception-backed, and not-applicable evidence.
---

# Verify Template Conformance

This skill verifies; it does not fix. Read [AGENTS.md](../../../AGENTS.md),
[docs/capabilities.md](../../../docs/capabilities.md), the relevant permanent
contracts, release workflow, and evidence.

## Outcome

Produce an evidence-backed result that cannot confuse described behavior with
working behavior.

## Workflow

1. Resolve the exact profile, release, change, or handoff target. Stop if the
   target is ambiguous.
2. Enumerate its required outcomes, commands, manual evidence, exceptions, and
   source-of-truth links.
3. Inspect implementation and contracts before running checks. Do not accept a
   document, mock, or generated file as runtime evidence by itself.
4. Run focused checks, then every applicable target gate in `AGENTS.md` and
   release documentation. Read complete failures and preserve the first root
   cause.
5. Confirm each accepted exception is unexpired, approved, scoped, evidenced,
   and linked to compensating controls.
6. Check capability status, documentation links, generated drift, secrets,
   unrelated changes, and final handoff ownership.

## Report

For each requirement report exactly one status:

- `passed`: observable evidence succeeded;
- `failed`: evidence ran and contradicted the requirement;
- `blocked`: the check could not run, with the exact blocker;
- `exception`: an accepted unexpired exception covers it; or
- `not applicable`: the standard's applicability condition is false.

Do not mark blocked or documented-only behavior as conforming. A release is
conforming only when every applicable gate has observable evidence.
