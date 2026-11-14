package clients

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// OptionID is a choice option's integer key. It is internal: users, REST, MCP,
// and responses see only its short ID (see String and ParseOptionID).
type OptionID int64

// ErrInvalidOptionID is returned for any short ID that is malformed or does not
// decode to an option key. Callers map it to the same refusal as a foreign option.
var ErrInvalidOptionID = errors.New("invalid client field option id")

var optionCodec = mustOptionCodec()

func mustOptionCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeClientFieldOption, "")
	if err != nil {
		panic("clients: client field option short-id codec: " + err.Error())
	}
	return codec
}

// String returns the option's short ID.
func (o OptionID) String() string {
	if o == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := optionCodec.Encode(uint64(o))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (o OptionID) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys are rejected.
func (o *OptionID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidOptionID
	}
	parsed, err := ParseOptionID(shortID)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// ParseOptionID decodes an option short ID strictly. Integer keys and short IDs
// of other types are not accepted.
func ParseOptionID(shortID string) (OptionID, error) {
	value, err := optionCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidOptionID
	}
	return OptionID(value), nil
}

// ParseOptionIDOrZero is for adapters: a malformed ID becomes the zero key, which
// matches no option and gets the same not-found as a missing one.
func ParseOptionIDOrZero(shortID string) OptionID {
	optionID, err := ParseOptionID(shortID)
	if err != nil {
		return 0
	}
	return optionID
}
