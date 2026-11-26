package web

import (
	"strings"
	"testing"
)

func TestEngagementMarkdownRendering(t *testing.T) {
	source := "# Heading\n\n**Bold** and [safe](https://example.com)\n\n- One\n- Two\n\n<script>alert(1)</script>\n\n[bad](javascript:alert%281%29)\n\n![bad](javascript:alert%281%29)"
	got := renderEngagementMarkdown(source)
	for _, want := range []string{"<h1>Heading</h1>", "<strong>Bold</strong>", `href="https://example.com"`, "<li>One</li>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"<script>", `href="javascript:`, `src="javascript:`} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unsafe %q: %s", forbidden, got)
		}
	}
}
