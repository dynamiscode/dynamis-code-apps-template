package web

import (
	"bytes"
	"context"
	"example.com/dynamis-code/apps-template/internal/web/components"
	atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSharedComponentsRenderAccessibleNativeControls(t *testing.T) {
	child := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<thead><tr><th scope=\"col\">Name</th></tr></thead><tbody><tr><td>Example</td></tr></tbody>")
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
		{"heading", components.PageHeading("Items <all>", "Workspace"), nil, []string{`<h1>Items &lt;all&gt;</h1>`, `class="eyebrow"`}},
		{"field", components.TextField("title", "Title", templ.Attributes{"name": "title", "required": true, "aria-describedby": "title-error", "aria-invalid": "true"}), nil, []string{`<label class="fieldset-legend" for="title">Title</label>`, `id="title"`, `name="title"`, `required`, `aria-describedby="title-error"`, `aria-invalid="true"`}},
		{"select", atoms.Select(atoms.SelectProps{ID: "locale", Name: "locale", Label: "Language", Options: []atoms.SelectOption{{Label: "English", Value: "en", Selected: true}}}), nil, []string{`for="locale"`, `id="locale"`, `name="locale"`, `<option selected value="en">English</option>`}},
		{"textarea", components.TextareaField("notes", "Notes", "<notes>", templ.Attributes{"name": "notes"}), nil, []string{`<label class="fieldset-legend" for="notes">Notes</label>`, `id="notes"`, `name="notes"`, `&lt;notes&gt;`}},
		{"field error", components.FieldError("title-error", "Required"), nil, []string{`id="title-error"`, `>Required</p>`}},
		{"button", components.SubmitButton("Save", templ.Attributes{"name": "action", "value": "save"}), nil, []string{`type="submit"`, `name="action"`, `value="save"`, `>Save</button>`}},
		{"link button", components.LinkButton("/items", "Items"), nil, []string{`href="/items"`, `class="btn btn-primary"`, `Items </a>`}},
		{"confirmation", components.ConfirmationField("confirm", "confirm", "I understand"), nil, []string{`for="confirm"`, `type="checkbox"`, `name="confirm"`, `required`, `I understand`}},
		{"destructive action", components.ConfirmButton("Delete", "Delete this item?", templ.Attributes{"name": "action", "value": "delete"}), nil, []string{`type="submit"`, `data-confirm="Delete this item?"`, `value="delete"`}},
		{"error", components.ErrorAlert("Invalid <title>"), nil, []string{`role="alert"`, `tabindex="-1"`, `Invalid &lt;title&gt;`}},
		{"status", components.StatusLine("Ready <now>", templ.Attributes{"aria-live": "polite", "id": "realtime-status"}), nil, []string{`<p class="status" role="status" aria-live="polite" id="realtime-status">Ready &lt;now&gt;</p>`}},
		{"success", components.SuccessAlert("Saved"), link, []string{`role="status"`, `Saved <a href="/items">Open</a>`}},
		{"table", components.DataTable("Items", true), child, []string{`<table`, `class="visually-hidden"`, `>Items</caption>`, `scope="col"`, `>Example</td>`}},
		{"card", components.LinkedCard("/items", "Item <one>", "Open"), nil, []string{`href="/items"`, `Item &lt;one&gt;`, `>Open</span>`}},
		{"responsive cards", components.ListCards(), link, []string{`<ul class="list-view-cards">`, `<a href="/items">Open</a>`}},
		{"empty", components.EmptyState("No items"), link, []string{`<p>No items <a href="/items">Open</a>`}},
		{"pagination", components.ListPager(viewmodel.PageData{ListPager: viewmodel.ListPager{
			Page: 2, PageCount: 3, Prev: "/items?page=1", Next: "/items?page=3",
			Links: []viewmodel.ListPageLink{{Number: 1, Href: "/items?page=1"}, {Number: 2, Href: "/items?page=2", Current: true}, {Number: 3, Href: "/items?page=3"}},
		}}), nil, []string{`href="/items?page=3"`, `aria-current="page"`, `rel="next"`}},
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
	if err := components.ListPager(viewmodel.PageData{ListPager: viewmodel.ListPager{Page: 1, PageCount: 1}}).Render(context.Background(), &out); err != nil || out.Len() != 0 {
		t.Fatalf("single-page pager = %q, %v", out.String(), err)
	}
}

func TestResponsiveListBrowserFixture(t *testing.T) {
	path := os.Getenv("COMPONENT_FIXTURE")
	if path == "" {
		t.Skip("browser fixture not requested")
	}
	tableRows := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<thead><tr><th scope="col">Item</th></tr></thead><tbody><tr><td><button id="table-action">Open item</button></td></tr></tbody>`)
		return err
	})
	cardRows := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<li><button id="card-action">Open item</button></li>`)
		return err
	})
	standaloneRows := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<thead><tr><th scope="col">History</th></tr></thead><tbody><tr><td><button id="standalone-action">Open history</button></td></tr></tbody>`)
		return err
	})
	var out bytes.Buffer
	out.WriteString(`<!doctype html><html lang="en"><head><title>List fixture</title></head><body><main><h1>Items</h1><button id="before">Before</button>`)
	for _, fixture := range []struct {
		component templ.Component
		children  templ.Component
	}{
		{components.ListTable("Items", true), tableRows},
		{components.ListCards(), cardRows},
		{components.DataTable("History", false), standaloneRows},
	} {
		if err := fixture.component.Render(templ.WithChildren(context.Background(), fixture.children), &out); err != nil {
			t.Fatal(err)
		}
	}
	out.WriteString(`</main></body></html>`)
	if err := os.WriteFile(path, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
