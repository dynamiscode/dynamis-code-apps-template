package identity

import (
	"encoding/json"
	"testing"
)

type shortKeyKind struct {
	name      string
	short     func(int64) string
	parse     func(string) (int64, error)
	json      func(int64) ([]byte, error)
	unmarshal func([]byte) (int64, error)
}

func shortKeyKinds() []shortKeyKind {
	return []shortKeyKind{
		{
			name:  "member",
			short: func(k int64) string { return MemberID(k).String() },
			parse: func(s string) (int64, error) { v, err := ParseMemberID(s); return int64(v), err },
			json:  func(k int64) ([]byte, error) { return json.Marshal(MemberID(k)) },
			unmarshal: func(b []byte) (int64, error) {
				var v MemberID
				err := json.Unmarshal(b, &v)
				return int64(v), err
			},
		},
		{
			name:  "invitation",
			short: func(k int64) string { return InvitationID(k).String() },
			parse: func(s string) (int64, error) { v, err := ParseInvitationID(s); return int64(v), err },
			json:  func(k int64) ([]byte, error) { return json.Marshal(InvitationID(k)) },
			unmarshal: func(b []byte) (int64, error) {
				var v InvitationID
				err := json.Unmarshal(b, &v)
				return int64(v), err
			},
		},
		{
			name:  "api token",
			short: func(k int64) string { return APITokenID(k).String() },
			parse: func(s string) (int64, error) { v, err := ParseAPITokenID(s); return int64(v), err },
			json:  func(k int64) ([]byte, error) { return json.Marshal(APITokenID(k)) },
			unmarshal: func(b []byte) (int64, error) {
				var v APITokenID
				err := json.Unmarshal(b, &v)
				return int64(v), err
			},
		},
	}
}

func TestShortKeysRoundTripAndEmitOnlyShortIDs(t *testing.T) {
	for _, kind := range shortKeyKinds() {
		for _, key := range []int64{1, 2, 1000, 1 << 40} {
			short := kind.short(key)
			if len(short) < 4 || short == "1" {
				t.Fatalf("%s short ID for %d = %q", kind.name, key, short)
			}
			parsed, err := kind.parse(short)
			if err != nil || parsed != key {
				t.Fatalf("%s parse(%q) = %d, %v; want %d", kind.name, short, parsed, err, key)
			}
			encoded, err := kind.json(key)
			if err != nil || string(encoded) != `"`+short+`"` {
				t.Fatalf("%s json for %d = %s, %v; want the short ID", kind.name, key, encoded, err)
			}
		}
	}
}

func TestShortKeysRejectIntegersAndNonCanonicalInput(t *testing.T) {
	inputs := []string{"", "1", "12", "0", "NOT-HEX", "0123456789abcdef0123456789abcdef", "9"}
	for _, kind := range shortKeyKinds() {
		for _, input := range inputs {
			if _, err := kind.parse(input); err == nil {
				t.Fatalf("%s parse(%q) accepted invalid input", kind.name, input)
			}
		}
		if _, err := kind.unmarshal([]byte(`42`)); err == nil {
			t.Fatalf("%s accepted an integer in JSON", kind.name)
		}
	}
}

func TestShortKeysDifferAcrossKinds(t *testing.T) {
	// Same integer key, different type codes: the short IDs must not match,
	// so an ID for one kind cannot be mistaken for another.
	member := MemberID(7).String()
	invitation := InvitationID(7).String()
	token := APITokenID(7).String()
	if member == invitation || member == token || invitation == token {
		t.Fatalf("same key produced equal short IDs: %q %q %q", member, invitation, token)
	}
	if _, err := ParseInvitationID(member); err == nil {
		t.Fatal("member short ID parsed as an invitation")
	}
	if _, err := ParseAPITokenID(invitation); err == nil {
		t.Fatal("invitation short ID parsed as an API token")
	}
}
