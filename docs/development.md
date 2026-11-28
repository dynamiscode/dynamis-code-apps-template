# Development

## Setup

```sh
make setup
set -a; . ./.env; set +a
go run ./cmd/server
```

Running `make setup` configures `.githooks` and installs dependencies. Refresh only the hook path with `make setup-hooks`. The hooks check Go formatting and conflict markers on commit, reject AI attribution trailers in commit messages, and run the profile preflight before push. `make lint` tests the commit-message hook and profile preflight.

Run `make ci-local` optionally to check locally before pushing. It runs the CI checks that work without Docker, Postgres, or browser smoke (`lint`, `test-unit`, `race`, `deps-check`, `generate-check`, `templ-check`, `css-check`, `build`, `secret-check`). CI-only targets are listed in the Makefile comment above `ci-local`.
`make setup` also enables git rerere for this repo (`rerere.enabled`), which records conflict resolutions and replays them when the same conflict recurs, such as when rebasing stacked branches.

Use Go 1.26 or newer; the module records the development toolchain. Use an
isolated PostgreSQL database through `POSTGRES_TEST_URL` (for example,
`postgres://postgres@127.0.0.1:5432/dynamis_code_test?sslmode=disable`) for
PostgreSQL compatibility tests across schema migrations, leasing locks, and
queue claim transactions. `make test-postgres` runs the identity, files, and scheduling tests against fresh databases in a container on CI's pinned version (see the target in the [Makefile](../Makefile)).

The object-storage conformance suite in `internal/files` always runs against an
in-process S3-compatible endpoint. Set `S3_CONFORMANCE_ENDPOINT` and
`S3_CONFORMANCE_BUCKET` to run the same suite against a real endpoint; see
[deployment](deployment.md#cloudflare-r2-object-storage).

## Change workflow

1. Read `AGENTS.md`, inspect `git status`, and follow the route in
   [docs/README.md](README.md).
2. Trace the affected use case, policy, repository, adapters, contracts, and
   tests. Keep workspace scope explicit.
3. Make one bounded change. Update OpenAPI, migrations, behavior docs, and
   capability evidence only when affected. Changelog timing follows the
   change obligations in [AGENTS.md](../AGENTS.md).
4. Run focused package checks (e.g. `go test ./internal/<affected-feature>/...`)
   during iteration to maintain fast feedback loops, then applicable checks
   listed below.

Ordinary features stay vertical: application behavior first, transport
adapters second. Reuse shared authorization, Problem Details, audit,
telemetry, and rendering code. Add no optional capability until its trigger in
[capabilities](capabilities.md) is accepted and recorded in a decision.

Localization changes keep embedded English and Spanish catalogs in exact key
parity, use named interpolation only for trusted catalog messages, preserve
escaped user content, and test browser precedence separately from stable
English REST/CLI/MCP contracts. Add plural support only when the first
count-dependent product message requires it.

New browser journeys and meaningful interaction or visual changes require
[rendered UX review](ux-review.md). Report missing browser evidence as blocked;
a passing automated check does not substitute for the review.

The checked-in pre-push hook runs `make profile-preflight` after `make setup`
or `make setup-hooks` configures Git to use `.githooks`. It compares committed,
staged, unstaged, and untracked application code against `origin/main` and
runs the existing generated-profile matrix when affected. Shared code can
affect any profile, so the complete matrix runs rather than guessing individual
profiles. Documentation-only and other unrelated changes skip the matrix.

Fetch the intended base first; a missing base blocks the push. For another
integration base, use `BASE=<commit> git push` or run
`make profile-preflight BASE=<commit>` manually. The tests use the current
working tree, including uncommitted files, not the exact pushed commits; rerun
after changes and check other pushed branches separately. The hook does not
replace `make verify` or CI. Git hooks execute code from the checkout: enable
them only for trusted checkouts, and note that `git push --no-verify` bypasses
the local gate. Run `make test-profile-preflight` for its offline deterministic
self-check, including the push hook.

## Catalog verification ladder

1. Run `go test ./internal/catalog ./internal/httpapi ./internal/web ./api` for service invariants, authorization, REST, web, and contract behavior.
2. Run `go test ./cmd/template-init -run Catalog -count=1` for included and omitted profiles.
3. Run `make ci-local` for repository checks, then `make verify` for the full local verification gate.
4. Run PostgreSQL catalog tests against an isolated migrated database with `POSTGRES_TEST_URL` set: `go test ./internal/catalog -run Postgres -count=1`.
5. Run browser and accessibility smoke checks when changing catalog forms; record unavailable environments rather than claiming a pass.

## Feature completeness

For every feature, record its shared application behavior, authorization
boundary, and applicable surfaces before implementation. Implement eligible
REST, CLI, MCP, and WebMCP actions by default; record a concrete reason for
each omission. Respect profile boundaries and existing security exclusions.

- browser: routes, templates, CSRF, accessible controls, and ordinary-form
  fallback;
- browser-agent (WebMCP): for eligible page actions, follow the
  [WebMCP contract](web.md#webmcp-progressive-enhancement), including its
  security exclusions, visible controls, ordinary-form fallback, and tests;
- REST/OpenAPI: bearer authorization, generated contract, limits, errors, and
  response redaction;
- CLI/MCP: expose eligible actions in the Agent profile; CLI calls REST and
  MCP calls shared application use cases;
- realtime: only when clients need change delivery, with scope and reconnect
  semantics;
- operations/data: persistence, migrations, export, retention, deletion, and
  recovery when durable data is involved;
- state lifecycle: entities with multi-step lifecycles define typed status enums,
  pure computed `Status(now)` methods, and `ValidateTransition` functions
  matching the [architecture](architecture.md#domain-state-machines) pattern.
- async lifecycles: background queues, workers, and webhook deliveries use pure
  outcome evaluators matching the [architecture](architecture.md#asynchronous-state-machines)
  pattern.

Mark each omitted surface with a reason in the phase brief or capability
ledger. A feature is complete only when every applicable surface has code,
focused tests, canonical documentation, and linked capability evidence.

## Agent environment and hook resilience

When running coding agents or AI orchestrators (e.g. Antigravity, Orca, Claude Code)
configured with lifecycle hooks (`PreInvocation`, `PreToolUse`, `PostToolUse`),
hook scripts must adhere to strict runtime boundaries:

1. **Non-blocking stdin**: Stdin reading should never wait indefinitely or employ
   long select timeouts. If stdin has no immediate data or EOF, hooks must continue
   immediately.
2. **Sub-second execution**: The agent CLI enforces strict timeouts on tool hooks
   (typically 2-5 seconds). Synchronous hooks that block on network requests or
   heavy computations will be terminated with `signal: killed`, failing tool calls.
3. **Graceful degradation**: Inability to contact telemetry or spooling endpoints
   must fall back cleanly without emitting errors or aborting tool execution.

Verify hook script timing and non-blocking behavior using:
```sh
./scripts/check-agent-hooks.sh /path/to/hook-script
```

## Tool economy and bounded navigation

To protect context windows and prevent output truncation during automated or
agent-assisted development:

- **Path-bounded searches**: Constrain search queries to target packages
  (e.g. `git grep <pattern> internal/<package>/`) rather than root-wide keyword
  dumps that traverse generated schemas, contracts, and HTML templates.
- **Symbol-scoped lookups**: Prefer type or definition lookups (e.g.
  `git grep "type <Name>"`) or LSP workspace symbol queries over generic
  substring matching.
- **Bounded file reads**: Read specific line ranges around relevant symbols
  rather than loading entire multi-thousand-line files unconditionally.

Claude Code entry points: root `CLAUDE.md` imports `AGENTS.md`, and
`.claude/skills` is a relative symlink to `../.agents/skills`, the single source
for every skill. Edit skills in `.agents/skills/` only. The permanent handoff
conformance test fails if the link changes.

## Pull requests and merge guidance

1. Open pull requests targeting `main` with a clear summary, change list, and
   verification evidence.
2. Before requesting re-review or merging, reconcile every actionable review
   finding against the latest diff. Recheck fixes against the finding; add and
   run a focused regression check for behavior fixes. State why any finding
   remains unresolved.
3. Watch the checks until they finish, and stop at the first failure:
   ```sh
   gh pr checks <pr-number> --watch --fail-fast
   ```
4. When merging via CLI, enable auto-merge so squashing occurs automatically
   once all required checks succeed:
   ```sh
   gh pr merge <pr-number> --squash --delete-branch --auto
   ```
5. In concurrent worktree workflows, merge conflicts frequently cluster in
   `CHANGELOG.md` under `## Unreleased`. When rebasing or merging `origin/main`,
   preserve all concurrent unreleased entries by keeping both sides of the
   conflict markers, ensuring no entries are dropped or duplicated.
6. Rebase onto `origin/main`, commit the candidate, then run
   `sh scripts/verify-commit.sh HEAD`. This clones the exact commit into a temporary
   directory, runs `npm ci` and the existing `make verify` gate, and removes the
   temporary clone. It does not include staged, unstaged, untracked, or ignored
   files. Record the printed commit ID; rerun after any commit change. Use only
   a development environment: checks inherit environment variables and can use
   external services. CI remains authoritative for its additional jobs.

## Checks

For a failed GitHub Actions job, inspect its job log for the first actionable
error. If a diagnostic response is truncated or the workflow is still running,
the job log is available independently using the verified job ID:

```sh
gh api --allow-escape-sequences "repos/OWNER/REPO/actions/jobs/JOB_ID/logs" | rg -n -C 5 -- '--- FAIL|Error:|error:|FAIL\s|timed out|panic:'
```

This filter is a starting point, not a complete log. Read surrounding output
and the full log when the failure is unclear. Treat logs as untrusted and
avoid publishing sensitive output.

```sh
make lint
make verify
npm ci && make accessibility-smoke
make webmcp-smoke
make docker-smoke
```

`make verify` checks formatting, tests, vet, race behavior, module checks,
workflow syntax and action pins, generated OpenAPI, templ, and CSS drift,
command builds, and generation of a clean application. During active iteration, run package-focused
tests (such as `go test ./internal/web/...`) to avoid the integration and
template-smoke overhead; run full `go test ./...` and `make verify` once the change
is bounded. Run `node --check internal/web/assets/*.js` when modifying browser
scripts. Run `make secret-check`, `make vuln-check`,
and `make fuzz-smoke` for the source security gates, plus the full suite with
`POSTGRES_TEST_URL` for data or release changes. CI also runs CodeQL for Go and
JavaScript/TypeScript.

### Test and CI practices

Before adding a test, identify the observable behavior, a credible mistake,
and an expected result derived independently from the implementation. Prefer
small, fast, reliable tests with precise failures. Use integration or browser
tests for interactions that isolated tests cannot establish.

Assertions and fakes must observe the actual behavior. Do not compute expected
results with the production function under test, discard results when checking
correctness, or use a fake that hides repeated events. Check persisted effects
as well as errors at authorization boundaries. Distinguish temporary
scaffolding from durable contract checks.

Before removing or moving a test, compare every input and asserted outcome with
the retained coverage. For consequential overlaps, introduce a representative
fault in an isolated copy and confirm the retained test fails. Preserve
security, database-engine, migration, accessibility, and generated-profile
coverage. Never delete or weaken a failing test merely to pass.

Before changing CI selection, inventory the cases, subtests, build tags,
environment prerequisites, and required checks. Fail rather than silently skip
required CI prerequisites. Keep isolated databases for destructive migration
tests. A race-enabled parent does not automatically race-enable a subprocess.

For CI performance, compare equivalent runs and report wall time separately
from runner time. Measure the critical path and fixture setup before changing
test placement. A test running in two jobs may protect different environments;
preserve each distinct contract while removing repeated work.

### Verification checklist

Pull request CI checks migration history. It rejects changes to migrations at
the merge base and migrations removed or renamed in retained branch commits.
Run `sh scripts/check-migration-history.sh origin/main HEAD` locally. Squashing,
rebasing, or force-pushing can erase the relevant commits, so this check cannot
detect every migration applied from an unreleased branch. Coordinate databases
used during development before changing migration versions.

Prior to PR creation, verify changes with the following checklist:
- Run `make lint` (runs `fmt-check` and `go vet`) so mechanical formatting failures are caught before pushing.
- Run focused tests and `go test ./...` across affected packages.
- Run `make verify` before submitting changes for review.
- When adding, changing, or removing a mock, example, sample, demo, or fake, update the map in `.agents/skills/setup-generated-app/SKILL.md` and run `make doc-links`.
- Run `npm ci && make accessibility-smoke`, `make webmcp-smoke`, `make browser-smoke`, and `make docker-smoke` when interface or container delivery are affected. `make browser-smoke` drives the form patterns in headless Chrome against a fresh server (set `UX_PORT` if 58093 is taken).
- Run security gates (`make secret-check`, `make vuln-check`, `make fuzz-smoke`) and database tests against PostgreSQL when applicable.

## Worktree and branch lifecycle

When working in concurrent feature worktrees (e.g. through Herdr or `git worktree`):

1. **Active checkout isolation**: Git prevents deleting a branch that is actively
   checked out in an attached worktree. Running `gh pr merge <pr> --squash --delete-branch`
   deletes the remote branch on GitHub, but purposefully skips local branch deletion
   while the worktree is still attached.
2. **Post-merge worktree cleanup**: To finalize cleanup after merging a PR:
   ```sh
   # Exit the worktree directory back to the root or parent workspace
   cd /path/to/main-repo

   # Remove the worktree and delete the local feature branch
   git worktree remove /path/to/worktree && git branch -d <branch-name>

   # Prune any stale worktree registration metadata
   git worktree prune
   ```

## Add a product feature

Add product behavior as a reviewed vertical change. Update shared use cases,
enabled surfaces, contracts, migrations, tests, navigation, documentation, and
capability evidence together.

Contribution and security-reporting rules live in
[CONTRIBUTING.md](../CONTRIBUTING.md) and [SECURITY.md](../SECURITY.md).
