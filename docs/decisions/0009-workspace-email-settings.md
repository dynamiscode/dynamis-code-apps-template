# 0009: Workspace Email Settings

- Status: accepted
- Date: 2026-11-11
- Owners: template maintainers

## Context

The template sends invitation email and account email (verification and
password recovery) through one optional instance-wide SMTP sender configured by
environment variables. A workspace cannot choose its own delivery provider or
sender identity. Two encryption keys already exist: `MFA_ENCRYPTION_KEY` for
TOTP secrets and `WEBHOOK_ENCRYPTION_KEY` for webhook secrets. Future
per-workspace providers need stored credentials, which would add a third
key-holding surface unless the keys are consolidated.

Account email must keep working when a workspace is misconfigured, and the
template has no vendor SDK dependencies today.

## Decision

- Each workspace may configure its own email provider and sender identity.
- Workspace settings win. Environment configuration (the instance default)
  fills in for workspaces that have not configured email.
- Provider tokens are encrypted in the database with a general
  `ENCRYPTION_KEY`. Each encrypted row must record its key version.
- `WEBHOOK_ENCRYPTION_KEY` remains supported as a fallback for webhook secrets
  until those secrets move to the general key.
- Account email (verification and password recovery) uses only the instance
  environment configuration, never workspace settings.
- Providers use plain HTTPS and SMTP. No vendor SDKs are added.

## Consequences

Once implemented, workspaces will be able to use their own delivery path
without changing deployment configuration, and account recovery stays
independent of workspace state. Stored provider credentials will be encrypted
at rest with versioned keys, which will make later rotation possible. Today the
template still has only the instance SMTP sender and the MFA and webhook keys.

Costs: until the keys are consolidated, operators manage both
`ENCRYPTION_KEY` and `WEBHOOK_ENCRYPTION_KEY`. Plain HTTPS and SMTP means each
new provider needs its own request code and tests.

Rejected alternatives: vendor SDKs (add dependencies for what plain requests
cover), environment-only configuration (workspaces cannot choose a provider),
and workspace-scoped account email (an account can belong to several
workspaces, so no single workspace owns its recovery mail).

Key rotation and new providers remain deferred until their triggers in
[capabilities](../capabilities.md#deferred-capabilities) are accepted.

## Revisit when

A provider is required whose API cannot be called over plain HTTPS or SMTP,
credentials must be rotated after a compromise or policy change, or multiple
application instances need shared email delivery ownership.
