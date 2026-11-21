//go:build browser

// Package browsertest drives a bootstrapped local server in headless Chrome
// through chromedp. It replaces the Playwright and Selenium browser suites.
// Run it with: go test -tags browser ./internal/browsertest
package browsertest

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

//go:embed axe.min.js
var axeSource string

const (
	fixturePassword    = "playwright-password-123"
	fixtureMemberPass  = "playwright-member-password-123"
	fixtureOwnerEmail  = "playwright@example.com"
	fixtureMemberEmail = "playwright-member@example.com"
	actionWait         = 5 * time.Second
	tabLifetime        = 2 * time.Minute
)

var (
	baseURL          string
	repoRoot         string
	allocCtx         context.Context
	chromeCandidates = []string{
		"/opt/google/chrome/chrome",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
)

func TestMain(m *testing.M) {
	cleanup, err := startFixtures()
	if err != nil {
		fmt.Fprintln(os.Stderr, "browsertest fixtures:", err)
		os.Exit(1)
	}
	code := m.Run()
	if code != 0 && serverLog != nil {
		fmt.Fprintln(os.Stderr, "--- main server log ---")
		fmt.Fprintln(os.Stderr, serverLog.String())
	}
	cleanup()
	os.Exit(code)
}

// startFixtures builds the server and seed binaries, creates a disposable
// SQLite database with an owner and a member, starts the server, and starts
// one headless Chrome for the tests to share.
func startFixtures() (func(), error) {
	var err error
	repoRoot, err = filepath.Abs("../..")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "browsertest-")
	if err != nil {
		return nil, err
	}
	cleanups := []func(){func() { os.RemoveAll(dir) }}
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	fail := func(err error) (func(), error) {
		cleanup()
		return nil, err
	}

	binaries := map[string]string{
		"server":          "./cmd/server",
		"bootstrap-admin": "./cmd/bootstrap-admin",
		"seed-member":     "./internal/browsertest/seedmember",
	}
	oidcBuild := exec.Command("go", "build", "-tags", "oidcfixture", "-o", filepath.Join(dir, "oidc-fixture"), "./internal/web/testdata/oidcfixture")
	oidcBuild.Dir = repoRoot
	if out, err := oidcBuild.CombinedOutput(); err != nil {
		return fail(fmt.Errorf("build oidc-fixture: %w\n%s", err, out))
	}
	for name, pkg := range binaries {
		build := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg)
		build.Dir = repoRoot
		if out, err := build.CombinedOutput(); err != nil {
			return fail(fmt.Errorf("build %s: %w\n%s", name, err, out))
		}
	}

	appDB := filepath.Join(dir, "app.db")
	appDBFile = appDB
	// membersDB is a copy of the seeded database, so the unkeyed server starts from the same data.
	membersDB := filepath.Join(dir, "members.db")
	membersDBFile = membersDB
	base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		"DATABASE_DRIVER=sqlite", "SQLITE_PATH=" + appDB, "STORAGE_DRIVER=local",
		"STORAGE_LOCAL_PATH=" + filepath.Join(dir, "files"), "BOOTSTRAP_ADMIN_PASSWORD=" + fixturePassword}
	if err := runTool(dir, base, "bootstrap-admin", "-email", fixtureOwnerEmail, "-workspace", "UI Baselines"); err != nil {
		return fail(err)
	}
	seedEnv := append(append([]string{}, base...),
		"PLAYWRIGHT_EMAIL="+fixtureOwnerEmail, "PLAYWRIGHT_MEMBER_EMAIL="+fixtureMemberEmail,
		"PLAYWRIGHT_MEMBER_PASSWORD="+fixtureMemberPass, "PLAYWRIGHT_SQLITE_PATH="+appDB)
	if err := runTool(dir, seedEnv, "seed-member"); err != nil {
		return fail(err)
	}

	if data, err := os.ReadFile(appDB); err != nil {
		return fail(err)
	} else if err := os.WriteFile(membersDB, data, 0o600); err != nil {
		return fail(err)
	}

	port, err := freePort()
	if err != nil {
		return fail(err)
	}
	serverEnv := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		"MULTI_WORKSPACE_ENABLED=true", "HTTP_REQUESTS_PER_MINUTE=10000", "HTTP_AUTH_REQUESTS_PER_MINUTE=1000",
		"WEBHOOK_ENCRYPTION_KEY=" + strings.Repeat("0", 64), "ENCRYPTION_KEY=",
		"DATABASE_DRIVER=sqlite", "SQLITE_PATH=" + appDB, "STORAGE_DRIVER=local",
		"STORAGE_LOCAL_PATH=" + filepath.Join(dir, "files"), "HTTP_SSE_POLL_INTERVAL=1s",
		"HTTP_SSE_MAX_LIFETIME=5m", fmt.Sprintf("HTTP_ADDRESS=127.0.0.1:%d", port)}
	server := exec.Command(filepath.Join(dir, "server"))
	server.Env = serverEnv
	serverLog = &bytes.Buffer{}
	server.Stdout, server.Stderr = serverLog, serverLog
	if err := server.Start(); err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	})
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitReady(baseURL + "/health/ready"); err != nil {
		return fail(err)
	}

	// The unkeyed server has no webhook encryption key, the state the admin pages explain.
	unkeyedPort, err := freePort()
	if err != nil {
		return fail(err)
	}
	unkeyedEnv := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		"MULTI_WORKSPACE_ENABLED=true", "HTTP_REQUESTS_PER_MINUTE=10000", "HTTP_AUTH_REQUESTS_PER_MINUTE=1000",
		"DATABASE_DRIVER=sqlite", "SQLITE_PATH=" + membersDB, "STORAGE_DRIVER=local",
		"STORAGE_LOCAL_PATH=" + filepath.Join(dir, "members-files"), "HTTP_SSE_POLL_INTERVAL=1s",
		"HTTP_SSE_MAX_LIFETIME=1m", fmt.Sprintf("HTTP_ADDRESS=127.0.0.1:%d", unkeyedPort)}
	unkeyed := exec.Command(filepath.Join(dir, "server"))
	unkeyed.Env = unkeyedEnv
	unkeyed.Stdout, unkeyed.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
	if err := unkeyed.Start(); err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, func() {
		_ = unkeyed.Process.Kill()
		_, _ = unkeyed.Process.Wait()
	})
	membersURL = fmt.Sprintf("http://127.0.0.1:%d", unkeyedPort)
	if err := waitReady(membersURL + "/health/ready"); err != nil {
		return fail(err)
	}

	// The OIDC fixture is a separate server with its own database.
	oidcPort, err := freePort()
	if err != nil {
		return fail(err)
	}
	oidc := exec.Command(filepath.Join(dir, "oidc-fixture"))
	oidc.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir,
		fmt.Sprintf("OIDC_FIXTURE_ADDRESS=127.0.0.1:%d", oidcPort), "OIDC_FIXTURE_DATABASE=" + filepath.Join(dir, "oidc-fixture.db")}
	oidc.Stdout, oidc.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
	if err := oidc.Start(); err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, func() {
		_ = oidc.Process.Kill()
		_, _ = oidc.Process.Wait()
	})
	oidcURL = fmt.Sprintf("http://127.0.0.1:%d", oidcPort)
	if err := waitReady(oidcURL + "/health/ready"); err != nil {
		return fail(err)
	}

	// The setup server starts with an empty database, so the first-owner wizard is open.
	setupPort, err := freePort()
	if err != nil {
		return fail(err)
	}
	setupEnv := append(append([]string{}, serverEnv...), "SQLITE_PATH="+filepath.Join(dir, "setup.db"),
		"STORAGE_LOCAL_PATH="+filepath.Join(dir, "setup-files"), fmt.Sprintf("HTTP_ADDRESS=127.0.0.1:%d", setupPort))
	setup := exec.Command(filepath.Join(dir, "server"))
	setup.Env = setupEnv
	setup.Stdout, setup.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
	if err := setup.Start(); err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, func() {
		_ = setup.Process.Kill()
		_, _ = setup.Process.Wait()
	})
	setupURL = fmt.Sprintf("http://127.0.0.1:%d", setupPort)
	if err := waitReady(setupURL + "/health/ready"); err != nil {
		return fail(err)
	}

	// The UX journey runs its own first-owner setup on a fresh database.
	uxPort, err := freePort()
	if err != nil {
		return fail(err)
	}
	uxEnv := append(append([]string{}, serverEnv...), "SQLITE_PATH="+filepath.Join(dir, "ux.db"),
		"STORAGE_LOCAL_PATH="+filepath.Join(dir, "ux-files"), fmt.Sprintf("HTTP_ADDRESS=127.0.0.1:%d", uxPort))
	ux := exec.Command(filepath.Join(dir, "server"))
	ux.Env = uxEnv
	ux.Stdout, ux.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
	if err := ux.Start(); err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, func() {
		_ = ux.Process.Kill()
		_, _ = ux.Process.Wait()
	})
	uxURL = fmt.Sprintf("http://127.0.0.1:%d", uxPort)
	if err := waitReady(uxURL + "/health/ready"); err != nil {
		return fail(err)
	}

	browserCtx, stopBrowser, err := startChrome()
	if err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, stopBrowser)
	allocCtx = browserCtx
	return cleanup, nil
}

func runTool(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(filepath.Join(dir, name), args...)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w\n%s", name, err, out)
	}
	return nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitReady(url string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("server not ready at %s", url)
}

// startChrome returns the headless Chrome allocator. Each test opens its own tab and browser.
func startChrome() (context.Context, func(), error) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(1280, 900),
		// The OIDC fixture redirects to an HTTPS origin, as the Playwright suite allowed.
		chromedp.Flag("ignore-certificate-errors", true),
	}
	if path := chromePath(); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	return allocator, cancelAllocator, nil
}

func chromePath() string {
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	for _, candidate := range chromeCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// newTab opens a fresh page at the viewport with the given color scheme and
// reduced motion, as the Playwright config did.
func newTab(t *testing.T, width, height int64, colorScheme string) context.Context {
	t.Helper()
	tab, cancelTab := chromedp.NewContext(allocCtx)
	t.Cleanup(cancelTab)
	// The first action on a tab owns its browser, so start it on the root
	// context before any per-action timeout is derived from it.
	if err := chromedp.Do(tab); err != nil {
		t.Fatalf("start Chrome: %v", err)
	}
	tab, cancelTimeout := context.WithTimeout(tab, tabLifetime)
	t.Cleanup(cancelTimeout)
	features := []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}
	if colorScheme != "" {
		features = append(features, &emulation.MediaFeature{Name: "prefers-color-scheme", Value: colorScheme})
	}
	run(t, tab, chromedp.EmulateViewport(width, height))
	call(t, tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: features})
	return tab
}

// setTheme stores the theme cookie for the server, as the page's own preference does.
func setTheme(t *testing.T, tab context.Context, theme string) {
	t.Helper()
	call(t, tab, network.SetCookie, network.SetCookieParams{Name: "theme", Value: theme, URL: baseURL, Path: "/"})
}

// run performs browser actions under actionWait so a missing element fails the test.
func run(t *testing.T, tab context.Context, steps ...chromedp.Action[chromedp.Void]) {
	t.Helper()
	deadline := time.Now().Add(actionWait)
	for {
		ctx, cancel := context.WithTimeout(tab, actionWait)
		err := chromedp.Do(ctx, steps...)
		cancel()
		if err == nil {
			return
		}
		// A realtime refresh can replace the nodes an action resolved; resolve them again.
		if strings.Contains(err.Error(), "Could not find node") && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		t.Fatalf("browser action: %v%s", err, pageSummary(tab))
	}
}

// call runs one DevTools protocol command on the tab.
func call[P, R any](t *testing.T, tab context.Context, command cdp.Command[P, R], params P) R {
	t.Helper()
	ctx, cancel := context.WithTimeout(tab, actionWait)
	defer cancel()
	result, err := chromedp.Run[R](ctx, func(ctx context.Context, target *chromedp.Target) (R, error) {
		return cdp.Call(ctx, target, command, params)
	})
	if err != nil {
		t.Fatalf("protocol command: %v", err)
	}
	return result
}

// eval reads an expression's value from the page.
func eval[T any](t *testing.T, tab context.Context, expr string, opts ...chromedp.EvaluateOption) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(tab, actionWait)
	defer cancel()
	value, err := chromedp.Run(ctx, chromedp.Evaluate[T](expr, opts...))
	if err != nil {
		t.Fatalf("evaluate %q: %v", expr, err)
	}
	return value
}

var awaitPromise = chromedp.EvalAwaitPromise

// eventually polls a page predicate until it holds or actionWait passes.
func eventually(t *testing.T, tab context.Context, expr, message string) {
	t.Helper()
	eventuallyWithin(t, tab, expr, message, actionWait)
}

// eventuallyWithin polls a page predicate until it holds or the wait passes.
func eventuallyWithin(t *testing.T, tab context.Context, expr, message string, wait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		ok, err := chromedp.Run(tab, chromedp.Evaluate[bool](expr))
		if err == nil && ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s (predicate %s, last error %v)%s", message, expr, err, pageSummary(tab))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitVisible waits for an element to be visible.
func waitVisible(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	run(t, tab, chromedp.WaitVisible(selector))
}

// fill replaces an input's value and types the new value, as Playwright's fill did.
func fill(t *testing.T, tab context.Context, selector, value string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf("(() => { const el = document.querySelector(%s); el.value = ''; return null; })()", jsString(selector)))
	run(t, tab, chromedp.SendKeys(selector, value))
}

func click(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	eval[any](t, tab, `window.__bt = 1`)
	run(t, tab, chromedp.Click(selector))
	settle(t, tab)
}

func press(t *testing.T, tab context.Context, key string) {
	t.Helper()
	run(t, tab, chromedp.KeyEvent(key))
}

func navigate(t *testing.T, tab context.Context, path string) {
	t.Helper()
	run(t, tab, chromedp.Navigate(baseURL+path))
}

func attribute(t *testing.T, tab context.Context, selector, name string) string {
	t.Helper()
	return eval[string](t, tab, fmt.Sprintf("document.querySelector(%s)?.getAttribute(%s) ?? null", jsString(selector), jsString(name)))
}

func expectAttribute(t *testing.T, tab context.Context, selector, name, want string) {
	t.Helper()
	if got := attribute(t, tab, selector, name); got != want {
		t.Fatalf("%s %s = %q, want %q", selector, name, got, want)
	}
}

// expectTheme checks the root data-theme. The "system" preference renders no
// data-theme attribute, so daisyUI follows prefers-color-scheme.
func expectTheme(t *testing.T, tab context.Context, theme string) {
	t.Helper()
	if theme == "system" {
		got := eval[string](t, tab, `document.querySelector("html")?.getAttribute("data-theme") ?? ""`)
		if got != "" {
			t.Fatalf("html data-theme = %q, want absent for system", got)
		}
		return
	}
	expectAttribute(t, tab, "html", "data-theme", theme)
}

func expectFocused(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf("document.activeElement === document.querySelector(%s)", jsString(selector)),
		fmt.Sprintf("focus did not reach %s", selector))
}

func expectText(t *testing.T, tab context.Context, selector string, pattern *regexp.Regexp) {
	t.Helper()
	text := eval[string](t, tab, fmt.Sprintf("document.querySelector(%s)?.textContent ?? ''", jsString(selector)))
	if !pattern.MatchString(text) {
		t.Fatalf("%s text %q does not match %s", selector, text, pattern)
	}
}

func expectNoOverflow(t *testing.T, tab context.Context, width int64) {
	t.Helper()
	scroll := eval[int64](t, tab, "document.documentElement.scrollWidth")
	if scroll > width {
		t.Fatalf("horizontal overflow: scrollWidth %d exceeds viewport %d", scroll, width)
	}
}

// expectAxeClean runs axe-core in the page and fails on any violation.
func expectAxeClean(t *testing.T, tab context.Context) {
	t.Helper()
	eval[any](t, tab, axeSource)
	type result struct {
		Error      string `json:"error"`
		Violations []struct {
			ID string `json:"id"`
		} `json:"violations"`
	}
	res := eval[result](t, tab, `new Promise((done) => {
		axe.run(document).then(done).catch((error) => done({ error: error.message }));
	})`, awaitPromise)
	if res.Error != "" {
		t.Fatalf("axe: %s", res.Error)
	}
	if len(res.Violations) != 0 {
		ids := make([]string, 0, len(res.Violations))
		for _, violation := range res.Violations {
			ids = append(ids, violation.ID)
		}
		t.Fatalf("axe violations: %s", strings.Join(ids, ", "))
	}
}

func jsString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// pageSummary describes the current page for a failure message.
func pageSummary(tab context.Context) string {
	ctx, cancel := context.WithTimeout(tab, 2*time.Second)
	defer cancel()
	location, _ := chromedp.Run(ctx, chromedp.Location())
	text, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`document.body ? document.body.innerText.slice(0, 600) : ""`))
	return fmt.Sprintf("\n  page: %s\n  text: %q", location, text)
}

// settle waits for a page navigation that a click started. A navigation replaces the
// document, which drops the marker set before the click; with no navigation the marker
// stays, and the call returns after the grace period.
func settle(t *testing.T, tab context.Context) {
	t.Helper()
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(tab, time.Second)
		marker, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`window.__bt === 1`))
		cancel()
		if err != nil || !marker {
			eventually(t, tab, `document.readyState === "complete"`, "navigation did not finish loading")
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// oidcURL and setupURL are the OIDC fixture and the empty-database setup server.
var oidcURL, setupURL string

// serverLog collects the main server's output, printed when a run fails.
var serverLog *bytes.Buffer

// membersURL is the server without webhook encryption configured.
var membersURL string

// membersDBFile is the unkeyed server's database, for fixtures that write to it directly.
var membersDBFile string

// appDBFile is the main server's database, for fixtures that inspect it.
var appDBFile string

// uxURL is a fresh server for the form UX journey, which creates its own owner.
var uxURL string

// navigationWait bounds a page load whose response status is read; CI runners can be slow.
const navigationWait = 15 * time.Second
