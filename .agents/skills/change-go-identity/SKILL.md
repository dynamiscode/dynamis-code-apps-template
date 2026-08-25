---
name: change-go-identity
description: Change Dynamis Code authentication, authorization, workspaces, roles, sessions, invitations, tokens, OIDC, identity providers, or related security and audit behavior.
---

# Change Go Identity

Read [AGENTS.md](../../../AGENTS.md), Phase 02 in
[PLAN.md](../../../PLAN.md), the accepted architecture decision, and the
implemented authentication contract. Treat this as security-sensitive work.

## Outcome

Preserve deny-by-default, explicit workspace authority, revocable credentials,
safe external identity binding, and auditable administrative changes.

## Workflow

1. Trace browser, REST, MCP, CLI, session/token, policy, repository, and audit
   call sites before changing a shared signature or permission.
2. Define principal, instance/workspace scope, permission, credential lifetime,
   revocation, reauthentication, enumeration, and audit behavior.
3. Keep roles as permission collections. Protect the final owner and prevent a
   credential or assigner from granting authority they do not hold.
4. Store password, session, invitation, and token secrets safely; display
   one-time values once; never log secrets or authorization headers.
5. For OIDC, bind provider, session, state, S256 PKCE verifier, and nonce;
   validate redirect, issuer, audience, signature, expiry, and verified email;
   identify external users by issuer plus subject.
6. Keep provider choice server-controlled and protect discovery or callback
   network access from SSRF.
7. Update all interface behavior, tests, audit events, authentication docs,
   changelog, and capability evidence together.

## Verify and stop

Run the complete role/scope matrix plus credential lifecycle, enumeration,
OIDC mismatch, redaction, audit, and both-database tests. Stop if account
linking depends silently on email, instance authority implies workspace access,
or a failure could expose credentials or remove the final owner.
