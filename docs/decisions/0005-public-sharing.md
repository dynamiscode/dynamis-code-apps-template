# 0005: Bounded Public Item Sharing

- Status: accepted
- Date: 2026-08-28
- Owners: template maintainers

## Context

Users need to show an Item to visitors without adding those visitors to a
workspace. The first slice must not broaden the resource model or expose
workspace identity, membership, creator, audit, or credential data.

## Decision

Share Items only through read-only browser pages addressed by opaque,
cryptographically random bearer tokens. Store only a token hash. Links expire
after seven days by default, may last at most 30 days, and cannot never expire.
Only workspace principals with `resources:write` may create or revoke links.
Public pages expose only Item title and status. Access is rate-limited,
audited without the token, and protected with no-store/private caching,
no-referrer, and noindex headers. Permanent Item deletion cascades to links.

There is no public write, search, listing, indexing, membership, REST, CLI,
MCP, or WebMCP surface. File sharing and other resource types remain later
work.

## Consequences

The shared service owns token lifecycle, workspace checks, projection, and
audits while the browser owns CSRF and rendering. Expired and revoked records
are pruned by normal maintenance. A visitor cannot be granted workspace
membership through a link.

## Revisit when

File sharing, public API consumers, guest collaboration, or resource types
beyond Items require a separate accepted contract.
