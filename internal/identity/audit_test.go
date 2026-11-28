package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestListAuditHistoryRejectsUnparseableActorRow(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceExport)

	if err := service.RecordAudit(ctx, AuditEvent{
		EventType: "test.corrupt", ActorUserID: owner.UserID, AuthMethod: "session",
		WorkspaceID: bootstrap.WorkspaceID, TargetType: "item", Action: "test.action",
		Outcome: "success", CreatedAt: service.now(),
	}); err != nil {
		t.Fatal(err)
	}
	// Actor keys are positive integers; a zero or negative stored key is corrupt.
	if _, err := db.ExecContext(ctx, `UPDATE audit_events SET actor_user_id = ? WHERE event_type = ?`, -1, "test.corrupt"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListAuditHistory(ctx, owner, bootstrap.WorkspaceID); !errors.Is(err, ErrInvalidShortID) {
		t.Fatalf("corrupt actor history error = %v, want ErrInvalidShortID", err)
	}
}

func TestListAuditHistoryIsScopedBoundedAndRedacted(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceExport)

	for index := 0; index <= AuditHistoryLimit; index++ {
		if err := service.RecordAudit(ctx, AuditEvent{
			EventType:     fmt.Sprintf("test.%03d", index),
			ActorUserID:   owner.UserID,
			AuthMethod:    "session",
			WorkspaceID:   bootstrap.WorkspaceID,
			TargetType:    "item",
			TargetKey:     987654321,
			Action:        "test.action",
			Outcome:       "success",
			RequestID:     "request-secret",
			SourceAddress: "192.0.2.1",
			Metadata:      `{"invite_secret":"invitation-secret","signed_url":"https://example.test/signed"}`,
			CreatedAt:     service.now().Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	history, err := service.ListAuditHistory(ctx, owner, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != AuditHistoryLimit {
		t.Fatalf("history length = %d, want %d", len(history), AuditHistoryLimit)
	}
	if history[0].EventType != "test.100" || history[len(history)-1].EventType != "test.001" {
		t.Fatalf("history order = %q ... %q", history[0].EventType, history[len(history)-1].EventType)
	}
	if history[0].ActorUserID != owner.UserID || history[0].TargetType != "item" ||
		history[0].Action != "test.action" || history[0].Outcome != "success" {
		t.Fatalf("history entry = %+v", history[0])
	}
	rendered := fmt.Sprintf("%+v", history[0])
	for _, forbidden := range []string{"request-secret", "192.0.2.1", "invitation-secret", "signed_url"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("history projection contains redacted data %q: %s", forbidden, rendered)
		}
	}

	adminID := insertUser(t, service, db, "admin@example.com")
	insertMembership(t, service, db, bootstrap.WorkspaceID, adminID, Admin)
	admin := mustAuthorize(t, service, adminID, bootstrap.WorkspaceID, WorkspaceExport)
	if _, err := service.ListAuditHistory(ctx, admin, bootstrap.WorkspaceID); err != nil {
		t.Fatalf("admin audit history error = %v", err)
	}

	viewerID := insertUser(t, service, db, "viewer@example.com")
	insertMembership(t, service, db, bootstrap.WorkspaceID, viewerID, Viewer)
	viewer := mustAuthorize(t, service, viewerID, bootstrap.WorkspaceID, WorkspaceRead)
	if _, err := service.ListAuditHistory(ctx, viewer, bootstrap.WorkspaceID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer audit history error = %v", err)
	}

	wrongWorkspace := insertWorkspace(t, service, db, "Wrong")
	if _, err := service.ListAuditHistory(ctx, owner, wrongWorkspace); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-workspace audit history error = %v", err)
	}
}

func TestAuditMethodTargetRejectsUntrustedMetadata(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceExport)
	if err := service.RecordAudit(ctx, AuditEvent{
		EventType: "transactions.method.updated", ActorUserID: owner.UserID, AuthMethod: "session",
		WorkspaceID: bootstrap.WorkspaceID, TargetType: "transactions_method", Action: "transactions.method.updated",
		Outcome: "success", Metadata: `{"methodId":"method-secret","after":{"name":"private"}}`, CreatedAt: service.now(),
	}); err != nil {
		t.Fatal(err)
	}
	history, err := service.ListAuditHistory(ctx, owner, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range history {
		if entry.EventType == "transactions.method.updated" {
			found = true
			if entry.TargetLabel != "" {
				t.Fatalf("untrusted target exposed: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("untrusted method audit event missing from history")
	}
}

func TestTransactionsAuditHistoryLabels(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceExport)
	events := []AuditEvent{
		{EventType: "transactions.method.updated", TargetType: "transactions_method", Metadata: `{"methodId":"cash"}`},
		{EventType: "transactions.category.rename", TargetType: "transactions_category", Metadata: `{"before":"Old","after":"New"}`},
	}
	for _, event := range events {
		event.ActorUserID, event.WorkspaceID, event.AuthMethod = owner.UserID, bootstrap.WorkspaceID, "session"
		event.Action, event.Outcome, event.CreatedAt = event.EventType, "success", service.now()
		if err := service.RecordAudit(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	history, err := service.ListAuditHistory(ctx, owner, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range history {
		switch entry.EventType {
		case "transactions.method.updated":
			found[entry.EventType] = true
			if entry.TargetType != "transactions_method" || entry.TargetLabel != "cash" {
				t.Fatalf("method label: %+v", entry)
			}
		case "transactions.category.rename":
			found[entry.EventType] = true
			if entry.TargetType != "transactions_category" || entry.CategoryPreviousName != "Old" || entry.CategoryNewName != "New" {
				t.Fatalf("category names: %+v", entry)
			}
		}
	}
	if len(found) != len(events) {
		t.Fatalf("found event types %v, want both method and category events", found)
	}
}
