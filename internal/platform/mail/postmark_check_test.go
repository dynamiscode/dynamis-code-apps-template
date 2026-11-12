package mail

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPostmarkDomainsFollowsPagesToSenderDomain(t *testing.T) {
	// 150 domains, sender domain only on page two. The fake caps pages at 100 like Postmark.
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		count = min(count, 100)
		const total = 150
		var items []string
		for i := offset; i < offset+count && i < total; i++ {
			name := fmt.Sprintf("filler%d.example", i)
			if i == 120 {
				name = "sender.example"
			}
			items = append(items, fmt.Sprintf(`{"Name":%q,"DKIMVerified":true,"ReturnPathDomainVerified":true}`, name))
		}
		_, _ = fmt.Fprintf(w, `{"TotalCount":%d,"Domains":[%s]}`, total, strings.Join(items, ","))
	}))
	defer server.Close()

	domains, err := PostmarkDomains(context.Background(), server.URL, "pm_account_good")
	if err != nil {
		t.Fatalf("PostmarkDomains() error = %v", err)
	}
	if len(domains) != 150 {
		t.Fatalf("domains = %d, want all 150 across pages; queries=%v", len(domains), queries)
	}
	found := false
	for _, d := range domains {
		if d.Name == "sender.example" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sender domain on page two was not returned; queries=%v", queries)
	}
}

func TestPostmarkDomainsFailsClearlyPastPageCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		count = min(count, 100)
		var items []string
		for i := offset; i < offset+count; i++ {
			items = append(items, fmt.Sprintf(`{"Name":"d%d.example","DKIMVerified":true,"ReturnPathDomainVerified":true}`, i))
		}
		_, _ = fmt.Fprintf(w, `{"TotalCount":1000000,"Domains":[%s]}`, strings.Join(items, ","))
	}))
	defer server.Close()

	_, err := PostmarkDomains(context.Background(), server.URL, "pm_account_good")
	if err == nil || !strings.Contains(err.Error(), "too many sending domains") {
		t.Fatalf("error = %v, want clear too-many-domains failure", err)
	}
}

func TestPostmarkClientDoesNotForwardTokensOnRedirect(t *testing.T) {
	var leaked []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.Header.Get("X-Postmark-Server-Token") + r.Header.Get("X-Postmark-Account-Token"); tok != "" {
			leaked = append(leaked, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"TotalCount":0,"Domains":[]}`)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirector.Close()

	_ = PostmarkServerCheck(context.Background(), redirector.URL, "pm_server_secret")
	_, _ = PostmarkDomains(context.Background(), redirector.URL, "pm_account_secret")
	if len(leaked) != 0 {
		t.Fatalf("tokens were forwarded to redirect target on %v", leaked)
	}
}

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
	if gotQuery != "count=100&offset=0" || len(domains) != 1 || domains[0].Name != "example.com" ||
		!domains[0].DKIMVerified || !domains[0].ReturnPathDomainVerified {
		t.Fatalf("query=%q domains=%+v", gotQuery, domains)
	}
	if _, err := PostmarkDomains(context.Background(), server.URL, "pm_account_bad"); err == nil ||
		strings.Contains(err.Error(), "pm_account_bad") {
		t.Fatalf("bad account token error = %v", err)
	}
}
