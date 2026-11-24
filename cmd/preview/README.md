# Local browser preview

Run a synthetic workspace page without a database, login, or external services:

```sh
go run ./cmd/preview -scenario populated
```

The command prints its localhost URL and chooses a free port. Stop it with
Ctrl-C when finished. Each process owns independent in-memory data. Restart it
to reset the scenario. Available scenarios are `empty`, `populated`, and
`error`; the last simulates a workspace creation failure.

The root URL shows the real workspace selection page. Creating a workspace
changes only this process's memory. `/components` lists available component
fixtures, including `/components/alert`. Other routes return a preview-specific
404 until a fixture is added. The workspace links are not yet implemented.

## Agent workflow

When inspecting or changing a browser journey, use this preview when it makes
the required page, component, or state easier to reach. If the fixture does
not exist, extend the preview as part of the task; do not require every feature
to have a fixture in advance. Reuse the real templ page or component and its
assets. Add a small synthetic-data adapter in `internal/web` only when an
unexported page needs one. Keep preview-specific routes, scenario state, and
fake side effects in `cmd/preview/`.

1. Trace the real page, its inputs, handlers, and relevant states. Select the
   smallest representative fixtures, including empty and failure states when
   relevant. Use synthetic data only.
2. Add named startup scenarios for server state and direct fixture URLs for
   component states. Fake the actions needed for the journey in memory; expose
   their results in the browser. Return a clear 404 for unsupported routes.
3. Run a separate preview process per agent. Open its printed URL with the
   approved browser controls, exercise the relevant actions, and inspect the
   rendered result at suitable viewport sizes. Stop only the process you
   started. Restart it to reset state.
4. Add focused regression coverage for non-trivial fixture behavior and run
   `go test ./cmd/preview`. Report the scenario, URL path, actions, results,
   and which behavior remains unimplemented.

Fixtures may be committed when useful to later work. Never add a preview flag,
authentication bypass, mock route, or fake service to the production server.
Keep the preview listener loopback-only with an automatically assigned port;
preserve its host and origin checks. Never use real credentials, production
data, durable storage, or external services in a preview fixture. A shared
rendering adapter must contain synthetic inputs only and must not change the
production page's authorization or behavior. The production container builds
`cmd/server`, not `cmd/preview`.

Preview does not validate production authentication, authorization, storage,
or real service integrations. It complements, but does not replace, the real
application journey and [UX review](../../docs/ux-review.md) when required.
