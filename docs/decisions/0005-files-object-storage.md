# 0005: Files and object storage

- Status: accepted
- Date: 2026-08-28
- Owners: application maintainers

## Context

Users need private workspace files. The template must work on one local
instance and with common S3-compatible providers without provider-specific
implementations. File bytes can be larger than ordinary JSON requests, but the
global HTTP body limit must remain bounded.

## Decision

Add an optional Files profile requiring Core and Identity. Store workspace-
scoped metadata in the database and object bytes in `data/files` by default or
one AWS-S3-compatible adapter selected by `STORAGE_DRIVER`. Use random
workspace-prefixed keys, retain original names only as metadata, reject unsafe
filenames and disallowed or signature-mismatched content, and enforce 16 MiB
object and 1 GiB workspace defaults. S3 flows use short-lived presigned PUT/GET
URLs; local flows stream through the application. Expose browser and REST only.

Pending, ready, and failed metadata rows provide bounded reconciliation hooks.
Deletion, scanning, reconciliation, workspace deletion, public sharing, and
generic attachments stay outside this slice. Background Jobs remains separate
and is required before work must outlive a request.

## Consequences

The database and object store can become temporarily inconsistent after a
process or provider failure; pending/failed rows make this visible without
adding a queue. S3 credentials use the SDK standard credential chain and are
never logged. Generated applications without Files prune its migration,
package, routes, and OpenAPI paths.

## Revisit when

Object cleanup, malware scanning, durable retry, public access, or multiple
attachment owners become product requirements, or when object volume makes
the synchronous bounded flow insufficient.
