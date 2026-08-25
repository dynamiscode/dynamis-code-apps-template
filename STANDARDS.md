# Scalable Web Application Template Standards

This document defines a reusable standard for web applications that start
small, remain easy to operate, and can scale without being rebuilt around a
different architecture. It is written for maintainers, contributors, and
software agents. It does not require a particular programming language,
framework, cloud, or vendor.

This is a temporary generation specification. It is authoritative while a new
application is being designed and generated. Once the application implements
the selected requirements and creates its permanent `docs/` documentation,
this file SHOULD be deleted. The generated application's code, contracts,
migrations, tests, `AGENTS.md`, and domain documentation then become its
sources of truth.

The words **MUST**, **SHOULD**, and **MAY** are normative:

- **MUST** marks a requirement for security, correctness, compatibility, or
  data safety.
- **SHOULD** marks the default choice. A documented reason may justify a
  different choice.
- **MAY** marks an optional capability.

## Table of contents

- [Conformance model](#conformance-model)
1. [Purpose and vision](#1-purpose-and-vision)
2. [Core architecture](#2-core-architecture)
3. [Required application interfaces](#3-required-application-interfaces)
4. [Authentication and authorization](#4-authentication-and-authorization)
5. [Data and scaling](#5-data-and-scaling)
6. [Build once](#6-build-once)
7. [Apply on every relevant change](#7-apply-on-every-relevant-change)
8. [Add only when triggered](#8-add-only-when-triggered)
9. [MCP and agent readiness](#9-mcp-and-agent-readiness)
10. [Documentation layout](#10-documentation-layout)
11. [Minimum AGENTS.md contract](#11-minimum-agentsmd-contract)
12. [Sources of truth](#12-sources-of-truth)
13. [Testing and acceptance](#13-testing-and-acceptance)
14. [Explicit non-goals](#14-explicit-non-goals)

## Conformance model

A profile is a bundle of requirements for an application. It is not a user
profile or role.

| Profile | Applies when | Requirements |
|---|---|---|
| `Core` | Every generated application | Architecture, configuration, SQLite-first data, optional PostgreSQL, REST/OpenAPI, health, security foundations, tests, containers, and documentation |
| `Identity` | Every generated application | Workspaces, owner/admin/member/viewer roles, invitations, local authentication, optional OIDC, sessions, scoped tokens, and identity audit events |
| `Agent` | Every generated application | MCP, remote REST-only CLI, tool scopes, structured schemas, safety annotations, and tool audit events |
| `Production` | A deployment serves real users or durable data | Observability, operational targets, rate limits, tested recovery, supply-chain evidence, release verification, and accessibility evidence |

Every generated application MUST implement `Core`, `Identity`, and `Agent`.
`Production` is a deployment conformance profile and MUST be completed before
a deployment serves real users or durable data. Production conformance does
not select a database: a conforming single-instance deployment MAY use SQLite,
while multiple application instances require PostgreSQL or another supported
shared database.

The generated `docs/capabilities.md` MUST record each profile as `conforming`,
`not applicable`, or `exception`, with links to its evidence. A requirement is
not conforming merely because it is described in documentation.

### Exceptions

An implementation MAY deviate from a requirement only through a structured
exception stored under `docs/decisions/exceptions/`. Each exception MUST state:

- the requirement and affected profile;
- reason and supporting evidence;
- user, security, compatibility, and operational risk;
- compensating controls;
- responsible owner and approving maintainer;
- creation date, review date, and expiration date;
- replacement or removal path.

Security-sensitive exceptions require approval from the repository's security
owner. Temporary exceptions MUST be reviewed within 90 days. An expired
exception is non-conforming until renewed or removed. A permanent change to
the architecture MUST update `docs/architecture.md`; it MUST NOT remain an
evergreen exception.

## 1. Purpose and vision

The template MUST produce an application that:

- runs as a useful single-instance web application with a small operational
  footprint;
- has a documented path to PostgreSQL and multiple application instances;
- supports humans through a web interface and REST API;
- supports automation through a remote command-line interface (CLI);
- supports software agents through Model Context Protocol (MCP);
- applies the same business rules and authorization through every interface;
- is secure, observable, recoverable, accessible, and testable by default;
- avoids infrastructure that the application does not yet need.

The default installation SHOULD be one application container with persistent
SQLite storage. It MUST NOT require a reverse proxy, cache, broker, object
store, orchestration platform, or external observability backend merely to
start.

Simple defaults do not excuse missing production fundamentals. Authentication,
authorization, health checks, structured errors, auditability, backups,
security controls, and release evidence are part of the baseline.

## 2. Core architecture

### Modular monolith

The application SHOULD begin as a modular monolith organized by vertical
feature. Each feature keeps its business rules, use cases, ports, and tests
close together.

```text
Web UI ─┐
REST ───┼──> application use cases ──> data and external adapters
MCP ────┤
CLI ────┘  (through REST only)
```

The architecture MUST follow these rules:

- Business rules live in domain or application code, not in HTTP, MCP, CLI,
  database, or user-interface adapters.
- Web, REST, and MCP adapters call the same application use cases.
- The CLI calls the remote REST API and MUST NOT access application storage
  directly.
- Dependencies are explicit and assembled in one composition root.
- Interfaces or ports exist only at real input/output boundaries or useful
  test seams. A single concrete implementation does not require an interface.
- Application use cases own transactions that span multiple data operations.
- Request context is passed explicitly and is not stored in long-lived global
  state.
- Package globals, service locators, hidden initialization, mutable
  singletons, and generic repositories SHOULD be avoided.
- Validation, transformation, authorization, and domain calculations SHOULD
  be pure where practical.

### Transport and deployment boundaries

Application instances MUST become stateless before horizontal scaling:

- durable sessions, tokens, memberships, and invitations live in the database;
- required data does not live in a container filesystem;
- in-memory caches are disposable;
- instance-local events are not treated as shared events;
- graceful shutdown drains active requests and closes resources.

Microservices MUST NOT be the default. Split a module only when deployment,
scaling, reliability, security, or ownership boundaries provide measured
benefit greater than the operational cost.

## 3. Required application interfaces

Every generated application MUST expose these interfaces:

| Interface | Audience | Contract |
|---|---|---|
| Web application | Human users | Accessible browser interface using the same use cases as the API |
| REST API | Users and integrations | Versioned HTTP API described by OpenAPI |
| MCP endpoint | Software agents | Authenticated Streamable HTTP tools with structured schemas |
| Remote CLI | Humans and agents | REST client with deterministic output and exit statuses |

Baseline capabilities MUST have a usable, accessible human browser surface
where a human workflow applies; service-level use cases and tests alone do not
make an Identity or Core capability conforming. At minimum this includes
workspace creation and selection, membership and ownership management,
invitation acceptance, scoped token and session management, configured OIDC
login/linking, and authorized workspace export. A decision MAY keep a specific
operation browser-only, such as workspace listing or creation, when the
permanent API and web documentation state that boundary explicitly.

WebMCP MAY provide an optional browser-tab agent enhancement for non-secret
human workflows. It is distinct from the persistent, bearer-authenticated
server MCP endpoint and MUST NOT be counted as `Agent` conformance. A WebMCP
surface MUST be feature-detected, same-origin constrained, schema-bound,
secret-free, and safe when unsupported; ordinary browser navigation and forms
remain authoritative.

### REST API

- Public paths MUST be versioned, for example `/api/v1`.
- OpenAPI MUST be the source of truth for public HTTP operations and schemas.
- The OpenAPI version SHOULD be a supported 3.1 release.
- Authentication, pagination, rate limits, errors, and idempotency behavior
  MUST be represented in the contract.
- Generated clients MAY be used to keep server and CLI types aligned.
- Breaking changes MUST use a new API version or a documented migration path
  after a stable release exists.

### CLI

The CLI MUST:

- accept a remote base URL and credential through flags, configuration, or
  environment variables;
- call REST only;
- use bounded request timeouts;
- support predictable machine-readable output;
- write normal results to standard output and errors to standard error;
- return meaningful, documented exit statuses;
- never print credentials or store them without explicit user action.

The CLI is an automation surface, not a second implementation of business
logic.

## 4. Authentication and authorization

### Authentication methods

The template MUST support local authentication and SHOULD support optional
OpenID Connect (OIDC).

Local authentication MUST use a current password-hashing algorithm with
per-password salts and documented cost parameters. It MUST provide a first-owner
bootstrap path through deployment-owned environment configuration and an
explicit operator command. It SHOULD also provide a protected browser setup
path for deployments where operators can configure environment variables but
cannot access a shell. It MUST NOT ship a public default password.

The first-owner bootstrap MUST accept the deployment variables
`BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_WORKSPACE`, and
`BOOTSTRAP_ADMIN_PASSWORD` as one complete set. A partial set MUST fail before
traffic is accepted. A protected browser path MAY use the deployment secret
`BOOTSTRAP_SETUP_TOKEN`; its form MUST use CSRF protection and MUST NOT expose
the token in a URL. The command and browser paths MUST call the same application
use case. Every path MUST create the first user, workspace, owner membership,
and separate instance-administrator record atomically, and a successful
bootstrap MUST disable all first-run paths.

OIDC MUST remain disabled without requiring dead configuration. When enabled,
it MUST implement:

- provider discovery;
- exact redirect URI matching;
- unpredictable state validation;
- Proof Key for Code Exchange (PKCE) using the S256 method;
- nonce generation and ID-token nonce validation;
- issuer, audience, signature, expiration, and verified-email validation;
- issuer plus subject as the stable external identity;
- local session termination and provider logout when supported.

Email MUST NOT be treated as the permanent external identity key.

### External identity provider configuration

An application that supports more than one external identity provider MUST use
a provider registry with stable provider identifiers. Provider-specific login
routes, buttons, configuration, and identity records MUST use that identifier;
they MUST NOT depend on display names or provider-specific conditionals spread
through the application.

Each provider record SHOULD contain, as applicable:

- stable provider identifier and display label;
- protocol type, such as OIDC, OAuth 2.0, or SAML;
- issuer or discovery URL;
- client identifier;
- secret reference, never the raw secret;
- enabled status and application, organization, or tenant scope;
- validated scopes, claim mappings, and domain restrictions;
- configuration version, timestamps, and audit metadata.

The application MUST identify external accounts by the provider issuer and
subject (or an equivalent provider-instance identifier and subject). Email is
an attribute used for display, policy, or explicit account linking; it MUST
NOT silently merge identities across providers.

Provider selection MUST be server-controlled. A request MUST select a known
provider identifier rather than supplying an arbitrary issuer or discovery
URL. Runtime-configurable discovery MUST restrict schemes, destinations, and
network access to prevent server-side request forgery (SSRF).

The authorization transaction MUST bind the provider identifier, browser
session, state, and PKCE verifier. OIDC transactions MUST also bind a nonce.
Callback handling MUST reject a provider, redirect URI, state, nonce when
used, issuer, audience, or code that does not match the transaction that
started the flow.

### Sessions and browser security

- Session identifiers MUST be high entropy, stored only as hashes, revocable,
  and bounded by expiration.
- Session cookies MUST be HTTP-only, SameSite, and Secure whenever HTTPS is
  used.
- State-changing browser requests MUST use Cross-Site Request Forgery (CSRF)
  protection.
- Users SHOULD be able to review and revoke active sessions or devices.
- Authentication errors MUST resist user and account enumeration.

### Roles and permissions

Authorization MUST be permission based. Roles are named collections of
permissions within an explicit application, organization, or workspace scope.
Even a single-workspace application SHOULD model that scope so authorization
does not depend on global assumptions.

The baseline roles are:

| Role | Baseline authority |
|---|---|
| `owner` | Full control, ownership transfer, membership and role assignments, security settings, and destructive workspace actions |
| `admin` | Manage settings, members, invitations, and normal resources, but not silently remove the final owner |
| `member` | Create and modify normal application resources allowed by assigned permissions |
| `viewer` | Read permitted resources without mutation rights |

The baseline template MUST ship only these four protected system roles. Custom
roles are deferred, not forbidden. They MAY be added when a product needs
permission combinations the baseline roles cannot express. When enabled:

- a custom role MUST be a named set of allowlisted permissions within one
  explicit application, organization, or workspace scope;
- it MUST NOT grant permissions unavailable to the assigning principal;
- it MUST NOT replace, modify, impersonate, or bypass the protected `owner`
  role or the last-owner rules;
- deleting or changing it MUST define safe handling for existing assignments;
- creation, permission changes, assignment, and deletion MUST be authorized
  and audited;
- authorization MUST evaluate the resolved permissions, not trust the custom
  role name.

Rules:

- Authorization MUST deny access by default.
- Every protected use case MUST check permissions, not role names scattered
  through transport handlers.
- Tokens and delegated credentials MUST NOT exceed the permissions of their
  principal.
- The last owner MUST NOT be removed, demoted, or deleted without a successful
  ownership transfer.
- Ownership transfer MUST be explicit, authenticated, authorized, and audited.
- Role and permission changes MUST invalidate or re-evaluate affected sessions
  and tokens as required by the security model.

### Workspace and tenancy boundary

The default product boundary is:

```text
installation or cloud instance
    └── workspace
          └── workspace membership and roles
                └── workspace-owned resources
```

The application MUST choose one product term for this boundary, such as
`workspace`. It MUST NOT introduce separate `tenant`, `organization`, `team`,
and `workspace` concepts unless they represent different, required scopes.

Identity and workspace ownership MUST be separate:

```text
users
workspaces
workspace_members(workspace_id, user_id, role)
items(workspace_id, created_by_user_id, ...)
```

Rules:

- A user MAY belong to many workspaces.
- A workspace MUST support many members through a membership table, not a
  single owner or `users.workspace_id` column.
- `workspace_members` MUST enforce one active membership per
  `(workspace_id, user_id)` and MUST store the role at workspace scope.
- Every workspace-owned domain table MUST have a non-null `workspace_id`.
  Creator, assignee, and editor user IDs are separate relationships and MUST
  NOT replace workspace ownership.
- Workspace-owned queries MUST filter by both the workspace context and the
  resource identifier. A globally unique resource ID does not remove the
  workspace authorization check.
- Foreign keys, workspace membership checks, and indexes beginning with
  `workspace_id` MUST protect the boundary and keep scoped lists efficient.
- Global records such as users, sessions, external identities, and instance
  configuration MUST remain global only when they are genuinely installation-
  or user-scoped.
- Cross-workspace reads or mutations MUST use an explicit, separately
  authorized use case. No request may fall back to a user's first or current
  workspace for a missing scope.

### Workspace request context

Protected web and API operations MUST resolve an explicit workspace context
from a route, subdomain, or equivalent server-controlled context. The default
API shape SHOULD be nested, for example:

```text
GET  /api/v1/workspaces/{workspaceID}/items
POST /api/v1/workspaces/{workspaceID}/items
GET  /api/v1/workspaces/{workspaceID}/items/{itemID}
```

The server MUST authenticate the user, resolve the workspace, verify active
membership, resolve permissions, and only then call the application use case.
An active-workspace cookie, header, client variable, or unverified token claim
MUST NOT be treated as authorization.

Workspace identifiers MUST be passed explicitly into use cases and repository
methods. Repositories MUST NOT infer workspace scope from global mutable state.
Handlers MUST remain thin and MUST NOT implement workspace filtering or role
logic independently from the application authorization boundary.

### OSS and cloud deployment modes

The same application code and schema MUST support these deployment modes:

| Mode | Default behavior |
|---|---|
| Self-hosted OSS | One installation, one default workspace, workspace switcher hidden when only one workspace exists |
| Shared cloud | One application instance or cluster, multiple workspaces, explicit membership and workspace selection |
| Dedicated cloud | One customer deployment or database when isolation or compliance requires it, without a second application implementation |

The OSS default MAY expose only one workspace while the data model remains
multi-workspace capable. Creating the first workspace and its owner membership
MUST be one transaction. A first-run bootstrap MUST NOT create a user without
an owner workspace when the application requires workspace-scoped data. A
self-hosted deployment MUST be usable when its platform provides environment
configuration but no interactive shell; browser setup is the fallback for that
boundary, while environment bootstrap remains the unattended path.

Cloud mode MUST NOT be implemented as a fork of the OSS codebase. Billing,
invitations, custom domains, richer roles, and workspace switching MAY be
enabled later at this boundary. The initial standard does not require nested
organizations, group hierarchies, database-per-workspace isolation, or custom
RBAC.

SQLite MAY support multiple workspaces inside one application instance.
PostgreSQL remains required before running multiple application instances, as
defined by the scaling path in this document.

### Workspace lifecycle and migrations

- Workspace creation MUST create the workspace, initial owner membership, and
  required onboarding state atomically.
- Workspace deletion, archival, and ownership transfer MUST define resource,
  membership, invitation, token, file, audit, and billing consequences before
  the operation is exposed.
- Invitations MUST belong to an explicit workspace and role; the general
  invitation rules below apply.
- API tokens MUST remain user principals unless a product requirement needs
  workspace automation. Workspace automation tokens MUST have explicit
  workspace scope and permissions.
- A greenfield application MUST add the workspace boundary before adding its
  first workspace-owned domain table.
- When adopting this standard in an existing user-owned application, the
  migration MUST create a default workspace, add the user as owner, backfill
  `workspace_id`, preserve creator user IDs, and verify every protected query
  before removing user-only ownership assumptions.

### Instance and workspace administration

The instance boundary and workspace boundary MUST have separate authority.

- Instance administrators manage deployment-wide settings, identity-provider
  configuration, maintenance, workspace-creation policy, and instance
  security operations.
- Workspace owners and administrators manage only their workspace settings,
  members, integrations, and resources.
- Instance-level authority MUST NOT silently become ordinary workspace
  membership. If an instance administrator can access workspace data, that
  access MUST be explicit, least-privileged, reasoned, and audited. The
  deployment MUST define whether affected users are notified.
- A single-workspace installation MAY hide workspace selection, but it MUST
  preserve explicit workspace authorization and MUST NOT fall back to a
  user's first, last, or current workspace when scope is absent.
- Workspace creation, suspension, transfer, and deletion MUST define who can
  perform each action, what happens to members and resources, and which audit
  events are produced.

Reference implementations reviewed for this standard include [Plane's
self-hosted instance and workspace model](https://developers.plane.so/self-hosting/govern/instance-admin),
[Twenty's single- and multi-workspace modes](https://docs.twenty.com/developers/self-host/capabilities/setup),
[Mattermost's single- and multi-team deployment guidance](https://docs.mattermost.com/end-user-guide/collaborate/organize-using-teams.html),
[Outline's top-level workspace model](https://docs.getoutline.com/s/guide/doc/terminology-fKoXA2YGzV),
[Appwrite's team memberships and permissions](https://appwrite.io/docs/products/auth/teams),
[GitLab's group and project hierarchy](https://docs.gitlab.com/user/group/), and
[Baserow's workspace/application boundary](https://github.com/baserow/baserow/blob/develop/docs/technical/introduction.md).
These references inform the standard; they are not runtime dependencies.

### Invitations

The baseline invitation system MUST support:

- inviting an email address to an explicit application or workspace role;
- expiration;
- single-use acceptance;
- revocation;
- resend with rotation or invalidation of the previous secret;
- prevention of duplicate active invitations for the same scope and email;
- safe handling when the invited address already belongs to a user;
- enumeration-safe responses;
- audit events for creation, resend, acceptance, expiration, and revocation.

Invitation secrets MUST be high entropy, expire, be stored only as hashes, and
never appear in logs.

### API tokens and delegated authorization

The application MUST provide an in-app token-management system.

- Token secrets are displayed once and stored only as hashes.
- Tokens have explicit permission scopes.
- A token cannot receive permissions its creator does not hold.
- Tokens support optional expiration, revocation, descriptive names, creation
  time, and last-used time.
- Token creation, use, scope changes, and revocation produce audit events.
- Token values and authorization headers never appear in logs.
- REST, MCP, and CLI use the same scoped authorization model.
- Delegated OAuth MAY replace local bearer tokens where third-party clients
  require consent and delegated access.

### Integration connections and delegated credentials

Every external integration connection MUST declare its ownership scope:
instance, workspace, or user. The connection's credentials, callbacks, data,
and actions MUST use that same scope.

- A user's connection MUST NOT become shared workspace authority without an
  explicit consent and ownership transition.
- Credentials MUST use least privilege, be protected at rest, never appear in
  logs or ordinary responses, and support rotation, revocation, and
  disconnect.
- Reauthorization, credential replacement, failed authentication, and
  disconnect MUST have defined behavior and audit events.
- Callback and inbound-event handling MUST verify the provider's authenticity,
  bind the event to the declared instance or workspace, and reject ambiguous
  scope.
- Integration actions MUST pass through the same authorization, rate limits,
  idempotency, and audit rules as equivalent first-party actions.

## 5. Data and scaling

### SQLite first

SQLite MUST be the default database for development, demos, and
single-instance installations. The application MUST configure foreign keys,
an appropriate busy timeout, and safe journaling behavior.

The SQLite database MUST live on persistent storage. SQLite mode supports one
application instance unless a documented and tested deployment proves safe
multi-instance access.

### Optional PostgreSQL

PostgreSQL MUST be an optional path for applications that need it. A production
deployment MAY use SQLite when it remains single-instance and satisfies its
documented durability, backup, and recovery requirements. Multiple application
instances MUST use PostgreSQL or another explicitly supported shared database.

The application SHOULD select the database through validated configuration
without changing domain behavior.

### Portable migrations

- One ordered migration history SHOULD support SQLite and PostgreSQL.
- Every migration MUST be tested against both databases.
- Identifiers SHOULD be application generated and portable.
- Timestamps MUST use a documented UTC representation.
- Shared migrations SHOULD avoid vendor-specific enums, extensions,
  procedures, and data types.
- A vendor-specific migration requires a documented reason, compatibility
  impact, and replacement or fallback path.
- Production migrations MUST use a database-backed lock or another single-
  executor mechanism before changing shared schema.
- Migrations MUST be forward safe and include an explicit restore strategy
  when rollback cannot be automated safely.

### Rolling upgrade compatibility

Deployments that can run more than one application version MUST support mixed
versions during a rollout or explicitly document a stop-the-world upgrade.
Schema and contract changes SHOULD use an expand-and-contract sequence:

1. add compatible structures or fields;
2. deploy versions that understand both old and new shapes;
3. backfill and verify data;
4. enforce new constraints;
5. remove old structures only after incompatible versions are no longer able
   to run.

Each release with migration impact MUST document preconditions, ordering,
estimated duration, pause and retry behavior, rollback limits, and recovery
steps. Migrations MUST have one safe executor, MUST tolerate interruption,
and MUST NOT require application instances to share in-memory state.

The compatibility window MUST cover background work, queued events, cached
responses, REST clients, MCP clients, CLI versions, and webhook consumers when
those surfaces exist. A release MUST NOT silently invalidate durable work or
external clients.

### Data governance

`Core` and `Identity` MUST assume the application processes ordinary personal
information because users, email addresses, source addresses, invitations,
sessions, and audit events can identify people.

| Class | Examples | Required treatment |
|---|---|---|
| Public | Published pages and public metadata | Integrity controls |
| Internal | Internal identifiers and non-secret configuration | Authorized access and no accidental disclosure |
| Personal | Email, name, source address, membership, activity | Minimize, encrypt in transit, define retention, export, correction, and deletion |
| Secret | Password, token, session, invitation secret, private key | Hash or encrypt, least privilege, rotate, never log |
| Regulated | Health, financial, government, or legally restricted data | Add only with a documented legal, security, retention, and isolation model |

Every persisted field MUST have an owner, classification, purpose, retention
rule, export behavior, and deletion or anonymization behavior. Logs, traces,
metrics, audit events, backups, and analytics MUST follow the same
classification. Production data MUST be encrypted in transit. Storage
encryption MUST be provided by the deployment platform or application and
documented with key ownership, rotation, backup, and loss behavior.

Account and workspace deletion MUST define what is deleted immediately, what
is retained for security or legal reasons, and when retained copies disappear
from backups. Data export MUST use a documented, machine-readable format and
must not expose another workspace's data.

### Data portability

An application with durable workspace data MUST provide an authorized,
workspace-scoped export. An application offered in both cloud and self-hosted
modes MUST define an import path or explicitly document why import is not
supported.

- Export formats MUST be versioned, machine-readable, documented, and safe to
  process without access to the source database.
- An export MUST define its coverage for records, relationships, identifiers,
  timestamps, settings, members, and files. Unsupported or excluded data MUST
  be listed explicitly.
- Export authorization MUST be checked at creation and download. A download
  MUST be bounded, protected, and prevented from crossing workspace scope.
- Large exports and imports MUST expose status, failure, and expiration
  behavior through the long-running operation contract below.
- Imports MUST treat input as untrusted, validate references and limits, avoid
  privilege escalation, define identifier mapping and duplicate behavior, and
  report partial or unsupported results.
- Import and export MUST be auditable. Portability formats MUST NOT be treated
  as a substitute for full database backup and restore.

### Deletion, archive, and restoration lifecycle

Every user-visible resource MUST define whether it supports active, archived,
soft-deleted, and permanently deleted states. The product MUST NOT use
"delete" for multiple irreversible behaviors without explaining the result.

- Each state transition MUST define authorization, confirmation or
  reauthentication needs, dependent-resource behavior, audit events, and
  notifications.
- Restore MUST define who may restore, what happens when dependencies are
  missing or conflicting, and whether identifiers and history are preserved.
- Permanent deletion MUST be irreversible by product behavior and MUST define
  treatment of files, search indexes, queues, external integrations, audit
  records, and backups.
- Retained copies for security, legal, or recovery reasons MUST have an owner,
  purpose, retention period, and access policy. Backups MUST NOT become an
  undocumented permanent copy of deleted workspace data.
- Large or cascading deletion and restoration MUST use the long-running
  operation contract, be idempotent, and expose safe progress or final status.

### Scaling path

```text
SQLite + one application instance
    ↓
PostgreSQL + one stateless application instance
    ↓
PostgreSQL + multiple instances + traffic distributor
    ↓
Requirement-driven shared events, jobs, files, or cache
```

Before multiple instances are enabled:

- sessions and tokens MUST be shared through durable storage;
- readiness MUST reflect database availability;
- required files MUST use shared durable storage;
- cross-instance events MUST use a shared delivery mechanism;
- rate-limit semantics MUST be documented as per-instance or made global;
- migrations MUST have a single safe execution strategy.

## 6. Build once

These capabilities belong in the reusable platform layer. Feature developers
should consume them rather than implement competing versions.

### Configuration and secrets

- Configuration MUST be validated before the application accepts traffic.
- Every option MUST have a type, default or required status, valid values, and
  documentation.
- Secrets MUST be distinguishable from ordinary configuration and support a
  deployment-safe injection mechanism.
- Configuration errors MUST fail startup with actionable messages and without
  secret values.
- Deploy-varying configuration SHOULD be external to the application image.

#### Source of truth

Every setting MUST have exactly one authoritative owner. The application MUST
NOT silently merge, override, or synchronize the same setting between
environment variables, configuration files, and database rows.

Configuration MUST be classified as one of:

| Class | Source of truth | Change behavior |
|---|---|---|
| Code-owned | Versioned application code | Release required |
| Deployment-owned | Environment, mounted config, or orchestrator configuration | Rollout or restart required unless explicitly reloadable |
| Runtime-owned | Versioned database records managed through an authorized application or admin API | Validated update, audit event, and cache invalidation or reload |
| Secret | Secret manager or encrypted secret store | Rotation workflow with access control and audit |

Environment variables are a portable deployment configuration and injection
mechanism. They are not a second database, a runtime configuration API, or a
secret-management system.

Bootstrap environment values are deployment-owned inputs, not runtime records.
They MAY create the first identity exactly once, but MUST NOT overwrite or
synchronize users, workspaces, memberships, or instance authority on later
starts. Password and setup-token values are secrets and MUST be injected
through deployment secret handling, never logged, committed, or placed in
command arguments.

Static provider configuration MUST use deployment-owned configuration and MUST
be changed through the deployment workflow. Runtime-managed or
tenant-specific provider configuration MAY use the database, but the database
MUST own that configuration for the selected deployment mode. An environment
change MUST NOT overwrite runtime records, and a runtime update MUST NOT be
silently replaced by an environment default.

Provider metadata MAY be stored in the database when administrators or
tenants need to add, disable, or change providers without a deployment. Raw
OAuth client secrets, private keys, signing keys, and encryption keys MUST NOT
be stored in plaintext in the database. Store a reference to a dedicated
secret manager instead. If an external secret manager is unavailable, use
envelope encryption with a root key held outside the database; document root
key backup, rotation, recovery, and loss behavior.

Secret values MUST NOT appear in source control, ordinary configuration files,
browser code, logs, audit events, error messages, or database backups in
plaintext. Secret access MUST use least privilege, support rotation and
revocation, and record safe metadata such as owner, purpose, version, and
rotation history. Secret rotation SHOULD support atomic activation and a
temporary overlap between old and new credentials when the provider allows it.

Configuration changes MUST be validated before activation. Invalid provider
metadata or credentials MUST leave the last known-good configuration active.
Runtime configuration changes MUST be auditable and reversible. Static
configuration changes MUST be reviewable through the deployment or release
system.

Reference guidance: [Twelve-Factor configuration](https://12factor.net/config),
[OWASP Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html),
and [RFC 9700 OAuth 2.0 Security Best Current Practice](https://www.rfc-editor.org/rfc/rfc9700.html).

### HTTP foundation

- Every request MUST have a request ID returned to the caller and included in
  logs and errors.
- Logs MUST be structured and correlate request IDs and trace IDs.
- Request headers and bodies MUST have explicit size and time limits.
- Long-lived streams MUST receive appropriate exceptions from ordinary
  response timeouts.
- Security headers MUST include an application-specific Content Security
  Policy, content-type protection, framing protection, and a restrictive
  referrer policy.
- HTTP Strict Transport Security MUST be enabled only where HTTPS is guaranteed.
- Browser-agent pages MUST send `Permissions-Policy: tools=(self)` and
  `Origin-Agent-Cluster: ?1` when WebMCP is enabled.
- Rate limiting MUST produce `429 Too Many Requests` with `Retry-After` where
  applicable.
- Authentication endpoints MUST have stricter abuse controls than ordinary
  authenticated endpoints.

The generated application MUST begin with these configurable defaults:

| Contract | Default |
|---|---|
| Trace propagation | W3C `traceparent` and `tracestate` |
| Request ID | Server-generated UUIDv7 or equivalent 128-bit opaque value in `X-Request-ID` |
| Request-header timeout | 10 seconds |
| Ordinary request timeout | 30 seconds |
| Graceful shutdown timeout | 10 seconds |
| Maximum ordinary request body | 1 MiB unless the endpoint documents another limit |
| Collection page size | Cursor pagination, default 50, maximum 100 |
| Rate-limit response | `429 Too Many Requests` and `Retry-After`; draft `RateLimit` fields MAY also be emitted |

Incoming request IDs MUST be validated for format and length before reuse.
Trace and request identifiers MUST contain no personal information. Streaming
endpoints MUST have separate idle, lifetime, payload, and concurrency limits.

### Observability

The template MUST provide:

- structured logs;
- OpenTelemetry-compatible traces and metrics;
- common semantic names for HTTP, database, and external-service operations;
- request and trace correlation;
- optional exporters configured without requiring a bundled backend;
- measurements for request rate, errors, duration, authentication failures,
  database health, and background delivery when those features exist.

No particular telemetry vendor or dashboard stack is required.

### Health and lifecycle

- Liveness reports whether the process can continue running and MUST NOT fail
  merely because an optional dependency is unavailable.
- Readiness reports whether the instance can serve traffic and MUST check the
  required database and other critical dependencies with bounded timeouts.
- Startup MUST fail if configuration, required dependencies, or mandatory
  migrations cannot initialize safely.
- Shutdown MUST stop accepting new work, drain bounded in-flight work, and
  close resources.

Health responses MUST use these contracts:

- liveness success: HTTP `200` with `{"status":"alive"}`;
- readiness success: HTTP `200` with `{"status":"ready"}`;
- readiness failure: HTTP `503` with `{"status":"not_ready"}` and safe names
  for failed required checks;
- health output MUST NOT include credentials, connection strings, stack
  traces, internal addresses, or optional dependency failures presented as
  required failures.

The default deployment SHOULD fit within 0.25 vCPU and 256 MiB of memory while
idle. Each generated application MUST measure and document idle and tested-load
CPU, memory, connection-pool, and stream usage; an implementation that exceeds
the default target documents its measured requirement rather than hiding it.
Database pools, concurrent streams, request queues, and background workers MUST
have bounded defaults derived from deployment capacity.

A `Production` deployment SHOULD target 99.9% monthly availability. Alerts
MUST cover sustained readiness failure, error-rate and latency SLO burn,
resource exhaustion, stale or failed backups, failed restore verification,
certificate expiration, and security-control failure. Exact thresholds MUST be
recorded in `docs/operations.md` from measured application behavior.

### REST behavior

- API errors MUST use RFC 9457 Problem Details with
  `application/problem+json`.
- Problem responses MUST include stable types, HTTP status, safe detail, an
  instance or request identifier, and a stable machine-readable extension
  when clients need one.
- Internal errors, secrets, stack traces, and database details MUST NOT be
  exposed.
- Collection endpoints MUST define pagination and stable ordering.
- Filtering and sorting MUST use documented fields and reject unsupported
  values.
- Resource responses SHOULD use entity tags (ETags) where caching or
  concurrency matters.
- Mutations that can overwrite concurrent changes SHOULD require `If-Match`
  or an equivalent version precondition.
- Retryable, expensive, or side-effectful creates MUST define idempotency-key
  behavior, request matching, retention, and conflict handling.

The standard Problem Details object MUST contain `type`, `title`, `status`,
`detail`, and `instance`, plus stable `code` and `requestId` extensions. Problem
type URIs MUST be stable and documented. `detail` is human-readable and MUST
NOT be parsed as a machine contract.

Idempotency keys MUST be 1 to 255 visible characters, scoped to principal and
operation, and stored as hashes where they could identify a user. Repeating a
key with the same canonical request returns the original result. Reusing it
with a different request returns a conflict. The default retention is 24
hours. Operations that do not support idempotency MUST say so explicitly.

### Contract lifecycle and deprecation

Every externally consumed contract MUST have a compatibility policy. This
includes REST, MCP schemas, CLI behavior, webhooks, events, and portability
formats when they exist.

- A stable contract MUST preserve existing meanings and valid client behavior.
  Additive changes MUST NOT silently change authorization, data meaning, error
  semantics, ordering, or side effects.
- A breaking change MUST use a new version or a documented compatibility
  transition. The transition MUST define coexistence, migration, and removal
  dates.
- Deprecation MUST publish the affected surface, reason, replacement, impact,
  migration guidance, support window, and removal date. A machine-readable
  deprecation signal SHOULD be provided where the protocol supports one.
- Removal MUST wait until the published support window ends and usage or
  migration evidence has been reviewed. Security emergencies MAY shorten the
  window with an incident record and an alternative path.
- Contract changes MUST update the canonical specification, examples, tests,
  and changelog together.

### Long-running operation contract

Any operation that can exceed an ordinary request, require retry or
resumption, or continue after the requester disconnects MUST expose a stable
operation resource or equivalent status contract.

- Each operation MUST have an opaque identifier, principal, workspace or
  instance scope, operation type, creation time, and expiration or retention
  rule.
- The lifecycle MUST distinguish at least queued, running, succeeded, failed,
  cancelled, and expired states, or document an equivalent state model.
- Clients MUST have a bounded status and result path. Progress MAY be
  approximate, but final success and failure MUST be unambiguous and safe to
  display.
- Operations MUST define idempotency, retry, cancellation, concurrency, and
  cleanup behavior. Retried work MUST NOT duplicate irreversible effects.
- Status and result access MUST use the same authorization and workspace
  isolation as the initiating action. Creation, completion, failure,
  cancellation, and administrative retry MUST be auditable.

### Realtime delivery contract

When the application provides realtime events or progress, every connection
and subscription MUST have an explicit authenticated principal and workspace
scope. The server MUST authorize delivery; a client-provided channel name or
workspace identifier is not proof of access.

- Events MUST have a stable ID, type, schema version, occurrence time, scope,
  and resource reference where applicable.
- The contract MUST define ordering, duplicate delivery, reconnect, missed
  events, and replay or resynchronization behavior.
- Connections MUST have bounded idle time, lifetime, payload, and concurrency
  limits, with heartbeat and failure behavior documented.
- Instance-local delivery MUST NOT be advertised as globally consistent. A
  multi-instance deployment MUST use a shared delivery mechanism or document
  the resulting limitation.
- Realtime payloads MUST obey the same data classification, redaction,
  authorization, and audit rules as REST responses.

### Quotas and resource limits

Every resource or operation that can consume bounded storage, memory, CPU,
connections, provider capacity, or operator attention MUST define a server-
enforced limit. Limits MUST be assigned at the smallest meaningful scope,
such as instance, workspace, user, credential, source address, or operation.

- Limits MUST cover each applicable dimension, such as request size, list
  size, active sessions, streams, concurrent operations, stored records,
  exports, files, or external calls.
- Limits MUST have one source of truth, safe defaults, an authorized change
  path, and observable usage or rejection behavior.
- Exceeded limits MUST return a documented machine-readable error and a safe
  retry or remediation signal. The server MUST enforce limits even when the
  client does not.
- Quotas MAY exist without billing. Billing, metering, and entitlements MUST
  remain separate concerns when they are introduced.

Token scopes MUST use an allowlisted `resource:action` form, such as
`items:read`. Unknown scopes are rejected, missing scopes deny access, and a
credential cannot receive authority unavailable to its principal.

### Audit events

Audit-event infrastructure MUST record security and administrative actions in
an append-only form with:

- event ID and type;
- actor and authentication method;
- application or workspace scope;
- target resource;
- action and outcome;
- timestamp;
- request or trace ID;
- source address where appropriate;
- redacted metadata.

Audit records MUST NOT contain passwords, token secrets, session identifiers,
OIDC secrets, invitation secrets, or raw authorization headers. Access,
retention, export, and deletion rules MUST be documented.

### Backup, restore, and upgrades

- SQLite and PostgreSQL MUST have separate documented backup and restore
  procedures.
- Backups MUST be consistent, checksummed, and verifiable.
- Restore procedures MUST be tested automatically against temporary data.
- Encryption keys and deployment secrets MUST have a separate recovery plan;
  they MUST NOT be copied into ordinary database backups by accident.
- Every release with migration impact MUST document backup prerequisites,
  upgrade order, rollback limits, and recovery steps.
- A backup is not considered valid until a restore test succeeds.

The default `Production` recovery objectives are:

- Recovery Point Objective (RPO): no more than one hour of accepted data loss;
- Recovery Time Objective (RTO): service restored within four hours.

The deployment owner MAY choose different objectives, but MUST record them in
`docs/operations.md`, configure backup frequency and retention to meet them,
name the recovery owner, and test restoration on that schedule. Backup age
greater than the selected RPO and restore verification older than the defined
review interval MUST alert.

### Accessibility

The web interface MUST target Web Content Accessibility Guidelines (WCAG) 2.2
Level AA:

- semantic structure and landmarks;
- keyboard operation;
- visible focus;
- associated labels and useful error messages;
- sufficient contrast;
- reduced-motion support;
- responsive text and layouts;
- automated checks for repeatable failures;
- manual checks for behavior automation cannot prove.

Accessibility MUST NOT be deferred as visual polish.

Accessibility evidence MUST include:

- automated checks on every critical page and authenticated critical flow;
- no unresolved critical or serious automated violations;
- a manual checklist covering keyboard-only use, focus order and visibility,
  forms and errors, zoom and reflow, reduced motion, contrast, and one
  screen-reader pass through each critical flow;
- a named owner and dated result before every stable release or major user-
  interface redesign;
- documented exceptions mapped to specific WCAG success criteria.

Automation MAY use axe-core, Pa11y, Lighthouse, or an equivalent tool. Tool
success alone does not prove WCAG conformance; manual evaluation remains
required.

### Containers and deployment

- The application MUST have a reproducible container build.
- The runtime container MUST use a non-root user and a minimal runtime image.
- The default deployment MUST run directly without requiring a specific
  reverse proxy.
- Direct TLS MAY be supported; external TLS termination MUST also work.
- Persistent data locations, ports, health checks, resource expectations, and
  shutdown behavior MUST be documented.
- Images SHOULD include standard Open Container Initiative metadata and be
  published by immutable digest.

### Continuous integration and releases

Continuous integration (CI) MUST verify formatting, static analysis, tests,
generated contracts, database migrations, container builds, and a runnable
smoke path as appropriate to the selected stack.

Releases MUST provide this evidence; named tools are recommended examples, not
exclusive requirements:

| Evidence | Minimum result | Example tools |
|---|---|---|
| Source dependency vulnerability scan | No unresolved exploitable critical vulnerability | Language scanner, OSV-Scanner |
| Container vulnerability scan | No unresolved exploitable critical vulnerability in shipped image | Trivy, Grype |
| SBOM | SPDX or CycloneDX document for every image and artifact | Syft |
| Build provenance | Verifiable source, workflow, builder, inputs, and artifact digest | SLSA or CI-provider attestation |
| Signature | Keyless or protected-key signature bound to artifact digest and publisher identity | Sigstore/Cosign |
| Checksums | SHA-256 checksum file for downloadable artifacts | Platform checksum utility |
| Dependency monitoring | Automated update proposals with normal CI verification | Dependabot, Renovate |
| Repository posture | Reviewable security-practice report | OpenSSF Scorecard |

Build workflow dependencies and container base images MUST use immutable
versions or digests where the ecosystem supports them. Verification commands
MUST be published with each release. Exceptions require the structured
exception process and compensating controls.

Dependency-update automation SHOULD be enabled, but updates MUST still pass
the normal compatibility and security checks.

### Documentation contract

The repository MUST have a documentation index, clear sources of truth, and
change-trigger rules. Documentation MUST describe implemented behavior, not
speculative behavior. Links are preferred over copied explanations.

## 7. Apply on every relevant change

The following rules are obligations for contributors and software agents.

| Change | Required work |
|---|---|
| New REST endpoint | Update OpenAPI; define authorization, limits, Problem Details, audit behavior, tests, and user documentation |
| New mutation | Validate input; check permission; define transaction, concurrency, idempotency, and audit behavior |
| New collection | Define stable order, pagination, supported filters, maximum page size, and index needs |
| New configuration | Add validation, safe default or required status, example, secret classification, container wiring, and configuration docs |
| New database change | Add an ordered migration and verify it on SQLite and PostgreSQL |
| New MCP tool | Reuse a use case; define scopes, structured schemas, safety annotations, audit behavior, and tests |
| New WebMCP tool | Keep it page-scoped and one-purpose; feature-detect registration; define an explicit schema; exclude secrets and hidden security fields; never auto-submit destructive forms; preserve ordinary-browser fallback; test supported and unsupported browsers |
| New CLI command | Call REST only; define timeout, output, exit statuses, and credential-safe errors |
| New role or permission | Deny by default; define scope; test all baseline roles, custom roles when enabled, and token-scope behavior |
| New invitation behavior | Preserve expiration, single use, revocation, duplicate prevention, safe errors, and auditing |
| New external URL | Validate scheme and destination; block server-side request forgery (SSRF) paths |
| New user interface | Verify keyboard use, labels, focus, contrast, responsive layout, errors, and reduced motion |
| New dependency or service | Show the unmet requirement, maintenance cost, security impact, deployment impact, and removal path |
| User-visible behavior | Update the relevant guide and changelog in the same change |

Additional rules:

- Reuse the shared platform capability before adding feature-local middleware,
  logging, authorization, retries, or error formats.
- Every protected operation MUST enforce authorization in the shared use case
  or policy layer, not only in a transport handler.
- Every non-trivial behavior MUST leave the smallest automated check that
  fails when the behavior regresses.
- Generated files MUST be reproducible and checked for drift.
- Unrelated refactoring and speculative extension points MUST NOT accompany a
  focused change.

## 8. Add only when triggered

These capabilities are valuable but MUST NOT become default dependencies
without the listed requirement.

| Capability | Add when | Minimum standard once added |
|---|---|---|
| Webhooks | External consumers need event delivery | CloudEvents envelope, signed delivery, timestamp and event ID, secret rotation, transactional outbox, retries with jitter, replay, dead-letter status, idempotency, and SSRF protection |
| Background jobs | Work must survive, retry, schedule, or outlive a request | Durable state, explicit retry policy, idempotency, concurrency limits, observability, dead-letter handling, and graceful shutdown |
| Shared event broker | Events must cross services or replicas and database delivery is insufficient | Durable delivery semantics, replay policy, ownership, retention, and failure monitoring |
| Object storage | Users upload or generate files | S3-compatible boundary, presigned access, size/type limits, quotas, metadata, lifecycle, and malware-scanning hook where risk requires it |
| SCIM | Enterprise customers require user or group provisioning | System for Cross-domain Identity Management (SCIM) 2.0, stable identifiers, group-to-role mapping, deprovisioning, and audit events |
| MFA and passkeys | Users manage identities or risk requires stronger authentication | WebAuthn/passkeys, time-based one-time passwords where needed, recovery codes, enrollment confirmation, recovery, and revocation |
| Feature flags | Releases need staged rollout, kill switches, or targeted behavior | OpenFeature-compatible application boundary; no bundled flag server is required |
| AsyncAPI | The application publishes a public asynchronous contract | Channels, messages, authentication, compatibility, and examples documented through AsyncAPI |
| A2A | The product hosts an independent agent that communicates with other agents | Agent discovery, task lifecycle, authentication, authorization, streaming, and auditability |
| GraphQL | Client query flexibility provides measured value beyond REST | Authorization, complexity limits, persisted-operation policy, and schema compatibility |
| gRPC | Internal typed streaming or high-throughput calls justify another contract | Versioned schemas, deadlines, authentication, reflection policy, and gateway strategy |
| Redis or cache | Measured latency or coordination needs cannot be met by application/database behavior | Expiration, invalidation, failure mode, memory limits, and correctness without cache |
| Kubernetes | Deployment scale or organization standards require orchestration | Resource limits, probes, disruption behavior, secret handling, migrations, and rollback |
| Billing | The product sells metered or subscription access | Provider boundary, webhook verification, idempotency, reconciliation, entitlements, and audit events |
| Search engine | Database search no longer meets measured relevance or scale needs | Index ownership, synchronization, rebuild, authorization filtering, and failure behavior |
| Internal AI | A product requirement needs model-driven behavior | Provider boundary, data policy, prompt/version control, tool authorization, cost controls, evaluation, and fallback behavior |
| Workspace domains and routing | Users need workspace-specific URLs, subdomains, or custom domains | Ownership and DNS verification, reserved names, collision handling, certificate lifecycle, routing isolation, redirects, and audit events |
| Public sharing or guest access | Resources must be reachable without ordinary workspace membership | Separate anonymous or guest principal, explicit scope, expiry and revocation, non-membership semantics, indexing policy, abuse limits, and audit events |
| Fine-grained permissions | Baseline roles cannot express a measured product requirement | Stable permission vocabulary, explicit scope, deny-by-default evaluation, assignment authority, inheritance rules, migration behavior, and complete authorization tests |
| WebMCP browser enhancement | A human browser workflow benefits from browser-agent assistance and the platform is available | Imperative feature detection, same-origin and Permissions Policy boundary, explicit bounded schemas, no credentials or hidden fields, user-controlled final mutation, fallback behavior, and browser smoke evidence |

Server-Sent Events (SSE) SHOULD be preferred for one-way notifications and
progress. WebSockets SHOULD be added only when continuous bidirectional
communication is required.

A database-backed outbox SHOULD be evaluated before adding a broker for the
first webhook or durable event requirement. A PostgreSQL-backed job mechanism
SHOULD be evaluated before adding Redis when PostgreSQL is already required.

## 9. MCP and agent readiness

MCP is the tool boundary for software agents. It MUST expose application
capabilities without bypassing application policies.

### Baseline MCP behavior

- Use MCP Streamable HTTP.
- Authenticate every protected request.
- Apply the same permission scopes used by REST and CLI.
- Call application use cases rather than SQL, shell commands, or transport
  handlers.
- Define structured input and output schemas.
- Return deterministic, bounded results with pagination where collections are
  exposed.
- Annotate tools as read-only, destructive, idempotent, and open-world as
  applicable.
- Audit tool calls with principal, tool, target, outcome, and request or trace
  ID, without recording secrets.
- Keep tool discovery deterministic and cacheable where the protocol permits.
- Require clients to present or obtain human approval before destructive
  operations. Server authorization remains mandatory even when approval is
  present.

### Optional WebMCP behavior

WebMCP is a browser-tab-bound progressive enhancement, not a replacement for
server MCP. When the browser exposes the imperative `document.modelContext`
API, pages MAY register narrow tools that prepare existing visible forms or
links. Registration MUST be feature-detected and MUST fail harmlessly when the
API is absent. Tools MUST use explicit, versioned schemas; never expose
passwords, tokens, invitation or session values, CSRF fields, hidden fields,
OIDC material, or unrelated DOM data; and MUST NOT automatically submit a
state-changing or destructive action. The human completes the existing
authorization and CSRF-protected flow. The browser MUST enforce same-origin
and `Permissions-Policy: tools=(self)` boundaries. Server MCP remains
persistent, bearer-authenticated, audited, and authoritative.

### MCP security contract

- HTTP requests that contain an `Origin` header MUST match an exact configured
  allowlist. Non-browser clients without `Origin` remain supported.
- Local development servers MUST bind to loopback by default. Public binding
  requires explicit configuration and authentication.
- Stateful session identifiers MUST be unpredictable, principal-bound,
  revocable, and excluded from logs. The default limit is 10 active sessions
  per principal, 30 minutes idle, and 24 hours absolute lifetime. Stateless
  protocol operation SHOULD be preferred when it satisfies the tool set.
- Tool names and structured schemas are public contracts. Breaking input,
  output, scope, or side-effect changes require a new tool version or a
  documented compatibility transition.
- Authentication failures, authorization failures, invalid parameters,
  domain failures, rate limits, and internal failures MUST map consistently to
  MCP and HTTP errors. Internal details and upstream credentials remain hidden.
- Tool inputs, retrieved content, prompts, and model output are untrusted data.
  They MUST NOT change authorization policy, approve their own actions, or
  cause execution outside the declared tool contract.
- Tools MUST apply data classification and least privilege to inputs and
  outputs. Secrets, unrelated workspace data, hidden prompts, and internal
  metadata MUST NOT be returned.
- Network-capable tools require destination allowlists or equivalent policy,
  SSRF protection, redirect validation, response-size limits, timeouts, and
  audit events.
- Shell, filesystem, database, and code-execution tools are not baseline MCP
  capabilities. If a product requires one, it needs sandboxing, explicit
  scopes, bounded resources, human approval, and a structured exception or
  feature-specific security design.
- The default structured tool-result limit is 1 MiB. Larger or streaming
  results require a documented contract and resource limits.

MCP audit records MUST include principal, workspace, token or delegated-client
identifier, tool name and version, request or trace ID, target references,
declared safety annotations, outcome, duration, and redacted error category.
They MUST NOT store raw credentials, hidden prompts, or unrestricted payloads.

### Authorization modes

Local and simple deployments MAY use the same scoped bearer tokens as REST.
Deployments requiring delegated authorization MUST additionally provide:

- OAuth Protected Resource Metadata;
- authorization-server discovery;
- PKCE;
- resource and audience-bound access tokens;
- advertised and enforced scopes;
- standards-compliant authentication challenges;
- client registration appropriate to the deployment;
- issuer and token validation with no token passthrough to unrelated services.

Long-running MCP task support MUST be added only for tools whose work genuinely
outlives a normal request. A2A is a separate agent-to-agent boundary and is not
a replacement for MCP tools.

## 10. Documentation layout

Start with a flat documentation directory. Create a document only when the
related behavior exists.

```text
docs/
  README.md
  architecture.md
  configuration.md
  deployment.md
  operations.md
  authentication.md
  api.md
  mcp.md
  cli.md
  development.md
  capabilities.md
```

| Document | Audience and responsibility |
|---|---|
| `docs/README.md` | Index that routes users, operators, developers, and agents to the right task |
| `docs/architecture.md` | Durable boundaries, data flow, trade-offs, scaling path, and intentional exclusions |
| `docs/configuration.md` | Complete option reference: type, default, required status, valid values, secret status, restart behavior, and examples |
| `docs/deployment.md` | Containers, SQLite, PostgreSQL, volumes, TLS, proxies, ports, and scaling |
| `docs/operations.md` | Health, metrics, logs, migrations, backup, restore, upgrades, rollback, and troubleshooting |
| `docs/authentication.md` | Bootstrap, local login, OIDC, roles, permissions, invitations, sessions, and tokens |
| `docs/api.md` | REST authentication, errors, pagination, filtering, concurrency, idempotency, rate limits, and examples |
| `docs/mcp.md` | Connection, authorization modes, scopes, tool catalog, safety, and troubleshooting |
| `docs/cli.md` | Installation, configuration, output, exit statuses, commands, and remote automation examples |
| `docs/development.md` | Local setup, architecture navigation, extension recipes, tests, generation, and contribution workflow |
| `docs/capabilities.md` | Built-in, optional, deferred, and rejected capabilities with revisit triggers |

Rules:

- The root README MUST provide a quick start and link to the documentation
  index. It MUST NOT become the full manual.
- Configuration SHOULD remain one searchable reference until its size or
  ownership makes domain-specific files easier to navigate.
- A document SHOULD split into a subdirectory only when readers can no longer
  find a task quickly or separate owners maintain distinct domains.
- Documentation MUST link to a canonical contract rather than copying it.
- Examples MUST be safe to copy and MUST use placeholders rather than secrets.
- Procedures that can lose data MUST include prerequisites, verification,
  rollback or recovery, and explicit target selection.
- Documentation and user-visible behavior MUST change together.

### Template lifecycle

Generation MUST:

1. create the application from a released template version;
2. record template source, semantic version, source commit, generation time,
   and the selected `Core`, `Identity`, and `Agent` profiles in a small
   `template.lock` file;
3. replace template names in application identifiers, packages or modules,
   images, configuration prefixes, API metadata, and documentation;
4. preserve one runnable sample feature until the generated application passes
   its smoke test;
5. remove or replace the sample through an explicit reviewed change;
6. create the applicable `docs/` files and capability evidence;
7. delete this temporary `STANDARDS.md` only after `docs/README.md` links to all
   generated sources of truth.

Template releases MUST use semantic versioning:

- patch: compatible fixes and documentation corrections;
- minor: additive capabilities and compatible defaults;
- major: required manual migration or incompatible generated structure.

Generated applications SHOULD support versioned template updates. An update
tool produces a reviewable patch from the version in `template.lock` to a
selected newer version. It MUST NOT overwrite customized files silently. The
patch includes migration notes, generated-file changes, documentation changes,
and required checks. Conflicts are resolved as application changes, and
`template.lock` advances only after verification passes. Applications MAY stop
following the template by explicitly removing `template.lock` and documenting
that decision.

The template maintainer owns the generation standard while this file exists.
It MUST be reviewed for every template release and at least every 90 days while
unreleased. Incompatible standard changes require a major template version.

## 11. Minimum AGENTS.md contract

`AGENTS.md` is the short, automatically loaded contract for software agents.
It MUST contain obligations and routing, not full tutorials. A repository MAY
use this copy-ready baseline:

```md
# Agent contract

- Read `docs/README.md`, then only the domain documentation relevant to the
  change.
- Follow `docs/architecture.md` before changing architecture, authentication,
  authorization, configuration, APIs, data, deployment, or security behavior.
- Reuse existing use cases, authorization policies, middleware, and adapters
  before adding another implementation.
- Do not add speculative services, dependencies, abstractions, or protocols.
- Keep web, REST, MCP, and CLI behavior on the same application use cases.
- The CLI calls REST only. MCP tools never execute arbitrary SQL or shell.
- Update implementation, contract, tests, changelog, and relevant docs in the
  same change.
- Follow the repository source-of-truth matrix; do not duplicate contracts.
- Add the smallest test that proves each non-trivial or security-sensitive
  behavior.
- Run all repository-defined checks and inspect their output before declaring
  completion.
- Record deviations under `docs/decisions/exceptions/` with owner, approval,
  evidence, risk, compensating controls, review date, expiration, and
  replacement path before violating a standard.
```

Repository-specific `AGENTS.md` rules SHOULD add concrete commands and paths.
They MUST NOT copy entire sections from this document. Nested agent files
SHOULD exist only where a subtree genuinely needs different instructions.

## 12. Sources of truth

Every fact MUST have one canonical source.

| Concern | Canonical source |
|---|---|
| Runtime behavior | Application code and executable tests |
| Configuration | Configuration schema or loading code |
| Deployment configuration | Environment, mounted configuration, or deployment manifests |
| Runtime provider configuration | Versioned database records and the authorized management API |
| Secret values | Secret manager or encrypted secret store; database stores only references |
| REST contract | OpenAPI document |
| Database shape | Ordered migrations |
| Deployment behavior | Container and deployment manifests |
| Agent obligations | `AGENTS.md` |
| Architectural standards | `docs/architecture.md` after generation |
| Usage and operational explanation | Relevant file under `docs/` |
| User-visible changes | Changelog |
| Template ancestry and selected profiles | `template.lock` while the application follows the template |

Derived examples, generated clients, environment examples, and prose
references MUST be checked against their canonical source. Where practical,
CI SHOULD detect drift automatically.

Documentation change routing:

| Behavior changed | Documentation destination |
|---|---|
| Setup or first-run experience | Root README and relevant guide |
| Configuration | `docs/configuration.md` and safe example configuration |
| Architecture or scaling boundary | `docs/architecture.md` |
| REST behavior | OpenAPI and `docs/api.md` |
| Authentication, roles, invitations, or tokens | `docs/authentication.md` |
| MCP behavior | `docs/mcp.md` |
| CLI behavior | `docs/cli.md` |
| Deployment | `docs/deployment.md` |
| Migrations, backup, restore, health, or upgrades | `docs/operations.md` |
| Capability status or revisit trigger | `docs/capabilities.md` |
| Contributor workflow | `docs/development.md` or contribution guide |
| User-visible release behavior | Changelog |

## 13. Testing and acceptance

### Test strategy

The template MUST support a layered test strategy:

```text
Few       deployment smoke tests and critical browser flows
          API contract and PostgreSQL compatibility tests
More      HTTP/MCP component tests and real temporary SQLite tests
Many      use-case, authorization, validation, and pure domain tests
```

Requirements:

- Unit tests MUST run without Docker, network access, or external services.
- Application use cases SHOULD use small hand-written fakes before generated
  mocks.
- Repositories MUST be tested against a real temporary SQLite database rather
  than SQL-string mocks.
- PostgreSQL migration and dialect compatibility MUST run in isolated CI
  infrastructure.
- HTTP handlers and MCP tools MUST have component tests at their transport
  boundaries.
- Authorization tests MUST cover owner, admin, member, viewer, missing
  membership, wrong scope, revoked token, and insufficient token scopes.
- When custom roles are enabled, tests MUST cover permission allowlisting,
  assignment authority, role changes and deletion, and last-owner protection.
- Invitation tests MUST cover expiration, duplicate active invitations,
  single-use acceptance, resend rotation, revocation, and enumeration-safe
  failure.
- Authentication, token, parser, and validation boundaries SHOULD use fuzzing
  where the selected language supports it.
- Concurrent code, streams, jobs, and shared state MUST run the language's
  available race or concurrency checks.
- Accessibility checks MUST combine repeatable automation with manual
  keyboard and assistive-technology review for critical flows.
- Generated contracts MUST be regenerated in CI and checked for drift.
- WebMCP checks MUST cover exact tool schemas, secret and hidden-field
  exclusion, visible-form preparation, focus, destructive-action non-
  submission, and ordinary-browser fallback when `document.modelContext` is
  absent.
- Container smoke tests MUST prove startup, liveness, readiness, and one useful
  request.
- Backup verification MUST restore into isolated storage and validate expected
  records.
- Release verification MUST validate checksums, signatures, provenance, and
  SBOM publication.

A blanket coverage percentage MUST NOT replace behavior-focused acceptance.
Each security, migration, API error, authorization, concurrency, and recovery
path MUST have the smallest test that would fail if it regressed.

### Template acceptance

A conforming template MUST demonstrate that:

- `Core`, `Identity`, and `Agent` are generated and evidenced;
- a real-user or durable-data deployment completes `Production` evidence;
- a clean checkout starts with one documented container command;
- the default installation uses SQLite and persistent storage;
- PostgreSQL is optional and supports the same shared migrations;
- the web application, REST, MCP, and CLI reach the same use cases;
- optional WebMCP registration is feature-detected, bounded to non-secret
  browser workflows, and harmless when unsupported;
- the CLI manipulates only remote REST resources;
- local authentication and optional secure OIDC work;
- owner, admin, member, and viewer permissions are enforced;
- a first-run owner workspace is created atomically;
- workspace-owned resources carry `workspace_id` and reject wrong-workspace
  access;
- invitations are expiring, single use, revocable, and audited;
- scoped API tokens are revocable, stored only as hashes, and never redisplayed;
- liveness and readiness have different, correct semantics;
- errors, pagination, rate limits, and conditional requests follow the public
  API contract;
- telemetry correlates requests, traces, metrics, and logs without requiring a
  bundled backend;
- backups can be verified and restored;
- the production backup schedule and restore test meet the selected RPO and
  RTO;
- the application can move to multiple stateless instances after PostgreSQL
  and a traffic distributor are added;
- release artifacts have vulnerability checks, an SBOM, provenance,
  signatures, and checksums;
- automated and manual evidence supports WCAG 2.2 AA review;
- MCP origin, session, tool-version, untrusted-input, data-exfiltration, and
  audit controls are verified;
- WebMCP same-origin, security-header, schema, redaction, focus, and fallback
  behavior is verified when the optional surface is selected;
- `docs/README.md` exists and routes each audience to current, canonical
  instructions before temporary generation standards are deleted;
- `template.lock` records template ancestry while update compatibility is
  maintained.

## 14. Explicit non-goals

This standard does not require:

- a programming language, framework, dependency-injection library, database
  library, or test framework;
- a cloud, CI provider, container registry, reverse proxy, identity provider,
  observability backend, or infrastructure vendor;
- a single-page application or frontend build runtime;
- microservices;
- Kubernetes for ordinary installations;
- Redis, Kafka, NATS, or another broker by default;
- object storage before files exist;
- WebSockets where SSE is sufficient;
- GraphQL or gRPC where REST is sufficient;
- an internal AI agent merely because MCP is available;
- billing, search, SCIM, feature flags, AsyncAPI, or A2A before their trigger
  exists;
- a blanket test-coverage percentage;
- nested organization or group hierarchies before a product requirement;
- database-per-workspace isolation before compliance, scale, or customer
  requirements justify it;
- speculative extension points or abstractions.

Implementations MAY choose any technology that satisfies the observable
contracts and operational standards in this document. Technology choices
SHOULD prefer the language standard library, native platform features, and
already-selected dependencies before adding new infrastructure.
