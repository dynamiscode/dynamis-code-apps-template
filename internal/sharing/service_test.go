package sharing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestPublicSharingLifecycleAndRedaction(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthMethod = "test"
	item, err := items.NewService(db, config.SQLite, auth, 10).Create(ctx, principal, owner.WorkspaceID, "Visible title", "share-item-key", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth)
	link, err := service.Create(ctx, principal, owner.WorkspaceID, item.Item.ID, 0, identity.AuditContext{RequestID: "create-share"})
	if err != nil || link.Token == "" || link.ExpiresAt.Sub(link.CreatedAt) != DefaultLifetime {
		t.Fatalf("Create() = %+v, %v", link, err)
	}
	if len(link.Token) < 40 {
		t.Fatalf("token length = %d", len(link.Token))
	}
	var stored string
	if err := db.QueryRowContext(ctx, "SELECT token_hash FROM public_links WHERE id = ?", link.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == link.Token || strings.Contains(stored, link.Token) {
		t.Fatalf("raw token stored: %q", stored)
	}
	public, err := service.Resolve(ctx, link.Token, identity.AuditContext{RequestID: "access-share"})
	if err != nil || public.Title != "Visible title" || public.Status != string(items.Active) {
		t.Fatalf("Resolve() = %+v, %v", public, err)
	}
	links, err := service.List(ctx, principal, owner.WorkspaceID)
	if err != nil || len(links) != 1 || links[0].Token != "" {
		t.Fatalf("List() = %+v, %v", links, err)
	}
	readOnly := principal
	readOnly.Permissions = map[identity.Permission]bool{identity.ResourcesRead: true}
	if _, err := service.List(ctx, readOnly, owner.WorkspaceID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("read-only List() error = %v", err)
	}
	if err := service.Revoke(ctx, principal, owner.WorkspaceID, item.Item.ID, link.ID, identity.AuditContext{RequestID: "revoke-share"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(ctx, link.Token, identity.AuditContext{RequestID: "revoked-access"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoked Resolve() error = %v", err)
	}
	if err := service.Revoke(ctx, principal, owner.WorkspaceID, item.Item.ID, link.ID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Revoke() error = %v", err)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE event_type = 'public_share.accessed'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("access audit count = %d, %v", auditCount, err)
	}

	second, err := items.NewService(db, config.SQLite, auth, 10).Create(ctx, principal, owner.WorkspaceID, "Deleted item", "share-delete-key", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	deletedLink, err := service.Create(ctx, principal, owner.WorkspaceID, second.Item.ID, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := items.NewService(db, config.SQLite, auth, 10).Delete(ctx, principal, owner.WorkspaceID, second.Item.ID, second.Item.Version, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(ctx, deletedLink.Token, identity.AuditContext{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("deleted item Resolve() error = %v", err)
	}
}

func TestPublicSharingExpiryBounds(t *testing.T) {
	if _, err := normalizeLifetime(-time.Minute); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative lifetime error = %v", err)
	}
	if _, err := normalizeLifetime(MaximumLifetime + time.Minute); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("maximum lifetime error = %v", err)
	}
}

func TestEvaluateLinkStatus(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name      string
		found     bool
		revokedAt *time.Time
		expiresAt time.Time
		now       time.Time
		want      LinkStatus
	}{
		{
			name:      "not found",
			found:     false,
			revokedAt: nil,
			expiresAt: future,
			now:       now,
			want:      LinkNotFound,
		},
		{
			name:      "not found even if revoked and expired",
			found:     false,
			revokedAt: &past,
			expiresAt: past,
			now:       now,
			want:      LinkNotFound,
		},
		{
			name:      "revoked before expiry",
			found:     true,
			revokedAt: &past,
			expiresAt: future,
			now:       now,
			want:      LinkRevoked,
		},
		{
			name:      "revoked after expiry takes precedence as revoked",
			found:     true,
			revokedAt: &past,
			expiresAt: past,
			now:       now,
			want:      LinkRevoked,
		},
		{
			name:      "expired when now is after expiresAt",
			found:     true,
			revokedAt: nil,
			expiresAt: past,
			now:       now,
			want:      LinkExpired,
		},
		{
			name:      "expired when now is equal to expiresAt",
			found:     true,
			revokedAt: nil,
			expiresAt: now,
			now:       now,
			want:      LinkExpired,
		},
		{
			name:      "active when not revoked and now before expiresAt",
			found:     true,
			revokedAt: nil,
			expiresAt: future,
			now:       now,
			want:      LinkActive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateLinkStatus(tc.found, tc.revokedAt, tc.expiresAt, tc.now)
			if got != tc.want {
				t.Fatalf("evaluateLinkStatus(%v, %v, %v, %v) = %v, want %v",
					tc.found, tc.revokedAt, tc.expiresAt, tc.now, got, tc.want)
			}
		})
	}
}
