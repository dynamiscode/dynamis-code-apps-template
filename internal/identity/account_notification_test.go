package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAccountLifecycleAndDeletionBoundaries(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	owner := bootstrapOwner(t, service)

	profile, err := service.GetUserProfile(ctx, owner.UserID)
	if err != nil || profile.Email != "owner@example.com" || profile.Timezone != "UTC" || profile.Theme != "system" || profile.EmailVerifiedAt == nil {
		t.Fatalf("initial profile = %+v, %v", profile, err)
	}
	if err := service.UpdateUserProfile(ctx, owner.UserID, ProfileUpdateInput{
		DisplayName: "Owner", Locale: "es", Timezone: "America/Bogota", Theme: "dark",
	}, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	profile, err = service.GetUserProfile(ctx, owner.UserID)
	if err != nil || profile.DisplayName != "Owner" || profile.Locale != "es" || profile.Timezone != "America/Bogota" || profile.Theme != "dark" {
		t.Fatalf("updated profile = %+v, %v", profile, err)
	}
	if err := service.UpdateUserProfile(ctx, owner.UserID, ProfileUpdateInput{Timezone: "not/a-zone", Theme: "system"}, AuditContext{}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("invalid profile error = %v", err)
	}

	session, err := service.CreateSession(ctx, owner.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ChangePassword(ctx, owner.UserID, "owner-long-password", "changed-owner-password", AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateLocal(ctx, owner.UserID, "owner-long-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password error = %v", err)
	}
	if _, err := service.AuthenticateLocal(ctx, "owner@example.com", "changed-owner-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("changed-password session error = %v", err)
	}
	if err := service.DeleteAccount(ctx, owner.UserID, "changed-owner-password", AuditContext{}); !errors.Is(err, ErrOwnedWorkspace) {
		t.Fatalf("owner deletion error = %v", err)
	}

	deletedUser := insertUser(t, service, db, "delete@example.com")
	if err := service.DeleteAccount(ctx, deletedUser, "user-long-password", AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetUserProfile(ctx, deletedUser); !errors.Is(err, ErrForbidden) {
		t.Fatalf("deleted profile error = %v", err)
	}
	assertAuditEvent(t, db, "user.account_deleted")
}

func TestEmailVerificationAndPasswordResetAreSingleUse(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	userID := insertUser(t, service, db, "unverified@example.com")

	verification, err := service.BeginEmailVerification(ctx, userID, AuditContext{})
	if err != nil || verification.Secret == "" || verification.Email != "unverified@example.com" {
		t.Fatalf("BeginEmailVerification() = %+v, %v", verification, err)
	}
	if err := service.VerifyEmail(ctx, verification.Secret, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyEmail(ctx, verification.Secret, AuditContext{}); !errors.Is(err, ErrInvalidVerification) {
		t.Fatalf("verification replay error = %v", err)
	}
	if _, err := service.BeginEmailVerification(ctx, userID, AuditContext{}); !errors.Is(err, ErrEmailAlreadyVerified) {
		t.Fatalf("verified resend error = %v", err)
	}

	session, err := service.CreateSession(ctx, userID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := service.BeginPasswordReset(ctx, "unverified@example.com", AuditContext{})
	if err != nil || reset.Secret == "" {
		t.Fatalf("BeginPasswordReset() = %+v, %v", reset, err)
	}
	if unknown, err := service.BeginPasswordReset(ctx, "missing@example.com", AuditContext{}); err != nil || unknown.Secret != "" {
		t.Fatalf("unknown reset = %+v, %v", unknown, err)
	}
	if err := service.CompletePasswordReset(ctx, reset.Secret, "reset-user-password", AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateLocal(ctx, "unverified@example.com", "reset-user-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("reset session error = %v", err)
	}
	if err := service.CompletePasswordReset(ctx, reset.Secret, "another-password", AuditContext{}); !errors.Is(err, ErrInvalidReset) {
		t.Fatalf("reset replay error = %v", err)
	}
	assertDatabaseDoesNotContain(t, db, verification.Secret)
	assertDatabaseDoesNotContain(t, db, reset.Secret)
}

func TestNotificationsHonorUserAndWorkspacePreferences(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	owner := bootstrapOwner(t, service)
	ownerPrincipal := mustAuthorize(t, service, owner.UserID, owner.WorkspaceID, WorkspaceRead)

	created, err := service.CreateNotification(ctx, Principal{AuthMethod: "system"}, NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: owner.WorkspaceID, NotificationType: "system",
		Title: "Welcome", Body: "A private notification",
	}, AuditContext{})
	if err != nil || created.ID == "" {
		t.Fatalf("CreateNotification() = %+v, %v", created, err)
	}
	if err := service.SetNotificationPreference(ctx, owner.UserID, "system", false, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	suppressed, err := service.CreateNotification(ctx, Principal{AuthMethod: "system"}, NotificationInput{
		RecipientUserID: owner.UserID, NotificationType: "system", Title: "Hidden", Body: "Not stored",
	}, AuditContext{})
	if err != nil || suppressed.ID != "" {
		t.Fatalf("suppressed notification = %+v, %v", suppressed, err)
	}
	if err := service.SetNotificationPreference(ctx, owner.UserID, "system", true, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetWorkspaceNotificationPreference(ctx, ownerPrincipal, "system", false, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	suppressed, err = service.CreateNotification(ctx, Principal{AuthMethod: "system"}, NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: owner.WorkspaceID, NotificationType: "system", Title: "Hidden workspace", Body: "Not stored",
	}, AuditContext{})
	if err != nil || suppressed.ID != "" {
		t.Fatalf("workspace-suppressed notification = %+v, %v", suppressed, err)
	}
	if err := service.SetWorkspaceNotificationPreference(ctx, ownerPrincipal, "system", true, AuditContext{}); err != nil {
		t.Fatal(err)
	}

	list, err := service.ListNotifications(ctx, owner.UserID, owner.WorkspaceID, false, 10)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("ListNotifications() = %+v, %v", list, err)
	}
	if err := service.MarkNotificationRead(ctx, owner.UserID, created.ID, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if unread, err := service.UnreadNotificationCount(ctx, owner.UserID); err != nil || unread != 0 {
		t.Fatalf("unread count = %d, %v", unread, err)
	}
	if err := service.MarkNotificationRead(ctx, "other-user", created.ID, AuditContext{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-user read error = %v", err)
	}
	if after, err := service.NotificationsAfter(ctx, owner.UserID, "", 10); err != nil || len(after) != 1 {
		t.Fatalf("NotificationsAfter() = %+v, %v", after, err)
	}
}
