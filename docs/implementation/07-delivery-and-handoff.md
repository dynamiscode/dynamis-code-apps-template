# Phase 07: Delivery and Handoff

## Goal

Produce a reproducible, verifiable template release and transfer every
temporary standard obligation to implemented sources of truth.

## Standards covered

`STANDARDS.md` Sections 6 for containers, CI, releases, and documentation;
Sections 10-13 for lifecycle, agent context, sources, tests, and acceptance.

## Prerequisites

Phases 01-06 are complete with linked evidence and no unresolved mandatory
behavior.

## Required outcomes

- Build a reproducible minimal non-root runtime image with immutable metadata,
  documented ports, storage, health, shutdown, TLS boundaries, and resource
  expectations.
- Provide one clean-checkout container command for persistent SQLite and an
  optional PostgreSQL Compose overlay; multiple instances require PostgreSQL
  and external traffic distribution.
- Make CI verify formatting, static analysis, tests, generated drift, both
  databases, container build, smoke behavior, and applicable accessibility and
  restore checks. Add a `webmcp-smoke` fallback check that passes when the
  browser lacks the optional API and runs native assertions when supported.
- Add source/container vulnerability gates, dependency monitoring, SPDX or
  CycloneDX SBOMs, verifiable provenance, signatures, SHA-256 checksums, and
  documented artifact verification.
- Define semantic template releases and generated `template.lock` contents:
  source, version, commit, generation time, and selected profiles.
- Define reviewable template updates that never overwrite customization,
  advance the lock only after verification, and document conflicts and
  migrations.
- Replace preparation README and construction docs with accurate quick start,
  architecture, configuration, deployment, operations, authentication, API,
  MCP, CLI, development, capabilities, security, contribution, and changelog
  documents as applicable.
- Update every repository-local skill to permanent paths and verified commands.
- Preserve the sample feature until browser/API/CLI/MCP/SSE smoke passes; then
  document its reviewed replacement/removal path.
- Delete `STANDARDS.md` only after the deletion gate below passes.

## Evidence

- `go test ./...`, `go vet ./...`, `go test -race ./...`, reproducible
  `go generate ./api`, both database checks, image build, and
  `make docker-smoke` pass.
- Live smoke proves browser login, token creation, authenticated REST, CLI over
  REST, MCP initialize/tool call, and SSE notification.
- WebMCP release evidence proves feature detection, safe schemas, fallback,
  security headers, and no secret or hidden-field exposure.
- Release evidence includes vulnerability results, SBOM, provenance,
  signature, checksums, restore verification, and accessibility review.
- Relative documentation links and skill packages validate with no temporary
  or broken source references.
- `docs/capabilities.md` links evidence for every mandatory group.

## STANDARDS.md deletion gate

- `Core`, `Identity`, and `Agent` have no pending groups.
- Every applicable requirement has one permanent owner and evidence link.
- No final document describes speculative behavior as implemented.
- Agent routing and all skills reference permanent sources of truth.
- All template acceptance checks pass and the released template emits a valid
  `template.lock`.
- The deletion is an explicit reviewed change after the conditions above, not
  part of an unrelated cleanup.

## Exclusions

No deployment to production, registry publication, secret creation, or remote
release mutation without a separate explicit user request and dry-run where
supported.

## Completion gate

A clean checkout can be built, verified, run, and generated from documented
commands; all release evidence is available; temporary standards can be
deleted without losing instructions.
