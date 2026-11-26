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

Source security checks include secret scanning, govulncheck, pinned workflow
validation, CodeQL analysis for Go and JavaScript/TypeScript, and bounded fuzz
smoke tests for import and URL validation.

## Pull request security review

A successful workflow means that its jobs passed, not that they emitted no
security findings. Before merging, a maintainer should inventory the open pull
request after its checks complete:

```sh
node scripts/pr-security-inventory.mjs OWNER/REPO PR_NUMBER > inventory.json
```

The command uses the authenticated GitHub CLI (`gh`); set `GH_PATH` to select
its executable explicitly. Its token needs read access to pull requests, checks
and code scanning alerts in the target
repository. Organization policy, fork permissions, disabled code scanning,
or missing access can prevent a complete inventory; the command fails rather
than interpreting an API error as zero findings. It reads check-run annotations
on the PR head and GitHub's merge commit, including non-security annotations
because checks do not reliably label security annotations. It also reads open
code scanning alerts for `refs/pull/PR_NUMBER/merge`. It does not inventory
other repositories, secret scanning, Dependabot, or closed alerts. Review those
separately when applicable. Checks or alerts may appear after the command runs;
rerun it after checks complete and immediately before merge.

For each finding, record an independent decision in a JSON review file. Copy the
repository, pull request, head, merge and each finding's `id` from the inventory:

```json
{
  "repository": "OWNER/REPO",
  "pull_request": 123,
  "head": "HEAD_SHA",
  "merge": "MERGE_SHA",
  "dispositions": [
    {
      "id": "alert:123",
      "disposition": "accepted-risk",
      "reviewer": "maintainer",
      "reason": "Document the impact and rationale here."
    }
  ]
}
```

Allowed dispositions are `fixed`, `false-positive`, and `accepted-risk`.
`fixed` requires evidence of the fix; a stale finding must not be marked fixed
without investigation. Keep sensitive details in an appropriately private
review record. Then run:

```sh
node scripts/pr-security-inventory.mjs OWNER/REPO PR_NUMBER review.json
```

Exit 0 means all currently observed findings have dispositions, all observed
checks completed, and the latest `CodeQL (Go)` and `CodeQL (JavaScript)` checks
on the merge commit succeeded. It is not a clean-security certification. Exit
1 means findings lack dispositions, dispositions are stale, checks are incomplete,
or either required CodeQL check is missing, incomplete, or unsuccessful.
Exit 2 means the inventory or review is invalid or unavailable. A changed head
or merge commit invalidates the review; rerun and reassess each finding. The
command only reads GitHub data and never dismisses or modifies alerts.

Release verification is documented in [docs/release.md](docs/release.md).
