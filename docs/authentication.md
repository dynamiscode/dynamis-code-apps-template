# Authentication and Workspace Authorization

Identity is global. Workspace authority comes only from an active membership;
instance administration never grants workspace access.

## First owner

Bootstrap runs once and creates the first user, workspace, and owner membership
and instance-admin record in one transaction. It has no default credentials.
Choose one of these deployment-safe paths:

### Environment bootstrap

Set all three admin variables before starting the server. The application
creates the first owner before accepting traffic:

```sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE=Example
read -s BOOTSTRAP_ADMIN_PASSWORD
export BOOTSTRAP_ADMIN_PASSWORD
go run ./cmd/server
unset BOOTSTRAP_ADMIN_PASSWORD
```

### Browser setup

Set a deployment secret before starting an empty instance, then open `/setup`.
The form accepts the setup token, email, workspace name, and password. The
token is not stored in the database; the route disables itself after successful
bootstrap:

```sh
read -s BOOTSTRAP_SETUP_TOKEN
export BOOTSTRAP_SETUP_TOKEN
go run ./cmd/server
unset BOOTSTRAP_SETUP_TOKEN
```

### CLI fallback

The explicit command remains useful for operators with shell access. Its
password is still supplied through the environment, never an argument:

```sh
read -s BOOTSTRAP_ADMIN_PASSWORD
export BOOTSTRAP_ADMIN_PASSWORD
go run ./cmd/bootstrap-admin \
  -email owner@example.com \
  -workspace Example
unset BOOTSTRAP_ADMIN_PASSWORD
```

All three paths grant the first user separate instance administration as well
as owner membership. Instance administration never grants workspace access.
When the database is already bootstrapped, bootstrap variables are ignored and
`/setup` is disabled. Any authenticated user may create another workspace and
becomes its owner. The browser home page exposes workspace creation and membership
listing. Workspace management is available through the Settings screens at
`/workspaces/{workspaceId}/settings/members`,
`/workspaces/{workspaceId}/settings/invitations`,
`/workspaces/{workspaceId}/settings/tokens`, and
`/workspaces/{workspaceId}/settings/export`;
the export screen's `Download JSON` action uses
`/workspaces/{workspaceId}/settings/export/download`. `/sessions` manages the
current user's browser sessions.
Workspace deletion, suspension, and archival are not exposed until their data
lifecycle is implemented.

## Local authentication

Passwords use Argon2id with a random 16-byte salt, 19 MiB memory, two
iterations, one lane, and a 32-byte result. Authentication returns the same
public error for an unknown email, invalid email, missing local password, or
wrong password.

Session and CSRF secrets are independent 256-bit values. Only SHA-256 hashes
are stored. Sessions expire, can be listed and revoked, and re-evaluate current
workspace membership on protected operations. Browser handlers must use the
provided cookie policy: `HttpOnly`, `SameSite=Lax`, and `Secure` under HTTPS;
they must verify the session-bound CSRF secret on state-changing requests.

OIDC sessions retain only the provider identifier. Revocation returns that
identifier so the web layer can also use the provider's discovered logout
endpoint when available.

## Workspace permissions

Roles are protected permission collections:

| Permission | owner | admin | member | viewer |
|---|:---:|:---:|:---:|:---:|
| Read workspace, members, resources | yes | yes | yes | yes |
| Write normal resources | yes | yes | yes | no |
| Update workspace, manage members and invitations | yes | yes | no | no |
| Export workspace data | yes | yes | no | no |
| Delete workspace or transfer ownership | yes | no | no | no |

Checks deny by default and require an explicit workspace. Tokens are
intersected with the user's current role on every use. Owners cannot be
removed or demoted through ordinary membership changes; ownership transfer is
the only path and demotes the previous owner to administrator.

## Invitations and API tokens

Invitations belong to one workspace, normalized email, and non-owner role.
They expire, are single use, prevent duplicates, support revocation, and rotate
their 256-bit secret on resend. A signed-in user may accept an invitation only
when its email matches. A new local account may be created from a valid invite;
an existing account must sign in instead. Failures use one safe error.

API tokens belong to one user and workspace. Their named scopes cannot exceed
the creator's current permissions. Secrets are shown only by the creation
result and stored as SHA-256 hashes. Expiration, scope changes, last use, role
changes, and revocation are enforced.

Bearer REST management uses the same workspace permission checks. Workspace listing and creation
remain browser-only. Invitation create/resend returns a copyable URL and delivery status; SMTP is
optional and invitation rows commit before delivery is attempted.

## OIDC

OIDC is disabled by default. One deployment-owned provider can be enabled by
the variables in [configuration](configuration.md). Startup performs discovery
through a public-HTTPS-only client that rejects loopback, private, link-local,
and unsafe redirect destinations.

Login selection uses the configured provider ID, never a request-supplied
issuer. Transactions bind provider, browser session, exact redirect, hashed
state, S256 PKCE verifier, and nonce. Callback processing validates the code,
signature, issuer, audience, expiration, nonce, and verified email. External
identity keys are issuer plus subject. Matching email never silently links an
identity; linking to an existing user is explicit and requires reauthentication.

Browser login starts at `GET /auth/oidc/{providerId}` and returns through the configured callback.
Login transactions use short-lived HTTP-only cookies. Linking starts from `/security`, requires the
current local password, and binds the transaction to the authenticated session and user ID.

WebMCP does not expose login, logout, passwords, reauthentication, OIDC state,
invitation URLs or secrets, token secrets, session credentials, or CSRF values.
It is an optional browser-tab enhancement for already authenticated pages and
cannot replace the server authorization and CSRF-protected authentication
flows.

## Audit data

Bootstrap, workspace creation, membership and ownership changes, session
creation/revocation, invitation lifecycle, token lifecycle/use, and external
identity creation/linking append audit events. Metadata contains role, scope,
provider ID, or outcome only—never credential values or authorization headers.
Audit access, retention, export, and deletion are defined in
[data lifecycle](data-lifecycle.md) and [operations](operations.md).
