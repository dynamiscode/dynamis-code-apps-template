package mail

import (
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestWorkspaceSenderHeaderCarriesDisplayName(t *testing.T) {
	cfg := config.Mail{Host: "smtp.example.com", Port: 587, Username: "mailer", Password: "x", From: "no-reply@example.com"}
	named, err := NewWorkspaceSMTP(cfg, "Dynamis Team")
	if err != nil {
		t.Fatal(err)
	}
	if got := named.fromHeader(); got != `"Dynamis Team" <no-reply@example.com>` {
		t.Fatalf("named From = %q", got)
	}
	bare, err := NewWorkspaceSMTP(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := bare.fromHeader(); got != "no-reply@example.com" {
		t.Fatalf("bare From = %q", got)
	}
}
