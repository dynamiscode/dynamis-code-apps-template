//go:build browser

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestFullPageBrowser(t *testing.T) {
	page, err := document(context.Background(), "login", "dark")
	if err != nil {
		t.Fatal(err)
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
