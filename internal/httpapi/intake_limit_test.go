package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIntakeRateLimitSharedAcrossSurfaces(t *testing.T) {
	created := 0
	h := LimitPublicIntake(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		created++
		w.WriteHeader(http.StatusNoContent)
	}), 1)
	request := func(path, source string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.RemoteAddr = source
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	get := httptest.NewRequest(http.MethodGet, "/intake/token", nil)
	get.RemoteAddr = "192.0.2.1:1234"
	h.ServeHTTP(httptest.NewRecorder(), get)
	created = 0
	if got := request("/intake/token", "192.0.2.1:1234").Code; got != http.StatusNoContent {
		t.Fatalf("first submission = %d", got)
	}
	if got := request("/api/v1/intake/token", "192.0.2.1:5678").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second submission = %d, want 429", got)
	}
	if created != 1 {
		t.Fatalf("created %d records, want 1", created)
	}
	if got := request("/api/v1/intake/token", "192.0.2.2:1234").Code; got != http.StatusNoContent {
		t.Fatalf("other source = %d", got)
	}
	if got := request("/other", "192.0.2.1:1234").Code; got != http.StatusNoContent {
		t.Fatalf("other route = %d", got)
	}
}
