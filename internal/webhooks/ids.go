package webhooks

import (
	"encoding/json"
	"errors"
	"math"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// WebhookID, DeliveryID, and EventID are integer keys. Only their short IDs
// leave the process, and each type encodes and decodes with its own type code.
// The accepted codec does not reject a short ID from another type code (ADR
// 0009): about 3% of webhook short IDs also decode under the delivery alphabet.
// Routes therefore rely on the typed parser plus the not-found rule, not on
// cross-type rejection.
type (
	WebhookID  int64
	DeliveryID int64
	EventID    int64
)

// ErrInvalidID is returned when a short ID is malformed or decodes under the
// wrong type code. Adapters map a zero key to the same not-found as a missing row.
var ErrInvalidID = errors.New("invalid webhook short id")

var (
	webhookCodec  = mustCodec(id.TypeWebhook)
	deliveryCodec = mustCodec(id.TypeWebhookDelivery)
	eventCodec    = mustCodec(id.TypeWebhookEvent)
)

func mustCodec(code id.TypeCode) *id.Codec {
	codec, err := id.NewCodec(code, "")
	if err != nil {
		panic("webhooks: short-id codec " + string(code) + ": " + err.Error())
	}
	return codec
}

func encodeKey(codec *id.Codec, key int64) string {
	if key <= 0 {
		return ""
	}
	// Keys are positive, so the conversion is exact and Encode cannot fail.
	short, _ := codec.Encode(uint64(key))
	return short
}

func decodeKey(codec *id.Codec, short string) (int64, error) {
	value, err := codec.Decode(short)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidID
	}
	return int64(value), nil
}

func (w WebhookID) String() string  { return encodeKey(webhookCodec, int64(w)) }
func (d DeliveryID) String() string { return encodeKey(deliveryCodec, int64(d)) }
func (e EventID) String() string    { return encodeKey(eventCodec, int64(e)) }

func (w WebhookID) MarshalJSON() ([]byte, error)  { return json.Marshal(w.String()) }
func (d DeliveryID) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (e EventID) MarshalJSON() ([]byte, error)    { return json.Marshal(e.String()) }

// ParseWebhookID decodes a webhook short ID strictly.
func ParseWebhookID(short string) (WebhookID, error) {
	key, err := decodeKey(webhookCodec, short)
	return WebhookID(key), err
}

// ParseDeliveryID decodes a delivery short ID strictly.
func ParseDeliveryID(short string) (DeliveryID, error) {
	key, err := decodeKey(deliveryCodec, short)
	return DeliveryID(key), err
}

// ParseEventID decodes an event short ID strictly.
func ParseEventID(short string) (EventID, error) {
	key, err := decodeKey(eventCodec, short)
	return EventID(key), err
}

// ParseWebhookIDOrZero is for adapters: a malformed ID becomes the zero key,
// which matches no row and gets the same not-found as a missing webhook.
func ParseWebhookIDOrZero(short string) WebhookID {
	parsed, err := ParseWebhookID(short)
	if err != nil {
		return 0
	}
	return parsed
}
