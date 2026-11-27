package molecules

import (
	"context"
	"strings"
	"testing"
)

func TestBackLinkRendersDestinationAndLabel(t *testing.T) {
	var output strings.Builder
	if err := BackLink("/workspaces/1/clients?q=Ada&sort=name", "Back to clients").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		`<a class="detail-back" href="/workspaces/1/clients?q=Ada&amp;sort=name">`,
		`<span aria-hidden="true">←</span> <u>Back to clients</u></a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("back link missing %q: %s", want, got)
		}
	}
}

func TestInlineEditorRendersMultilineDraft(t *testing.T) {
	var output strings.Builder
	props := InlineEditorProps{
		ID: "notes", Name: "value", Field: "notes", InputType: "textarea", MaxLength: "5000",
		Value: "First line\nSecond <line>", Saved: "Saved", Display: "Saved", InputLabel: "Notes",
		Draft: true, Error: "Invalid notes", Action: "/field", CancelURL: "/client",
	}
	if err := InlineEditor(props).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		`<label class="sr-only" for="notes">Notes</label>`,
		`<textarea id="notes" name="value" maxlength="5000"`,
		`data-inline-multiline`, `aria-invalid="true"`, `aria-describedby="notes-error"`,
		"First line\nSecond &lt;line&gt;</textarea>", `id="notes-error"`, `action="/field"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inline editor missing %q: %s", want, got)
		}
	}
}
