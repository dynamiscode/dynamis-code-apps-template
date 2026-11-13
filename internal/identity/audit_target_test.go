package identity

import (
	"context"
	"database/sql"
	"testing"
)

// A keyed audit target stores its type code and integer key; a target without
// an integer key (for example a session or MFA challenge credential) stores NULL.
func TestAuditTargetStoresTypedKeyOrNull(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceExport)

	if err := service.RecordAudit(ctx, AuditEvent{
		EventType: "test.keyed", ActorUserID: owner.UserID, AuthMethod: "session",
		WorkspaceID: bootstrap.WorkspaceID, TargetType: "user", TargetKey: int64(owner.UserID),
		Action: "test.action", Outcome: "success", CreatedAt: service.now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordAudit(ctx, AuditEvent{
		EventType: "test.keyless", ActorUserID: owner.UserID, AuthMethod: "session",
		WorkspaceID: bootstrap.WorkspaceID, TargetType: "session", Action: "test.action",
		Outcome: "success", CreatedAt: service.now(),
	}); err != nil {
		t.Fatal(err)
	}

	var targetType string
	var targetKey sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT target_type, target_key FROM audit_events WHERE event_type = ?`, "test.keyed").Scan(&targetType, &targetKey); err != nil {
		t.Fatal(err)
	}
	if targetType != "user" || !targetKey.Valid || targetKey.Int64 != int64(owner.UserID) {
		t.Fatalf("keyed target = %q/%v; want user/%d", targetType, targetKey, owner.UserID)
	}
	if err := db.QueryRowContext(ctx, `SELECT target_key FROM audit_events WHERE event_type = ?`, "test.keyless").Scan(&targetKey); err != nil {
		t.Fatal(err)
	}
	if targetKey.Valid {
		t.Fatalf("keyless target key = %d; want NULL", targetKey.Int64)
	}
}
