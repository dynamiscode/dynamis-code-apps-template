package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogWebWorkspaceIsolation(t *testing.T) {
	webHandler, auth, workspace, owner := testWebRaw(t, 2)
	enableTestBitcoin(t, webHandler, auth, workspace, owner.UserID)
	routes := webHandler.Routes()
	ctx := context.Background()
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enableTestBitcoin(t, webHandler, auth, other, owner.UserID)
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/catalog"
	otherPath := "/workspaces/" + other.String() + "/catalog"
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Private"}, "mode": {"fixed"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Private"}}
	if r := request(routes, http.MethodPost, path, form, cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatal(r.Code)
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	detail := catalogDetailLinks(t, page.Body.String())[0]
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
	match := regexp.MustCompile(`action="([^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("missing entry")
	}
	entry := strings.TrimSuffix(match[1], "/edit")
	if r := request(routes, http.MethodPost, entry+"/prices", form, cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatal(r.Code)
	}
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
	match = regexp.MustCompile(`action="([^"]+/prices/[^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("missing price")
	}
	price := match[1]
	page = request(routes, http.MethodGet, otherPath+"?includeArchived=true", nil, cookies, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Private") {
		t.Fatalf("foreign list: %d", page.Code)
	}
	foreign := strings.Replace(entry, path, otherPath, 1)
	foreignPrice := strings.Replace(price, path, otherPath, 1)
	localForm := url.Values{"csrf": {session.CSRFSecret}, "name": {"Local entry"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Local price"}}
	if r := request(routes, http.MethodPost, otherPath, localForm, cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatalf("create local entry: %d", r.Code)
	}
	localPage := request(routes, http.MethodGet, otherPath, nil, cookies, nil)
	localPage = request(routes, http.MethodGet, catalogDetailLinks(t, localPage.Body.String())[0], nil, cookies, nil)
	localMatch := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/edit)"`).FindStringSubmatch(localPage.Body.String())
	if len(localMatch) != 2 {
		t.Fatal("missing local entry")
	}
	localEntry := strings.TrimSuffix(localMatch[1], "/edit")
	foreignPriceOnLocal := localEntry + strings.TrimPrefix(foreignPrice, foreign)
	invalidEntry := url.Values{"csrf": {session.CSRFSecret}, "name": {"Rejected foreign draft"}, "duration_minutes": {"-3"}}
	invalidPrice := url.Values{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"-1"}, "currency": {"DRAFT"}, "variant": {"Rejected foreign draft"}}
	for _, op := range []struct {
		name, target string
		values       url.Values
	}{
		{"foreign entry edit", foreign + "/edit", invalidEntry},
		{"foreign price edit", foreignPrice, invalidPrice},
		{"foreign price on local entry", foreignPriceOnLocal, invalidPrice},
		{"foreign price addition", foreign + "/prices", invalidPrice},
	} {
		t.Run(op.name, func(t *testing.T) {
			r := request(routes, http.MethodPost, op.target, op.values, cookies, nil)
			if r.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d", r.Code)
			}
			body := r.Body.String()
			if !strings.Contains(body, "Local entry") || !strings.Contains(body, "Local price") {
				t.Fatal("local entry or price missing")
			}
			for _, forbidden := range []string{"Private", "0.123456789012345678", "Rejected foreign draft", `value="DRAFT"`, `value="-1"`, `value="-3"`} {
				if strings.Contains(body, forbidden) {
					t.Errorf("rejected foreign submission exposed %q", forbidden)
				}
			}
		})
	}
	for _, op := range []struct {
		target string
		want   int
	}{{foreign + "/edit", 404}, {foreign + "/archive", 404}, {foreign + "/unarchive", 404}, {foreign + "/delete", 404}, {foreign + "/prices", 422}, {foreignPrice, 404}, {strings.TrimSuffix(foreignPrice, "/edit") + "/delete", 404}} {
		r := request(routes, http.MethodPost, op.target, form, cookies, nil)
		if r.Code != op.want {
			t.Errorf("%s: %d want %d", op.target, r.Code, op.want)
		}
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Private") || !strings.Contains(page.Body.String(), "0.123456789012345678") {
		t.Fatal("original changed")
	}
}
