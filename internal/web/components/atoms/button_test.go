package atoms

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestButtonRejectsClassAttributeOverride supersedes an earlier version of
// this test that expected Button to silently ignore a "class" key in Attrs.
// Adversarial review of actionattrs correctly flagged silent-drop as a
// fail-open weakness (it masks an override attempt instead of rejecting
// it, and diverged from Anchor/Link, which already reject via
// actionattrs.From). Button now shares that same fail-closed behavior: an
// attempted "class" override panics before render.
func TestButtonRejectsClassAttributeOverride(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a class attribute override to panic")
		}
	}()
	attrs := templ.Attributes{"class": "btn-ghost", "type": "submit"}
	render(t, Button(ButtonProps{Class: "btn-primary", Label: "Save", Attrs: attrs}))
}

func TestButtonAllowsOrdinaryAttributes(t *testing.T) {
	attrs := templ.Attributes{"type": "submit"}
	got := render(t, Button(ButtonProps{Class: "btn-primary", Label: "Save", Attrs: attrs}))
	if !strings.Contains(got, `class="btn btn-primary"`) || !strings.Contains(got, `type="submit"`) {
		t.Fatalf("unexpected button: %s", got)
	}
}
