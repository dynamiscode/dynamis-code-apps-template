package files

import (
	"encoding/json"
	"errors"
	"testing"
)

// Short IDs are the only file ID form that leaves the process. Integer keys
// are never accepted from input and never emitted in JSON.
func TestFileIDRoundTripsAndHidesIntegerKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []FileID{1, 2, 42, 1 << 40} {
		short := key.String()
		if short == "" || short == "42" {
			t.Fatalf("short ID for %d = %q", key, short)
		}
		parsed, err := ParseFileID(short)
		if err != nil || parsed != key {
			t.Fatalf("ParseFileID(%q) = %d, %v; want %d", short, parsed, err, key)
		}
		encoded, err := json.Marshal(struct {
			ID FileID `json:"id"`
		}{key})
		if err != nil || string(encoded) != `{"id":"`+short+`"}` {
			t.Fatalf("json for %d = %s, %v; want the short ID", key, encoded, err)
		}
	}
}

func TestFileIDJSONAcceptsOnlyShortIDs(t *testing.T) {
	t.Parallel()

	var decoded struct {
		ID FileID `json:"id"`
	}
	if err := json.Unmarshal([]byte(`{"id":"`+FileID(9).String()+`"}`), &decoded); err != nil || decoded.ID != 9 {
		t.Fatalf("decode short ID = %d, %v", decoded.ID, err)
	}
	if err := json.Unmarshal([]byte(`{"id":9}`), &decoded); err == nil {
		t.Fatal("integer key decoded from JSON, want rejection")
	}
}

func TestParseFileIDRejectsIntegerAndMalformedInput(t *testing.T) {
	t.Parallel()

	valid := FileID(7).String()
	for _, input := range []string{"", "1", "12", "00000000000000000000000000000000", valid + "!", valid[:len(valid)-1]} {
		if _, err := ParseFileID(input); !errors.Is(err, ErrInvalidFileID) {
			t.Fatalf("ParseFileID(%q) error = %v, want ErrInvalidFileID", input, err)
		}
		if got := ParseFileIDOrZero(input); got != 0 {
			t.Fatalf("ParseFileIDOrZero(%q) = %d, want 0", input, got)
		}
	}
}
