# Documentation and Context Router

The template is under construction. Documents under `docs/implementation/`
are construction briefs; capability claims require linked evidence.

## Choose context by task

| Task | Read | Skill |
|---|---|---|
| Start or complete a build phase | `PLAN.md`, active phase brief, linked `STANDARDS.md` sections | None; one-time construction stays in the plan |
| Add an ordinary feature slice | Active phase, Sections 2, 3, 6, and 7 | `implement-go-feature` |
| Change schema, migrations, queries, or repositories | Phase 01 or 06; Sections 4 and 5 | `change-go-data` |
| Change authentication, authorization, workspaces, roles, sessions, invitations, tokens, or OIDC | Phase 02; Sections 4, 6, and 7 | `change-go-identity` |
| Add or change MCP or CLI behavior | Phase 05; Sections 3, 7, and 9 | `change-go-agent-surfaces` |
| Verify a phase, profile, release, or final handoff | `docs/capabilities.md`, active phase, Section 13 | `verify-template-conformance` |
| Propose a deferred capability | `docs/capabilities.md`, Section 8 | None; confirm its trigger before planning implementation |
| Change architecture or standards | Relevant accepted decision and `STANDARDS.md` | None; present options before editing the decision |

## Current sources of truth

| Concern | Source |
|---|---|
| Normative generation requirements | `STANDARDS.md` |
| Accepted technology choices | `docs/decisions/` |
| Construction order and status | `PLAN.md` |
| Phase outcomes and gates | `docs/implementation/` |
| Agent obligations | `AGENTS.md` |
| Conformance status and deferred triggers | `docs/capabilities.md` |
| Recurring procedures | `.agents/skills/` |

Implemented sources: [architecture](architecture.md),
[configuration](configuration.md), and [authentication](authentication.md).
As later phases land, code, tests, OpenAPI,
migrations, deployment files, and final domain documents replace temporary
sources defined by `STANDARDS.md` Section 12.

## Documentation timing

Create final application documents only when their behavior exists:
`architecture.md`, `configuration.md`, `deployment.md`, `operations.md`,
`authentication.md`, `api.md`, `mcp.md`, and `cli.md`. Replace this temporary
construction workflow with the final `development.md`. Phase 07 verifies all
links and removes temporary material only after the application is conforming.
