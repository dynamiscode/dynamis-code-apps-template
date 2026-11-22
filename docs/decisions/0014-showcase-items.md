# 0014: Showcase items

- Status: proposed
- Date: 2026-11-22
- Owners: template maintainers

## Context

Businesses on this template want to show finished work to the public. Decision
[0005 public sharing](0005-public-sharing.md) covers one case only: a
bearer-token link to an Item, exposing title and status. It cannot carry
photos, a category, or a public listing, and it must not grow to.

Issue #567 (CompanySite showcase) resolved the product questions. This record
turns that resolution into an architecture decision.

## Decision

Showcase items are a new resource type with their own public, read-only pages.
They are not a mode of Items or of Item sharing links.

- **Profile.** Showcase belongs to an optional `CompanySite` profile, outside
  Core. It requires Identity and Files. It gives the public no write path.
- **Shape.** One item has a title, a short description, a category, and a few
  photos. It holds no client name, address, or contact detail, and no public
  link to a client record.
- **Publication.** Items are unpublished by default. A staff member publishes
  one only after recording the client's consent: who gave it, when, and how.
  Unpublishing is always allowed and takes effect immediately.
- **Photos.** Photos live in Files, workspace scoped. A showcase item stores
  file ids (rows in the file table), not links or URLs. Files gains no client
  or engagement link. The public route resolves the ids and serves a photo
  only while its item is published.
- **Indexing.** Public pages send `noindex` by default. The workspace owner
  opts in per workspace.
- **Languages.** English and Spanish. Item text is entered per language and
  falls back to English.
- **Decision 0005 stays as written.** Its status and wording do not change.
  Showcase pages do not use sharing tokens.

## Consequences

Benefits: the public surface is small, read-only, and separate from the Item
sharing contract. Consent is recorded before anything is public. Photos stay
inside existing Files storage and workspace scoping.

Costs: a new resource, a new public route, and a consent record to build,
test, and document. Public photo serving needs its own publication check.

Rejected alternatives:

- Extend Item sharing links with photos and a listing. This breaks the bounded
  contract of 0005 and mixes bearer-token access with public listing.
- Store photo URLs on the item. URLs can outlive unpublishing and bypass the
  publication check.
- Link showcase items to client records. This risks exposing client identity.

## Revisit when

- A workspace needs public write paths, such as comments or intake. That is a
  separate decision.
- Showcase must appear in Core rather than the optional `CompanySite` profile.
- Files gains a general public-serving contract that replaces the per-item
  publication check.
