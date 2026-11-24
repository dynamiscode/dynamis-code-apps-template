---
name: setup-generated-app
description: Set up a project generated from the Dynamis Code template. Maps the template code that must change (identity, the product resources, the demo seed, unselected profiles), the recommended changes, and the checks that prove the app still works. Use right after template-init generates an application, or when asked what to change in a generated app.
---

# Set Up a Generated Application

This skill maps what a generated application must change, what it should
change, and what to keep. It ships in the generated app's `.agents/skills/`.
It does not decide product behavior: product purpose and domain rules stay
with the team that owns the app.

Read [template lifecycle](../../../docs/template-lifecycle.md) first.

## Outcome

Each template-owned identity value, sample, demo, and fake is replaced,
removed, or kept on purpose, and the verification gates pass in the generated
repository. Report each requirement as `passed`, `failed`, `blocked`, or `not
applicable`, the same statuses that
[verify-template-conformance](../verify-template-conformance/SKILL.md) uses.

Generated applications retain [UX standards](../../../docs/ux.md) and the
[browser review gate](../../../docs/ux-review.md). Set product context and
visual identity for the application; keep the task and accessibility criteria.

## Before you start

1. Confirm the target is a generated app: `template.lock` exists at the root.
   If it does not, stop. This skill maps a generated app, not the template
   repository.
2. Read `template.lock` for the selected profiles. Core and Identity are always
   present. Agent and Files are present only when listed.
3. Run `git status` and keep unrelated work. Land each change below as its own
   reviewed commit or PR.

## Steps

1. **Verify identity (must).** The generator rewrites the module path, name,
   slug, repository and security URLs, maintainer, and copyright.
   1. Run `go build ./...` with your module path.
   2. Run `git grep -n -E "dynamiscode/|davidlondono|Dynamis Code|apps-template" -- ':!CHANGELOG.md'`.
      Each remaining hit is either a change to make or a deliberate keep.
2. **Set up agent docs (must).** Follow the
   [lifecycle steps](../../../docs/template-lifecycle.md#set-up-agent-docs-in-a-generated-application).
   1. Change the tracker repository in `AGENTS.md` and
      `docs/agents/issue-tracker.md` to your repository. The generator does not
      set it.
   2. Create the five triage labels: `needs-triage`, `needs-info`,
      `ready-for-agent`, `ready-for-human`, `wontfix`.
   3. Add your product terms to the app-owned part of `GLOSSARY.md`.
   4. Commit `AGENTS.md`, `docs/agents/`, and `GLOSSARY.md`.
3. **Define your product resource (must).** Add your feature through the shared application services and update routes, contracts, tests, navigation, and documentation together.

4. **Fix the verify gate (must).** `make verify` runs `template-smoke`, which
   runs `go run ./cmd/template-init`. The generator excludes `cmd/template-init`,
   so that step cannot run in a generated app. Remove `template-smoke` from the
   `verify:` line and from the `.PHONY:` line in `Makefile`.
5. **Review the demo seed (recommended).** `cmd/demo/` creates a demo workspace and owner. With Clients selected, it also creates a `Lead source` single-choice field with `Social media`, `Referral`, `Walk-in`, `Phone`, and `Website` options. Set up the same field in your workspace if useful; adjust options for your business. Remove the demo unless needed.
6. **Remove files for unselected profiles (recommended).** The generator drops
   code for unselected profiles but still copies their docs and models.
   1. Without Agent: delete `docs/mcp.md` and `docs/cli.md`, then remove their
      rows and links from `docs/README.md`.
   2. Without Files: delete `verification/files/`.
   3. Run `make doc-links`.
7. **Rewrite product docs (recommended).**
   1. `README.md`: replace the purpose text. After step 3, delete the
      "Replace the sample feature" section.
   2. `CHANGELOG.md`: replace the template history with your own.
   3. `.github/CODEOWNERS`: replace the default owner with a verified maintainer
      or team. The file's comment asks for this.
   4. `docs/capabilities.md`: keep evidence that still holds and remove rows for
      features you removed.
8. **Check the release workflow (verify).** `.github/workflows/release.yml`
   currently runs validation only on `v*.*.*` tags; it does not publish
   binaries, images, artifacts, or GitHub releases. Review `docs/release.md`
   before defining a release process for the generated app.
9. **Verify (must).** Run these in order and report each result:
   1. `make verify`, after step 4.
   2. `npm ci && make accessibility-smoke`. It checks the routes currently supported.
   3. `make webmcp-smoke`.
   4. `make docker-smoke`. It checks the workspace before and after restart.
   5. For data changes, set `POSTGRES_TEST_URL` and run `make test-postgres`.
   6. For OpenAPI changes, `make generate-check` must show no drift.

## Retired Items feature

Items and public links were removed by migration 000030. Earlier migrations and decision records are historical; never edit applied migrations. Add new product resources with forward migrations.

## Map: demo, fakes, and examples

| Path | Kind | Action |
|---|---|---|
| `cmd/demo/` | Demo seed | Delete unless you want demo data (step 5) |
| `cmd/preview/` and `internal/web/screenshot_fixture.go` | Synthetic browser fixtures | Adapt or remove for your app; check with `go test ./cmd/preview` |
| `internal/files/filestest/s3.go` | Fake S3 server for tests | Keep. Check that only `_test.go` files import it: `git grep -l "files/filestest" -- ':!*_test.go'` prints nothing |
| `verification/invitations/`, `verification/jobs/` | TLA+ models of core behavior | Keep while the feature they model exists |
| `verification/files/` | TLA+ model of the Files profile | Delete without the Files profile (step 6) |

## Keep: template machinery

- `template.lock`: provenance. Do not edit it by hand.
- `Makefile` gates (after step 4), `scripts/check-*`, `.githooks/`, and
  `.gitleaks.toml`: keep. They gate the application.

## Keep this map current

Mocks, examples, samples, demos, fakes, and fixtures change often in the
template. When you add, change, or remove one, update this skill in the same
change:

1. Add or update its row in a map above, with an action (must or recommended)
   and the command that checks it.
2. Update the discovery command if the new code uses other names.
3. Edit only this file. `.claude/skills` links to `.agents/skills`, so Claude
   Code reads the change with no copy step.
4. Run `make doc-links` and `go test ./internal/conformance/...`.

[development](../../../docs/development.md#verification-checklist) repeats this
rule for contributors.
