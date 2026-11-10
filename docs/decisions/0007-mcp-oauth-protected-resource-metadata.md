# 0007: MCP protected-resource metadata

- Status: accepted
- Date: 2026-11-10
- Owners: template maintainers
- Trigger: accepted by the template maintainer on 2026-11-10 as the requirement
  for demonstrating optional MCP OAuth discovery in the Agent profile

## Context

MCP clients find an authorization server through RFC 9728 protected-resource
metadata, and a resource server points them there with a `resource_metadata`
parameter in `WWW-Authenticate`. The Go SDK (`auth` and `oauthex`, already in
`go.mod`) implements both. The `/mcp` endpoint is stateless and accepts only
bearer API tokens, with read and write scopes enforced by the shared item use
cases.

No accepted trigger in [capabilities](../capabilities.md) covers MCP OAuth
discovery. This record is written against a maintainer request to demonstrate
applicable SDK capabilities in the template, so the feature is optional and
disabled unless an operator configures it. The maintainer accepted this trigger
on 2026-11-10.

## Decision

- Discovery is enabled only when `MCP_OAUTH_AUTHORIZATION_SERVER` is set. It
  requires `APP_PUBLIC_URL` as an origin without a path.
- The resource identifier is `${APP_PUBLIC_URL}/mcp`. It comes from
  configuration, never from request headers.
- `GET /.well-known/oauth-protected-resource/mcp` is unauthenticated, serves
  only `resource`, `authorization_servers`, `scopes_supported`, and
  `bearer_methods_supported`, and uses the SDK's public CORS handler.
- `401` challenges and insufficient-scope `403` responses on `/mcp` carry
  `WWW-Authenticate` with `resource_metadata` only when discovery is enabled.
- The token verifier is unchanged. Bearer API tokens stay the only accepted
  credentials, and the endpoint stays stateless.

## Consequences

Benefits: hosted MCP clients can discover the configured authorization server
from standard headers and metadata, with no change to existing token behavior
or to deployments that leave the setting empty.

Costs: when enabled, metadata can advertise an authorization server whose
access tokens this server rejects. Operators must enable it only where clients
can use API tokens, and the operations guidance must say so.

Rejected alternatives:

- Deriving the resource from the request `Host`: spoofable, so metadata could
  be redirected.
- Always serving metadata: it would advertise an authorization server nobody
  configured and change default responses.
- Implementing an external-issuer JWT or introspection verifier now: it needs
  an accepted trust and key-management decision and new dependencies, and no
  trigger is accepted for it.

## Revisit when

- An accepted requirement needs MCP access tokens from an external issuer. That
  requires a verifier decision and deployment evidence.
- Measured client failures show discovery is misused or unusable.
- A deployment needs `APP_PUBLIC_URL` with a path prefix. That needs a
  routing decision first.
