# 0009: Short-ID codec uses sqids-go

- Status: accepted
- Date: 2026-11-11
- Owners: template maintainers

## Context

Entities need a short, URL-safe public identifier derived from an integer ID,
with a per-type alphabet so an ID from one type cannot be mistaken for another.
Sqids is a small, documented encoding for this. Writing and maintaining a
compatible implementation in-repo would duplicate a known algorithm, and
CONTRIBUTING.md requires an accepted decision before adding a dependency.

## Decision

- Add `github.com/sqids/sqids-go` v0.4.1 (MIT) as a runtime dependency, used
  only by `internal/platform/id`. The upstream license is listed in
  [NOTICE](../../NOTICE).
- The codec uses the alphabet `23456789abcdefghjkmnpqrstuvwxyz`, minimum
  length 4, and no prefix. The per-type alphabet is shuffled from the type
  code and, when set, an optional seed. The seed is hashed with SHA-256 and
  joined to the type code with a NUL separator, so a type code and a seed
  cannot swap roles.
- Strict decode rejects bad length, characters outside the codec alphabet, and
  any input that does not re-encode to the same string.

## Consequences

- Benefit: the encoding is the upstream algorithm, not a local reimplementation.
- Cost: one new runtime module. Release SBOMs must include it.
- Cost: passing Sqids options disables its default blocklist, so a short ID
  could spell a banned word. This is accepted for now and marked in code.
- Rejected: a hand-written Sqids implementation inside the repository. It
  would duplicate the upstream algorithm without a benefit that justifies the
  maintenance.

## Revisit when

- An encoded ID is found to contain a word that must be blocked.
- The upstream library stops receiving security or correctness fixes.
