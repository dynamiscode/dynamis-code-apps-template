//go:build browser

package browsertest

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func TestNotesEditorMarkdownCSP(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	call(t, tab, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `
		document.addEventListener('securitypolicyviolation', event => {
			const violations = JSON.parse(sessionStorage.getItem('notesCSPViolations') || '[]');
			violations.push({ directive: event.violatedDirective, blocked: event.blockedURI });
			sessionStorage.setItem('notesCSPViolations', JSON.stringify(violations));
		});
	`})
	openClients(t, tab)
	click(t, tab, `a[href$="/clients/new"]`)
	fillByLabel(t, tab, "Display name", "Markdown CSP browser fixture")
	clickByText(t, tab, "Create client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client detail did not open")
	waitVisible(t, tab, `[data-notes-editor] summary`)

	policy := eval[string](t, tab, `fetch(location.href).then(response => response.headers.get('Content-Security-Policy') || '')`, awaitPromise)
	if !strings.Contains(policy, "script-src 'self'") || !strings.Contains(policy, "style-src 'self'") || strings.Contains(policy, "'unsafe-inline'") {
		t.Fatalf("expected production CSP without unsafe-inline, got %q", policy)
	}

	click(t, tab, `[data-notes-editor] summary`)
	waitVisible(t, tab, `[data-notes-rich] .ProseMirror`)
	click(t, tab, `[data-notes-format="strong"]`)
	run(t, tab, chromedp.SendKeys(`[data-notes-rich] .ProseMirror`, "bold"))
	click(t, tab, `[data-notes-format="strong"]`)
	run(t, tab, chromedp.SendKeys(`[data-notes-rich] .ProseMirror`, " text"))
	click(t, tab, `[data-notes-format="em"]`)
	run(t, tab, chromedp.SendKeys(`[data-notes-rich] .ProseMirror`, " italic"))
	eventually(t, tab, `document.querySelector('[data-notes-rich] em')?.textContent.trim() === 'italic'`, "visual editor did not apply italic")
	eventually(t, tab, `document.querySelector('[data-notes-rich] strong')?.textContent === 'bold'`, "visual editor did not apply bold")
	click(t, tab, `[data-notes-form] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('[data-notes-editor] summary strong')?.textContent === 'bold' && document.querySelector('[data-notes-editor] summary em')?.textContent.trim() === 'italic' && document.querySelector('[data-notes-editor] summary').textContent.includes(' text')`, "formatted notes were not saved and rendered")

	click(t, tab, `[data-notes-editor] summary`)
	waitVisible(t, tab, `[data-notes-rich] .ProseMirror strong`)
	click(t, tab, `[data-notes-source-mode]`)
	waitVisible(t, tab, `[data-notes-source]`)
	const unsupported = "# Heading\n\n> quote\n\n| A | B |\n| --- | --- |\n| 1 | 2 |"
	fill(t, tab, `[data-notes-source]`, unsupported)
	click(t, tab, `[data-notes-form] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('[data-notes-editor] summary h1')?.textContent === 'Heading'`, "source notes were not saved and rendered")

	click(t, tab, `[data-notes-editor] summary`)
	waitVisible(t, tab, `[data-notes-warning]`)
	waitVisible(t, tab, `[data-notes-source]`)
	if got := eval[string](t, tab, `document.querySelector('[data-notes-source]').value`); got != unsupported {
		t.Fatalf("unsupported Markdown changed on reopen: got %q, want %q", got, unsupported)
	}
	click(t, tab, `[data-notes-source-mode]`)
	if !eval[bool](t, tab, `!document.querySelector('[data-notes-source]').hidden && !document.querySelector('[data-notes-warning]').hidden && document.querySelector('[data-notes-format="strong"]').disabled`) {
		t.Fatal("unsupported Markdown did not remain in source mode with a warning")
	}
	click(t, tab, `[data-notes-form] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('[data-notes-editor] summary h1')?.textContent === 'Heading'`, "unchanged source was not saved")
	click(t, tab, `[data-notes-editor] summary`)
	waitVisible(t, tab, `[data-notes-warning]`)
	if got := eval[string](t, tab, `document.querySelector('[data-notes-source]').value`); got != unsupported {
		t.Fatalf("unsupported Markdown changed after save: got %q, want %q", got, unsupported)
	}

	if violations := eval[string](t, tab, `sessionStorage.getItem('notesCSPViolations') || '[]'`); violations != "[]" {
		t.Fatalf("CSP violations during editor journey: %s", violations)
	}
	// A blocked inline script proves the listener detects real browser violations.
	eval[any](t, tab, `(() => { const script = document.createElement('script'); script.textContent = 'window.notesInlineScriptRan = true'; document.body.appendChild(script); })()`)
	eventually(t, tab, `JSON.parse(sessionStorage.getItem('notesCSPViolations') || '[]').some(v => v.directive.startsWith('script-src') && v.blocked === 'inline')`, "CSP listener did not detect a blocked inline script")
	if eval[bool](t, tab, `window.notesInlineScriptRan === true`) {
		t.Fatalf("inline script ran despite CSP %q", policy)
	}
}
