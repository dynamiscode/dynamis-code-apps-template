package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/chromedp/chromedp"
)

// nativeWebMCP is what the page reports about document.modelContext.
type nativeWebMCP struct {
	Supported bool     `json:"supported"`
	Error     string   `json:"error"`
	Names     []string `json:"names"`
	Redacted  bool     `json:"redacted"`
	Schema    bool     `json:"schema"`
}

type exportNativeResult struct {
	Error     string `json:"error"`
	Focused   bool   `json:"focused"`
	Activated bool   `json:"activated"`
	Redacted  bool   `json:"redacted"`
}

// runWebMCP is the Go port of scripts/webmcp.mjs: it checks the ordinary
// browser fallback on the items and export pages, then the native WebMCP tools
// when the browser exposes document.modelContext.
func runWebMCP() error {
	base := os.Getenv("WEBMCP_BASE_URL")
	email := os.Getenv("WEBMCP_EMAIL")
	password := os.Getenv("WEBMCP_PASSWORD")
	if base == "" || email == "" || password == "" {
		return fmt.Errorf("WEBMCP_BASE_URL, WEBMCP_EMAIL, and WEBMCP_PASSWORD are required: %w", errUsage)
	}
	for _, name := range strings.Split(envDefault("WEBMCP_BROWSERS", "chrome"), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := chromeOnly(name); err != nil {
			return err
		}
		if err := runWebMCPChrome(name, base, email, password); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func runWebMCPChrome(label, base, email, password string) error {
	b, stop, err := newBrowser()
	if err != nil {
		return err
	}
	defer stop()

	if err := b.goTo(base + "/login"); err != nil {
		return err
	}
	if err := b.typeText("#email", email); err != nil {
		return err
	}
	if err := b.typeText("#password", password); err != nil {
		return err
	}
	if err := b.click(`button[type="submit"]`); err != nil {
		return err
	}
	if err := b.waitURL(base + "/"); err != nil {
		return err
	}
	if err := b.click(".cards a"); err != nil {
		return err
	}
	if err := b.waitFor("[data-workspace-home]"); err != nil {
		return err
	}
	if err := b.click(`a[href$="/items"]`); err != nil {
		return err
	}
	if err := b.waitFor("#item-list"); err != nil {
		return err
	}

	fallback, err := eval[struct {
		Form   bool `json:"form"`
		Marker bool `json:"marker"`
		Script bool `json:"script"`
	}](b, `(() => ({
		form: Boolean(document.querySelector('form[data-webmcp-actions="item-create"]')),
		marker: Boolean(document.querySelector('[data-webmcp-page="items"]')),
		script: [...document.scripts].some((script) => script.src.endsWith("/assets/alpine-components.js")),
	}))()`)
	if err != nil {
		return err
	}
	if !fallback.Form || !fallback.Marker || !fallback.Script {
		return errors.New("ordinary browser fallback is incomplete")
	}

	native, err := eval[nativeWebMCP](b, `new Promise((done) => {
		const context = document.modelContext;
		if (!context) return done({ supported: false });
		if (typeof context.getTools !== "function") return done({ supported: true, error: "getTools unavailable" });
		Promise.resolve(context.getTools()).then((tools) => {
			const summaries = tools.map(({ name, inputSchema, description, annotations }) => ({ name, inputSchema, description, annotations }));
			const createTool = tools.find((tool) => tool.name === "item-create-v1");
			done({
				supported: true,
				names: summaries.map((tool) => tool.name),
				redacted: !JSON.stringify(summaries).match(/csrf|password|invitationsecret|tokensecret|oidc/i),
				schema: Boolean(createTool) && JSON.stringify(createTool.inputSchema) === JSON.stringify({ type: "object", properties: { title: { type: "string", minLength: 1, maxLength: 200 } }, required: ["title"], additionalProperties: false }),
			});
		}).catch((error) => done({ supported: true, error: error.message }));
	})`, chromedp.EvalAwaitPromise)
	if err != nil {
		return err
	}
	if !native.Supported {
		fmt.Printf("%s: fallback passed; WebMCP unavailable\n", label)
		return nil
	}
	if native.Error != "" {
		return fmt.Errorf("native WebMCP check failed: %s", native.Error)
	}
	for _, name := range []string{"item-create-v1", "workspace-export-v1"} {
		if !contains(native.Names, name) {
			return fmt.Errorf("missing native tool %s", name)
		}
	}
	if !native.Redacted {
		return errors.New("native tools expose a forbidden secret field")
	}
	if !native.Schema {
		return errors.New("native item-create schema changed")
	}

	prepared, err := eval[struct {
		Error     string `json:"error"`
		Value     string `json:"value"`
		Focused   string `json:"focused"`
		Submitted bool   `json:"submitted"`
	}](b, `new Promise((done) => {
		const context = document.modelContext;
		const form = document.querySelector('form[data-webmcp-actions="item-create"]');
		let submitted = false;
		form.addEventListener("submit", () => { submitted = true; }, { once: true });
		Promise.resolve(context.getTools()).then((tools) => tools.find((tool) => tool.name === "item-create-v1")).then((tool) =>
			context.executeTool(tool, JSON.stringify({ title: "WebMCP smoke" }))).then(() => done({
				value: document.querySelector("#new-title")?.value, focused: document.activeElement?.id, submitted,
			})).catch((error) => done({ error: error.message }));
	})`, chromedp.EvalAwaitPromise)
	if err != nil {
		return err
	}
	if prepared.Error != "" || prepared.Submitted || prepared.Value != "WebMCP smoke" || prepared.Focused != "new-title" {
		return fmt.Errorf("native preparation failed: %+v", prepared)
	}

	current, err := b.location()
	if err != nil {
		return err
	}
	workspaceRoot := strings.TrimSuffix(current, "/items")
	if err := b.goTo(workspaceRoot + "/settings/export"); err != nil {
		return err
	}
	if err := b.waitFor("a[data-webmcp-export]"); err != nil {
		return err
	}
	exported, err := eval[exportNativeResult](b, `new Promise((done) => {
		const context = document.modelContext;
		const link = document.querySelector("[data-webmcp-export]");
		let activated = false;
		link.addEventListener("click", () => { activated = true; }, { once: true });
		Promise.resolve(context.getTools()).then((tools) => {
			const tool = tools.find((candidate) => candidate.name === "workspace-export-v1");
			if (!tool) return done({ error: "workspace export tool unavailable" });
			if (JSON.stringify(tool.inputSchema) !== JSON.stringify({ type: "object", properties: {}, additionalProperties: false })) {
				return done({ error: "workspace export schema changed" });
			}
			return context.executeTool(tool, JSON.stringify({})).then((result) => done({
				focused: document.activeElement === link,
				activated,
				redacted: !JSON.stringify(result).match(/csrf|password|invitationsecret|tokensecret|oidc/i),
			}));
		}).catch((error) => done({ error: error.message }));
	})`, chromedp.EvalAwaitPromise)
	if err != nil {
		return err
	}
	if exported.Error != "" || !exported.Focused || exported.Activated || !exported.Redacted {
		return fmt.Errorf("native export preparation failed: %+v", exported)
	}
	fmt.Printf("%s: fallback and native WebMCP checks passed\n", label)
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
