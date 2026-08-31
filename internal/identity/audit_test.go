package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

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
			TargetID:      "target-secret",
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
	for _, forbidden := range []string{"target-secret", "request-secret", "192.0.2.1", "invitation-secret", "signed_url"} {
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
