// Command browsersmoke drives a bootstrapped local server in headless Chrome
// for the accessibility and WebMCP smoke gates. scripts/accessibility-smoke.sh
// and scripts/webmcp-smoke.sh start the server and call this program.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	actionTimeout = 15 * time.Second
	waitTimeout   = 5 * time.Second
	runTimeout    = 15 * time.Minute
)

// errUsage marks a misconfigured invocation, which exits with status 2.
var errUsage = errors.New("usage")

// chromeCandidates are probed when CHROME_PATH is unset. The first is where
// Playwright's `install --with-deps chrome` puts Chrome on Linux CI.
var chromeCandidates = []string{
	"/opt/google/chrome/chrome",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
}

func main() {
	mode := flag.String("mode", "", "accessibility or webmcp")
	flag.Parse()
	var err error
	switch *mode {
	case "accessibility":
		err = runAccessibility()
	case "webmcp":
		err = runWebMCP()
	default:
		err = fmt.Errorf("-mode must be accessibility or webmcp: %w", errUsage)
	}
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	os.Exit(1)
}

// browser is one headless Chrome tab. failed records that an audit found
// critical or serious violations; the run keeps going so every page reports.
type browser struct {
	ctx    context.Context
	failed bool
}

// newBrowser starts headless Chrome at 1280x900 with scrollbars visible, as
// the Selenium gate did. CHROME_PATH wins; otherwise a known install is used,
// and chromedp's own lookup runs when none exists.
func newBrowser() (*browser, func(), error) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(1280, 900),
	}
	if path := chromeExecPath(); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	// The first Run on a chromedp context starts the browser and owns it, so
	// start it here on the root context rather than under a per-action timeout.
	if err := chromedp.Do(ctx); err != nil {
		cancelCtx()
		cancelAlloc()
		return nil, nil, fmt.Errorf("starting Chrome: %w", err)
	}
	ctx, cancelTimeout := context.WithTimeout(ctx, runTimeout)
	stop := func() {
		cancelTimeout()
		cancelCtx()
		cancelAlloc()
	}
	return &browser{ctx: ctx}, stop, nil
}

func chromeExecPath() string {
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

// do runs actions under actionTimeout so a missing element fails instead of hanging.
func (b *browser) do(steps ...chromedp.Action[chromedp.Void]) error {
	deadline := time.Now().Add(actionTimeout)
	for {
		ctx, cancel := context.WithTimeout(b.ctx, actionTimeout)
		err := chromedp.Do(ctx, steps...)
		cancel()
		// A realtime refresh can replace the nodes an action resolved; resolve them again.
		if err != nil && strings.Contains(err.Error(), "Could not find node") && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
}

// cdpCall runs one DevTools protocol command on the page.
func cdpCall[P, R any](b *browser, command cdp.Command[P, R], params P) (R, error) {
	ctx, cancel := context.WithTimeout(b.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run[R](ctx, func(ctx context.Context, t *chromedp.Target) (R, error) {
		return cdp.Call(ctx, t, command, params)
	})
}

// eval runs an expression in the page and decodes its value into T.
func eval[T any](b *browser, expr string, opts ...chromedp.EvaluateOption) (T, error) {
	ctx, cancel := context.WithTimeout(b.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Evaluate[T](expr, opts...))
}

func (b *browser) goTo(url string) error {
	return b.do(chromedp.Navigate(url))
}

func (b *browser) setViewport(width, height int64) error {
	return b.do(chromedp.EmulateViewport(width, height))
}

func (b *browser) typeText(selector, text string) error {
	return b.do(chromedp.SendKeys(selector, text))
}

func (b *browser) click(selector string) error {
	return b.do(chromedp.Click(selector))
}

// press sends a key to whatever element has focus, as Selenium's Actions did.
func (b *browser) press(key string) error {
	return b.do(chromedp.KeyEvent(key))
}

func (b *browser) setFiles(selector string, paths ...string) error {
	return b.do(chromedp.SetUploadFiles(selector, paths))
}

func (b *browser) deleteCookie(name, base string) error {
	_, err := cdpCall(b, network.DeleteCookies, network.DeleteCookiesParams{Name: name, URL: base})
	return err
}

func (b *browser) setCookie(name, value, base string) error {
	_, err := cdpCall(b, network.SetCookie, network.SetCookieParams{Name: name, Value: value, URL: base, Path: "/"})
	return err
}

func (b *browser) waitFor(selector string) error {
	if err := b.do(chromedp.WaitReady(selector)); err != nil {
		return fmt.Errorf("waiting for %s: %w", selector, err)
	}
	return nil
}

// waitUntil polls a page expression for up to waitTimeout.
func (b *browser) waitUntil(expr string) error {
	deadline := time.Now().Add(waitTimeout)
	for {
		if ok, err := eval[bool](b, expr); err == nil && ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s", waitTimeout, expr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *browser) location() (string, error) {
	return eval[string](b, "location.href")
}

// waitURL waits for the page URL to equal want exactly, like Selenium's urlIs.
func (b *browser) waitURL(want string) error {
	return b.waitUntil(fmt.Sprintf("location.href === %s", jsString(want)))
}

// jsString quotes s as a JavaScript string literal.
func jsString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// chromeOnly rejects browser names other than chrome, the only browser chromedp drives.
func chromeOnly(name string) error {
	if name != "chrome" {
		return fmt.Errorf("browser %q is not supported: chromedp drives Chromium only: %w", name, errUsage)
	}
	return nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
