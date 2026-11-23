# 0016: Public Intake Form

- Status: accepted
- Date: 2026-11-22
- Owners: template maintainers

## Context

Prospects reach a business by link, QR code, or message. Staff cannot take in
a prospect without a login, a late request can be lost, and nothing tells staff
a request arrived. 0005-public-sharing forbids any public write path, so a
public form needs its own record.

Public intake is one of the six items accepted into the template (#554). It
waits for the showcase decision (#567): both real cases get leads by referral,
phone, and messaging, so the form is not the first intake to build. Decision
#551 chose a narrow form over deferral; the spec is #565.

## Decision

Allow one narrow public write path: a workspace intake form. This supersedes
only the public-write clause of 0005-public-sharing ("no public write").
Everything else in 0005 stands: Item sharing stays read-only, and 0005's status
is unchanged.

- **Link:** one active link per workspace, no expiry, token stored hashed.
  Owners and admins can rotate it (the old link stops working at once) or
  disable it.
- **Creates:** one client and one engagement in the workspace's first open
  stage by order, through the existing clients and engagement services. The
  visitor never chooses a stage. With no open stage, the form is refused.
- **Fields:** name (required); at least one of phone or email (required);
  notes (optional, 2,000 characters). No custom fields, no file uploads.
- **Consent:** the workspace sets the consent text. A required checkbox must be
  ticked; the consent time is stored with the client.
- **Spam control:** per-source rate limiting reusing the share-access limiter,
  plus a hidden honeypot field. No CAPTCHA in v1.
- **Duplicates:** a client whose phone or email matches an existing client is
  created and flagged as a possible duplicate. No automatic merge.
- **Confirmation:** a generic thank-you with no workspace name, client data, or
  submitted details. No read-back of workspace data to the visitor.
- **Language:** English and Spanish by browser language, English fallback.
- **Staff notification:** owners and admins get an in-app notification and an
  email per submission. Email runs as a background job, never inline. The
  notification links by short ID and carries no client data.

The surface is the web form only. There is no public listing, search, or read,
and no MCP, WebMCP, or CLI surface.

## Consequences

The intake service is the one seam. It reuses clients and engagement use cases
so public input never bypasses their policy. Public input is untrusted: the
service validates every field, and the audit record omits the token.

Costs: a public write surface now exists, and spam control is limited to rate
limits and a honeypot. Duplicates are flagged for staff review, not resolved.

Rejected: custom fields on the form; CAPTCHA; automatic duplicate merge; several
active links per workspace; link expiry.

Build is blocked until the showcase decision is made and the engagement item is
built. Email notification waits for email support in
the job queue; the in-app notification ships first.

## Revisit when

Measured spam gets through the rate limit and honeypot, a fork needs custom
fields or several forms per workspace, or the showcase decision changes the
public surface.
