package items

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// ItemID is an item's integer key. It is internal: users, REST, MCP, CLI, and
// responses see only its short ID (see String and ParseItemID).
type ItemID int64

// ErrInvalidItemID is returned for any short ID that is malformed or does not
// decode to an item key. Callers map it to the same not-found response as a
// missing item.
var ErrInvalidItemID = errors.New("invalid item id")

var itemCodec = mustItemCodec()

func mustItemCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeItem, "")
	if err != nil {
		panic("items: item short-id codec: " + err.Error())
	}
	return codec
}

// String returns the item's short ID.
func (i ItemID) String() string {
	if i == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := itemCodec.Encode(uint64(i))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (i ItemID) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys are rejected.
func (i *ItemID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidItemID
	}
	parsed, err := ParseItemID(shortID)
	if err != nil {
		return err
	}
	*i = parsed
	return nil
}

// ParseItemID decodes an item short ID strictly. Integer keys and short IDs of
// other types are not accepted.
func ParseItemID(shortID string) (ItemID, error) {
	value, err := itemCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidItemID
	}
	return ItemID(value), nil
}

// ParseItemIDOrZero is for adapters: a malformed ID becomes the zero key, which
// matches no item and gets the same not-found as a missing one.
func ParseItemIDOrZero(shortID string) ItemID {
	itemID, err := ParseItemID(shortID)
	if err != nil {
		return 0
	}
	return itemID
}
