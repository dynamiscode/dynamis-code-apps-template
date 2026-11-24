//go:build browser

package main

import "testing"

func TestExpectFocusRoleWaitsForPageScript(t *testing.T) {
	b, stop, err := newBrowser()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := b.goTo(`data:text/html,<div role="alert" tabindex="-1">Error</div><script>setTimeout(() => document.querySelector('[role=alert]').focus(), 300)</script>`); err != nil {
		t.Fatal(err)
	}
	if err := (&a11yRun{browser: b}).expectFocusRole("alert", "alert did not receive focus"); err != nil {
		t.Fatal(err)
	}
}
