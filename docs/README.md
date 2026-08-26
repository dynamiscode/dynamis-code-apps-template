# Documentation Router

Read only the route needed for the change.

| Task | Read | Skill |
|---|---|---|
| Add an ordinary feature slice | [Architecture](architecture.md), affected interface contract, [development](development.md) | `implement-go-feature` |
| Change schema, migrations, repositories, retention, export, or restore | [Architecture](architecture.md), [data lifecycle](data-lifecycle.md), [operations](operations.md) | `change-go-data` |
| Change authentication, authorization, workspaces, roles, credentials, or OIDC | [Authentication](authentication.md), [configuration](configuration.md), [security](../SECURITY.md) | `change-go-identity` |
| Change MCP or CLI behavior | [API](api.md), [MCP](mcp.md), [CLI](cli.md) | `change-go-agent-surfaces` |
| Change web, WebMCP, or realtime behavior | [Web](web.md), [accessibility](accessibility.md), [API](api.md) | `implement-go-feature` |
| Deploy, operate, back up, restore, or upgrade | [Deployment](deployment.md), [operations](operations.md), [configuration](configuration.md) | None |
| Generate, update, or release the template | [Template lifecycle](template-lifecycle.md), [release](release.md), [capabilities](capabilities.md) | `verify-template-conformance` for verification |
| Propose a deferred capability | [Capabilities](capabilities.md), [decisions](decisions/README.md) | None; accept its trigger first |

## Sources of truth

| Concern | Source |
|---|---|
| Architecture and dependency boundaries | [Architecture](architecture.md), accepted [decisions](decisions/README.md) |
| Runtime configuration | [Configuration](configuration.md), `internal/platform/config` |
| HTTP contract | [OpenAPI](../api/openapi.json), [API](api.md) |
| Database history | `internal/platform/database/migrations/` |
| Authentication and authorization | [Authentication](authentication.md), shared identity policy |
| Browser, WebMCP, and realtime behavior | [Web](web.md), [accessibility](accessibility.md) |
| Agent surfaces | [MCP](mcp.md), [CLI](cli.md), optional [WebMCP contract](web.md#webmcp-progressive-enhancement) |
| Operations and data lifecycle | [Operations](operations.md), [data lifecycle](data-lifecycle.md) |
| Conformance and deferred triggers | [Capabilities](capabilities.md) |
| Recurring procedures | `.agents/skills/` |

Behavior claims require code or runnable evidence. Link generated contracts and
migrations; do not duplicate them in prose.
