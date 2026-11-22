# Coding standards

This file lists judgement-call standards for review. Mechanical rules belong in tooling, not here.

## Behavior docs

Describe only the paths the code covers.

- Do not claim "every", "always", "all", or "each path" unless a test enumerates each case.
- Name an exception in the same paragraph as the claim it limits.

## Scope fidelity

Every changed file and column traces to the stated task. Reviewer flags additions outside it, and any step presented as complete that was applied to only some surfaces.

## Credential columns

Schema changes touching credential, secret, or token columns need explicit user confirmation in the session before the migration lands.

## Shared components

List and table toolbar markup lives in shared templ components. Per-page copies of shared markup fail review.

## Verify by effect

Claim CI parity only after running that smoke locally. Verify remote mutations by re-fetched state or remote SHA, not command output.
