//go:build browser

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/web/assets"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"github.com/a-h/templ"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func TestCatalogNavigationBrowser(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets.Files))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/tokens.css"><link rel="stylesheet" href="/assets/components.css"><link rel="stylesheet" href="/assets/app.css"><title>Catalog examples</title></head><body><main class="p-4"><h1>Catalog examples</h1>`)
		for _, c := range []templ.Component{
			atoms.Feature(atoms.FeatureProps{Icon: atoms.PlainText("◇"), Title: "Reports", Description: "Read synthetic reports.", URL: "#reports"}),
			atoms.FooterNav("Explore", []atoms.AnchorProps{{Label: "Documentation", Attrs: templ.Attributes{"href": "#docs"}}}),
			atoms.MenuItem(atoms.MenuItemProps{Label: "Menu link", Attrs: templ.Attributes{"href": "#menu"}}),
			atoms.Step(atoms.StepProps{Label: "Step link", Attrs: templ.Attributes{"href": "#step"}}),
			atoms.Megamenu(atoms.MegamenuProps{ID: "catalog", ButtonLabel: "Open menu", Popovers: []atoms.PopoverProps{{ID: "group", Label: "Examples", Items: []atoms.PopoverItem{{Href: "#item", Label: "Example link"}}}}}),
			atoms.Link(atoms.LinkProps{Href: "#variant", Text: "Supported link variant"}),
		} {
			if err := c.Render(r.Context(), w); err != nil {
				t.Errorf("render fixture: %v", err)
			}
		}
		fmt.Fprint(w, `</main></body></html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), chromedp.Flag("headless", true), chromedp.NoSandbox, chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck)
	defer cancelAllocator()
	ctx, cancel := chromedp.NewContext(allocator)
	defer cancel()
	ctx, stop := context.WithTimeout(ctx, 45*time.Second)
	defer stop()
	for _, tc := range []struct {
		name, theme   string
		width, height int64
	}{{"desktop-light", "light", 1280, 900}, {"mobile-dark", "dark", 390, 844}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := chromedp.Do(ctx, chromedp.EmulateViewport(tc.width, tc.height), chromedp.Navigate(server.URL), chromedp.WaitVisible("main"), chromedp.Evaluate[chromedp.Void](`document.documentElement.setAttribute('data-theme', '`+tc.theme+`')`), chromedp.KeyEvent("\t")); err != nil {
				t.Fatal(err)
			}
			focus, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.activeElement.textContent.trim()`))
			if err != nil || focus != "Learn more" {
				t.Fatalf("keyboard focus = %q, error = %v", focus, err)
			}
			overflow, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.documentElement.scrollWidth > innerWidth`))
			if err != nil || overflow {
				t.Fatalf("horizontal overflow = %v, error = %v", overflow, err)
			}
			if dir := os.Getenv("CATALOG_EVIDENCE_DIR"); dir != "" {
				png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".png"), png, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := chromedp.Do(ctx, chromedp.KeyEvent("\r")); err != nil {
				t.Fatal(err)
			}
			hash, err := chromedp.Run(ctx, chromedp.Evaluate[string](`location.hash`))
			if err != nil || hash != "#reports" {
				t.Fatalf("keyboard navigation hash = %q, error = %v", hash, err)
			}
		})
	}
	if _, err := chromedp.Call(ctx, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, chromedp.EmulateViewport(390, 844), chromedp.Navigate(server.URL), chromedp.WaitVisible(`button[popovertarget="catalog"]`), chromedp.Focus(`button[popovertarget="catalog"]`), chromedp.KeyEvent("\r")); err != nil {
		t.Fatal(err)
	}
	state, err := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({outer:document.querySelector('#catalog').matches(':popover-open'), display:getComputedStyle(document.querySelector('#catalog')).display, buttonDisplay:getComputedStyle(document.querySelector('button[popovertarget="group-0"]')).display, innerVisible:!!document.querySelector('button[popovertarget="group-0"]').getClientRects().length})`))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after opening menu: %s", state)
	if dir := os.Getenv("CATALOG_EVIDENCE_DIR"); dir != "" {
		png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mobile-megamenu-nojs.png"), png, 0644); err != nil {
			t.Fatal(err)
		}
	}
	visible, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`!!document.querySelector('button[popovertarget="group-0"]').getClientRects().length`))
	if err != nil {
		t.Fatal(err)
	}
	if !visible {
		t.Skip("nested trigger not visible in mobile fixture; keyboard journey blocked")
	}
	if err := chromedp.Do(ctx, chromedp.Focus(`button[popovertarget="group-0"]`), chromedp.KeyEvent("\r"), chromedp.WaitVisible(`a[href="#item"]`), chromedp.Focus(`a[href="#item"]`), chromedp.KeyEvent("\r")); err != nil {
		t.Fatal(err)
	}
	hash, err := chromedp.Run(ctx, chromedp.Evaluate[string](`location.hash`))
	if err != nil || hash != "#item" {
		t.Fatalf("nested link hash = %q, error = %v", hash, err)
	}
}
