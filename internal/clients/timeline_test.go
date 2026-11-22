package clients

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestTimelineSQLite(t *testing.T)   { exerciseTimeline(t, newFixture(t)) }
func TestTimelinePostgres(t *testing.T) { exerciseTimeline(t, newPostgresFixture(t)) }

func exerciseTimeline(t *testing.T, f fixture) {
	t.Helper()
	ctx, ws, audit := f.ctx, f.workspaceID, identity.AuditContext{}
	client, err := f.service.Create(ctx, f.owner, ws, Input{DisplayName: "Timeline"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.service.Create(ctx, f.owner, ws, Input{DisplayName: "Other"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.service.now = func() time.Time { return now }
	first, err := f.service.AddTimelineEntry(ctx, f.member, ws, client.ID, TimelineInput{Type: "note", Text: "First"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.timeline.add", "client.timeline_added", "client", int64(client.ID), f.member, "{}"})
	now = now.Add(time.Second)
	second, err := f.service.AddTimelineEntry(ctx, f.member, ws, client.ID, TimelineInput{Type: "call", Channel: "phone", Text: "Second", CorrectsID: &first.ID}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if first.AuthorID == nil || *first.AuthorID != f.member.UserID || !first.CreatedAt.Equal(now.Add(-time.Second)) || second.CorrectsID == nil || *second.CorrectsID != first.ID {
		t.Fatalf("entries: %+v %+v", first, second)
	}
	entries, err := f.service.ListTimelineEntries(ctx, f.viewer, ws, client.ID)
	if err != nil || len(entries) != 2 || entries[0].ID != first.ID || entries[1].ID != second.ID {
		t.Fatalf("list = %+v, %v", entries, err)
	}
	for _, tc := range []struct {
		input TimelineInput
		want  error
	}{
		{TimelineInput{Type: "note", Text: " "}, ErrTimelineTextRequired},
		{TimelineInput{Type: "unknown", Text: "x"}, ErrTimelineType},
		{TimelineInput{Type: "note", Channel: "unknown", Text: "x"}, ErrTimelineChannel},
		{TimelineInput{Type: "note", Text: strings.Repeat("é", 2001)}, ErrTimelineTextTooLong},
		{TimelineInput{Type: "note", Text: "x", CorrectsID: &first.ID}, ErrTimelineCorrection},
	} {
		target := client.ID
		if errors.Is(tc.want, ErrTimelineCorrection) {
			target = other.ID
		}
		if _, err := f.service.AddTimelineEntry(ctx, f.member, ws, target, tc.input, audit); !errors.Is(err, tc.want) {
			t.Errorf("input %+v: %v, want %v", tc.input, err, tc.want)
		}
	}
	missing := TimelineEntryID(999999999)
	if _, err := f.service.AddTimelineEntry(ctx, f.member, ws, client.ID, TimelineInput{Type: "note", Text: "x", CorrectsID: &missing}, audit); !errors.Is(err, ErrTimelineCorrection) {
		t.Fatalf("missing correction: %v", err)
	}
	if _, err := f.service.AddTimelineEntry(ctx, forgedPrincipal(f.viewer), ws, client.ID, TimelineInput{Type: "note", Text: "x"}, audit); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer add: %v", err)
	}
	if got, err := f.service.ListTimelineEntries(ctx, f.viewer, ws, 0); !errors.Is(err, ErrNotFound) || len(got) != 0 {
		t.Fatalf("missing client: %v %v", got, err)
	}
	ws2, err := f.auth.CreateWorkspace(ctx, f.owner, identity.WorkspaceCreateInput{Name: "Other"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	actor := principalFor(t, f.auth, f.owner.UserID, ws2, identity.ResourcesRead)
	if got, err := f.service.ListTimelineEntries(ctx, actor, ws2, client.ID); !errors.Is(err, ErrNotFound) || len(got) != 0 {
		t.Fatalf("foreign client: %v %v", got, err)
	}
	changes, err := f.service.Changes(ctx, f.viewer, ws, changeOrigin, 20)
	if err != nil || len(changes.Changes) != 4 || changes.Changes[0].Action != "timeline_added" {
		t.Fatalf("changes: %+v %v", changes, err)
	}
	if got, err := f.service.ListTimelineEntries(ctx, f.viewer, ws2, client.ID); !errors.Is(err, ErrNotFound) || len(got) != 0 {
		t.Fatalf("unauthorized client: %v %v", got, err)
	}
	for _, entryType := range []string{"note", "call", "visit", "document", "update", "change", "delay"} {
		for _, channel := range []string{"", "phone", "whatsapp", "email", "in_person", "other"} {
			if _, err := f.service.AddTimelineEntry(ctx, f.owner, ws, other.ID, TimelineInput{Type: entryType, Channel: channel, Text: strings.Repeat("é", 2000)}, audit); err != nil {
				t.Fatalf("valid type %q channel %q: %v", entryType, channel, err)
			}
		}
	}
	// Deleting an author anonymizes entries without breaking corrections.
	if _, err := f.db.ExecContext(ctx, database.Rebind(f.service.driver, "DELETE FROM users WHERE id = ?"), f.member.UserID); err != nil {
		t.Fatal(err)
	}
	entries, err = f.service.ListTimelineEntries(ctx, f.viewer, ws, client.ID)
	if err != nil || len(entries) != 2 || entries[0].AuthorID != nil || entries[1].CorrectsID == nil {
		t.Fatalf("anonymized: %+v %v", entries, err)
	}
	if err := f.service.Delete(ctx, f.owner, ws, client.ID, audit); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.db.QueryRowContext(ctx, database.Rebind(f.service.driver, "SELECT COUNT(*) FROM client_timeline_entries WHERE client_id = ?"), client.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("client cascade: %d %v", count, err)
	}
	_, err = f.service.AddTimelineEntry(ctx, f.owner, ws, other.ID, TimelineInput{Type: "note", Text: "workspace"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ExecContext(ctx, database.Rebind(f.service.driver, "DELETE FROM workspaces WHERE id = ?"), ws); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRowContext(ctx, database.Rebind(f.service.driver, "SELECT COUNT(*) FROM client_timeline_entries WHERE workspace_id = ?"), ws).Scan(&count); err != nil || count != 0 {
		t.Fatalf("workspace cascade: %d %v", count, err)
	}
}
