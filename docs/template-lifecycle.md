# Template Lifecycle

## Generate

Generate only from a verified semantic release checkout. `template-init`
requires the release source URL and full commit SHA, refuses symlinks and an
existing output directory, regenerates OpenAPI, and writes a mode-0600
`template.lock` containing:

- template source, semantic version, and commit;
- UTC generation time; and
- selected `Core`, `Identity`, and `Agent` profiles.

The command is documented in the root [README](../README.md). Commit the lock
with the generated application; it contains provenance, not credentials.

## Update without overwriting customization

1. Back up the application and read release notes and migrations between the
   version in `template.lock` and the target release.
2. Generate the target release into a new empty directory with the same name,
   module, source, and selected profiles.
3. Review `git diff --no-index <application> <generated-directory>` and port
   changes through normal reviewed commits. Resolve product-specific conflicts
   explicitly; never copy over the application tree wholesale.
4. Run `make verify`, the relevant database checks, accessibility checks,
   `make webmcp-smoke`, and runtime smoke in the application. WebMCP support
   is optional; its fallback path must remain green when the browser API is
   absent.
5. Replace `template.lock` with the newly generated lock only after every
   applicable check passes.

If a release needs a data migration, follow its release notes and the backup,
restore, and upgrade procedure before advancing the lock. A customized
application may stop following the template by documenting that decision and
removing `template.lock`; no automatic merge path is implied.
