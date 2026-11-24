# Documentation Router

Read only the route needed for the change.

| Task | Read | Skill |
|---|---|---|
| Add an ordinary feature slice | [Architecture](architecture.md), affected interface contract, [development](development.md) | `implement-go-feature` |
| Change schema, migrations, repositories, retention, export, or restore | [Architecture](architecture.md), [data lifecycle](data-lifecycle.md), [operations](operations.md) | `change-go-data` |
| Change authentication, authorization, workspaces, roles, credentials, or OIDC | [Authentication](authentication.md), [configuration](configuration.md), [security](../SECURITY.md) | `change-go-identity` |
| Change workspace member management, role changes, removal, or ownership transfer | [Authentication](authentication.md), [web behavior](web.md), [accessibility](accessibility.md) | `change-go-identity` |
| Change workspace invitation management, acceptance, or return state | [Authentication](authentication.md), [web behavior](web.md), [data lifecycle](data-lifecycle.md), [operations](operations.md) | `change-go-identity` and `change-go-data` |
| Change MCP or CLI behavior | [API](api.md), [MCP](mcp.md), [CLI](cli.md) | `change-go-agent-surfaces` |
| Change entity keys, short IDs, type codes, or typed audit, idempotency, and job keys | [Identity keys decision](decisions/0011-identity-integer-keys.md), [Short-ID codec decision](decisions/0009-short-id-codec.md), [Architecture](architecture.md#ids-time-and-transactions) | `change-go-data` |
| Change web, WebMCP, or realtime behavior | [Web](web.md), [accessibility](accessibility.md), [UX standard](ux.md), [UX review](ux-review.md), [API](api.md) | `implement-go-feature`, `ux-workflow` for browser journeys |
| Inspect a page or component visually with synthetic data, without running the app | [Offline screenshot workflow](../cmd/screenshot/README.md) | None; use `verify` for real journeys |
| Measure browser performance | [Method and asset inventory](ui-measurement.md), [workspace evidence](ui-measurement-workspace.md) | None |
| Measure administration and data screens | [Administration evidence](ui-measurement-admin.md), [reconciliation and budget recommendation](ui-measurement-reconciliation-450.md) | None |
| Measure account, authentication, and public screens | [Identity evidence](ui-measurement-identity.md) | None |
| Deploy, operate, back up, restore, or upgrade | [Deployment](deployment.md), [operations](operations.md), [configuration](configuration.md) | None |
| Generate, update, or release the template | [Template lifecycle](template-lifecycle.md), [release](release.md), [capabilities](capabilities.md) | `verify-template-conformance` for verification |
| Set up a generated application (identity, sample, demo, verify gate) | [Template lifecycle](template-lifecycle.md) | `setup-generated-app` |
| Maintain licensing, contribution, dependency attribution, or repository governance | [Governance](governance.md), [security](../SECURITY.md), [contribution guide](../CONTRIBUTING.md) | None |
| Maintain formal verification after covered code changes | [Formal verification maintenance](agents/formal-verification.md) | `maintain-formal-verification-skill` |
| Propose a deferred capability | [Capabilities](capabilities.md), [decisions](decisions/README.md) | None; accept its trigger first |

## Sources of truth

| Concern | Source |
|---|---|
| Architecture and dependency boundaries | [Architecture](architecture.md), accepted [decisions](decisions/README.md) |
| Runtime configuration | [Configuration](configuration.md), `internal/platform/config` |
| HTTP contract | [OpenAPI](../api/openapi.json), [API](api.md) |
| Database history | `internal/platform/database/migrations/` |
| Authentication and authorization | [Authentication](authentication.md), shared identity policy |
| Browser, WebMCP, and realtime behavior | [Web](web.md), [accessibility](accessibility.md), [UX standard](ux.md) |
| Agent surfaces | [MCP](mcp.md), [CLI](cli.md), optional [WebMCP contract](web.md#webmcp-progressive-enhancement) |
| Operations and data lifecycle | [Operations](operations.md), [data lifecycle](data-lifecycle.md) |
| Conformance and deferred triggers | [Capabilities](capabilities.md) |
| Recurring procedures | `.agents/skills/` |
| Domain vocabulary | [Glossary](../GLOSSARY.md) |
| Issue tracker, triage labels, domain-doc rules | [Issue tracker](agents/issue-tracker.md), [triage labels](agents/triage-labels.md), [domain docs](agents/domain.md) |

Behavior claims require code or runnable evidence. Link generated contracts and
migrations; do not duplicate them in prose.
