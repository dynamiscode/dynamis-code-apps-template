# 0003: Embedded browser localization

- Status: accepted
- Date: 2026-08-27
- Owners: template maintainer

## Context

The server-rendered browser application and invitation email need English and
Spanish without a runtime translation dependency, JavaScript-only UI state, or
a second API contract. Workspace language must provide a stable fallback for
browser pages and determine invitation email language.

## Decision

Keep Git-reviewed JSON catalogs embedded in `internal/i18n`, with a small
application-owned translator using `golang.org/x/text/language` for supported
locale validation and `Accept-Language` matching. Catalogs use namespaced keys,
English fallback, named interpolation, and plain strings rendered through the
existing `html/template` escaping path. Dates are formatted from stored UTC
values according to the resolved locale. No plural engine is added until a
count-dependent product message exists.

Browser precedence is user preference, explicit locale cookie,
workspace/invitation locale where applicable, `Accept-Language`, then English.
Workspace locale is always `en` or `es`; a user Automatic preference is NULL.
Invitation emails use the current workspace locale. REST, CLI, MCP, and WebMCP
identifiers/contracts remain language-neutral and REST error details remain
stable English.

## Consequences

Translations are reviewable, deterministic, offline, and available during
startup. Catalog parity and malformed-template tests catch drift. Adding a
language changes versioned source files and tests, rather than runtime state.
The workspace locale is additive in the existing `dynamis-code.workspace/v1`
export. The browser carries a small request-scoped locale wrapper, while
existing CSRF, authorization, realtime, and WebMCP behavior remain shared.

## Revisit when

Add CLDR plural support with `golang.org/x/text/message` or `go-i18n` when the
first localized product message depends on a count, or adopt a translation
service only when catalog volume or translation workflow requires it.
