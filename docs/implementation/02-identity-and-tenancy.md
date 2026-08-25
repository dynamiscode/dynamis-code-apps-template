# Phase 02: Identity and Tenancy

## Goal

Implement the complete baseline identity and workspace boundary before the
first workspace-owned feature exists.

## Standards covered

`STANDARDS.md` Section 4, relevant configuration and audit requirements in
Section 6, change obligations in Section 7, and identity acceptance in Section
13.

## Prerequisites

Phase 01 configuration, transaction, migration, identifier, and repository
boundaries are complete.

## Required outcomes

- Model global users and external identities separately from workspaces and
  memberships. Enforce one active membership per workspace and user.
- Ship protected `owner`, `admin`, `member`, and `viewer` roles as permission
  collections. Deny by default; protect the final owner and require explicit
  ownership transfer.
- Create the first user, workspace, owner membership, and separate
  instance-admin record atomically through environment bootstrap, a protected
  browser setup form, or the explicit CLI fallback. Do not ship default
  credentials; disable all first-run paths after success.
- Implement current password hashing, enumeration-safe local authentication,
  revocable hashed sessions, CSRF-ready browser sessions, and secure cookie
  policy.
- Implement expiring, single-use, revocable invitations with resend rotation,
  duplicate prevention, hashed secrets, and audit events.
- Implement named, scoped API tokens displayed once and stored only as hashes.
- Implement optional OIDC provider discovery, exact redirects, state, S256
  PKCE, nonce, issuer/audience/signature/expiry/verified-email validation, and
  issuer-plus-subject identity keys.
- Use a server-controlled provider registry. Prevent arbitrary issuer input,
  SSRF, silent email merging, and authority above the principal.
- Separate instance administration from workspace authority and audit every
  security or administrative transition without secret material.
- Expose identity application use cases through accessible browser flows and
  bearer-authenticated workspace REST management where applicable. Workspace
  listing and creation remain browser-only by decision.
- Keep passwords, OIDC and reauthentication material, login state, invitation
  secrets and URLs, token secrets, sessions, and CSRF fields outside any
  browser-agent surface. WebMCP may prepare only non-secret visible controls.
- Create the final authentication document after behavior exists.

## Evidence

- Authorization matrix covers all four roles, missing membership, wrong
  workspace, revoked credentials, insufficient scopes, and final-owner rules.
- Invitation tests cover expiry, duplicate active invitations, acceptance,
  resend rotation, revocation, and safe errors.
- OIDC tests cover state, PKCE, nonce, issuer, audience, redirect, provider,
  and identity-linking failures.
- Secret/log redaction and session/token revocation tests pass on both database
  adapters.
- Environment bootstrap rejects partial configuration, browser setup enforces
  a deployment setup token and CSRF, and repeated starts do not duplicate the
  first owner.
- Full Go test, vet, and race gates pass.

## Exclusions

No custom roles, SCIM, MFA, passkeys, nested organizations, database per
workspace, or silent instance-admin access to workspace data.

## Completion gate

A first owner workspace is atomic, every protected use case evaluates explicit
workspace permissions, all credentials are safely stored and revocable, and
the Identity profile evidence has no pending Phase 02 group.
