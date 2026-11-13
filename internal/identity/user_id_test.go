package identity

import (
	"encoding/json"
	"testing"
)

func TestUserIDJSONEmitsShortIDNotIntegerKey(t *testing.T) {
	t.Parallel()

	const key UserID = 42
	data, err := json.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"`+key.String()+`"` {
		t.Fatalf("json = %s, want the short ID %q", data, key.String())
	}
	if key.String() == "42" || key.String() == "" {
		t.Fatalf("short ID = %q, want an encoded value", key.String())
	}

	var parsed UserID
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed != key {
		t.Fatalf("round trip = %d, want %d", parsed, key)
	}
	if err := json.Unmarshal([]byte("42"), &parsed); err == nil {
		t.Fatal("integer JSON key was accepted")
	}
}

func TestParseUserIDRejectsMalformedAndZero(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "not-a-short-id", "42", "0000"} {
		if _, err := ParseUserID(input); err == nil {
			t.Fatalf("ParseUserID(%q) error = nil, want rejection", input)
		}
	}
	if got := (UserID(0)).String(); got != "" {
		t.Fatalf("zero UserID string = %q, want empty", got)
	}
}

func TestParseUserIDRoundTripsKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []UserID{1, 2, 999999} {
		parsed, err := ParseUserID(key.String())
		if err != nil {
			t.Fatal(err)
		}
		if parsed != key {
			t.Fatalf("ParseUserID(%q) = %d, want %d", key.String(), parsed, key)
		}
	}
}
