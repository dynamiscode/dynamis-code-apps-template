package identity

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// UserID is a user's integer key. It is internal: REST, MCP, CLI, and web see
// only its short ID (see String and ParseUserID). Users are never accepted by
// key or by integer, so a row key is not a credential.
type UserID int64

// ErrInvalidUserID is returned for any short ID that is malformed or does not
// decode to a user key.
var ErrInvalidUserID = errors.New("invalid user id")

var userCodec = mustUserCodec()

func mustUserCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeUser, "")
	if err != nil {
		panic("identity: user short-id codec: " + err.Error())
	}
	return codec
}

// String returns the user's short ID, or "" for no user.
func (u UserID) String() string {
	if u <= 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := userCodec.Encode(uint64(u))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (u UserID) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys in JSON are rejected.
func (u *UserID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidUserID
	}
	parsed, err := ParseUserID(shortID)
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// ParseUserID decodes a user short ID strictly. Integer keys are not accepted.
func ParseUserID(shortID string) (UserID, error) {
	value, err := userCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidUserID
	}
	return UserID(value), nil
}

// nullableUserID stores an absent user as SQL NULL. Audit rows keep the actor's
// short ID as text: audit_events.actor_user_id is not a foreign key.
func nullableUserID(userID UserID) any {
	if userID == 0 {
		return nil
	}
	return userID.String()
}

// nullableUserKey stores an absent user as SQL NULL and a user as its integer key.
func nullableUserKey(userID UserID) any {
	if userID == 0 {
		return nil
	}
	return int64(userID)
}

// ParseUserIDOrZero is for adapters: a malformed ID becomes the zero key, which
// matches no user and gets the same denial as a missing one.
func ParseUserIDOrZero(shortID string) UserID {
	userID, err := ParseUserID(shortID)
	if err != nil {
		return 0
	}
	return userID
}
