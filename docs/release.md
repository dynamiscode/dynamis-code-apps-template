# Release

Template versions use semantic versioning. `VERSION`, the annotated `vX.Y.Z`
tag, and the changelog entry must match.

## Publish

1. Update `VERSION` and `CHANGELOG.md`; run `make verify`, accessibility,
   `make webmcp-smoke`, PostgreSQL, image smoke, restore, and vulnerability
   gates. WebMCP native assertions are conditional; unsupported browsers must
   pass the ordinary-browser fallback check.
2. Review the complete diff and create an annotated tag matching
   `v$(cat VERSION)`.
3. Push the tag only with release authority. The release workflow builds
   Linux and macOS binaries, an SPDX SBOM, SHA-256 checksums, keyless Sigstore
   bundles, build provenance, and a signed GHCR image with registry SBOM and
   provenance attestations.

Remote publication, tags, credentials, and production mutation require
separate explicit authority.

## Verify artifacts

Download release assets from the repository release, then run:

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
