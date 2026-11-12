package webhooks

import (
	"encoding/json"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestWebhookIDMarshalsToShortIDThatDecodesWithWebhookCode(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(WebhookID(7))
	if err != nil {
		t.Fatal(err)
	}
	var short string
	if err := json.Unmarshal(data, &short); err != nil {
		t.Fatalf("webhook JSON %s is not a string", data)
	}
	codec, err := id.NewCodec(id.TypeWebhook, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := codec.Decode(short); err != nil || got != 7 {
		t.Fatalf("Decode(%q) = %d, %v; want 7", short, got, err)
	}
}

func TestParseWebhookIDRejectsIntegerHexAndEmptyIDs(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"7", "0", "", strings.Repeat("a", 32)} {
		if _, err := ParseWebhookID(input); err == nil {
			t.Fatalf("ParseWebhookID(%q) accepted input", input)
		}
		if got := ParseWebhookIDOrZero(input); got != 0 {
			t.Fatalf("ParseWebhookIDOrZero(%q) = %d; want 0", input, got)
		}
	}
}

// Cross-type rejection is not guaranteed: the accepted codec (ADR 0009) lets
// roughly 3% of webhook short IDs also decode under the delivery alphabet. Each
// type must still round-trip through its own parser.
func TestEachIDTypeRoundTripsThroughItsOwnParser(t *testing.T) {
	t.Parallel()

	for n := int64(1); n <= 30; n++ {
		if got, err := ParseWebhookID(WebhookID(n).String()); err != nil || int64(got) != n {
			t.Fatalf("webhook round trip %d = %d, %v", n, got, err)
		}
		if got, err := ParseDeliveryID(DeliveryID(n).String()); err != nil || int64(got) != n {
			t.Fatalf("delivery round trip %d = %d, %v", n, got, err)
		}
		if got, err := ParseEventID(EventID(n).String()); err != nil || int64(got) != n {
			t.Fatalf("event round trip %d = %d, %v", n, got, err)
		}
	}
}

func TestDeliveryAndEventIDsRoundTripAndStayOutOfJSONIntegers(t *testing.T) {
	t.Parallel()

	deliveryJSON, err := json.Marshal(DeliveryID(9))
	if err != nil {
		t.Fatal(err)
	}
	eventJSON, err := json.Marshal(EventID(9))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(deliveryJSON), `"`) {
		t.Fatalf("delivery JSON %s is not a string", deliveryJSON)
	}
	if !strings.HasPrefix(string(eventJSON), `"`) {
		t.Fatalf("event JSON %s is not a string", eventJSON)
	}
	parsed, err := ParseDeliveryID(strings.Trim(string(deliveryJSON), `"`))
	if err != nil || parsed != 9 {
		t.Fatalf("ParseDeliveryID round trip = %d, %v", parsed, err)
	}
}
