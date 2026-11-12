package mail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostmarkServerCheckSendsNothingAndRejectsBadToken(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-Postmark-Server-Token") != "pm_server_good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"ID":1,"Name":"Dynamis"}`)
	}))
	defer server.Close()

	if err := PostmarkServerCheck(context.Background(), server.URL, "pm_server_good"); err != nil {
		t.Fatalf("good token error = %v", err)
	}
	err := PostmarkServerCheck(context.Background(), server.URL, "pm_server_bad")
	if err == nil || strings.Contains(err.Error(), "pm_server_bad") {
		t.Fatalf("bad token error = %v, want rejection without secret", err)
	}
	if len(gotPaths) != 2 || gotPaths[0] != "GET /server" {
		t.Fatalf("calls = %v, want only GET /server", gotPaths)
	}
}

func TestPostmarkDomainsReadsWithAccountToken(t *testing.T) {
	var gotToken, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken, gotQuery = r.Header.Get("X-Postmark-Account-Token"), r.URL.RawQuery
		if gotToken != "pm_account_good" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"TotalCount":1,"Domains":[{"Name":"example.com","DKIMVerified":true,"ReturnPathDomainVerified":true}]}`)
	}))
	defer server.Close()

	domains, err := PostmarkDomains(context.Background(), server.URL, "pm_account_good")
	if err != nil {
		t.Fatalf("PostmarkDomains() error = %v", err)
	}
	if gotQuery != "count=500&offset=0" || len(domains) != 1 || domains[0].Name != "example.com" ||
		!domains[0].DKIMVerified || !domains[0].ReturnPathDomainVerified {
		t.Fatalf("query=%q domains=%+v", gotQuery, domains)
	}
	if _, err := PostmarkDomains(context.Background(), server.URL, "pm_account_bad"); err == nil ||
		strings.Contains(err.Error(), "pm_account_bad") {
		t.Fatalf("bad account token error = %v", err)
	}
}
