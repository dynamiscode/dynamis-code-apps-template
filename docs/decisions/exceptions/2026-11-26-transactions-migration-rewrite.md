# Exception: rewrite unreleased transaction migrations

- Requirement: applied migrations are forward-only.
- Status: accepted. The repository owner explicitly approved this exception on 2026-11-26.
- Responsible owner: repository maintainers.
- Approving maintainer: repository owner, 2026-11-26.
- Created: 2026-11-26.
- Review by: 2027-02-26.
- Expires: 2027-02-26.

## Scope and reason

Rewrite unreleased migrations 000057–000063 and the feature branch history so
that the feature uses the Transactions name from its first commit. The owner
confirmed that databases which applied those unreleased migrations are disposable.
This exception does not cover migrations present on the target branch.

## Risk and controls

Databases that applied the old unreleased migrations cannot be upgraded safely
by this rewrite. Stop the application and recreate those databases before using
this branch. Recreating a database deletes its contents; do not perform that
operation on any database that must be retained. Never apply this exception to
production data.

Verify fresh SQLite and PostgreSQL databases, profile generation, and the
migration history guard against the target branch. If any non-disposable database
is discovered, stop and design a separate forward migration before deployment.

## Removal

Review before 2027-02-26. Remove this exception once the feature is merged and all
affected disposable development databases have been recreated.
