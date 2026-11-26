package actionattrs

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMarkdownBoundsLinksAndRejectsHTML(t *testing.T) {
	input := "**bold** [link](https://example.com) [bad](javascript:alert%281%29)\n\n<script>alert(1)</script>"
	var output bytes.Buffer
	if err := Markdown(input).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"<strong>bold</strong>", `href="https://example.com"`, `class="border rounded px-1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, forbidden := range []string{"<script", `href="javascript:`} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unsafe %q in %s", forbidden, got)
		}
	}
	preview, err := MarkdownHTML(input)
	if err != nil || preview != got {
		t.Fatalf("preview differs from rendered component: %q, %v", preview, err)
	}
}

func TestMarkdownAppliesHrefPolicy(t *testing.T) {
	input := "[external](//evil.example/path) [ftp](ftp://example.com) [relative](/clients) [secure](https://example.com)"
	got, err := MarkdownHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`href="//evil.example/path"`, `href="ftp://example.com"`} {
		if strings.Contains(got, forbidden) {
			t.Errorf("unsafe %q in %s", forbidden, got)
		}
	}
	for _, want := range []string{"external", "ftp", `href="/clients"`, `href="https://example.com"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestMarkdownHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := Markdown("text").Render(ctx, &output); err != context.Canceled || output.Len() != 0 {
		t.Fatalf("got %v and %q", err, output.String())
	}
}
