# Architecture Decisions

Use a decision record for a durable choice with meaningful alternatives or a
revisit trigger. Do not create records for routine implementation details.

File name: `NNNN-short-title.md`.

Each record contains:

```md
# NNNN: Title

- Status: proposed | accepted | superseded
- Date: YYYY-MM-DD
- Owners: names or roles

## Context
Problem, constraints, and evidence.

## Decision
Chosen behavior and boundaries.

## Consequences
Benefits, costs, and rejected alternatives.

## Revisit when
Concrete evidence that reopens the decision.
```

Standards deviations use the stricter format under `exceptions/`, not an
ordinary decision record.
