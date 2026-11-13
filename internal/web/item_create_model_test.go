package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// TestItemCreateModelSteps replays the steps of verification/web/ItemCreate.tla
// against the real handler. Each subtest pins one server or page fact the model
// relies on. If one fails, the code changed: update the model before trusting
// it (see the item-create-submission skill).
func TestItemCreateModelSteps(t *testing.T) {
	handler, _, _, workspaceID, _ := testWeb(t, 10)
	cookies, csrf := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"
	hx := map[string]string{"HX-Request": "true"}
	post := func(title, key string) (int, string) {
		response := request(handler, http.MethodPost, path, url.Values{
			"title": {title}, "idempotency_key": {key}, "csrf": {csrf},
		}, cookies, hx)
		return response.Code, response.Body.String()
	}
	createKey := regexp.MustCompile(`name="idempotency_key" value="([^"]+)"`)
	newTitle := regexp.MustCompile(`<input id="new-title"[^>]*>`)

	t.Run("ServerAnswers ok then replay: one press resent makes one item", func(t *testing.T) {
		code, body := post("Alpha", "model-key-one")
		if code != http.StatusOK || strings.Count(body, `value="Alpha"`) != 1 {
			t.Fatalf("first press = %d", code)
		}
		code, body = post("Alpha", "model-key-one")
		if code != http.StatusOK || strings.Count(body, `value="Alpha"`) != 1 {
			t.Fatalf("resent press = %d, Alpha shown %d times", code, strings.Count(body, `value="Alpha"`))
		}
	})

	t.Run("ResponseArrives swaps in a new form with a new key", func(t *testing.T) {
		_, first := post("Beta", "model-key-two")
		_, second := post("Gamma", "model-key-three")
		a, b := createKey.FindStringSubmatch(first), createKey.FindStringSubmatch(second)
		if a == nil || b == nil || a[1] == b[1] {
			t.Fatalf("create keys %v and %v, want two different keys", a, b)
		}
	})

	t.Run("ServerAnswers conflict: a reused key with another title is rejected and kept", func(t *testing.T) {
		post("Delta", "model-key-four")
		code, body := post("Epsilon", "model-key-four")
		input := newTitle.FindString(body)
		if code != http.StatusOK || !strings.Contains(body, `role="alert"`) || !strings.Contains(input, `value="Epsilon"`) {
			t.Fatalf("reused key, new title = %d, input %q; the model says an alert and the title kept", code, input)
		}
	})

	t.Run("ServerAnswers invalid: a blank title keeps what you typed", func(t *testing.T) {
		code, body := post("   ", "model-key-five")
		input := newTitle.FindString(body)
		if code != http.StatusOK || !strings.Contains(body, `role="alert"`) || !strings.Contains(input, `value="   "`) {
			t.Fatalf("blank title = %d, input %q; the model says an alert and the typed title kept", code, input)
		}
	})

	t.Run("ResponseArrives after a save: an empty box and a toast", func(t *testing.T) {
		response := request(handler, http.MethodPost, path, url.Values{
			"title": {"Zeta"}, "idempotency_key": {"model-key-six"}, "csrf": {csrf},
		}, cookies, hx)
		input := newTitle.FindString(response.Body.String())
		if strings.Contains(input, "value=\"Zeta\"") || !strings.Contains(response.Header().Get("HX-Trigger"), "app:toast") {
			t.Fatalf("saved create: input %q, HX-Trigger %q", input, response.Header().Get("HX-Trigger"))
		}
	})

	t.Run("YouPressAdd: the form drops presses while one is in flight", func(t *testing.T) {
		_, body := post("Eta", "model-key-seven")
		form := regexp.MustCompile(`<form class="create"[^>]*>`).FindString(body)
		if !strings.Contains(form, `hx-sync="this:drop"`) {
			t.Fatalf("create form %q lost hx-sync drop; update YouPressAdd in the model", form)
		}
	})
}
