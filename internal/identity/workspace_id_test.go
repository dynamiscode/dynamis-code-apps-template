package identity

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceShortIDRoundTripsAndHidesIntegerKeys(t *testing.T) {
	for _, key := range []WorkspaceID{1, 2, 1000, 1 << 40} {
		short := key.String()
		if len(short) < 4 || short == "1" {
			t.Fatalf("short ID for %d = %q", key, short)
		}
		parsed, err := ParseWorkspaceID(short)
		if err != nil || parsed != key {
			t.Fatalf("ParseWorkspaceID(%q) = %d, %v; want %d", short, parsed, err, key)
		}
		encoded, err := json.Marshal(key)
		if err != nil || string(encoded) != `"`+short+`"` {
			t.Fatalf("json for %d = %s, %v; want the short ID", key, encoded, err)
		}
	}
}

func TestParseWorkspaceIDRejectsIntegersAndNonCanonicalInput(t *testing.T) {
	for _, input := range []string{"", "1", "12", "0", "NOT-HEX", "0123456789abcdef0123456789abcdef", "9"} {
		if _, err := ParseWorkspaceID(input); err != ErrInvalidWorkspaceID {
			t.Fatalf("ParseWorkspaceID(%q) error = %v, want ErrInvalidWorkspaceID", input, err)
		}
	}
}

func TestWorkspaceIDJSONAcceptsOnlyShortIDs(t *testing.T) {
	short := WorkspaceID(42).String()
	var decoded struct {
		WorkspaceID WorkspaceID `json:"workspaceId"`
	}
	if err := json.Unmarshal([]byte(`{"workspaceId":"`+short+`"}`), &decoded); err != nil || decoded.WorkspaceID != 42 {
		t.Fatalf("decode short ID = %d, %v", decoded.WorkspaceID, err)
	}
	if err := json.Unmarshal([]byte(`{"workspaceId":42}`), &decoded); err == nil {
		t.Fatal("integer workspaceId was accepted")
	}
	if err := json.Unmarshal([]byte(`{"workspaceId":null}`), &decoded); err != nil || decoded.WorkspaceID != 0 {
		t.Fatalf("null workspaceId = %d, %v; want no workspace", decoded.WorkspaceID, err)
	}
}

func TestZeroWorkspaceIDIsNoWorkspace(t *testing.T) {
	if got := WorkspaceID(0).String(); got != "" {
		t.Fatalf("zero workspace string = %q, want empty", got)
	}
	if value := nullableWorkspaceID(0); value != nil {
		t.Fatalf("zero workspace stored as %v, want NULL", value)
	}
}
