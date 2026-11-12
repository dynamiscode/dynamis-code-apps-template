package items

import (
	"encoding/json"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestItemShortIDRoundTripsAndHidesIntegerKeys(t *testing.T) {
	for _, key := range []ItemID{1, 2, 1000, 1 << 40} {
		short := key.String()
		if len(short) < 4 {
			t.Fatalf("short ID for %d = %q", key, short)
		}
		parsed, err := ParseItemID(short)
		if err != nil || parsed != key {
			t.Fatalf("ParseItemID(%q) = %d, %v; want %d", short, parsed, err, key)
		}
		encoded, err := json.Marshal(key)
		if err != nil || string(encoded) != `"`+short+`"` {
			t.Fatalf("json for %d = %s, %v; want the short ID", key, encoded, err)
		}
	}
}

func TestParseItemIDRejectsIntegersAndForeignShortIDs(t *testing.T) {
	workspaceShort := identity.WorkspaceID(42).String()
	for _, input := range []string{"", "1", "12", "0", "NOT-HEX", "0123456789abcdef0123456789abcdef", workspaceShort} {
		if _, err := ParseItemID(input); err != ErrInvalidItemID {
			t.Fatalf("ParseItemID(%q) error = %v, want ErrInvalidItemID", input, err)
		}
	}
	if got := ParseItemIDOrZero(workspaceShort); got != 0 {
		t.Fatalf("ParseItemIDOrZero(workspace short ID) = %d, want 0", got)
	}
}

func TestItemIDJSONAcceptsOnlyShortIDs(t *testing.T) {
	short := ItemID(42).String()
	var decoded struct {
		ItemID ItemID `json:"itemId"`
	}
	if err := json.Unmarshal([]byte(`{"itemId":"`+short+`"}`), &decoded); err != nil || decoded.ItemID != 42 {
		t.Fatalf("decode short ID = %d, %v", decoded.ItemID, err)
	}
	if err := json.Unmarshal([]byte(`{"itemId":42}`), &decoded); err == nil {
		t.Fatal("integer itemId was accepted")
	}
}
