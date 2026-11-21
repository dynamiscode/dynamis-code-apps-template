//go:build browser

package browsertest

import (
	"encoding/json"
	"testing"
)

// TestContrastDebug dumps axe color-contrast details and environment for CI diagnosis.
func TestContrastDebug(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	navigate(t, tab, workspacePath+"/settings/tokens")
	waitVisible(t, tab, "#token-name")
	eval[any](t, tab, axeSource)
	type result struct {
		Violations []json.RawMessage `json:"violations"`
		Contrast   json.RawMessage   `json:"contrast"`
		Env        string            `json:"env"`
	}
	res := eval[result](t, tab, `new Promise((done) => {
		axe.run(document).then((r) => {
			const cc = r.violations.filter((v) => v.id === "color-contrast");
			const nodes = [];
			for (const v of cc) for (const n of v.nodes) nodes.push({ target: n.target, any: n.any.map((a) => a.data) });
			const label = document.querySelector(".label");
			const env = {
				ua: navigator.userAgent,
				dark: matchMedia("(prefers-color-scheme: dark)").matches,
				theme: document.documentElement.getAttribute("data-theme"),
				labelColor: label ? getComputedStyle(label).color : null,
				labelBg: label ? getComputedStyle(label).backgroundColor : null,
				sheets: [...document.styleSheets].map((s) => s.href),
			};
			done({ violations: cc.map((v) => v.id), contrast: JSON.stringify(nodes), env: JSON.stringify(env) });
		}).catch((error) => done({ env: JSON.stringify({ err: error.message }) }));
	})`, awaitPromise)
	violations, _ := json.Marshal(res.Violations)
	t.Logf("VIOLATIONS: %s", violations)
	t.Logf("CONTRAST: %s", string(res.Contrast))
	t.Logf("ENV: %s", res.Env)
	t.Fatalf("debug dump complete")
}
