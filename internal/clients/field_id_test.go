package clients

import (
	"encoding/json"
	"testing"
)

func TestFieldIDRoundTripsAndRefusesOtherTypes(t *testing.T) {
	t.Parallel()

	const key = FieldID(7)
	short := key.String()
	if short == "" {
		t.Fatal("FieldID(7).String() is empty")
	}
	if got := ParseFieldIDOrZero(short); got != key {
		t.Fatalf("ParseFieldIDOrZero(%q) = %d, want %d", short, got, key)
	}
	if got := ParseFieldIDOrZero(ClientID(7).String()); got != 0 {
		t.Fatalf("client short ID accepted as a field ID: %d", got)
	}
	if got := ParseFieldIDOrZero("7"); got != 0 {
		t.Fatalf("integer key accepted as a field ID: %d", got)
	}
	data, err := json.Marshal(key)
	if err != nil || string(data) != `"`+short+`"` {
		t.Fatalf("json.Marshal(FieldID) = %s, %v; want the short ID string", data, err)
	}
}
