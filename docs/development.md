# Development

## Setup

```sh
make setup
set -a; . ./.env; set +a
go run ./cmd/server
```

Running `make setup` configures `.githooks` (enforcing `gofmt` on staged Go files, plus `go vet` and generate drift checks on commit, and `make ci-local` on push) and installs dependencies. Refresh only the hook path with `make setup-hooks`. The `commit-msg` hook rejects AI attribution trailers (`Co-Authored-By: Claude` and `Generated with Claude Code`), and `make lint` tests that hook via `scripts/test-commit-msg-hook.sh`.

`make ci-local` runs the CI checks that work without Docker, Postgres, or browser smoke (`lint`, `test-unit`, `race`, `deps-check`, `generate-check`, `build`, `secret-check`); the pre-push hook runs it. CI-only targets are listed in the Makefile comment above `ci-local`.
`make setup` also enables git rerere for this repo (`rerere.enabled`), which records conflict resolutions and replays them when the same conflict recurs, such as when rebasing stacked branches.

Use Go 1.26 or newer; the module records the development toolchain. Use an
isolated PostgreSQL database through `POSTGRES_TEST_URL` (for example,
`postgres://postgres@127.0.0.1:5432/dynamis_code_test?sslmode=disable`) for
PostgreSQL compatibility tests across schema migrations, leasing locks, and
queue claim transactions. `make test-postgres` runs the identity Postgres tests against a fresh container on CI's pinned version, because those tests require a fresh database per run (see the target in the [Makefile](../Makefile)).

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
   during iteration to maintain fast feedback loops, then the applicable
   verification ladder before committing.

Ordinary features stay vertical: application behavior first, transport
adapters second. Reuse shared authorization, Problem Details, audit,
telemetry, and rendering code. Add no optional capability until its trigger in
[capabilities](capabilities.md) is accepted and recorded in a decision.

Localization changes keep embedded English and Spanish catalogs in exact key
parity, use named interpolation only for trusted catalog messages, preserve
escaped user content, and test browser precedence separately from stable
English REST/CLI/MCP contracts. Add plural support only when the first
count-dependent product message requires it.

## Feature completeness

For every feature, record its shared application behavior, authorization
boundary, and applicable surfaces before implementation:

- browser: routes, templates, CSRF, accessible controls, and ordinary-form
  fallback;
- browser-agent (WebMCP): decide whether assistance helps, define one-purpose
  bounded schemas, exclude secrets and hidden security fields, preserve
  ordinary-form fallback, and test supported and unsupported browsers;
- REST/OpenAPI: bearer authorization, generated contract, limits, errors, and
  response redaction;
- CLI/MCP: only when automation needs the capability, using shared use cases;
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
2. Watch the checks until they finish, and stop at the first failure:
   ```sh
   gh pr checks <pr-number> --watch --fail-fast
   ```
3. When merging via CLI, enable auto-merge so squashing occurs automatically
   once all required checks succeed:
   ```sh
   gh pr merge <pr-number> --squash --delete-branch --auto
   ```
4. In concurrent worktree workflows, merge conflicts frequently cluster in
   `CHANGELOG.md` under `## Unreleased`. When rebasing or merging `origin/main`,
   preserve all concurrent unreleased entries by keeping both sides of the
   conflict markers, ensuring no entries are dropped or duplicated.
5. Rebase onto `origin/main` before running `make verify`. A branch cut from an
   older `main` can miss Makefile steps that CI runs, so a local pass proves less
   than it appears to.

## Checks

```sh
make lint
make verify
npm ci && make accessibility-smoke
make webmcp-smoke
make docker-smoke
```

`make verify` checks formatting, tests, vet, race behavior, module checks,
workflow syntax and action pins, generated OpenAPI drift, command builds, and
generation of a clean application. During active iteration, run package-focused
tests (such as `go test ./internal/web/...`) to avoid the integration and
template-smoke overhead; run full `go test ./...` and `make verify` once the change
is bounded. Run `node --check internal/web/assets/*.js` when modifying browser
scripts. Run `make secret-check`, `make vuln-check`,
and `make fuzz-smoke` for the source security gates, plus the full suite with
`POSTGRES_TEST_URL` for data or release changes. CI also runs CodeQL for Go and
JavaScript/TypeScript.

### Verification checklist

Prior to PR creation, verify changes with the following checklist:
- Run `make lint` (runs `fmt-check` and `go vet`) so mechanical formatting failures are caught before pushing.
- Run focused tests and `go test ./...` across affected packages.
- Run `make verify` before submitting changes for review.
- When adding, changing, or removing a mock, example, sample, demo, or fake, update the map in `.agents/skills/setup-generated-app/SKILL.md` and run `make doc-links`.
- Run `npm ci && make accessibility-smoke`, `make webmcp-smoke`, `make ux-smoke`, and `make docker-smoke` when interface or container delivery are affected. `make ux-smoke` drives the form patterns in headless Chrome against a fresh server (set `UX_PORT` if 58093 is taken).
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

## Replace the sample feature

The item feature is the executable reference slice. Replace or remove it only
as one reviewed vertical change after the replacement passes browser, REST,
CLI, MCP, SSE, and WebMCP fallback smoke. Update routes, shared use cases,
OpenAPI, migrations, tests, navigation, docs, and capability evidence
together; do not leave a partially disconnected sample.

Contribution and security-reporting rules live in
[CONTRIBUTING.md](../CONTRIBUTING.md) and [SECURITY.md](../SECURITY.md).
