//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// openFiles signs in, opens the first workspace and its Files page. It returns the workspace path.
func openFiles(t *testing.T, tab context.Context) string {
	t.Helper()
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	click(t, tab, ".cards a")
	waitVisible(t, tab, "[data-workspace-home]")
	workspacePath := regexp.MustCompile(`^(/workspaces/[^/]+)`).FindString(eval[string](t, tab, "location.pathname"))
	navigate(t, tab, workspacePath+"/files")
	waitVisible(t, tab, "#file-upload")
	return workspacePath
}

// visibleDownloadSelector finds a visible download link for a file name.
func visibleDownloadExpr(name string) string {
	return fmt.Sprintf(`[...document.querySelectorAll("a")].some((a) => a.getClientRects().length > 0 && new RegExp(%s + "$").test(a.textContent.trim()))`, jsString(regexp.QuoteMeta(name)))
}

// tempFile writes content to a file in a temporary directory and returns its path.
func tempFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFilesLayouts ports the Files layout, theme and list-display checks from the files
// screenshot tests. Screenshot comparisons are deferred to the baseline recapture.
func TestFilesLayouts(t *testing.T) {
	for _, c := range []struct {
		name, theme string
		width       int64
	}{
		{"desktop-light", "light", 1280}, {"desktop-dark", "dark", 1280},
		{"mobile-light", "light", 390}, {"mobile-dark", "dark", 390},
	} {
		t.Run(c.name, func(t *testing.T) {
			tab := newTab(t, c.width, 900, c.theme)
			workspacePath := openFiles(t, tab)
			restore := setAccountTheme(t, tab, c.theme)
			t.Cleanup(restore)
			navigate(t, tab, workspacePath+"/files")
			waitVisible(t, tab, "#file-upload")
			expectTheme(t, tab, c.theme)
			canvas := eval[string](t, tab, `getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim()`)
			want := "#f3f6fa"
			if c.theme == "dark" {
				want = "#111826"
			}
			if canvas != want {
				t.Fatalf("canvas %q, want %q", canvas, want)
			}
			table := eval[string](t, tab, `getComputedStyle(document.querySelector(".list-view-table")).display`)
			cards := eval[string](t, tab, `getComputedStyle(document.querySelector(".list-view-cards")).display`)
			wantTable, wantCards := "block", "none"
			if c.width < 608 {
				wantTable, wantCards = "none", "grid"
			}
			if table != wantTable || cards != wantCards {
				t.Fatalf("list display table=%s cards=%s, want %s and %s", table, cards, wantTable, wantCards)
			}
		})
	}
}

// TestFilesSystemThemeFollowsOSPreference ports the system-theme test.
func TestFilesSystemThemeFollowsOSPreference(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	workspacePath := openFiles(t, tab)
	restore := setAccountTheme(t, tab, "system")
	t.Cleanup(restore)
	navigate(t, tab, workspacePath+"/files")
	expectTheme(t, tab, "system")
	canvas := `getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim()`
	setScheme := func(scheme string) {
		call(t, tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{
			{Name: "prefers-color-scheme", Value: scheme}, {Name: "prefers-reduced-motion", Value: "reduce"},
		}})
	}
	setScheme("light")
	eventually(t, tab, fmt.Sprintf("%s === %s", canvas, jsString("#f3f6fa")), "light OS preference did not give the light canvas")
	setScheme("dark")
	eventually(t, tab, fmt.Sprintf("%s === %s", canvas, jsString("#111826")), "dark OS preference did not give the dark canvas")
}

// TestFilesKeyboardAndSpanishLabels ports the keyboard and Spanish labels test. The Spanish
// list-options summary is not ported: the sort and filter disclosure was removed in #533 by design.
func TestFilesKeyboardAndSpanishLabels(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	workspacePath := openFiles(t, tab)
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	eval[any](t, tab, `document.getElementById("file-upload").focus()`)
	press(t, tab, "\t")
	expectFocused(t, tab, `.file-upload-form button[type="submit"]`)

	navigate(t, tab, "/language?locale=es&return_to="+urlQuery(workspacePath+"/files"))
	expectAttribute(t, tab, "html", "lang", "es")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Archivos")`, "Spanish Files heading not shown")
	eventually(t, tab, `[...document.querySelectorAll("label")].some((l) => l.textContent.trim() === "Archivo")`, "Spanish file label not shown")
}

func urlQuery(value string) string {
	return strings.NewReplacer("/", "%2F", "?", "%3F", "&", "%26", "=", "%3D").Replace(value)
}

// TestFilesPassAxe ports the Files axe check.
func TestFilesPassAxe(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openFiles(t, tab)
	expectAxeClean(t, tab)
}

// TestFilesUploadProgressRecoversAndRetries ports the upload progress, failure and retry test.
// The first POST is held and answered with a 422 through the Fetch domain; the page's
// progress reports 512 of 1024 bytes, as the Playwright test injected.
func TestFilesUploadProgressRecoversAndRetries(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openFiles(t, tab)
	uploadURL := eval[string](t, tab, `(() => {
		const form = document.querySelector("form[data-file-upload]");
		const originalXHR = window.XMLHttpRequest;
		// Capture the uploader's listener so native progress cannot race this synthetic event.
		window.XMLHttpRequest = class extends originalXHR {
			constructor() {
				super();
				const upload = this.upload;
				const addEventListener = upload.addEventListener.bind(upload);
				let reportProgress;
				upload.addEventListener = (type, listener, options) => {
					if (type === "progress") {
						reportProgress = listener;
						return;
					}
					return addEventListener(type, listener, options);
				};
				addEventListener("loadstart", () => {
					reportProgress?.call(upload, new ProgressEvent("progress", { lengthComputable: true, loaded: 512, total: 1024 }));
				});
			}
		};
		return form.action;
	})()`)

	held := make(chan string, 1)
	release := make(chan struct{})
	failNext := true
	call(t, tab, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{URLPattern: uploadURL, RequestStage: fetch.RequestStageRequest}}})
	_ = chromedp.Listen(tab, func(ev any) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		if paused.Request.Method != "POST" {
			go func(requestID fetch.RequestID) {
				_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
					return cdp.Call(ctx, target, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: requestID})
				})
			}(paused.RequestID)
			return
		}
		go func(requestID fetch.RequestID) {
			if failNext {
				failNext = false
				held <- string(requestID)
				<-release
				body := `<div role="alert">The selected file is invalid.</div>`
				_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
					return cdp.Call(ctx, target, fetch.FulfillRequest, fetch.FulfillRequestParams{
						RequestID:       requestID,
						ResponseCode:    422,
						ResponseHeaders: []*fetch.HeaderEntry{{Name: "Content-Type", Value: "text/html"}},
						Body:            []byte(body),
					})
				})
				return
			}
			if _, err := chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
				return cdp.Call(ctx, target, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: requestID})
			}); err != nil {
				t.Logf("continue request: %v", err)
			}
		}(paused.RequestID)
	})

	run(t, tab, chromedp.SetUploadFiles("#file-upload", []string{tempFile(t, "retry-upload.txt", []byte(strings.Repeat("x", 1024*1024)))}))
	click(t, tab, `.file-upload-form button[type="submit"]`)
	select {
	case <-held:
	case <-time.After(actionWait):
		t.Fatal("upload request was not sent")
	}
	eventually(t, tab, `document.querySelector('.file-upload-form button[type="submit"]').disabled && document.getElementById("file-upload").disabled`, "controls were not disabled while uploading")
	eventually(t, tab, `document.querySelector("[data-upload-progress]").value === 50`, "progress did not reach 50")
	eventually(t, tab, `document.querySelector("[data-upload-status]").textContent.includes("Uploading file… 50%")`, "status did not show 50%")
	close(release)
	eventually(t, tab, `document.querySelector("[data-upload-status]").textContent.includes("The selected file is invalid.")`, "failure message not shown")
	eventually(t, tab, `!document.getElementById("file-upload").disabled && !document.querySelector('.file-upload-form button[type="submit"]').disabled`, "controls did not re-enable after failure")
	eventually(t, tab, `document.querySelector("[data-upload-progress]").hidden`, "progress stayed visible after failure")

	click(t, tab, `.file-upload-form button[type="submit"]`)
	eventuallyWithin(t, tab, visibleDownloadExpr("retry-upload.txt"), "retried upload did not list the file", 15*time.Second)
}

// TestFilesViewerListsAndDownloadsWithoutUploadControls ports the read-only viewer test.
func TestFilesViewerListsAndDownloadsWithoutUploadControls(t *testing.T) {
	owner := newTab(t, 1280, 900, "")
	workspacePath := openFiles(t, owner)
	run(t, owner, chromedp.SetUploadFiles("#file-upload", []string{tempFile(t, "viewer-readable.txt", []byte("private file"))}))
	click(t, owner, `.file-upload-form button[type="submit"]`)
	eventually(t, owner, visibleDownloadExpr("viewer-readable.txt"), "owner upload not listed")

	navigate(t, owner, workspacePath+"/settings/invitations")
	fill(t, owner, "#invite-email", "viewer-417@example.com")
	selectByLabelID(t, owner, "invite-role", "viewer")
	click(t, owner, `form[action$="/settings/invitations"] button[name="action"][value="create"]`)
	waitVisible(t, owner, `input[aria-label="Copy this invitation link"]`)
	invitation := eval[string](t, owner, `document.querySelector('input[aria-label="Copy this invitation link"]').value`)

	viewer := newTab(t, 1280, 900, "")
	navigate(t, viewer, invitation)
	fill(t, viewer, "#invite-password", "viewer-long-password")
	click(t, viewer, `button[name="action"][value="register"]`)
	eventually(t, viewer, fmt.Sprintf(`new RegExp(%s + "$").test(location.pathname)`, jsString(regexp.QuoteMeta(workspacePath)+"/items")), "viewer did not reach items")
	navigate(t, viewer, workspacePath+"/files")
	eventually(t, viewer, `document.querySelectorAll(".list-view-table, .list-view-cards").length > 0`, "viewer list not rendered")
	eventually(t, viewer, visibleDownloadExpr("viewer-readable.txt"), "viewer does not list the file")
	expectCount(t, viewer, "form[data-file-upload]", 0)
	expectCount(t, viewer, `input[type="file"]`, 0)

	downloads := t.TempDir()
	call(t, viewer, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{Behavior: browser.SetDownloadBehaviorBehaviorAllow, DownloadPath: downloads})
	clickByText(t, viewer, "viewer-readable.txt")
	deadline := time.Now().Add(actionWait)
	for {
		if data, err := os.ReadFile(filepath.Join(downloads, "viewer-readable.txt")); err == nil && string(data) == "private file" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("viewer download did not arrive with the file name")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// selectByLabelID chooses an option of the select with the given id.
func selectByLabelID(t *testing.T, tab context.Context, id, value string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => { const control = document.getElementById(%s); control.value = %s; control.dispatchEvent(new Event("change", { bubbles: true })); })()`, jsString(id), jsString(value)))
}
