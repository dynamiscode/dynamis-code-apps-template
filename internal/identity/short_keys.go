package identity

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// MemberID, InvitationID, and APITokenID are integer record keys. Users, REST,
// MCP, CLI, and web see only their short IDs (ADR 0009). Parsing a malformed,
// unknown, or integer input yields the zero key, which matches no record, so
// callers get the same not-found as a missing record.
type (
	MemberID     int64
	InvitationID int64
	APITokenID   int64
)

// ErrInvalidShortID is returned when a short ID does not decode to a key.
var ErrInvalidShortID = errors.New("invalid short id")

var (
	memberCodec     = mustKeyCodec(id.TypeMember)
	invitationCodec = mustKeyCodec(id.TypeInvitation)
	apiTokenCodec   = mustKeyCodec(id.TypeToken)
)

func mustKeyCodec(code id.TypeCode) *id.Codec {
	codec, err := id.NewCodec(code, "")
	if err != nil {
		panic("identity: " + string(code) + " short-id codec: " + err.Error())
	}
	return codec
}

type shortKey interface{ ~int64 }

func encodeShortKey[T shortKey](codec *id.Codec, key T) string {
	if key <= 0 {
		return "" // no record
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := codec.Encode(uint64(key))
	return short
}

func parseShortKey[T shortKey](codec *id.Codec, short string) (T, error) {
	value, err := codec.Decode(short)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidShortID
	}
	return T(value), nil
}

func unmarshalShortKey[T shortKey](codec *id.Codec, data []byte, dst *T) error {
	var short string
	if err := json.Unmarshal(data, &short); err != nil {
		return ErrInvalidShortID
	}
	parsed, err := parseShortKey[T](codec, short)
	if err != nil {
		return err
	}
	*dst = parsed
	return nil
}

// String returns the member's short ID.
func (m MemberID) String() string { return encodeShortKey(memberCodec, m) }

// MarshalJSON emits the short ID, never the integer key.
func (m MemberID) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

// UnmarshalJSON accepts only a short-ID string.
func (m *MemberID) UnmarshalJSON(data []byte) error {
	return unmarshalShortKey(memberCodec, data, m)
}

// ParseMemberID decodes a member short ID strictly; malformed input yields 0.
func ParseMemberID(short string) (MemberID, error) {
	return parseShortKey[MemberID](memberCodec, short)
}

// String returns the invitation's short ID.
func (i InvitationID) String() string { return encodeShortKey(invitationCodec, i) }

// MarshalJSON emits the short ID, never the integer key.
func (i InvitationID) MarshalJSON() ([]byte, error) { return json.Marshal(i.String()) }

// UnmarshalJSON accepts only a short-ID string.
func (i *InvitationID) UnmarshalJSON(data []byte) error {
	return unmarshalShortKey(invitationCodec, data, i)
}

// ParseInvitationID decodes an invitation short ID strictly; malformed input yields 0.
func ParseInvitationID(short string) (InvitationID, error) {
	return parseShortKey[InvitationID](invitationCodec, short)
}

// String returns the API token's short ID. The token secret is never encoded.
func (a APITokenID) String() string { return encodeShortKey(apiTokenCodec, a) }

// TokenShortID returns the short ID of the credential that authenticated the
// principal, or "" for a session. It is for display; authorization and
// idempotency use TokenKind and TokenKey.
func (p Principal) TokenShortID() string { return id.EncodeKey(id.TypeCode(p.TokenKind), p.TokenKey) }

// MarshalJSON emits the short ID, never the integer key.
func (a APITokenID) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

// UnmarshalJSON accepts only a short-ID string.
func (a *APITokenID) UnmarshalJSON(data []byte) error {
	return unmarshalShortKey(apiTokenCodec, data, a)
}

// ParseAPITokenID decodes an API token short ID strictly; malformed input yields 0.
func ParseAPITokenID(short string) (APITokenID, error) {
	return parseShortKey[APITokenID](apiTokenCodec, short)
}
