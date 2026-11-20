package clients

import (
	"slices"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// TestSearchMatchesEveryVisibleTextField checks that a search reaches the common text fields, a text
// custom field, and the label of a single-choice field, and that it finds nothing for text that no field holds.
func TestSearchMatchesEveryVisibleTextField(t *testing.T) {
	f := newFixture(t)
	company, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Company", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	tier := defineChoiceField(t, f, "Tier")
	gold := addOptionOK(t, f, tier, "Gold")
	addOptionOK(t, f, tier, "Silver")

	ada, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{
		DisplayName: "Ada Lovelace", Email: "ada@example.test", Phone: "+1 555 0100", Notes: "Prefers evening calls",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, ada.ID, ada.Version, company.ID, "Analytical Engines", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	bo, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Bo Chen"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, bo.ID, bo.Version, tier.ID, gold.ID.String(), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]ClientID{
		"lovelace":   {ada.ID},
		"example":    {ada.ID},
		"555":        {ada.ID},
		"evening":    {ada.ID},
		"analytical": {ada.ID},
		"gold":       {bo.ID},
		"nothing":    nil,
	}
	for search, want := range cases {
		page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 10, Search: search})
		if err != nil {
			t.Fatalf("List(%q) error = %v", search, err)
		}
		got := make([]ClientID, 0, len(page.Clients))
		for _, c := range page.Clients {
			got = append(got, c.ID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("List(%q) = %v, want %v", search, got, want)
		}
	}
}
