# Dynamis Code Apps Template Implementation Plan

This plan controls construction of the template. `STANDARDS.md` remains the
normative generation source until Phase 07 completes.

## Status rules

Allowed phase states: `pending`, `in progress`, `blocked`, `complete`.

- Work on one phase at a time unless an independent prerequisite is explicitly
  recorded.
- A phase is complete only when every required outcome exists, its commands
  pass, and `docs/capabilities.md` links the evidence.
- A failure stays visible. Do not weaken a test or relabel a mandatory item to
  finish a phase.
- A deviation requires an accepted decision or structured exception.

## Feature surface gate

Each feature phase must name its shared application behavior, authorization
boundary, and applicable delivery surfaces: browser, optional browser-agent
(WebMCP), REST/OpenAPI, CLI/MCP, realtime, and operations/data. An omitted
surface needs a written reason and stays outside that phase's acceptance. A
phase cannot complete until each applicable surface has implementation,
focused tests, canonical documentation, and linked capability evidence.

## Build order

| Phase | State | Brief | Depends on |
|---|---|---|---|
| 01 Foundation | complete | [Foundation](docs/implementation/01-foundation.md) | Preparation harness |
| 02 Identity and tenancy | pending | [Identity and tenancy](docs/implementation/02-identity-and-tenancy.md) | 01 |
| 03 HTTP and REST | pending | [HTTP and REST](docs/implementation/03-http-and-rest.md) | 01, 02 authorization boundary |
| 04 Web, realtime, accessibility | pending | [Web, realtime, accessibility](docs/implementation/04-web-realtime-accessibility.md) | 02, 03 |
| 05 MCP and CLI | pending | [MCP and CLI](docs/implementation/05-mcp-and-cli.md) | 02, 03, one shared use case from 04 |
| 06 Operations and data lifecycle | pending | [Operations and data lifecycle](docs/implementation/06-operations-and-data-lifecycle.md) | 01-05 |
| 07 Delivery and handoff | pending | [Delivery and handoff](docs/implementation/07-delivery-and-handoff.md) | 01-06 |

## Global acceptance

The finished template must:

- satisfy and evidence `Core`, `Identity`, and `Agent`;
- expose web, REST, MCP, and CLI through shared use cases;
- optionally expose selected non-secret browser workflows through feature-
  detected WebMCP tools without changing server MCP or REST contracts;
- expose baseline human identity workflows in the browser, including workspace
  creation/selection, membership, invitations, tokens, sessions, configured
  OIDC, and workspace export; REST identity management must be bearer-authenticated
  while workspace listing/creation remain browser-only;
- start as one documented SQLite-backed container and support optional
  PostgreSQL without domain changes;
- enforce workspace isolation and owner/admin/member/viewer permissions;
- be observable, secure, recoverable, accessible, and releasable without
  bundling infrastructure that lacks a trigger;
- retain one runnable sample feature through the full smoke path;
- emit `template.lock` in generated applications; and
- support first-owner bootstrap through environment variables, a protected
  browser setup form, and the explicit CLI fallback, with all paths atomic and
  granting separate instance administration; and
- replace temporary plans with implemented-behavior documentation before
  deleting `STANDARDS.md`.

WebMCP is browser-tab-bound and progressive: unsupported browsers retain
ordinary HTML behavior, server MCP remains persistent and authoritative, and
browser-agent tools never receive passwords, OIDC material, invitation or
token secrets, sessions, CSRF fields, or hidden form values.

Capabilities listed as deferred remain outside implementation unless their
trigger is accepted and the plan is updated first.
