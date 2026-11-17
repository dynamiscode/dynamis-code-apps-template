# Release

Template versions use semantic versioning. `VERSION`, the annotated `vX.Y.Z`
tag, and the changelog entry must match.

## Current workflow status

Version-tag pushes currently run validation only: secret and vulnerability
checks, `make verify`, a `VERSION`/tag match, and Linux/macOS cross-compilation
to the runner's temporary directory. The workflow has read-only repository
permissions. It does not create release assets, upload workflow artifacts,
push an image, or create a GitHub release.

Publication can be re-enabled only after the #355 release controls and #357
merge-authorized publication evidence are complete and reviewed. Re-enable it
in a separately reviewed change that restores narrowly scoped permissions,
proves exact-merge-SHA authorization and retry/conflict behavior, and records
passing adversarial scenario evidence. Until then, a version tag is not a
published release.

Remote publication, tags, credentials, and production mutation require
separate explicit authority.

## Verify published artifacts

These commands apply only to releases produced after publication is explicitly
re-enabled. Download release assets from a published repository release, then
run:

```sh
sha256sum --check SHA256SUMS
cosign verify-blob --bundle server_linux_amd64.sigstore.json \
  --certificate-identity-regexp 'https://github.com/.+/.github/workflows/release.yml@refs/tags/v.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  server_linux_amd64
cosign verify-attestation --type slsaprovenance \
  --certificate-identity-regexp 'https://github.com/.+/.github/workflows/release.yml@refs/tags/v.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  IMAGE@sha256:DIGEST
cosign verify \
  --certificate-identity-regexp 'https://github.com/.+/.github/workflows/release.yml@refs/tags/v.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  IMAGE@sha256:DIGEST
```

Inspect `release.spdx.json` and registry attestations before deployment. Pin
the verified image digest, not a mutable tag.
