//go:build browser

package browsertest

import (
	"encoding/json"
	"testing"
)

// TestContrastDebug dumps axe color-contrast details for the CI-failing pages.
func TestContrastDebug(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	navigate(t, tab, workspacePath+"/settings/tokens")
	waitVisible(t, tab, "#token-name")
	eval[any](t, tab, axeSource)
	res := eval[json.RawMessage](t, tab, `new Promise((done) => {
		axe.run(document).then((r) => {
			const out = [];
			for (const v of r.violations) {
				if (v.id !== "color-contrast") continue;
				for (const n of v.nodes) {
					out.push({ target: n.target, any: n.any.map((a) => a.data) });
				}
			}
			done(JSON.stringify(out));
		});
	})`, awaitPromise)
	t.Logf("contrast debug: %s", string(res))
}
