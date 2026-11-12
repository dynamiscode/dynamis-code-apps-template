# Glossary

Canonical vocabulary for the Dynamis Code Apps template and the applications
generated from it. Use these terms in issues, docs, tests, and identifiers.
Definitions point to the source that owns the behavior; when they disagree,
the source wins and this file gets fixed.

## Template terms

| Term | Meaning | Source |
|---|---|---|
| Template | This repository: a Go modular monolith that generates applications. | [Architecture](docs/architecture.md) |
| Release checkout | A verified semantic-version checkout used as generator input. | [Template lifecycle](docs/template-lifecycle.md) |
| Generated application | A project created by `template-init` from a release checkout. Inherits template terms; owns its product behavior. | [Template lifecycle](docs/template-lifecycle.md) |
| Template lock | `template.lock`: committed provenance (source, version, commit, UTC time, profiles). Contains no credentials. | [Template lifecycle](docs/template-lifecycle.md) |
| Profile | A selectable bundle of template content: `Core`, `Identity`, `Agent`, or `Files`. | [Template lifecycle](docs/template-lifecycle.md) |
| Core | Always composed. The base application foundation. | [Template lifecycle](docs/template-lifecycle.md) |
| Identity | Always composed with Core. The shared authentication and authorization boundary. | [Authentication](docs/authentication.md) |
| Agent | Optional. Adds the MCP server, the REST-only `appctl` client, a bootstrap route, and an Agent smoke test. | [Template lifecycle](docs/template-lifecycle.md) |
| Files | Optional. Adds file support on object storage; requires Identity. | [Files decision](docs/decisions/0005-files-object-storage.md) |
| Instance administrator | Installation-level administration. Never grants workspace access. | [Authentication](docs/authentication.md) |
| Workspace | The tenant boundary. Workspace authority comes only from an active membership. | [Authentication](docs/authentication.md) |
| Workspace short ID | The public form of a workspace's integer key: a Sqids string. The only form in paths, arguments, bodies, and responses. | [Architecture](docs/architecture.md#ids-time-and-transactions) |
| Workspace mode | The instance setting that selects `single` or `multi` workspace operation. | [Configuration](docs/configuration.md) |
| Membership | A user's link to one workspace, carrying one role. | [Authentication](docs/authentication.md) |
| Role | A protected permission collection: `owner`, `admin`, `member`, or `viewer`. Prose may say "administrator" for `admin`. | [Authentication](docs/authentication.md#workspace-permissions) |
| Owner | The role that can delete the workspace and transfer ownership. Cannot be removed or demoted by ordinary membership changes. | [Authentication](docs/authentication.md#workspace-permissions) |
| Invitation | A workspace invitation to join with a role. Managed by owners and admins. | [Authentication](docs/authentication.md#workspace-permissions) |
| Session | A browser authentication record. Stores only hashes of its secret and CSRF values; re-checks membership on protected operations. | [Authentication](docs/authentication.md) |
| Theme preference | A user's profile appearance setting: `system` (follows the OS), `light`, or `dark`. Owned by the user, not the workspace. | [Authentication](docs/authentication.md) |
| Token | A bearer credential for REST, MCP, or CLI access. Shown once; intersected with the user's current role on every use. | [Authentication](docs/authentication.md#workspace-permissions) |
| Item | The sample workspace-scoped resource used by the item API and browser. | [API](docs/api.md) |
| Public item sharing | A bounded, browser-only public link to one Item. | [Public sharing decision](docs/decisions/0005-public-sharing.md) |
| Audit event | An append-only identity record, shown read-only in Settings → Audit history. | [Audit decision](docs/decisions/0006-workspace-audit-history.md) |
| Export | The authorized `dynamis-code.workspace/v1` workspace export. | [Data lifecycle](docs/data-lifecycle.md) |
| Backup | A complete encrypted database backup. Not the same as an export. | [Data lifecycle](docs/data-lifecycle.md), [Operations](docs/operations.md) |
| Webhook | A workspace-scoped HTTP delivery of item events, signed with HMAC-SHA256. | [Webhooks decision](docs/decisions/0004-webhooks.md) |
| Outbox | Database rows written in the same transaction as an item change, delivered later by a `webhook.delivery` job. | [Background jobs decision](docs/decisions/0005-background-jobs.md), [Operations](docs/operations.md) |
| Background job | A durable database-queued job, claimed under a lease. | [Background jobs decision](docs/decisions/0005-background-jobs.md) |
| Realtime | One-way SSE delivery, such as recipient-scoped notification events. | [Web](docs/web.md#realtime-contract) |
| MCP | Server-side Model Context Protocol tools. Never access SQL or run shell commands. | [MCP](docs/mcp.md) |
| WebMCP | Optional browser-tab enhancement, separate from server MCP. Feature-detected; the ordinary form path always works. | [Web](docs/web.md#webmcp-progressive-enhancement) |
| `appctl` | The REST-only remote CLI for the item API. Has no database access. | [CLI](docs/cli.md) |
| Capability | A standard or feature recorded in the conformance ledger with its status and evidence. | [Capabilities](docs/capabilities.md) |
| Deferred trigger | The concrete condition that must be accepted before a deferred capability is added. | [Capabilities](docs/capabilities.md) |
| Exception | An approved, time-limited deviation from a normative standard. Security exceptions need security-owner approval. Expired exceptions are non-conforming. | [Exceptions](docs/decisions/exceptions/README.md) |
| Decision record | A durable architecture choice in `docs/decisions/NNNN-title.md`. Status: proposed, accepted, or superseded. | [Decisions](docs/decisions/README.md) |

## Application-owned terms

Generated applications add their product and domain terms here. Template terms
above stay canonical; do not redefine them.

| Term | Meaning | Source |
|---|---|---|
| _Add product terms here as they are resolved._ | | |
