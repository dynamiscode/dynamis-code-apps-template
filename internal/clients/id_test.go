package clients

import (
	"encoding/json"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestClientShortIDRoundTripsAndHidesIntegerKeys(t *testing.T) {
	for _, key := range []ClientID{1, 2, 1000, 1 << 40} {
		short := key.String()
		parsed, err := ParseClientID(short)
		if err != nil || parsed != key {
			t.Fatalf("ParseClientID(%q) = %d, %v; want %d", short, parsed, err, key)
		}
		encoded, err := json.Marshal(key)
		if err != nil || string(encoded) != `"`+short+`"` {
			t.Fatalf("json for %d = %s, %v; want the short ID", key, encoded, err)
		}
	}
}

func TestParseClientIDRejectsIntegersAndForeignShortIDs(t *testing.T) {
	workspaceShort := identity.WorkspaceID(42).String()
	for _, input := range []string{"", "1", "12", "0", "NOT-HEX", "0123456789abcdef0123456789abcdef", workspaceShort} {
		if _, err := ParseClientID(input); err != ErrInvalidClientID {
			t.Fatalf("ParseClientID(%q) error = %v, want ErrInvalidClientID", input, err)
		}
	}
	if got := ParseClientIDOrZero(workspaceShort); got != 0 {
		t.Fatalf("ParseClientIDOrZero(workspace short ID) = %d, want 0", got)
	}
}

func TestClientIDJSONAcceptsOnlyShortIDs(t *testing.T) {
	short := ClientID(42).String()
	var decoded struct {
		ClientID ClientID `json:"clientId"`
	}
	if err := json.Unmarshal([]byte(`{"clientId":"`+short+`"}`), &decoded); err != nil || decoded.ClientID != 42 {
		t.Fatalf("decode short ID = %d, %v", decoded.ClientID, err)
	}
	if err := json.Unmarshal([]byte(`{"clientId":42}`), &decoded); err == nil {
		t.Fatal("integer clientId was accepted")
	}
}
