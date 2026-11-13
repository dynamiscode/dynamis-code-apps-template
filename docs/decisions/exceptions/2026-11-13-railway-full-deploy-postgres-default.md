# Exception: full Railway template defaults to Railway-managed PostgreSQL for one instance

- Profile and requirement: `AGENTS.md` "Default to SQLite for one instance."
  The Railway full template, drafted in
  [#262](https://github.com/dynamiscode/dynamis-code-apps-template/issues/262)
  and not yet published, defaults to Railway-managed PostgreSQL injected through
  `DATABASE_URL` for a single instance. The rule "PostgreSQL is required before
  multiple instances" is unchanged, and the full template is single-instance
  only. The minimal template, manual Compose, and the "SQLite: one instance"
  guidance in [deployment](../../deployment.md) are unchanged.
- Status: proposed
- Responsible owner: David Londono (@davidlondono)
- Approving maintainer: David Londono (@davidlondono), pending approval
- Security-owner approval, when required: not required. No security control is
  weakened. Postgres traffic stays on Railway's private network, which Railway
  documents as WireGuard-encrypted. TLS is recommended, not enforced.
- Created: 2026-11-13
- Review by: 2027-02-11
- Expires: 2027-02-11

## Reason and evidence

The one-click full deploy ([map #258](https://github.com/dynamiscode/dynamis-code-apps-template/issues/258))
needs zero inputs from a non-technical user. [#262](https://github.com/dynamiscode/dynamis-code-apps-template/issues/262)
shows the full template reaches `/setup` and setup completes, with `DATABASE_URL`
carrying no `sslmode`. The minimal SQLite path is in
[#261](https://github.com/dynamiscode/dynamis-code-apps-template/issues/261).
Wording follows [#260](https://github.com/dynamiscode/dynamis-code-apps-template/issues/260):
"required" means the app breaks or loses data without it, and backups and TLS
are "recommended."

## User, security, compatibility, and operational risk

- The one-click deploy schedules no backups.
- No SQLite-to-PostgreSQL migration tool exists. Workspace export/import covers
  items only (1,000 records, 4 MiB) and omits memberships, audit events, and
  credentials ([data lifecycle](../../data-lifecycle.md#portability)).
- Postgres connections are not TLS-enforced; the network layer encrypts them.
- The full deploy is single-instance only.

## Compensating controls

These are planned for the docs follow-up described in #260 and are not in the
repository yet:

- The full template's docs label the deploy single-instance.
- Backups and TLS are recommended in [deployment](../../deployment.md) and
  [operations](../../operations.md), replacing the current "must" wording.
- The minimal SQLite template remains the default one-click path.
- The exception is linked from the deployment docs.

## Replacement or removal path

Remove this exception when either (a) an accepted ADR covers the full profile, or
(b) the full template is retired or switched to SQLite. If neither happens by
2027-02-11, the exception expires and the template must change or be re-excepted
with a new review.

## Verification

- Deploy evidence from [#261](https://github.com/dynamiscode/dynamis-code-apps-template/issues/261)
  and [#262](https://github.com/dynamiscode/dynamis-code-apps-template/issues/262).
  The S3 conformance part of #262 is owner-reported.
- No code tests: the change is packaging and docs only.
