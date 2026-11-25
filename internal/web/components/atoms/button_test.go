package atoms

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestButtonIgnoresClassAttributesWithoutMutatingCaller(t *testing.T) {
	attrs := templ.Attributes{"class": "btn-ghost", "CLASS": "border-0", "type": "submit"}
	got := render(t, Button(ButtonProps{Class: "btn-primary", Label: "Save", Attrs: attrs}))
	if !strings.Contains(got, `class="btn btn-primary"`) || strings.Contains(got, "btn-ghost") || strings.Contains(got, "border-0") || !strings.Contains(got, `type="submit"`) {
		t.Fatalf("unexpected button: %s", got)
	}
	if attrs["class"] != "btn-ghost" || attrs["CLASS"] != "border-0" {
		t.Fatal("Button modified caller attributes")
	}
}
