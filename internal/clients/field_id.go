package clients

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// FieldID is a custom field's integer key. It is internal: users, REST, MCP, and
// responses see only its short ID (see String and ParseFieldID).
type FieldID int64

// ErrInvalidFieldID is returned for any short ID that is malformed or does not
// decode to a field key. Callers map it to the same not-found response as a
// missing field.
var ErrInvalidFieldID = errors.New("invalid client field id")

var fieldCodec = mustFieldCodec()

func mustFieldCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeClientField, "")
	if err != nil {
		panic("clients: client field short-id codec: " + err.Error())
	}
	return codec
}

// String returns the field's short ID.
func (f FieldID) String() string {
	if f == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := fieldCodec.Encode(uint64(f))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (f FieldID) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys are rejected.
func (f *FieldID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidFieldID
	}
	parsed, err := ParseFieldID(shortID)
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// ParseFieldID decodes a field short ID strictly. Integer keys and short IDs of
// other types are not accepted.
func ParseFieldID(shortID string) (FieldID, error) {
	value, err := fieldCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidFieldID
	}
	return FieldID(value), nil
}

// ParseFieldIDOrZero is for adapters: a malformed ID becomes the zero key, which
// matches no field and gets the same not-found as a missing one.
func ParseFieldIDOrZero(shortID string) FieldID {
	fieldID, err := ParseFieldID(shortID)
	if err != nil {
		return 0
	}
	return fieldID
}
