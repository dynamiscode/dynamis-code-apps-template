# 0010: Streamed S3 uploads

- Status: accepted
- Date: 2026-11-13
- Owners: template maintainers

## Context

ADR 0005 sent S3 upload bytes straight from the client to a presigned bucket
URL. The browser therefore needed the bucket's CORS rules to list every
application origin. A deployment whose origin was missing from that list, such
as a Railway URL that differs from `APP_PUBLIC_URL`, received a preflight
response without an allow-origin header, and the upload failed. Every new
deployment had to repeat the bucket CORS step, and the origin checks and the
signed-header contract lived outside the application. Local storage and the
multipart form path already stream bytes through the application.

## Decision

- Uploads stream through the application on every storage driver. For S3,
  `POST .../files/uploads` returns `uploadUrl` pointing at the application's
  content route, `.../files/{fileId}/content`, with no `uploadHeaders`, and
  returns `completeUrl`. The content route requires the same bearer token as
  other REST writes.
- For S3, the content write stores the bytes and leaves the file pending.
  `complete` verifies the stored object (its size, detected content type, and
  SHA-256 digest) and only then makes the file ready. Local storage keeps its
  behavior: the content write makes the file ready, and no completion URL is
  returned.
- A content write that fails at the object store releases its quota
  reservation. Content validation failures keep their existing handling.
- Presigned URLs remain for downloads. S3 downloads still redirect to a
  short-lived presigned GET URL. Bucket CORS is no longer part of upload setup.
- REST request and response shapes, the 16 MiB per-object and 1 GiB
  per-workspace defaults, the global body limit, and error responses are
  unchanged.
- The browser uploader moves to the multipart route in a companion change
  (#271). Until that change lands, the browser's initiate and complete path
  still uses presigned PUT.

This amends the upload clause of ADR 0005, which said S3 flows use presigned
PUT and GET URLs. The presigned GET clause and the rest of ADR 0005 stand.
The presigned PUT code and its flag are removed in a follow-up change (#272).

## Consequences

Benefits: upload setup needs no bucket CORS rule; uploads use one origin and
the application's existing bearer and CSRF rules; the origin and signed-header
contract stays inside the application.

Costs: for S3, bytes are read twice: once when the content write stores them
and again when `complete` reads the object back to verify it. Both reads are
bounded by the per-object limit. The completed file is verified from the
stored object, so `complete` costs a read proportional to file size.

Rejected alternatives: keeping presigned PUT and documenting a bucket CORS
rule (keeps origin drift and operator setup); a wildcard bucket CORS rule
(broader than any deployment needs); raising the global HTTP body limit
(breaks ADR 0005's bounded-body rule).

Existing pending rows from interrupted presigned uploads are left alone. The
existing reconciliation hooks handle them, so no migration is needed. Object
cleanup, scanning, deletion, and public access stay outside this slice, as ADR
0005 states.

## Revisit when

Upload bandwidth through the application becomes a measured bottleneck, the
per-object limit is raised far enough that double reads matter, or direct
client-to-bucket uploads become a product requirement.
