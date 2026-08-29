package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"errors"
	"fmt"
	"sync"
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
	otherUserID := insertUser(t, service, db, "other@example.com")
	if _, err := service.BeginTOTPEnrollment(ctx, otherUserID, session.ID, "owner-long-password", AuditContext{}); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("cross-user MFA enrollment error = %v", err)
	}
	oidcUserID := insertUser(t, service, db, "oidc@example.com")
	if _, err := db.Exec("UPDATE users SET password_hash = NULL WHERE id = ?", oidcUserID); err != nil {
		t.Fatal(err)
	}
	oidcSession, err := service.CreateSession(ctx, oidcUserID, "oidc", "company", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BeginTOTPEnrollment(ctx, oidcUserID, oidcSession.ID, "", AuditContext{}); err != nil {
		t.Fatalf("OIDC fresh MFA enrollment = %v", err)
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
	required, err := service.MFARequired(ctx, owner.UserID)
	if err != nil || !required {
		t.Fatal("admin MFA policy did not require enrolled factor")
	}
	enrolled, err := service.MFAEnrolled(ctx, owner.UserID)
	if err != nil || !enrolled {
		t.Fatalf("MFA enrollment status = %v, %v", enrolled, err)
	}
	if _, err := service.CreateWorkspace(ctx, Principal{UserID: owner.UserID, AuthMethod: "local", AuthLevel: AuthLevelPassword}, WorkspaceCreateInput{Name: "Blocked"}, AuditContext{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("password workspace creation error = %v", err)
	}
	ownerPrincipal := mustAuthorize(t, service, owner.UserID, owner.WorkspaceID, WorkspaceRead)
	ownerToken, err := service.CreateAPIToken(ctx, ownerPrincipal, "owner-automation", []Permission{WorkspaceRead}, nil, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, ownerToken.Secret, WorkspaceRead, AuditContext{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("owner API token bypassed MFA policy: %v", err)
	}
	memberID := insertUser(t, service, db, "member-with-mfa@example.com")
	memberSession, err := service.CreateSession(ctx, memberID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberEnrollment, err := service.BeginTOTPEnrollment(ctx, memberID, memberSession.ID, "user-long-password", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteTOTPEnrollment(ctx, memberSession.ID, memberEnrollment.Challenge, totpTestCode(memberEnrollment.Secret, now), AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateWorkspace(ctx, Principal{UserID: memberID, AuthMethod: "local", AuthLevel: AuthLevelPassword}, WorkspaceCreateInput{Name: "Member blocked"}, AuditContext{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("member password workspace creation error = %v", err)
	}
	pendingSession, err := service.CreateSession(ctx, owner.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	pendingEnrollment, err := service.BeginTOTPEnrollment(ctx, owner.UserID, pendingSession.ID, "owner-long-password", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeSession(ctx, owner.UserID, pendingSession.ID, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteTOTPEnrollment(ctx, pendingSession.ID, pendingEnrollment.Challenge, totpTestCode(pendingEnrollment.Secret, now), AuditContext{}); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("revoked enrollment session error = %v", err)
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
	recoveryA, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	recoveryB, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	recoveryResults := make(chan error, 2)
	go func() {
		_, err := service.CompleteRecoveryLogin(ctx, recoveryA.Token, codes[2], AuditContext{})
		recoveryResults <- err
	}()
	go func() {
		_, err := service.CompleteRecoveryLogin(ctx, recoveryB.Token, codes[2], AuditContext{})
		recoveryResults <- err
	}()
	var recoverySuccess, recoveryFailure int
	for i := 0; i < 2; i++ {
		if err := <-recoveryResults; err == nil {
			recoverySuccess++
		} else if errors.Is(err, ErrInvalidMFACode) {
			recoveryFailure++
		} else {
			t.Fatalf("concurrent recovery error = %v", err)
		}
	}
	if recoverySuccess != 1 || recoveryFailure != 1 {
		t.Fatalf("concurrent recovery results = %d success, %d invalid", recoverySuccess, recoveryFailure)
	}
	oidcLogin, err := service.BeginMFALoginWithMethod(ctx, owner.UserID, "oidc", "company", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	oidcMFASession, err := service.CompleteRecoveryLogin(ctx, oidcLogin.Token, codes[1], AuditContext{})
	if err != nil || oidcMFASession.AuthMethod != "oidc" || oidcMFASession.OIDCProviderID != "company" {
		t.Fatalf("OIDC MFA session = %+v, %v", oidcMFASession.Session, err)
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
	var expiredChallengeID string
	if err := db.QueryRow("SELECT id FROM mfa_challenges WHERE token_hash = ?", hashSecret(expired.Token)).Scan(&expiredChallengeID); err != nil {
		t.Fatal(err)
	}
	now = base.Add(mfaChallengeLifetime + time.Second)
	if _, err := service.CompleteTOTPLogin(ctx, expired.Token, totpTestCode(enrollment.Secret, now), AuditContext{}); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("expired MFA challenge error = %v", err)
	}
	var expiredAuditCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_type = 'mfa.challenge.failed' AND target_id = ?", expiredChallengeID).Scan(&expiredAuditCount); err != nil {
		t.Fatal(err)
	}
	if expiredAuditCount != 1 {
		t.Fatalf("expired MFA challenge audit count = %d", expiredAuditCount)
	}
	now = base
	late, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	loadedLate, _, err := service.loadChallenge(ctx, late.Token, "login")
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(mfaChallengeLifetime + time.Second)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if service.consumeChallenge(ctx, tx, loadedLate.ID) {
		t.Fatal("expired MFA challenge consumed")
	}
	_ = tx.Rollback()
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
	parallel, err := service.BeginMFALogin(ctx, owner.UserID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	const concurrentAttempts = maxMFAAttempts * 4
	errs := make(chan error, concurrentAttempts)
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrentAttempts)
	for i := 0; i < concurrentAttempts; i++ {
		go func() {
			defer waitGroup.Done()
			_, err := service.CompleteTOTPLogin(ctx, parallel.Token, "000000", AuditContext{})
			errs <- err
		}()
	}
	waitGroup.Wait()
	close(errs)
	invalidCodes, invalidChallenges := 0, 0
	for err := range errs {
		switch {
		case errors.Is(err, ErrInvalidMFACode):
			invalidCodes++
		case errors.Is(err, ErrInvalidMFAChallenge):
			invalidChallenges++
		default:
			t.Fatalf("concurrent MFA attempt error = %v", err)
		}
	}
	if invalidCodes != maxMFAAttempts || invalidChallenges != concurrentAttempts-maxMFAAttempts {
		t.Fatalf("concurrent MFA attempts = %d invalid codes, %d invalid challenges", invalidCodes, invalidChallenges)
	}
	now = base
	expiredPasswordSession, err := service.CreateSession(ctx, owner.UserID, "local", "", time.Minute, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	expiredMFASession, err := service.CreateMFASession(ctx, owner.UserID, "local", "", time.Minute, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(2 * time.Minute)
	if err := service.VerifyFreshAuthentication(ctx, owner.UserID, expiredPasswordSession.ID, "owner-long-password"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired password session error = %v", err)
	}
	if err := service.VerifyFreshAuthentication(ctx, owner.UserID, expiredMFASession.ID, ""); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired MFA session error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if required, err := service.MFARequired(ctx, owner.UserID); err == nil || required {
		t.Fatalf("MFA policy lookup after database failure = %v, %v", required, err)
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
