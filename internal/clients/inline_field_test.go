package clients

import (
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPartialUpdateWritesOnlySelectedField(t *testing.T) {
	for _, tc := range []struct {
		field string
		input func(*string) UpdateInput
		value string
		want  Input
	}{
		{"display_name", func(v *string) UpdateInput { return UpdateInput{DisplayName: v} }, "  Grace  ", Input{DisplayName: "Grace", Email: "ada@example.com", Phone: "123", Notes: "Private"}},
		{"email", func(v *string) UpdateInput { return UpdateInput{Email: v} }, "  grace@example.com  ", Input{DisplayName: "Ada", Email: "grace@example.com", Phone: "123", Notes: "Private"}},
		{"phone", func(v *string) UpdateInput { return UpdateInput{Phone: v} }, "  456  ", Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "456", Notes: "Private"}},
		{"notes", func(v *string) UpdateInput { return UpdateInput{Notes: v} }, "  Line one\nLine two  ", Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "123", Notes: "Line one\nLine two"}},
		{"notes", func(v *string) UpdateInput { return UpdateInput{Notes: v} }, strings.Repeat("界", 5000), Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "123", Notes: strings.Repeat("界", 5000)}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			f := newFixture(t)
			svc := newOutboxService(t, f, config.SQLite)
			created, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "123", Notes: "Private"}, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			var other []string
			for _, field := range []string{"display_name", "email", "phone", "notes"} {
				if field != tc.field {
					other = append(other, field)
				}
			}
			_, err = f.db.ExecContext(f.ctx, `CREATE TRIGGER reject_unrelated BEFORE UPDATE OF `+strings.Join(other, ", ")+` ON clients BEGIN SELECT RAISE(FAIL, 'unrelated column updated'); END`)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := svc.Update(f.ctx, f.member, f.workspaceID, created.ID, 1, tc.input(&tc.value), identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			if updated.DisplayName != tc.want.DisplayName || updated.Email != tc.want.Email || updated.Phone != tc.want.Phone || updated.Notes != tc.want.Notes || updated.Version != 2 {
				t.Fatalf("updated client = %+v", updated)
			}
			stored, err := svc.Get(f.ctx, f.viewer, f.workspaceID, created.ID)
			if err != nil || stored.DisplayName != tc.want.DisplayName || stored.Email != tc.want.Email || stored.Phone != tc.want.Phone || stored.Notes != tc.want.Notes || stored.Version != 2 {
				t.Fatalf("stored client = %+v, %v", stored, err)
			}
			requireAudit(t, f, wantAudit{"client.update", "client.updated", "client", int64(created.ID), f.member, `{"version":2}`})
			events := requireOutbox(t, f, config.SQLite, "client.created", "client.updated")
			if events[1].Data.DisplayName != tc.want.DisplayName || events[1].Data.Email != tc.want.Email || events[1].Data.Phone != tc.want.Phone || events[1].Data.Notes != tc.want.Notes || events[1].Data.Version != 2 {
				t.Fatalf("updated webhook = %+v", events[1])
			}
			changes, err := svc.Changes(f.ctx, f.viewer, f.workspaceID, changeOrigin, 10)
			if err != nil || len(changes.Changes) != 2 || changes.Changes[1].Action != "updated" || changes.Changes[1].ClientVersion != 2 {
				t.Fatalf("changes = %+v, %v", changes, err)
			}
		})
	}
}

func TestPartialUpdateRejectsInvalidStaleUnauthorizedAndCrossWorkspace(t *testing.T) {
	f := newFixture(t)
	created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "123", Notes: "Private"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	name, badEmail, longPhone, longNotes := "Grace", "bad-email", strings.Repeat("x", 51), strings.Repeat("界", 5001)
	for _, tc := range []struct {
		name      string
		actor     identity.Principal
		workspace identity.WorkspaceID
		version   int64
		input     UpdateInput
		want      error
	}{
		{"viewer", f.viewer, f.workspaceID, 1, UpdateInput{DisplayName: &name}, identity.ErrForbidden},
		{"stale", f.member, f.workspaceID, 2, UpdateInput{DisplayName: &name}, ErrPreconditionFailed},
		{"zero version", f.member, f.workspaceID, 0, UpdateInput{DisplayName: &name}, ErrInvalidInput},
		{"bad email", f.member, f.workspaceID, 1, UpdateInput{Email: &badEmail}, ErrInvalidEmail},
		{"long phone", f.member, f.workspaceID, 1, UpdateInput{Phone: &longPhone}, ErrInvalidInput},
		{"long notes", f.member, f.workspaceID, 1, UpdateInput{Notes: &longNotes}, ErrInvalidInput},
		{"stale notes", f.member, f.workspaceID, 2, UpdateInput{Notes: &name}, ErrPreconditionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.service.Update(f.ctx, tc.actor, tc.workspace, created.ID, tc.version, tc.input, identity.AuditContext{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Update error = %v, want %v", err, tc.want)
			}
		})
	}
	other, err := f.auth.CreateWorkspace(f.ctx, f.owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := principalFor(t, f.auth, f.owner.UserID, other, identity.ResourcesWrite)
	if _, err := f.service.Update(f.ctx, otherOwner, other, created.ID, 1, UpdateInput{DisplayName: &name}, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace update error = %v", err)
	}
	stored, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, created.ID)
	if err != nil || stored.Version != 1 || stored.DisplayName != "Ada" || stored.Email != "ada@example.com" || stored.Phone != "123" || stored.Notes != "Private" {
		t.Fatalf("client after rejected writes = %+v, %v", stored, err)
	}
	var updates int
	if err := f.db.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND target_key = ?`, "client.updated", int64(created.ID)).Scan(&updates); err != nil || updates != 0 {
		t.Fatalf("rejected update audits = %d, %v", updates, err)
	}
}

func TestPostgresConcurrentPartialUpdatesRejectStaleVersion(t *testing.T) {
	f := newPostgresFixture(t)
	created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada", Email: "ada@example.com", Phone: "123", Notes: "Private"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	name, phone := "Grace", "456"
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, input := range []UpdateInput{{DisplayName: &name}, {Phone: &phone}} {
		go func(input UpdateInput) {
			<-start
			_, err := f.service.Update(f.ctx, f.member, f.workspaceID, created.ID, created.Version, input, identity.AuditContext{})
			results <- err
		}(input)
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPreconditionFailed):
			conflicts++
		default:
			t.Fatalf("concurrent update error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates = %d successes, %d conflicts", successes, conflicts)
	}
	stored, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, created.ID)
	if err != nil || stored.Version != 2 || stored.Email != "ada@example.com" || stored.Notes != "Private" ||
		!((stored.DisplayName == "Grace" && stored.Phone == "123") || (stored.DisplayName == "Ada" && stored.Phone == "456")) {
		t.Fatalf("stored client after concurrent updates = %+v, %v", stored, err)
	}
}
