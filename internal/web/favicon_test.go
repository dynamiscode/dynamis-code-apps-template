package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFaviconRoutes(t *testing.T) {
	handler := (&Handler{}).Routes()

	for _, test := range []struct {
		path        string
		contentType string
		prefix      []byte
	}{
		{path: "/favicon.ico", contentType: "image/x-icon", prefix: []byte{0, 0, 1, 0}},
		{path: "/assets/favicon.svg", contentType: "image/svg+xml", prefix: []byte("<svg")},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d", test.path, response.Code)
			}
			if got := response.Header().Get("Content-Type"); got != test.contentType {
				t.Errorf("Content-Type = %q, want %q", got, test.contentType)
			}
			if !strings.HasPrefix(response.Body.String(), string(test.prefix)) {
				t.Errorf("GET %s returned unexpected content prefix", test.path)
			}
		})
	}
}
