# Action guardrail review (#658)

Baseline: integration `455c15b`; `origin/main` was an ancestor at review time.
The source inventory scans all `.templ` files under `internal/web`, including
catalog atoms and feature-gated templates. Four shared shell controls used
`btn-ghost`: theme selection, both mobile drawer triggers, and sign out. They
now use `btn-outline`, retaining their native button semantics and behavior.

`make web-conformance` uses the templ parser and a Go type pass to check
declared controls and supported render paths. It rejects conditional-only
boundaries, invisible-border and ghost/link overrides, and unsupported
render or spread sources. Static exceptions are audited in
`scripts/actioncheck/exceptions.json` by exact file, line and opening tag.
The runtime scanner does not inherit those source exceptions. Compose
exceptional controls using statically checked template children, not opaque
component slots. Those slots remain buffered as described in [Web](web.md).

The app's JavaScript toast dismiss, draft discard, wizard Back and lazy
Retry buttons are outlined. Wizard Next is primary and validation-summary
links keep their native anchors with compact borders. Runtime JavaScript
can construct or restyle actions through arbitrary code, so the guardrail
rejects unreviewed executable browser assets rather than infer safety from
syntax. `scripts/reviewed-browser-scripts.json` binds each reviewed script to
its path, byte size and SHA-256 digest. Script changes require a fresh action
review, relevant automated and browser checks, and an explicit digest update
with its review reason. Do not refresh digests automatically. The checker
rejects new, changed, stale and linked assets and reads only files whose
size matches the reviewed entry. It does not check CSS in arbitrary scripts.
Templ script tags must select a reviewed asset or match an exact inline
snippet recorded by path and digest. Alpine expressions likewise require a
reviewed literal binding and cannot be supplied through attribute spreads.

Existing opaque component slots are constrained in source by template path,
expression, parent element chain and occurrence count. This prevents adding or
moving a buffered slot silently. It does not establish parent-independent
safety of arbitrary component return values. Use statically checked children
for new composition.

## Browser response boundary

`internal/web/handler.go` registers 150 browser routes, counting optional
profiles and feature branches. Bootstrap routes `/api/`, `/health/` and agent
traffic separately before mounting the browser router. Classify by response
capability, not HTTP method. Mutations can render HTML validation errors, and
list routes can render either complete pages or fragments.

Login, setup, password reset, MFA challenge/completion, email verification,
OIDC, logout, notification pages and mutations, and account pages register through
`serveAuthPage`. Their callbacks receive no writer and return only a
typed outcome, with no arbitrary template producer. The adapter retains cookie
settings, locale middleware, native redirects and verified renderer selection.
Other browser routes still use the legacy writer. To avoid rewriting their
request and authorization behavior, the conformance command checks
`scripts/reviewed-go-output.json` before relying on the reviewed browser ingress,
rendering, transport and build inputs. The manifest records exact paths and
SHA-256 digests. It covers Go and templ source, build-tagged and committed
generated source, native build inputs, Go dependency files, the supported
build/check scripts, generator metadata, and Go embed payloads. Test-only Go
files are not release-app inputs.

An unreviewed addition, edit, rename or deletion fails. Before updating the
manifest, review all reported changes and their callers, registration,
response framing, component provenance, and consumers for every affected
profile. Then run `go run ./scripts/update-go-output-manifest --reviewed`,
inspect the manifest diff, and rerun full and generated-profile conformance.
Do not use the update command to clear a failure without review. This is a
deliberately broad maintenance tradeoff rather than a claim that arbitrary Go
writer effects can be inferred safely. The manifest and the standard pinned Go
toolchain are trust roots. An active workspace, Go flags or experiments are
outside the verified build; module cache verification supplements the pinned
`go.mod` and `go.sum`.

`template-init` checks the released source manifest and build environment
before copying. It derives a new manifest only after the known profile
transformations, code generation, and dependency pruning succeed. Application
metadata is escaped in supported data contexts and unsupported executable
substitutions fail. Generated applications need the same explicit review for
later changes.

Uploaded file bytes and presigned storage responses are user data outside the
app-authored action-markup contract. File rows and menus navigate to native
download links. Local file delivery sets attachment disposition and remote
delivery retains its existing storage redirect. The app's HTMX response hook
refuses attachment bodies and final external responses before parsing, even
when they advertise HTML. SSE uses EventSource and API data uses its own
consumers. Standard `net/http.Redirect` may emit a small escaped fallback HTML
body for native GET navigation; it is not application-authored action markup.
The reviewed baseline includes preview fixture HTML and excludes arbitrary
external content. These boundaries do not certify arbitrary same-origin HTML
output.

## Real-app Playwright review

On `da09dd2` plus the guardrail-only changes, a disposable SQLite app and
isolated Playwright contexts used synthetic owner and member accounts. Captures
were inspected and contain synthetic data only.

- Owner, 1280 × 900 light: signed in, opened workspace and Clients; document
  width stayed 1280 px. [Clients](reviews/issue-658/owner-desktop-light.png).
- Owner, 390 × 844 dark: selected Dark, focused the navigation trigger and
  pressed Enter; workspace navigation opened and document width stayed 390 px.
  [Open drawer](reviews/issue-658/owner-mobile-dark.png).
- Member, 390 × 844 light: signed in and opened Clients. Share action was
  absent and direct intake settings returned HTTP 403; document width stayed
  390 px. [Clients](reviews/issue-658/member-mobile-light.png).

## Password reveal follow-up

On `ad51647` with the password reveal fix, a new disposable SQLite server was
verified by its process ID and listening socket before Playwright connected.
At 390 × 844, Spanish login showed a bordered reveal control. Enter changed the
password field to text, `aria-pressed` to true, and its accessible label to
“Ocultar contraseña”; document width remained 390 px. [Spanish login](reviews/issue-658/password-reveal-es-mobile.png).

With JavaScript disabled, owner login succeeded through the ordinary form and
reached Workspaces; no reveal control was rendered. With default OIDC disabled
and no providers configured, login had no OIDC links and an unknown provider
route returned 404. This does not isolate flag behavior from provider setup.
`TestFormUXPatterns` failed before the fix on the transparent reveal border and
passed afterward; it also checks the toggle and accessible state.

## Review gaps

Enabled feature-flag states, other localized journeys, full keyboard traversal,
and other roles and states remain unreviewed. The JavaScript control migration
has automated checks but is not covered by the older screenshots above.
