package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestDocument(t *testing.T) {
	page, err := document(context.Background(), "alert", "dark")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-theme="dark"`, "This is a synthetic example.", "<style>", "Content-Security-Policy"} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("missing %q", want)
		}
	}
	system, err := document(context.Background(), "alert", "system")
	if err != nil || bytes.Contains(system, []byte(`data-theme="system"`)) {
		t.Fatalf("system theme: %v", err)
	}
	if _, err := document(context.Background(), "missing", "light"); err == nil {
		t.Error("unknown fixture accepted")
	}
	if _, err := document(context.Background(), "alert", "bad"); err == nil {
		t.Error("invalid theme accepted")
	}
}

func TestMissingAssets(t *testing.T) {
	for _, source := range []string{`<img src="/missing.png">`, `<link rel="stylesheet" href="x.css">`, `<div style="background:url(https://example.com/x)"></div>`, `<script src="data:text/javascript,x"></script>`} {
		if err := checkAssets(strings.NewReader(source)); err == nil {
			t.Errorf("accepted %s", source)
		}
	}
	for _, css := range []string{`a{background:url(missing.png)}`, `@import "missing.css";`} {
		if err := checkCSS(css); err == nil {
			t.Errorf("accepted %s", css)
		}
	}
	if err := checkAssets(strings.NewReader(`<img src="data:image/png;base64,AA==">`)); err != nil {
		t.Fatal(err)
	}
}

func TestFullPage(t *testing.T) {
	page, err := document(context.Background(), "login", "dark")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-theme="dark"`, `data:text/css;base64,`, `data:text/javascript;base64,`, `name="password"`} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("missing %q", want)
		}
	}
	if bytes.Contains(page, []byte(`/assets/`)) {
		t.Error("unresolved page asset")
	}
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	if _, err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString(page))); err != nil {
		t.Fatal(err)
	}
	initialized, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`!!window.Alpine && !!document.body._x_dataStack`))
	if err != nil || !initialized {
		t.Fatalf("Alpine initialization: %v, initialized=%v", err, initialized)
	}
	if _, err := offlinePage([]byte(`<html><head><link href="/assets/missing.css"></head></html>`), "light"); err == nil || !strings.Contains(err.Error(), "missing.css") {
		t.Fatalf("missing asset: %v", err)
	}
}

func TestScreenshot(t *testing.T) {
	page, err := document(context.Background(), "alert", "light")
	if err != nil {
		t.Fatal(err)
	}
	image, err := screenshot(context.Background(), page, 320, 240)
	if err != nil {
		t.Fatal(err)
	}
	full, err := document(context.Background(), "login", "light")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := screenshot(context.Background(), full, 320, 240); err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(image))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 320 || config.Height != 240 {
		t.Fatalf("viewport = %dx%d", config.Width, config.Height)
	}
}
