# Exception: catalog v1 update precondition

- Profile and requirement: REST compatibility, new major path and 90-day deprecation window for breaking changes (`docs/api.md`).
- Status: accepted
- Responsible owner: repository maintainer
- Approving maintainer: user, explicitly in this conversation on 2026-11-25
- Security-owner approval, when required: not required; no authentication or authorization change
- Created: 2026-11-25
- Review by: 2027-02-23
- Expires: 2027-02-23

## Reason and evidence

The user confirmed there has been no release and approved requiring a revision precondition on the existing catalog v1 update without a 90-day migration window. Unconditional writes cannot protect against stale updates or ABA.

## User, security, compatibility, and operational risk

Existing unpublished clients that issue unconditional catalog entry PUT requests must read the entry revision and send `If-Match`. They receive 428 until updated. Other v1 operations are unchanged.

## Compensating controls

List and create responses expose the revision. Stale updates fail with 412. Browser forms use the same revision. Workspace authorization remains unchanged.

## Replacement or removal path

Remove this temporary exception when this pre-release contract has been reviewed and incorporated into the published API baseline. It does not authorize other breaking changes.

## Verification

Run the focused catalog service, REST, browser, migration, and OpenAPI contract checks before release.
