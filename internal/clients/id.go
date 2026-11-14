package clients

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// ClientID is a client's integer key. It is internal: users, REST, MCP, and
// responses see only its short ID (see String and ParseClientID).
type ClientID int64

// ErrInvalidClientID is returned for any short ID that is malformed or does not
// decode to a client key. Callers map it to the same not-found response as a
// missing client.
var ErrInvalidClientID = errors.New("invalid client id")

var clientCodec = mustClientCodec()

func mustClientCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeClient, "")
	if err != nil {
		panic("clients: client short-id codec: " + err.Error())
	}
	return codec
}

// String returns the client's short ID.
func (c ClientID) String() string {
	if c == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := clientCodec.Encode(uint64(c))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (c ClientID) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// UnmarshalJSON accepts only a short-ID string. Integer keys are rejected.
func (c *ClientID) UnmarshalJSON(data []byte) error {
	var shortID string
	if err := json.Unmarshal(data, &shortID); err != nil {
		return ErrInvalidClientID
	}
	parsed, err := ParseClientID(shortID)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// ParseClientID decodes a client short ID strictly. Integer keys and short IDs
// of other types are not accepted.
func ParseClientID(shortID string) (ClientID, error) {
	value, err := clientCodec.Decode(shortID)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidClientID
	}
	return ClientID(value), nil
}

// ParseClientIDOrZero is for adapters: a malformed ID becomes the zero key, which
// matches no client and gets the same not-found as a missing one.
func ParseClientIDOrZero(shortID string) ClientID {
	clientID, err := ParseClientID(shortID)
	if err != nil {
		return 0
	}
	return clientID
}
