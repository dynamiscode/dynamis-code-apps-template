package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestClientDetailBackPreservesListState(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada")
	list := request(env.routes, http.MethodGet, env.listPath()+"?q=Ada&sort=displayName&order=desc", nil, env.cookies, nil)
	link := regexp.MustCompile(`href="([^"]+/clients/[^"?]+\?return=[^"]+)"`).FindStringSubmatch(list.Body.String())
	if len(link) != 2 {
		t.Fatal("detail link does not preserve list state")
	}
	detail := request(env.routes, http.MethodGet, html.UnescapeString(link[1]), nil, env.cookies, nil).Body.String()
	back := regexp.MustCompile(`class="detail-back" href="([^"]+)"`).FindStringSubmatch(detail)
	if len(back) != 2 {
		t.Fatal("back link missing")
	}
	target, err := url.Parse(html.UnescapeString(back[1]))
	if err != nil || target.Path != env.listPath() || target.Query().Get("q") != "Ada" || target.Query().Get("sort") != "displayName" || target.Query().Get("order") != "desc" {
		t.Fatalf("back link did not preserve list state: %q", back[1])
	}
	if strings.Index(detail, `class="detail-back"`) > strings.Index(detail, "<h1>") {
		t.Fatal("back link should precede the heading")
	}
	if got := clientReturnHref(env.listPath(), "https://example.com"); got != env.listPath() {
		t.Fatalf("invalid return query accepted: %q", got)
	}
}
