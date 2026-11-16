package web

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestSharedComponentsRenderAccessibleNativeControls(t *testing.T) {
	child := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<thead><tr><th scope=\"col\">Name</th></tr></thead><tbody><tr><td>Example</td></tr></tbody>")
		return err
	})
	option := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<option value="en" selected>English</option>`)
		return err
	})
	link := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<a href="/items">Open</a>`)
		return err
	})
	tests := []struct {
		name      string
		component templ.Component
		children  templ.Component
		want      []string
	}{
		{"heading", pageHeading("Items <all>", "Workspace"), nil, []string{`<h1>Items &lt;all&gt;</h1>`, `class="eyebrow"`}},
		{"field", textField("title", "Title", templ.Attributes{"name": "title", "required": true, "aria-describedby": "title-error", "aria-invalid": "true"}), nil, []string{`<label for="title">Title</label>`, `id="title"`, `name="title"`, `required`, `aria-describedby="title-error"`, `aria-invalid="true"`}},
		{"select", selectField("locale", "Language", templ.Attributes{"name": "locale"}), option, []string{`<label for="locale">Language</label>`, `id="locale"`, `name="locale"`, `value="en" selected`}},
		{"textarea", textareaField("notes", "Notes", "<notes>", templ.Attributes{"name": "notes"}), nil, []string{`<label for="notes">Notes</label>`, `id="notes"`, `name="notes"`, `&lt;notes&gt;`}},
		{"field error", fieldError("title-error", "Required"), nil, []string{`id="title-error"`, `>Required</p>`}},
		{"button", submitButton("Save", templ.Attributes{"name": "action", "value": "save"}), nil, []string{`type="submit"`, `name="action"`, `value="save"`, `>Save</button>`}},
		{"link button", linkButton("/items", "Items"), nil, []string{`href="/items"`, `>Items</a>`}},
		{"confirmation", confirmationField("confirm", "confirm", "I understand"), nil, []string{`for="confirm"`, `type="checkbox"`, `name="confirm"`, `required`, `I understand`}},
		{"destructive action", confirmButton("Delete", "Delete this item?", templ.Attributes{"name": "action", "value": "delete"}), nil, []string{`type="submit"`, `data-confirm="Delete this item?"`, `value="delete"`}},
		{"error", errorAlert("Invalid <title>"), nil, []string{`role="alert"`, `tabindex="-1"`, `Invalid &lt;title&gt;`}},
		{"success", successAlert("Saved"), link, []string{`role="status"`, `Saved <a href="/items">Open</a>`}},
		{"table", dataTable("Items", true), child, []string{`<table`, `class="visually-hidden"`, `>Items</caption>`, `scope="col"`, `>Example</td>`}},
		{"card", linkedCard("/items", "Item <one>", "Open"), nil, []string{`href="/items"`, `Item &lt;one&gt;`, `>Open</span>`}},
		{"responsive cards", listCards(), link, []string{`<ul class="list-view-cards">`, `<a href="/items">Open</a>`}},
		{"responsive card", listCard(), link, []string{`<li><a href="/items">Open</a></li>`}},
		{"empty", emptyState("No items"), link, []string{`<p>No items <a href="/items">Open</a>`}},
		{"pagination", nextPage("/items?cursor=a&sort=title", "Next"), nil, []string{`href="/items?cursor=a&amp;sort=title"`, `>Next</a>`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			ctx := context.Background()
			if tt.children != nil {
				ctx = templ.WithChildren(ctx, tt.children)
			}
			if err := tt.component.Render(ctx, &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("rendered %q; missing %q", out.String(), want)
				}
			}
		})
	}
	var out bytes.Buffer
	if err := nextPage("", "Next").Render(context.Background(), &out); err != nil || out.Len() != 0 {
		t.Fatalf("empty next page = %q, %v", out.String(), err)
	}
}
