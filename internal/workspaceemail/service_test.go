package workspaceemail

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
)

type recordingSender struct{ sent []string }

func (r *recordingSender) Send(_ context.Context, recipient, _, _ string) error {
	r.sent = append(r.sent, recipient)
	return nil
}

func validInput(password string) SaveInput {
	return SaveInput{
		SenderAddress: "mailer@example.com", SenderName: "Dynamis",
		Host: "smtp.example.com", Port: 587, Username: "mailer", Password: password,
	}
}

func testService(t *testing.T, fallback appmail.Sender) (*Service, *sql.DB, identity.Principal, *string) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	db, err := database.Open(ctx, cfg.Database)
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
	bootstrapped, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, bootstrapped.UserID, bootstrapped.WorkspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	keys := secretbox.New(map[int][]byte{config.GeneralKeyVersion: []byte("01234567890123456789012345678901")}, config.GeneralKeyVersion)
	service := NewService(db, config.SQLite, auth, keys, fallback)
	seen := new(string)
	service.login = func(_ context.Context, cfg config.Mail, _ string) error {
		*seen = cfg.Password
		return nil
	}
	return service, db, owner, seen
}

func TestSaveSealsPasswordAfterSuccessfulLoginTest(t *testing.T) {
	ctx := context.Background()
	service, db, owner, seen := testService(t, nil)
	if err := service.Save(ctx, owner, owner.WorkspaceID, validInput("smtp-pass-1"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if *seen != "smtp-pass-1" {
		t.Fatalf("login test used password %q", *seen)
	}
	var ciphertext string
	var version int
	if err := db.QueryRow("SELECT encrypted_password, secret_key_version FROM workspace_email_settings").Scan(&ciphertext, &version); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "smtp-pass-1") || version != config.GeneralKeyVersion {
		t.Fatalf("stored password plaintext or version = %d", version)
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Configured || !settings.PasswordSet || settings.Host != "smtp.example.com" {
		t.Fatalf("Get() = %+v", settings)
	}
}

func TestFailedLoginTestBlocksSave(t *testing.T) {
	ctx := context.Background()
	service, _, owner, _ := testService(t, nil)
	service.login = func(context.Context, config.Mail, string) error { return errors.New("SMTP authentication failed") }
	err := service.Save(ctx, owner, owner.WorkspaceID, validInput("wrong"), identity.AuditContext{})
	var loginErr *LoginError
	if !errors.As(err, &loginErr) || loginErr.Reason != "SMTP authentication failed" {
		t.Fatalf("Save() error = %v, want LoginError", err)
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil || settings.Configured {
		t.Fatalf("Get() after failed login = %+v, err = %v; nothing must be stored", settings, err)
	}
}

func TestBlankPasswordKeepsStoredValue(t *testing.T) {
	ctx := context.Background()
	service, db, owner, seen := testService(t, nil)
	if err := service.Save(ctx, owner, owner.WorkspaceID, validInput("first-pass"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow("SELECT encrypted_password FROM workspace_email_settings").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := service.Save(ctx, owner, owner.WorkspaceID, validInput(""), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if *seen != "first-pass" {
		t.Fatalf("blank-password save tested with %q, want stored value", *seen)
	}
	var after string
	if err := db.QueryRow("SELECT encrypted_password FROM workspace_email_settings").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("blank password replaced the stored ciphertext")
	}
}

func TestNonMemberIsDeniedSettings(t *testing.T) {
	ctx := context.Background()
	service, _, owner, _ := testService(t, nil)
	outsider := identity.Principal{UserID: identity.UserID(987654), WorkspaceID: owner.WorkspaceID}
	if _, err := service.Get(ctx, outsider, owner.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("Get() error = %v, want forbidden", err)
	}
	if err := service.Save(ctx, outsider, owner.WorkspaceID, validInput("x"), identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("Save() error = %v, want forbidden", err)
	}
}

func TestInvitationRoutesToInstanceSenderWhenWorkspaceUnconfigured(t *testing.T) {
	ctx := context.Background()
	instance := &recordingSender{}
	service, _, owner, _ := testService(t, instance)
	if err := service.SendFor(ctx, owner.WorkspaceID.String(), "invitee@example.com", "s", "b"); err != nil {
		t.Fatal(err)
	}
	if len(instance.sent) != 1 || instance.sent[0] != "invitee@example.com" {
		t.Fatalf("instance sender got %v", instance.sent)
	}
}

func TestInvitationWithNoSenderAnywhereIsNotConfigured(t *testing.T) {
	service, _, owner, _ := testService(t, nil)
	if err := service.SendFor(context.Background(), owner.WorkspaceID.String(), "invitee@example.com", "s", "b"); !errors.Is(err, appmail.ErrNotConfigured) {
		t.Fatalf("SendFor() error = %v, want not configured", err)
	}
}

func TestAccountEmailIgnoresWorkspaceSettings(t *testing.T) {
	ctx := context.Background()
	service, _, owner, _ := testService(t, nil)
	if err := service.Save(ctx, owner, owner.WorkspaceID, validInput("pw"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if service.Available() {
		t.Fatal("Available() = true with no instance sender and a workspace configured")
	}
	if err := service.Send(ctx, "owner@example.com", "s", "b"); !errors.Is(err, appmail.ErrNotConfigured) {
		t.Fatalf("account Send() error = %v, want not configured", err)
	}
}

func TestNormalizeRejectsUnsafeInput(t *testing.T) {
	bad := []SaveInput{
		{SenderAddress: "mailer@example.com", Host: "smtp.example.com", Port: 587, Username: "u", SenderName: "bad\r\nBcc: x"},
		{SenderAddress: "not-an-address", Host: "smtp.example.com", Port: 587, Username: "u"},
		{SenderAddress: "mailer@example.com", Host: "smtp.example.com/x", Port: 587, Username: "u"},
		{SenderAddress: "mailer@example.com", Host: "smtp.example.com", Port: 70000, Username: "u"},
		{SenderAddress: "mailer@example.com", Host: "smtp.example.com", Port: 587},
	}
	for _, input := range bad {
		if _, err := normalize(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("normalize(%+v) error = %v, want invalid input", input, err)
		}
	}
}

func TestBlankPasswordIsRefusedWhenServerChanges(t *testing.T) {
	ctx := context.Background()
	service, _, owner, seen := testService(t, nil)
	if err := service.Save(ctx, owner, owner.WorkspaceID, validInput("stored-pass"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	*seen = "not-called"
	moved := validInput("")
	moved.Host = "attacker.example.net"
	if err := service.Save(ctx, owner, owner.WorkspaceID, moved, identity.AuditContext{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("Save() with changed host error = %v, want ErrPasswordRequired", err)
	}
	if *seen != "not-called" {
		t.Fatalf("login test ran against changed host with stored password %q", *seen)
	}
}
