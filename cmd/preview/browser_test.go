//go:build browser

package main

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestPreviewBrowser(t *testing.T) {
	address := startPreview(t, buildPreview(t), "empty")
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(),
		chromedp.Flag("headless", true), chromedp.NoSandbox, chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck)
	defer cancelAllocator()
	ctx, cancel := chromedp.NewContext(allocator)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()
	steps := []struct {
		name   string
		action chromedp.Action[chromedp.Void]
	}{
		{"open workspace page", chromedp.Navigate(address)},
		{"show workspace page", chromedp.WaitVisible("main h1")},
		{"open gallery", chromedp.Navigate(address + "/components")},
		{"open alert from gallery", chromedp.Click(`a[href="/components/alert?state=error"]`)},
		{"show alert", chromedp.WaitVisible(`[role="alert"]`)},
	}
	for _, step := range steps {
		if err := chromedp.Do(ctx, step.action); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	shown, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('[role="alert"]').textContent.includes('This is a simulated error.')`))
	if err != nil || !shown {
		t.Fatalf("error alert: shown=%v, error=%v", shown, err)
	}
}
