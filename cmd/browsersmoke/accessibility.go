package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// axeSource is the vendored axe-core 4.14.0 build (matches package-lock.json).
//
//go:embed axe.min.js
var axeSource string

const axeRunExpr = `new Promise((done) => {
	axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })
		.then(done)
		.catch((error) => done({ error: error.message }));
})`

type axeRun struct {
	Error      string         `json:"error"`
	Violations []axeViolation `json:"violations"`
}

type axeViolation struct {
	ID     string    `json:"id"`
	Impact string    `json:"impact"`
	Help   string    `json:"help"`
	Nodes  []axeNode `json:"nodes"`
}

type axeNode struct {
	Target         json.RawMessage `json:"target"`
	FailureSummary string          `json:"failureSummary"`
}

type focusInfo struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	Role      string `json:"role"`
	Class     string `json:"class"`
	Href      string `json:"href"`
	BoxShadow string `json:"boxShadow"`
}

type oauthRegistration struct {
	Status int `json:"status"`
	Body   struct {
		ClientID string `json:"client_id"`
	} `json:"body"`
	Error string `json:"error"`
}

const activeFocusExpr = `(() => {
	const e = document.activeElement;
	return {
		id: e.id, tag: e.tagName.toLowerCase(), type: e.getAttribute("type") ?? "",
		role: e.getAttribute("role") ?? "", class: e.getAttribute("class") ?? "",
		href: e.getAttribute("href") ?? "", boxShadow: getComputedStyle(e).boxShadow,
	};
})()`

const documentOverflowExpr = `document.documentElement.scrollWidth > document.documentElement.clientWidth`

const settingsOverflowExpr = `(() => ({
	document: document.documentElement.scrollWidth > document.documentElement.clientWidth,
	navigation: Array.from(document.querySelectorAll(".workspace-settings-nav > *"))
		.some((element) => element.scrollWidth > element.clientWidth + 1),
	scrollWidth: document.documentElement.scrollWidth,
	clientWidth: document.documentElement.clientWidth,
	innerWidth,
	overflowing: [...document.querySelectorAll("body *")].filter((element) => {
		const rect = element.getBoundingClientRect();
		return rect.width > 0 && (rect.left < 0 || rect.right > innerWidth);
	}).slice(0, 10).map((element) => {
		const rect = element.getBoundingClientRect();
		return {
			tag: element.tagName.toLowerCase(),
			id: element.id,
			className: typeof element.className === "string" ? element.className : "",
			left: rect.left,
			right: rect.right,
			scrollWidth: element.scrollWidth,
			clientWidth: element.clientWidth,
		};
	}),
}))()`

const deletionFormExpr = `(() => ({
	confirmationRequired: document.querySelector('#delete-confirm[required]') !== null,
	passwordRequired: document.querySelector('#delete-password[required]') !== null,
}))()`

const fileUploadContractExpr = `(() => {
	const form = document.querySelector('form[data-file-upload][method="post"][enctype="multipart/form-data"]');
	const progress = form?.querySelector("progress[data-upload-progress]");
	return Boolean(form && progress && progress.max === 100 && progress.getAttribute("aria-label"));
})()`

const showUploadProgressExpr = `(() => {
	const progress = document.querySelector("progress[data-upload-progress]");
	progress.value = 1;
	progress.removeAttribute("hidden");
})()`

type a11yRun struct {
	*browser
	base, email, password, locale string
}

// runAccessibility is the Go port of scripts/accessibility.mjs. It audits the
// same pages with axe and drives the same keyboard, focus, reflow, reduced-motion
// and accessibility-tree checks for one locale.
func runAccessibility() error {
	base := os.Getenv("A11Y_BASE_URL")
	email := os.Getenv("A11Y_EMAIL")
	password := os.Getenv("A11Y_PASSWORD")
	locale := envDefault("A11Y_LOCALE", "en")
	if base == "" || email == "" || password == "" || (locale != "en" && locale != "es") {
		return fmt.Errorf("A11Y_BASE_URL, A11Y_EMAIL, A11Y_PASSWORD, and A11Y_LOCALE=en|es are required: %w", errUsage)
	}

	b, stop, err := newBrowser()
	if err != nil {
		return err
	}
	defer stop()

	run := &a11yRun{browser: b, base: base, email: email, password: password, locale: locale}
	if err := run.flow(); err != nil {
		return err
	}
	if b.failed {
		return errors.New("accessibility audit found critical or serious violations")
	}
	fmt.Println("manual-contract: keyboard, focus, 320px reflow, reduced motion, and accessibility tree passed")
	return nil
}

func (s *a11yRun) flow() error {
	base, locale := s.base, s.locale

	if err := s.goTo(fmt.Sprintf("%s/language?locale=%s&return_to=/login", base, locale)); err != nil {
		return err
	}
	if err := s.audit("login"); err != nil {
		return err
	}
	if err := s.expectKeyboardOrder(); err != nil {
		return err
	}

	if err := s.signIn(); err != nil {
		return err
	}
	if err := s.audit("workspaces"); err != nil {
		return err
	}
	href, err := eval[string](s.browser, `document.querySelector(".cards a").getAttribute("href")`)
	if err != nil {
		return err
	}
	workspacePath, err := pathOf(base, href)
	if err != nil {
		return err
	}
	if err := s.click(".cards a"); err != nil {
		return err
	}
	if err := s.waitFor("[data-workspace-home]"); err != nil {
		return err
	}
	if err := s.audit("workspace"); err != nil {
		return err
	}
	current, err := s.location()
	if err != nil {
		return err
	}
	wsURL, err := url.Parse(current)
	if err != nil {
		return err
	}
	wsBase := wsURL.Scheme + "://" + wsURL.Host + wsURL.Path

	for _, page := range []struct{ path, waitFor, name string }{
		{"/settings/members", "#member-list", "members"},
		{"/settings/invitations", "#invite-email", "invitations"},
	} {
		if err := s.goTo(wsBase + page.path); err != nil {
			return err
		}
		if err := s.waitFor(page.waitFor); err != nil {
			return err
		}
		if err := s.audit(page.name); err != nil {
			return err
		}
	}
	if err := s.goTo(base + "/notifications"); err != nil {
		return err
	}
	if err := s.waitFor("h1"); err != nil {
		return err
	}
	if err := s.audit("notifications"); err != nil {
		return err
	}
	if err := s.goTo(base + workspacePath + "/settings/general"); err != nil {
		return err
	}
	if err := s.waitFor("#workspace-locale"); err != nil {
		return err
	}
	if err := s.audit("general settings"); err != nil {
		return err
	}
	for _, page := range []struct{ path, name string }{
		{"email", "email settings"},
		{"tokens", "API tokens"},
		{"webhooks", "webhooks"},
		{"provisioning", "SCIM provisioning"},
		{"audit", "audit history"},
	} {
		fmt.Printf("checking %s (%s)\n", page.name, locale)
		if err := s.goTo(wsBase + "/settings/" + page.path); err != nil {
			return err
		}
		if err := s.waitFor("h1"); err != nil {
			current, _ := s.location()
			body, _ := eval[string](s.browser, "document.body.innerText")
			fmt.Fprintf(os.Stderr, "failed URL: %s\n%s\n", current, body)
			return err
		}
		if err := s.audit(page.name); err != nil {
			return err
		}
	}

	if err := s.goTo(base + workspacePath + "/settings/general"); err != nil {
		return err
	}
	if err := s.waitFor("#workspace-locale"); err != nil {
		return err
	}
	if err := s.setViewport(320, 800); err != nil {
		return err
	}
	settingsOverflow, err := eval[struct {
		Document    bool `json:"document"`
		Navigation  bool `json:"navigation"`
		ScrollWidth int  `json:"scrollWidth"`
		ClientWidth int  `json:"clientWidth"`
		InnerWidth  int  `json:"innerWidth"`
		Overflowing []struct {
			Tag         string  `json:"tag"`
			ID          string  `json:"id"`
			ClassName   string  `json:"className"`
			Left        float64 `json:"left"`
			Right       float64 `json:"right"`
			ScrollWidth int     `json:"scrollWidth"`
			ClientWidth int     `json:"clientWidth"`
		} `json:"overflowing"`
	}](s.browser, settingsOverflowExpr)
	if err != nil {
		return err
	}
	if settingsOverflow.Document || settingsOverflow.Navigation {
		return fmt.Errorf("general settings has horizontal overflow at 320 CSS pixels: %+v", settingsOverflow)
	}
	if err := s.setViewport(1280, 900); err != nil {
		return err
	}

	if err := s.goTo(base + "/account"); err != nil {
		return err
	}
	if err := s.waitFor("#delete-confirm"); err != nil {
		return err
	}
	if err := s.audit("account"); err != nil {
		return err
	}
	deletion, err := eval[struct {
		ConfirmationRequired bool `json:"confirmationRequired"`
		PasswordRequired     bool `json:"passwordRequired"`
	}](s.browser, deletionFormExpr)
	if err != nil {
		return err
	}
	if !deletion.ConfirmationRequired || !deletion.PasswordRequired {
		return errors.New("account deletion confirmation and password are required")
	}
	if err := s.expectMobileNoOverflow("account"); err != nil {
		return err
	}

	if err := s.goTo(base + "/settings/language"); err != nil {
		return err
	}
	if err := s.waitFor("#language"); err != nil {
		return err
	}
	if err := s.audit("language settings"); err != nil {
		return err
	}
	if err := s.expectMobileNoOverflow("language settings"); err != nil {
		return err
	}

	if err := s.goTo(base + "/sessions"); err != nil {
		return err
	}
	if err := s.waitFor(".session-card"); err != nil {
		return err
	}
	if err := s.audit("sessions"); err != nil {
		return err
	}
	if err := s.expectMobileNoOverflow("sessions"); err != nil {
		return err
	}

	if err := s.goTo(base + "/security"); err != nil {
		return err
	}
	if err := s.waitFor(".settings-panel"); err != nil {
		return err
	}
	if err := s.audit("security"); err != nil {
		return err
	}
	if err := s.expectMobileNoOverflow("security"); err != nil {
		return err
	}

	if err := s.deleteCookie("session", base); err != nil {
		return err
	}
	if err := s.deleteCookie("csrf", base); err != nil {
		return err
	}
	if err := s.resetPasswordMatrix(); err != nil {
		return err
	}

	if err := s.goTo(fmt.Sprintf("%s/language?locale=%s&return_to=%%2Flogin", base, locale)); err != nil {
		return err
	}
	if err := s.goTo(base + "/auth/oidc/company/callback?error=access_denied"); err != nil {
		return err
	}
	if err := s.waitFor(`[role="alert"]`); err != nil {
		return err
	}
	if err := s.audit("OIDC callback failure"); err != nil {
		return err
	}
	if err := s.expectFocusRole("alert", "OIDC callback failure did not focus its error alert"); err != nil {
		return err
	}

	if err := s.goTo(base + "/login"); err != nil {
		return err
	}
	if err := s.typeText("#email", s.email); err != nil {
		return err
	}
	if err := s.typeText("#password", s.password); err != nil {
		return err
	}
	if err := s.click(`form[action="/login"] button[type="submit"]`); err != nil {
		return err
	}
	if err := s.waitUntil(`location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`); err != nil {
		current, _ := s.location()
		body, _ := eval[string](s.browser, "document.body.innerText")
		return fmt.Errorf("re-login after auth accessibility checks failed at %s: %s: %w", current, body, err)
	}

	return s.workspacePages(wsBase)
}

// signIn fills the login form on the current page and waits for the home page.
func (s *a11yRun) signIn() error {
	if err := s.typeText("#email", s.email); err != nil {
		return err
	}
	if err := s.typeText("#password", s.password); err != nil {
		return err
	}
	if err := s.click(`button[type="submit"]`); err != nil {
		return err
	}
	if err := s.waitUntil(`location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`); err != nil {
		return err
	}
	return s.goTo(s.base + "/")
}

// expectKeyboardOrder checks the login form's Tab order: skip link, email,
// password, reveal button, submit, then the password reset link.
func (s *a11yRun) expectKeyboardOrder() error {
	if err := s.press("\t"); err != nil {
		return err
	}
	f, err := s.active()
	if err != nil {
		return err
	}
	if f.Class != "skip-link" {
		return errors.New("first Tab does not reach the skip link")
	}
	if err := s.press("\t"); err != nil {
		return err
	}
	if err := s.expectActive("email"); err != nil {
		return err
	}
	if err := s.press("\t"); err != nil {
		return err
	}
	if err := s.expectActive("password"); err != nil {
		return err
	}
	if err := s.press("\t"); err != nil {
		return err
	}
	if f, err = s.active(); err != nil {
		return err
	}
	if f.Tag != "button" || f.Type != "button" {
		return errors.New("login keyboard order does not reach the password reveal control")
	}
	if err := s.press("\t"); err != nil {
		return err
	}
	if f, err = s.active(); err != nil {
		return err
	}
	if f.Tag != "button" || f.Type != "submit" {
		return errors.New("login keyboard order does not reach submit after password reveal")
	}
	if err := s.press("\t"); err != nil {
		return err
	}
	if f, err = s.active(); err != nil {
		return err
	}
	resetPath, err := pathOf(s.base, f.Href)
	if err != nil {
		return err
	}
	if f.Tag != "a" || resetPath != "/password-reset" {
		return errors.New("login keyboard order does not reach the password reset link")
	}
	return nil
}

// resetPasswordMatrix checks password reset and its unavailable-state error in
// both locales, both themes and both viewports.
func (s *a11yRun) resetPasswordMatrix() error {
	base := s.base
	for _, resetLocale := range []string{"en", "es"} {
		for _, theme := range []string{"light", "dark"} {
			for _, viewport := range []struct {
				name          string
				width, height int64
			}{{"desktop", 1280, 900}, {"mobile", 390, 844}} {
				matrix := fmt.Sprintf("%s-%s-%s", resetLocale, theme, viewport.name)
				if err := s.setViewport(viewport.width, viewport.height); err != nil {
					return err
				}
				if err := s.goTo(fmt.Sprintf("%s/language?locale=%s&return_to=%%2Fpassword-reset", base, resetLocale)); err != nil {
					return err
				}
				if err := s.deleteCookie("theme", base); err != nil {
					return err
				}
				if err := s.setCookie("theme", theme, base); err != nil {
					return err
				}
				if err := s.goTo(base + "/password-reset"); err != nil {
					return err
				}
				if err := s.waitFor(`form[action="/password-reset"]`); err != nil {
					return err
				}
				html, err := eval[struct {
					Theme string `json:"theme"`
					Lang  string `json:"lang"`
				}](s.browser, `({ theme: document.documentElement.getAttribute("data-theme") ?? "null", lang: document.documentElement.getAttribute("lang") ?? "null" })`)
				if err != nil {
					return err
				}
				if html.Theme != theme || html.Lang != resetLocale {
					return fmt.Errorf("password reset locale/theme mismatch: %s; got %s/%s", matrix, html.Lang, html.Theme)
				}
				if err := s.audit("password reset " + matrix); err != nil {
					return err
				}
				if viewport.width < 608 {
					if err := s.expectMobileNoOverflow("password reset " + matrix); err != nil {
						return err
					}
				}

				if err := s.goTo(base + "/password-reset/unavailable"); err != nil {
					return err
				}
				if err := s.waitFor("#reset-password"); err != nil {
					return err
				}
				if err := s.typeText("#reset-password", "first-valid-password-123"); err != nil {
					return err
				}
				if err := s.typeText("#reset-password-confirmation", "first-valid-password-123"); err != nil {
					return err
				}
				if err := s.click(`form[action="/password-reset/unavailable"] button[type="submit"]`); err != nil {
					return err
				}
				if err := s.waitFor(`.error[role="alert"]`); err != nil {
					return err
				}
				if err := s.expectFocusRole("alert", "password reset unavailable error did not receive focus: "+matrix); err != nil {
					return err
				}
				if err := s.audit("password reset unavailable " + matrix); err != nil {
					return err
				}
				if viewport.width < 608 {
					if err := s.expectMobileNoOverflow("password reset unavailable " + matrix); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// workspacePages covers Clients, reduced motion, MCP consent, and Files.
func (s *a11yRun) workspacePages(wsBase string) error {
	base := s.base

	if err := s.goTo(wsBase + "/clients"); err != nil {
		return err
	}
	if err := s.waitFor("#client-list"); err != nil {
		return err
	}
	if err := s.audit("clients"); err != nil {
		return err
	}
	if err := s.click(`a[href$="/clients/new"]`); err != nil {
		return err
	}
	if err := s.typeText("#client-display-name", "Accessibility client "+s.locale); err != nil {
		return err
	}
	// Submit the form itself: a realtime list refresh can re-render the open menu and strand a button click.
	if _, err := eval[any](s.browser, `document.querySelector("form.create").requestSubmit()`); err != nil {
		return err
	}
	if err := s.waitFor("#client-detail"); err != nil {
		return err
	}
	if err := s.audit("client"); err != nil {
		return err
	}
	if err := s.goTo(wsBase + "/client-fields"); err != nil {
		return err
	}
	if err := s.waitFor("#client-field-list"); err != nil {
		return err
	}
	if err := s.audit("client fields"); err != nil {
		return err
	}

	if err := s.setViewport(320, 800); err != nil {
		return err
	}
	if overflow, err := eval[bool](s.browser, documentOverflowExpr); err != nil {
		return err
	} else if overflow {
		details, err := eval[string](s.browser, `JSON.stringify(Array.from(document.querySelectorAll("body *")).filter(el => el.getBoundingClientRect().right > document.documentElement.clientWidth + 1).map(el => ({tag: el.tagName, class: el.className, width: Math.round(el.getBoundingClientRect().width), right: Math.round(el.getBoundingClientRect().right)})).slice(0, 20))`)
		if err != nil {
			return err
		}
		return fmt.Errorf("client fields page has horizontal overflow at 320 CSS pixels: %s", details)
	}
	if _, err := cdpCall(s.browser, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{
		Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}},
	}); err != nil {
		return err
	}
	if reduced, err := eval[bool](s.browser, `matchMedia("(prefers-reduced-motion: reduce)").matches`); err != nil {
		return err
	} else if !reduced {
		return errors.New("reduced-motion preference was not honored")
	}
	if err := s.setViewport(1280, 900); err != nil {
		return err
	}

	registration, err := eval[oauthRegistration](s.browser, `new Promise((done) => {
		fetch("/oauth/register", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ client_name: "Accessibility smoke", redirect_uris: ["https://client.example/callback"] }),
		}).then(async (response) => done({ status: response.status, body: await response.json() }))
			.catch((error) => done({ error: error.message }));
	})`, chromedp.EvalAwaitPromise)
	if err != nil {
		return err
	}
	if registration.Status != 201 || registration.Body.ClientID == "" {
		return fmt.Errorf("OAuth smoke client registration failed: %+v", registration)
	}
	verifier := "accessibility-smoke-verifier-with-enough-entropy"
	digest := sha256.Sum256([]byte(verifier))
	consent := url.Values{
		"response_type":         {"code"},
		"client_id":             {registration.Body.ClientID},
		"redirect_uri":          {"https://client.example/callback"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(digest[:])},
		"code_challenge_method": {"S256"},
		"resource":              {base + "/mcp"},
		"scope":                 {"resources:read"},
		"state":                 {"accessibility-smoke-state"},
	}
	if err := s.goTo(base + "/oauth/authorize?" + consent.Encode()); err != nil {
		return err
	}
	if err := s.waitFor(`main.auth-card form[action="/oauth/authorize"]`); err != nil {
		return err
	}
	if err := s.audit("MCP consent"); err != nil {
		return err
	}

	if err := s.goTo(wsBase + "/files"); err != nil {
		return err
	}
	if err := s.waitFor("#file-upload"); err != nil {
		return err
	}
	if err := s.audit("files"); err != nil {
		return err
	}
	if ok, err := eval[bool](s.browser, fileUploadContractExpr); err != nil {
		return err
	} else if !ok {
		return errors.New("files page ordinary upload and named progress contract failed")
	}
	if _, err := eval[any](s.browser, showUploadProgressExpr); err != nil {
		return err
	}
	progressName := "Upload progress"
	if s.locale == "es" {
		progressName = "Progreso de carga"
	}
	if err := s.requireAXNodes("files", [][2]string{{"progressbar", progressName}}); err != nil {
		return err
	}
	if err := s.setViewport(320, 800); err != nil {
		return err
	}
	if overflow, err := eval[bool](s.browser, documentOverflowExpr); err != nil {
		return err
	} else if overflow {
		return errors.New("files page has horizontal overflow at 320 CSS pixels")
	}
	return nil
}

// audit runs axe with the WCAG 2 A/AA and 2.1/2.2 AA tags and records a
// failure when any critical or serious violation remains.
func (s *browser) audit(name string) error {
	if _, err := eval[any](s, axeSource); err != nil {
		return fmt.Errorf("%s: loading axe: %w", name, err)
	}
	results, err := eval[axeRun](s, axeRunExpr, chromedp.EvalAwaitPromise)
	if err != nil {
		return fmt.Errorf("%s: running axe: %w", name, err)
	}
	if results.Error != "" {
		return fmt.Errorf("%s: %s", name, results.Error)
	}
	var blocking []axeViolation
	for _, violation := range results.Violations {
		if violation.Impact == "critical" || violation.Impact == "serious" {
			blocking = append(blocking, violation)
		}
	}
	fmt.Printf("%s: %d violations, %d critical/serious\n", name, len(results.Violations), len(blocking))
	for _, violation := range blocking {
		fmt.Fprintf(os.Stderr, "%s: %s %s: %s\n", name, violation.Impact, violation.ID, violation.Help)
		targets := make([]json.RawMessage, 0, len(violation.Nodes))
		for _, node := range violation.Nodes {
			targets = append(targets, node.Target)
		}
		targetJSON, _ := json.Marshal(targets)
		fmt.Fprintf(os.Stderr, "%s: targets %s\n", name, targetJSON)
		for _, node := range violation.Nodes {
			fmt.Fprintf(os.Stderr, "%s %s\n", node.Target, node.FailureSummary)
		}
	}
	if len(blocking) > 0 {
		s.failed = true
	}
	return nil
}

// active reads the focused element.
func (s *a11yRun) active() (focusInfo, error) {
	return eval[focusInfo](s.browser, activeFocusExpr)
}

// expectActive checks that id has keyboard focus and a visible focus ring.
func (s *a11yRun) expectActive(id string) error {
	f, err := s.active()
	if err != nil {
		return err
	}
	if f.ID != id {
		return fmt.Errorf("keyboard focus expected %s", id)
	}
	if f.BoxShadow == "" || f.BoxShadow == "none" {
		return fmt.Errorf("visible focus style missing on %s", id)
	}
	return nil
}

// expectFocusRole checks that the focused element has the given ARIA role.
func (s *a11yRun) expectFocusRole(role, message string) error {
	if err := s.waitUntil(fmt.Sprintf(`document.activeElement?.getAttribute("role") === %s`, jsString(role))); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

// expectMobileNoOverflow checks the page at 320 CSS pixels, then restores desktop size.
func (s *a11yRun) expectMobileNoOverflow(name string) error {
	if err := s.setViewport(320, 800); err != nil {
		return err
	}
	overflow, err := eval[bool](s.browser, documentOverflowExpr)
	if err != nil {
		return err
	}
	if overflow {
		return fmt.Errorf("%s page has horizontal overflow at 320 CSS pixels", name)
	}
	return s.setViewport(1280, 900)
}

// requireAXNodes checks that the accessibility tree has each role with its name.
func (s *a11yRun) requireAXNodes(prefix string, pairs [][2]string) error {
	result, err := cdpCall(s.browser, accessibility.GetFullAXTree, accessibility.GetFullAXTreeParams{})
	if err != nil {
		return err
	}
	nodes := result.Nodes
	for _, pair := range pairs {
		if !hasAXNode(nodes, pair[0], pair[1]) {
			return fmt.Errorf("%s accessibility tree lacks %s named %s", prefix, pair[0], pair[1])
		}
	}
	return nil
}

func hasAXNode(nodes []*accessibility.Node, role, name string) bool {
	for _, node := range nodes {
		if axText(node.Role) == role && axText(node.Name) == name {
			return true
		}
	}
	return false
}

func axText(value *accessibility.Value) string {
	if value == nil {
		return ""
	}
	var text string
	if err := json.Unmarshal([]byte(value.Value), &text); err != nil {
		return ""
	}
	return text
}

// pathOf resolves href against base and returns its path, like new URL(href, base).pathname.
func pathOf(base, href string) (string, error) {
	root, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return root.ResolveReference(ref).Path, nil
}
