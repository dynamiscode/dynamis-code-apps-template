# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers use the template to generate, customize, deploy, and maintain
applications. The users and purpose of each generated application are
application-owned decisions.

## Product Purpose

Dynamis Code Apps Template provides a reusable foundation for Go applications
with browser, API, and optional agent interfaces. Success means a generated
application can be customized without losing the template's documented
security, accessibility, and operational boundaries.

## Positioning

The template combines a resource-conscious Go modular monolith with
server-rendered HTML, shared application use cases, and optional agent
interfaces. SQLite is the default for a single instance; PostgreSQL supports
multiple instances.

## Operating Context

Developers generate applications from verified releases, choose supported
profiles, then replace template identity and sample behavior with
application-owned decisions. Template updates are reviewed and ported rather
than copied over customizations.

Workspace members use the browser interface to navigate workspaces, manage
resources, and perform actions allowed by their role. Owners and administrators
manage workspace settings.

## Capabilities and Constraints

- The web interface uses server-rendered HTML, ordinary forms, and progressive
  enhancement; forms retain a non-JavaScript fallback.
- Web, REST, and optional MCP interfaces use shared application use cases; the
  remote CLI calls REST only.
- Workspace scope and permissions apply to application operations.
- Browser localization supports English and Spanish.
- Product purpose, domain behavior, and end-user needs for generated
  applications remain undecided by the template.

## Brand Commitments

The template name is Dynamis Code Apps Template. Generated applications receive
application-owned identity and branding.

## Evidence on Hand

- `README.md` describes setup, generation, and verification.
- `docs/template-lifecycle.md` describes generation and updates.
- `docs/web.md` records browser capabilities and workflows.
- `docs/accessibility.md` records accessibility checks and evidence.

## Product Principles

- Preserve application owners' control over product identity and domain behavior.
- Keep browser tasks usable with ordinary navigation and forms.
- Share business rules across interfaces while enforcing workspace permissions.
- Keep deployment resource-conscious and support documented operational paths.

## Accessibility & Inclusion

The template targets WCAG 2.2 Level AA. Preserve keyboard access, visible
focus, accessible names and feedback, narrow-screen reflow, and reduced-motion
support.
