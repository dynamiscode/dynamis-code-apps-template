package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"errors"
	"fmt"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestMFAEnrollmentLoginRecoveryAndReplay(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	service, err := NewServiceWithMFA(db, config.SQLite, MFAConfig{Enabled: true, EncryptionKey: []byte("01234567890123456789012345678901"), RelyingPartyID: "localhost", Origins: []string{"http://localhost:8080"}, DisplayName: "Dynamis Code", RequireForAdmins: true})
	if err != nil {
		t.Fatal(err)
	}
	service.passwordParams = passwordParams{memory: 64, iterations: 1, parallelism: 1, saltLength: 16, keyLength: 32}
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	owner := bootstrapOwner(t, service)
	session, err := service.CreateSession(ctx, owner.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := service.BeginTOTPEnrollment(ctx, owner.UserID, session.ID, "owner-long-password", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	codes, err := service.CompleteTOTPEnrollment(ctx, session.ID, enrollment.Challenge, totpTestCode(enrollment.Secret, now), AuditContext{})
	if err != nil || len(codes) != recoveryCodeCount {
		t.Fatalf("complete enrollment = %d, %v", len(codes), err)
	}
	status, err := service.MFAStatus(ctx, owner.UserID)
	if err != nil || !status.TOTPEnabled || status.RecoveryRemain != recoveryCodeCount {
		t.Fatalf("MFA status = %+v, %v", status, err)
	}
	if !service.MFARequired(ctx, owner.UserID) {
		t.Fatal("admin MFA policy did not require enrolled factor")
	}
	if _, err := service.AuthenticateSessionForWorkspace(ctx, session.Secret, owner.WorkspaceID, WorkspaceRead); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("password session bypassed MFA policy: %v", err)
	}
	login, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	mfaSession, err := service.CompleteTOTPLogin(ctx, login.Token, totpTestCode(enrollment.Secret, now), AuditContext{})
	if err != nil || mfaSession.AuthLevel != AuthLevelMFA {
		t.Fatalf("MFA session = %+v, %v", mfaSession.Session, err)
	}
	if _, err := service.CompleteTOTPLogin(ctx, login.Token, totpTestCode(enrollment.Secret, now), AuditContext{}); err == nil {
		t.Fatal("replayed MFA challenge accepted")
	}
	recoveryLogin, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteRecoveryLogin(ctx, recoveryLogin.Token, codes[0], AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteRecoveryLogin(ctx, recoveryLogin.Token, codes[0], AuditContext{}); err == nil {
		t.Fatal("replayed recovery challenge accepted")
	}
	oidcLogin, err := service.BeginMFALoginWithMethod(ctx, owner.UserID, "oidc", "company", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	oidcSession, err := service.CompleteRecoveryLogin(ctx, oidcLogin.Token, codes[1], AuditContext{})
	if err != nil || oidcSession.AuthMethod != "oidc" || oidcSession.OIDCProviderID != "company" {
		t.Fatalf("OIDC MFA session = %+v, %v", oidcSession.Session, err)
	}
	var encrypted string
	if err := db.QueryRow("SELECT encrypted_secret FROM mfa_totp WHERE user_id = ?", owner.UserID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == enrollment.Secret || encrypted == "" {
		t.Fatal("TOTP secret stored in plaintext")
	}
	assertAuditEvent(t, db, "mfa.challenge.created")
	assertAuditEvent(t, db, "mfa.challenge.completed")
	assertAuditEvent(t, db, "mfa.policy.enforced")
	assertDatabaseDoesNotContain(t, db, enrollment.Secret)

	base := now
	expired, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(mfaChallengeLifetime + time.Second)
	if _, err := service.CompleteTOTPLogin(ctx, expired.Token, totpTestCode(enrollment.Secret, now), AuditContext{}); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("expired MFA challenge error = %v", err)
	}
	now = base
	limited, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxMFAAttempts; i++ {
		if _, err := service.CompleteTOTPLogin(ctx, limited.Token, "000000", AuditContext{}); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("invalid MFA attempt %d error = %v", i+1, err)
		}
	}
	if _, err := service.CompleteTOTPLogin(ctx, limited.Token, "000000", AuditContext{}); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("rate-limited MFA challenge error = %v", err)
	}
}

func totpTestCode(secret string, now time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	counter := uint64(now.Unix() / 30)
	var input [8]byte
	for i := 7; i >= 0; i-- {
		input[i] = byte(counter)
		counter >>= 8
	}
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(input[:])
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := (uint32(sum[offset])&127)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000)
}
