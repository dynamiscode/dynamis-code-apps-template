# Development Workflow During Construction

## Start a task

1. Read `AGENTS.md` and inspect `git status`.
2. Select the first incomplete phase in `PLAN.md` unless the user names a
   smaller task inside that phase.
3. Read the phase brief and only its linked standard sections.
4. Load a repository-local skill only for a matching recurring workflow.
5. Define the observable outcome and smallest verification before editing.

## Work boundaries

- Keep one task independently reviewable.
- Preserve unrelated and untracked files.
- Match the architecture established by accepted decisions.
- Add no optional capability until `docs/capabilities.md` records that its
  trigger is met and `PLAN.md` assigns it to a phase.
- Do not create final documentation for behavior that is still planned.
- Update capability evidence when behavior becomes real.
- For every feature, record applicable browser, REST/OpenAPI, CLI/MCP,
  browser-agent (WebMCP), realtime, and operations/data surfaces. For WebMCP,
  decide whether a browser-agent surface is useful, define one-purpose
  bounded schemas, exclude secrets and hidden security fields, preserve
  ordinary form fallback, and test supported and unsupported browsers. Record
  a reason for each omitted surface instead of leaving the decision implicit.

## Verification by maturity

| Repository state | Required checks |
|---|---|
| Documentation preparation | Relative links, skill validation, standards coverage, `git diff --check` |
| Go module exists | Focused tests, `go test ./...`, `go vet ./...` |
| Concurrent behavior exists or phase closes | `go test -race ./...` |
| OpenAPI generation exists | `go generate ./api`, then verify no unexplained generated diff |
| Both databases exist | Real SQLite tests and isolated PostgreSQL migration/repository tests |
| Containers exist | Image build and `make docker-smoke` |
| Stable release exists | Vulnerability, SBOM, provenance, signature, checksum, accessibility, restore, and conformance evidence |

Read command output. Record environment blockers separately from failures.
Version-only checks do not replace a runnable smoke path.

## Documentation updates

While building, update the phase brief and capability evidence. Once behavior
exists, create or update the canonical destination listed in `STANDARDS.md`
Section 12. Link to contracts such as OpenAPI and migrations; do not copy them.

Before deleting `STANDARDS.md`, replace this construction workflow with the
verified local setup, generation, test, contribution, and extension workflow.
