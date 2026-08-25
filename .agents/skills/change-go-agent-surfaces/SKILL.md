---
name: change-go-agent-surfaces
description: Add or change Dynamis Code MCP tools or remote CLI commands while preserving shared use cases, scoped authorization, bounded schemas, safety annotations, and the CLI REST-only boundary.
---

# Change Go Agent Surfaces

Read [AGENTS.md](../../../AGENTS.md), Phase 05 in
[PLAN.md](../../../PLAN.md), and the canonical REST, MCP, and CLI contracts that
currently exist.

## Outcome

Expose one deterministic automation capability without duplicating business
logic or bypassing authorization, REST, or application use cases.

## MCP workflow

1. Reuse an application use case; never call SQL, shell, filesystem, code
   execution, or transport handlers directly.
2. Define stable tool name/version, scoped permission, bounded structured input
   and output, pagination, errors, and read-only/destructive/idempotent/open-
   world annotations.
3. Treat every input and retrieved value as untrusted. Enforce Origin,
   principal/workspace scope, SSRF controls, result limits, and human approval
   signals where destructive.
4. Audit safe identifiers, annotations, target, outcome, duration, and
   request/trace correlation without raw payloads or secrets.

## CLI workflow

1. Call the versioned REST API only with bounded timeouts.
2. Keep results on stdout, errors on stderr, machine-readable output stable,
   exit statuses documented, and credentials absent from logs and errors.
3. Reuse REST authentication, errors, pagination, concurrency, idempotency, and
   rate-limit semantics rather than recreating them.

## WebMCP boundary

WebMCP is optional and browser-tab-bound; it is not server MCP and does not
change MCP scopes, tools, transport, or audit semantics. Feature-detect the
imperative browser API, register one-purpose versioned tools with explicit
bounded schemas, and expose only non-secret visible browser actions. Never
include passwords, OIDC material, invitation or token secrets, sessions,
CSRF fields, hidden inputs, or operator controls. Prepare and focus existing
controls without automatic state-changing submission, then preserve ordinary
HTML fallback when the API is unavailable.

## Verify and stop

Run MCP component tests, CLI-over-HTTP integration tests, wrong-scope and
redaction tests, then scan source and dependencies for forbidden database or
shell paths. Stop when the underlying use case or REST contract is missing;
implement that canonical capability first.
