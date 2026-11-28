package intake

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
)

func TestLinkLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	token, err := svc.Create(ctx, owner, boot.WorkspaceID, "Consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT encrypted_token, token_key_version FROM public_intake_links WHERE workspace_id = ?`, boot.WorkspaceID).Scan(&ciphertext, &version); err != nil || ciphertext == token || version != 1 {
		t.Fatalf("encrypted storage: version=%d err=%v", version, err)
	}
	if got, err := svc.GetToken(ctx, owner, boot.WorkspaceID); err != nil || got != token {
		t.Fatalf("retrieved token mismatch: %v", err)
	}
	if _, err := svc.GetToken(ctx, identity.Principal{}, boot.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized retrieval: %v", err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, "Consent", identity.AuditContext{}); !errors.Is(err, ErrActive) {
		t.Fatalf("duplicate create: %v", err)
	}
	workspace, consent, err := svc.Resolve(ctx, token)
	if err != nil || workspace != boot.WorkspaceID || consent != "Consent" {
		t.Fatalf("resolve: %v %q %v", workspace, consent, err)
	}
	if err := svc.SetConsent(ctx, owner, boot.WorkspaceID, "New consent", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, consent, err := svc.Resolve(ctx, token); err != nil || consent != "New consent" {
		t.Fatalf("updated consent: %q %v", consent, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE public_intake_links SET encrypted_token = NULL, token_key_version = NULL WHERE workspace_id = ?`, boot.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetToken(ctx, owner, boot.WorkspaceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy retrieval: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, token); err != nil {
		t.Fatalf("legacy link: %v", err)
	}
	svc.ConfigureEncryption(secretbox.New(nil, 0))
	if _, err := svc.Rotate(ctx, owner, boot.WorkspaceID, "Updated", identity.AuditContext{}); !errors.Is(err, secretbox.ErrNoPrimaryKey) {
		t.Fatalf("rotate without key: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, token); err != nil {
		t.Fatalf("failed rotation invalidated link: %v", err)
	}
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	rotated, err := svc.Rotate(ctx, owner, boot.WorkspaceID, "Updated", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Resolve(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, rotated); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.GetToken(ctx, owner, boot.WorkspaceID); err != nil || got != rotated {
		t.Fatalf("rotated retrieval: %v", err)
	}
	if err := svc.Disable(ctx, owner, boot.WorkspaceID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, owner, boot.WorkspaceID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disable inactive: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, rotated); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled token: %v", err)
	}
	svc.ConfigureEncryption(secretbox.New(nil, 0))
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, "Again", identity.AuditContext{}); !errors.Is(err, secretbox.ErrNoPrimaryKey) {
		t.Fatalf("create without key: %v", err)
	}
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	newToken, err := svc.Create(ctx, owner, boot.WorkspaceID, "Again", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rotate(ctx, identity.Principal{}, boot.WorkspaceID, "No", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized: %v", err)
	}
	var memberID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "member@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, memberID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Authorize(ctx, memberID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetToken(ctx, member, boot.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member retrieval: %v", err)
	}
	if _, err := svc.GetSettings(ctx, member, boot.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member settings: %v", err)
	}
	if err := svc.Disable(ctx, member, boot.WorkspaceID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member disable: %v", err)
	}
	if workspace, consent, err := svc.Resolve(ctx, newToken); err != nil || workspace != boot.WorkspaceID || consent != "Again" {
		t.Fatalf("link after forbidden changes: workspace=%v consent=%q err=%v", workspace, consent, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type LIKE 'public_intake.%'`, boot.WorkspaceID).Scan(&count); err != nil || count != 5 {
		t.Fatalf("audit count: %d, %v", count, err)
	}
}
