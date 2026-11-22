# 0015: Engagement agreement file reference

- Status: proposed
- Date: 2026-11-22
- Owners: template maintainers

## Context

Staff need to record that a signed agreement exists for a piece of client
work and point to the stored file. Today a file cannot link to a client or an
engagement: the Files metadata holds workspace ID and owner user ID only, and
[0005-files-object-storage](0005-files-object-storage.md) keeps generic
attachments outside the Files slice. Issue #552 confirmed this is a gap, and
its update records the decision from the contractor and gym evidence. The
engagement spec is #563.

Files use a short ID ([0009](0009-short-id-codec.md)) as their public
identifier.

## Decision

- An engagement carries an agreement-recorded flag and an optional internal
  file reference. The reference is the file's short ID, stored as text on the
  engagement.
- The reference is not an attachment. There is no link table, no foreign key
  to the files table, and no file browser on the template surface.
- The Files slice is unchanged. It does not learn about engagements, and
  [0005-files-object-storage](0005-files-object-storage.md) and
  [0005-public-sharing](0005-public-sharing.md) stand as written.
- The reference grants no access. Reading a file still goes through Files
  and its workspace-scoped authorization.

## Consequences

- Benefit: the engagement records the agreement without widening the Files
  slice or the resource model.
- Cost: the database does not enforce the reference. The file can be removed
  or the ID mistyped, leaving a dangling reference. Display code must treat
  an unresolved reference as normal.
- Cost: no query answers "which engagement uses this file".
- Rejected: a link table between files and engagements. It is a generic
  attachment, which 0005-files-object-storage keeps out.
- Rejected: adding an owner reference to the files table. It makes Files
  depend on engagements.
- Rejected: a file browser or picker in the template. Forks may add one.

## Revisit when

Attachments to work are needed in the template.
