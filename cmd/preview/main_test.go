package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIndependentScenarios(t *testing.T) {
	a, err := newPreview("empty")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newPreview("error")
	if err != nil {
		t.Fatal(err)
	}
	create := func(p *preview, name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/workspaces", strings.NewReader(url.Values{"name": {name}, "locale": {"en"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://localhost")
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		return w
	}
	if got := create(a, "New studio").Code; got != http.StatusSeeOther {
		t.Fatalf("create status = %d", got)
	}
	if got := create(b, "Rejected"); !strings.Contains(got.Body.String(), "Simulated save failure") {
		t.Fatal("missing simulated failure")
	}
	for _, tc := range []struct {
		p            *preview
		want, absent string
	}{{a, "New studio", "Acme Studio"}, {b, "Acme Studio", "New studio"}} {
		w := httptest.NewRecorder()
		tc.p.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
		if !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.absent) {
			t.Fatalf("unexpected workspace state: %s", w.Body.String())
		}
	}
}

func TestRejectsCrossOriginMutation(t *testing.T) {
	p, err := newPreview("empty")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://localhost/workspaces", nil)
	r.Header.Set("Origin", "http://other.example")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}
