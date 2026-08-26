# Security Policy

## Supported versions

The latest released minor version receives security fixes. Pre-release and
customized generated applications remain the operator's responsibility.

## Report a vulnerability

Use the repository Security tab and private vulnerability reporting. If that
channel is unavailable, contact the repository owner through its configured
private channel. Do not file a public issue containing exploit details,
credentials, personal data, or signed URLs.

Include affected version, impact, reproduction, and suggested remediation if
known. Maintainers acknowledge reports, coordinate disclosure, issue a
semantic patch or minor release, and rotate exposed credentials when needed.

## Security boundaries

- TLS termination, secret storage, PostgreSQL TLS, backup protection, traffic
  distribution, and host hardening belong to the deployment operator.
- The application denies authorization by default and scopes workspace access
  in shared policy. Report any bypass, cross-workspace access, credential leak,
  SSRF, injection, or unsafe restore behavior privately.
- Never commit `.env`, database URLs, authorization headers, session or
  invitation values, token secrets, OIDC secrets, OTLP headers, backups, or
  signing material.

Release verification is documented in [docs/release.md](docs/release.md).
