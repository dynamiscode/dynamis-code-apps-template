# 0008: Built-in MCP authorization server

- Status: accepted
- Date: 2026-11-10
- Owners: template maintainers
- Supersedes: the external-only assumption in [0007](0007-mcp-oauth-protected-resource-metadata.md)

## Context

MCP clients need OAuth tokens to call `/mcp`. The application authenticates
users through local passwords, passkeys with TOTP, optional OIDC, SCIM, and
invitations, and every login produces a browser session. Browser sessions use
cookies and CSRF and cannot be presented as bearer credentials. API tokens are
long-lived and have no consent or refresh flow.

## Decision

- The application is the authorization server for MCP. A user logs in with any
  supported method, approves one workspace on a consent page, and the server
  issues MCP tokens.
- Clients register dynamically (RFC 7591). Redirect URIs must be HTTPS or
  loopback HTTP and match exactly.
- Each grant is bound to one user, one workspace, one client, and a set of scopes
  capped by the user's workspace role. This matches API token binding.
- Authorization codes last 60 seconds and are single-use. PKCE S256 is required.
  Access tokens are opaque, last 15 minutes, and are looked up per request. Refresh
  tokens last 30 days from issue and rotate on every use; presenting a rotated
  token revokes the grant.
- Access is checked live on every use: the consenting session must be active, the
  user must still be a workspace member, and MFA policy applies. Session
  revocation, password change, passkey removal, membership removal, and account
  deletion therefore end MCP access without separate revocation hooks.
- Token and revocation endpoints return OAuth-format errors (RFC 6749 §5.2 and
  RFC 7009), not Problem Details. This is a protocol requirement and is recorded
  here as the approved exception to the REST error convention.
- `MCP_OAUTH_ENABLED` turns the built-in server on. It uses `APP_PUBLIC_URL` as
  the issuer and conflicts with `MCP_OAUTH_AUTHORIZATION_SERVER`.

## Consequences

Benefits: any login method works with MCP, with no external identity provider
and no OIDC dependency. Revocation is immediate, and no signing keys are managed.

Costs: a new table set and migration (`000016`), with export, retention, and
deletion obligations recorded in [data lifecycle](../data-lifecycle.md). Consent
pages need localization and accessibility work. Expired-record pruning is not yet
implemented and is tracked as follow-up.

Rejected alternatives:

- JWT access tokens: revocation would need short lifetimes or a deny list, and
  signing keys would need management.
- An external authorization server only: it would require an identity provider
  for MCP even when users sign in by password or passkey.
- Session cookies for MCP: MCP clients are not browsers, and cookies bypass the
  bearer model.

## Revisit when

- Measured client behavior requires JWT access tokens or token introspection.
- Refresh tokens need idle expiry instead of absolute expiry.
- Dynamic registration needs to be restricted to pre-registered clients only.
