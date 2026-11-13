---
name: setup-generated-app
description: Set up a project generated from the Dynamis Code template. Maps the template code that must change (identity, the sample items feature, the demo seed, unselected profiles), the recommended changes, and the checks that prove the app still works. Use right after template-init generates an application, or when asked what to change in a generated app.
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
3. **Decide the sample items feature (must).** Replace it with your first real
   resource, or remove it. Use the [sample map](#map-sample-items-feature). The
   change must cover routes, use cases, OpenAPI, migrations, tests, navigation,
   docs, and smoke checks together, as
   [development](../../../docs/development.md#replace-the-sample-feature) requires.
4. **Fix the verify gate (must).** `make verify` runs `template-smoke`, which
   runs `go run ./cmd/template-init`. The generator excludes `cmd/template-init`,
   so that step cannot run in a generated app. Remove `template-smoke` from the
   `verify:` line and from the `.PHONY:` line in `Makefile`.
5. **Remove the demo seed (recommended).** `cmd/demo/` inserts a sample item
   titled "Replace this sample item" and needs `DEMO_OWNER_PASSWORD`. Delete it
   unless you want demo data. Then remove the "Local demo seed" section of
   `docs/cli.md` and the demo paragraph in `README.md`.
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
   publishes on `v*.*.*` tags and runs `make secret-check` and `make vuln-check`.
   Confirm it matches your release process.
9. **Verify (must).** Run these in order and report each result:
   1. `make verify`, after step 4.
   2. `npm ci && make accessibility-smoke`. It opens the items page, so it fails
      until you replace or remove that page.
   3. `make webmcp-smoke`.
   4. `make docker-smoke`. It posts to the items endpoint.
   5. For data changes, set `POSTGRES_TEST_URL` and run `make test-postgres`.
   6. For OpenAPI changes, `make generate-check` must show no drift.

## Map: sample items feature

Items is the reference vertical slice. The file list drifts, so search for
current references first:

```sh
git grep -l -E "items|Items|item_|Item" -- internal cmd api scripts docs README.md verification
```

Decision records and `CHANGELOG.md` are history. Do not rewrite them.

| Area | Where | Action |
|---|---|---|
| Use cases | `internal/items/` | Replace with your resource, or delete |
| Wiring | `internal/bootstrap/app.go`, `internal/bootstrap/agent.go` | Remove or replace the service and its arguments |
| Web | `internal/web/templates/items.html`, `item-list.html`, `internal/web/handler.go`, `internal/web/assets/app.js`, `app.css` | Replace routes, templates, and script |
| REST | `internal/httpapi/` (for example `item_path_test.go`), `api/openapi.json` | Update routes and schemas, then `go generate ./api` |
| Schema | `internal/platform/database/migrations/000003_items_http.sql`, `000004_item_events.sql`, `000009_nullable_item_creator.sql` | Do not edit applied migrations. Add a forward migration |
| Public sharing | `000011_public_sharing.sql` (`public_links.item_id` references `items`), `internal/sharing/`, `internal/web/sharing.go` | Retarget sharing to your resource, or remove it with the items |
| Webhooks | `item.created`, `item.updated`, `item.deleted` in `api/openapi.json` and `internal/webhooks/` tests | Rename to your events |
| Export | `internal/portability/`, `WorkspaceExport` in `api/openapi.json` | Update the export format |
| Short IDs | `internal/platform/id/short.go` (`TypeItem`, `TypeItemEvent`) | Keep registered codes while stored IDs exist. See [ADR 0009](../../../docs/decisions/0009-short-id-codec.md) |
| Agent surfaces | `internal/mcpserver/`, `internal/appctl/`, `internal/bootstrap/agent_smoke_test.go` | Update tools, commands, and smoke tests (Agent only) |
| Smoke checks | `scripts/accessibility.mjs`, `scripts/webmcp.mjs`, `scripts/docker-smoke.sh` | Point at the replacement page and endpoint |
| Strings | `internal/i18n/locales/en.json`, `es.json` | Replace item strings |
| Behavior docs | `docs/api.md`, `web.md`, `mcp.md`, `cli.md`, `data-lifecycle.md`, `capabilities.md`, `GLOSSARY.md` ("Item") | Match the code. Never describe behavior the code lacks |
| Demo seed | `cmd/demo/` | Delete or repoint (step 5) |

## Map: demo, fakes, and examples

| Path | Kind | Action |
|---|---|---|
| `cmd/demo/` | Demo seed | Delete unless you want demo data (step 5) |
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
