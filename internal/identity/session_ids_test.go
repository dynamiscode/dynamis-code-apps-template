package identity

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// sessionRowKey returns the integer row key behind a session short ID, for
// tests that write the sessions table directly.
func sessionRowKey(t *testing.T, shortID string) int64 {
	t.Helper()
	key, err := id.DecodeKey(id.TypeSession, shortID)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// Session short IDs are exposed for listing, revocation, and audit targets.
// They are never accepted as the session secret, and a malformed, integer,
// or foreign ID gets the same invalid-session error as an unknown session.
func TestSessionShortIDsAreExposedAndNeverAccepted(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)

	session, err := service.CreateSession(ctx, bootstrap.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := id.DecodeKey(id.TypeSession, session.ID); err != nil {
		t.Fatalf("session ID %q is not a session short ID: %v", session.ID, err)
	}
	if regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(session.ID) {
		t.Fatalf("session ID %q is still a 32-hex value", session.ID)
	}
	var key int64
	if err := db.QueryRow("SELECT id FROM sessions WHERE user_id = ?", bootstrap.UserID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if got := id.EncodeKey(id.TypeSession, key); got != session.ID {
		t.Fatalf("session ID = %q, want %q", session.ID, got)
	}

	// The row key and its short ID never work as the session secret.
	for _, candidate := range []string{session.ID, strconv.FormatInt(key, 10)} {
		if _, err := service.AuthenticateSession(ctx, candidate); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("AuthenticateSession(%q) error = %v, want ErrInvalidSession", candidate, err)
		}
	}
	// The short ID is the session's public ID for CSRF; the integer key is not.
	if !service.VerifyCSRF(ctx, session.ID, session.CSRFSecret) {
		t.Fatal("VerifyCSRF rejected the session's own short ID")
	}
	if service.VerifyCSRF(ctx, strconv.FormatInt(key, 10), session.CSRFSecret) {
		t.Fatal("VerifyCSRF accepted an integer row key")
	}

	// Malformed, integer, and foreign-type IDs revoke nothing and report invalid-session.
	other := insertUser(t, service, db, "other-session@example.com")
	otherSession, err := service.CreateSession(ctx, other, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{
		"", "!!", strconv.FormatInt(key, 10), "1",
		id.EncodeKey(id.TypeAuditEvent, key), otherSession.ID,
	} {
		if _, err := service.RevokeSession(ctx, bootstrap.UserID, candidate, AuditContext{}); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("RevokeSession(%q) error = %v, want ErrInvalidSession", candidate, err)
		}
	}
	if _, err := service.AuthenticateSession(ctx, otherSession.Secret); err != nil {
		t.Fatalf("another user's session was revoked: %v", err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); err != nil {
		t.Fatalf("session was revoked by a rejected ID: %v", err)
	}

	sessions, err := service.ListSessions(ctx, bootstrap.UserID)
	if err != nil || len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("ListSessions() = %+v, %v; want one session with ID %q", sessions, err, session.ID)
	}

	var targetID string
	if err := db.QueryRow(`SELECT target_id FROM audit_events
		WHERE event_type = 'session.created' AND actor_user_id = ? ORDER BY id DESC LIMIT 1`, bootstrap.UserID.String()).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if targetID != session.ID {
		t.Fatalf("session.created target_id = %q, want %q", targetID, session.ID)
	}

	if _, err := service.RevokeSession(ctx, bootstrap.UserID, session.ID, AuditContext{}); err != nil {
		t.Fatalf("RevokeSession(own short ID) error = %v", err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked session error = %v", err)
	}
}
