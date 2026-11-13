package identity

import (
	"encoding/json"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// UserID is a user's integer key. Users, REST, MCP, CLI, and web see only its
// short ID (ADR 0009); parsing a malformed or integer input yields the zero key.
type UserID int64

var userCodec = mustKeyCodec(id.TypeUser)

// String returns the user's short ID.
func (u UserID) String() string { return encodeShortKey(userCodec, u) }

// MarshalJSON emits the short ID, never the integer key.
func (u UserID) MarshalJSON() ([]byte, error) { return json.Marshal(u.String()) }

// UnmarshalJSON accepts only a short-ID string.
func (u *UserID) UnmarshalJSON(data []byte) error {
	return unmarshalShortKey(userCodec, data, u)
}

// ParseUserID decodes a user short ID strictly; malformed input yields 0.
func ParseUserID(short string) (UserID, error) {
	return parseShortKey[UserID](userCodec, short)
}

// nullableUserID stores an absent user as SQL NULL.
func nullableUserID(userID UserID) any {
	if userID == 0 {
		return nil
	}
	return int64(userID)
}
