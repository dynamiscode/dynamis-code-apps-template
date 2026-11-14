package clients

import (
	"encoding/json"
	"testing"
)

func TestOptionIDRoundTripsAndRefusesOtherTypes(t *testing.T) {
	t.Parallel()

	const key = OptionID(7)
	short := key.String()
	if short == "" {
		t.Fatal("OptionID(7).String() is empty")
	}
	if got := ParseOptionIDOrZero(short); got != key {
		t.Fatalf("ParseOptionIDOrZero(%q) = %d, want %d", short, got, key)
	}
	if got := ParseOptionIDOrZero(FieldID(7).String()); got != 0 {
		t.Fatalf("field short ID accepted as an option ID: %d", got)
	}
	if got := ParseOptionIDOrZero(ClientID(7).String()); got != 0 {
		t.Fatalf("client short ID accepted as an option ID: %d", got)
	}
	if got := ParseOptionIDOrZero("7"); got != 0 {
		t.Fatalf("integer key accepted as an option ID: %d", got)
	}
	data, err := json.Marshal(key)
	if err != nil || string(data) != `"`+short+`"` {
		t.Fatalf("json.Marshal(OptionID) = %s, %v; want the short ID string", data, err)
	}
	var decoded OptionID
	if err := json.Unmarshal([]byte("7"), &decoded); err == nil {
		t.Fatal("integer JSON accepted as an option ID")
	}
	if err := json.Unmarshal(data, &decoded); err != nil || decoded != key {
		t.Fatalf("json.Unmarshal(short ID) = %d, %v; want %d", decoded, err, key)
	}
}
