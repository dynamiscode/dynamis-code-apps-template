# 0015: Engagement agreement file reference

- Status: accepted
- Date: 2026-11-23
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
identifier. The short ID is derived from the file's internal key, so the
internal key is the identity and the short ID is its public form.
[0011](0011-identity-integer-keys.md) requires foreign references to use
integer keys. The owner chose that form for this reference over the text short
ID described in issues #605 and #608.

## Decision

- An engagement carries an agreement-recorded flag and an optional file
  reference. The reference is a foreign key column on the engagement that holds
  the file's internal primary key and references the files table.
- The short ID is the public form of that key. Pages and APIs show the short
  ID. Storage and joins use the internal key. A lookup by short ID decodes it
  to the internal key first. When setting the reference, the Files service
  verifies that the file belongs to the engagement's workspace.
- The reference is not an attachment. There is no link table and at most one
  reference per engagement. The Files slice does not learn about engagements;
  the foreign key points from engagements into files.
- The foreign key uses ON DELETE RESTRICT. A file that an engagement references
  cannot be deleted until the reference is cleared. File deletion is outside
  the Files slice today, so this only constrains it once deletion exists.
- The reference grants no access. Reading the file still goes through Files
  and its workspace-scoped authorization. The engagement's read permission does
  not imply file read.
- The column and its foreign key exist only when the Files profile and the
  engagement module are both generated. Omitting Files prunes the column and
  its migration step.
- [0005-files-object-storage](0005-files-object-storage.md) and
  [0005-public-sharing](0005-public-sharing.md) stand as written.

## Consequences

- Benefit: the database enforces that the file exists, and one query answers
  which engagements use a file.
- Benefit: the public short ID can change its encoding without changing stored
  references, because storage uses the internal key.
- Cost: the engagement migration depends on the files table, so Files becomes
  a required profile for agreement references. Generation must prune the column
  when Files is not selected.
- Cost: a file referenced by an engagement cannot be removed until the reference
  is cleared. Any future deletion design must handle this.
- Cost: a cross-slice foreign key couples the schemas. It is a reference only,
  with no shared behavior.
- Rejected: storing the short ID as the value. It would require decoding on
  every join and would leave the database unable to enforce the reference.
- Rejected: a link table between files and engagements. It is a generic
  attachment, which 0005-files-object-storage keeps out.
- Rejected: an owner reference on the files table. It makes Files depend on
  engagements.
- Rejected: a file browser or picker in the template. Forks may add one.

## Revisit when

- Attachments to work are needed in the template, or one engagement needs more
  than one file reference.
- File deletion is added and ON DELETE RESTRICT blocks a required flow.
