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

func TestClientEditPreservesListReturn(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	detail := created.Header().Get("Location")
	raw := "q=Ada&sort=displayName"
	detailWithReturn := detail + "?return=" + url.QueryEscape(raw)
	page := request(env.routes, http.MethodGet, detailWithReturn, nil, env.cookies, nil).Body.String()
	action := regexp.MustCompile(`action="([^"]+/field\?return=[^"]+)"`).FindStringSubmatch(page)
	if len(action) != 2 {
		t.Fatal("inline editor lost list return context")
	}
	saved := request(env.routes, http.MethodPost, html.UnescapeString(action[1]), url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"display_name"}, "value": {"Ada Updated"}}, env.cookies, nil)
	location, err := url.Parse(saved.Header().Get("Location"))
	if err != nil || saved.Code != http.StatusSeeOther || location.Query().Get("return") != raw {
		t.Fatalf("edit lost list return context: %d %q", saved.Code, saved.Header().Get("Location"))
	}
}
