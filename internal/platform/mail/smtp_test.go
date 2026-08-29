package mail

import "testing"

func TestFormatMessageUsesStaticRecipientHeader(t *testing.T) {
	got := formatMessage("no-reply@example.com", "Subject", "Body")
	want := "From: no-reply@example.com\r\nTo: undisclosed-recipients:;\r\nSubject: Subject\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nBody\r\n"
	if got != want {
		t.Fatalf("formatMessage() = %q, want %q", got, want)
	}
}
