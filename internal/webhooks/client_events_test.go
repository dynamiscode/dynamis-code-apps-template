package webhooks

import "testing"

func TestClientEventsAreAcceptedAsSubscriptions(t *testing.T) {
	events := []string{"client.created", "client.updated", "client.archived", "client.deleted"}
	if _, err := validateEvents(events); err != nil {
		t.Fatalf("validateEvents(%v) = %v", events, err)
	}
}
