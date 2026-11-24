package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// dashboardLink returns the actual destination and visible text of a dashboard card.
func dashboardLink(t *testing.T, body, href string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var text func(*html.Node) string
	text = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var result string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			result += text(c)
		}
		return result
	}
	var find func(*html.Node) string
	find = func(n *html.Node) string {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && a.Val == href {
					return strings.Join(strings.Fields(text(n)), " ")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if result := find(c); result != "" {
				return result
			}
		}
		return ""
	}
	return find(doc)
}

func TestEngagementDashboardWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	path := "/workspaces/" + env.workspaceID.String()
	empty := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "No open stages yet.") || strings.Count(empty.Body.String(), "No actions match these filters.") < 2 {
		t.Fatalf("empty dashboard: %d", empty.Code)
	}
	for _, filter := range []string{"overdue", "today"} {
		href := path + "/engagements/due?filter=" + filter
		if got := dashboardLink(t, empty.Body.String(), href); !strings.Contains(got, "0") {
			t.Fatalf("empty %s card = %q", filter, got)
		}
		response := request(env.routes, http.MethodGet, href, nil, env.viewerCookies, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "No actions match these filters.") {
			t.Fatalf("empty %s destination: %d", filter, response.Code)
		}
	}
	emptyStage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Empty stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Dashboard stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Dashboard client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	for _, due := range []string{"2000-01-01", today} {
		item, err := svc.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Dashboard work " + due, ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{Text: "Follow up " + due, DueDate: due, OwnerID: env.ownerID}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	other, err := env.auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := env.auth.Authorize(ctx, env.ownerID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	otherStage, err := svc.CreateStage(ctx, otherActor, other, engagement.StageInput{Name: "Other stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherClient, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, otherActor, other, clients.Input{DisplayName: "Other client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherItem, err := svc.Create(ctx, otherActor, other, engagement.Input{Name: "Other secret work", ClientID: otherClient.ID, StageID: otherStage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, otherActor, other, otherItem.ID, engagement.NextAction{Text: "Other action", DueDate: "2000-01-01", OwnerID: env.ownerID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
		response := request(env.routes, http.MethodGet, path, nil, cookies, nil)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "Other stage") || strings.Contains(response.Body.String(), "Other action") || !strings.Contains(response.Body.String(), "Follow up 2000-01-01") || !strings.Contains(response.Body.String(), "Follow up "+today) || !strings.Contains(response.Body.String(), "Dashboard client") {
			t.Fatalf("dashboard status or workspace isolation: %d", response.Code)
		}
		for _, tc := range []struct {
			href, card         string
			included, excluded string
		}{
			{path + "/engagements/due?filter=overdue", "Overdue1 ›", "Dashboard work 2000-01-01", "Dashboard work " + today},
			{path + "/engagements/due?filter=today", "Due today1 ›", "Dashboard work " + today, "Dashboard work 2000-01-01"},
			{path + "/engagements?stage=" + stage.ID.String(), "Dashboard stage — 2", "Dashboard work 2000-01-01", "Other secret work"},
			{path + "/engagements?stage=" + emptyStage.ID.String(), "Empty stage — 0", "No engagements match these filters.", "Dashboard work"},
		} {
			card := dashboardLink(t, response.Body.String(), tc.href)
			if card != tc.card {
				t.Errorf("card %s = %q, want %q", tc.href, card, tc.card)
			}
			destination := request(env.routes, http.MethodGet, tc.href, nil, cookies, nil)
			body := destination.Body.String()
			if destination.Code != http.StatusOK || !strings.Contains(body, tc.included) || strings.Contains(body, tc.excluded) || strings.Contains(body, "Other secret work") || (tc.href == path+"/engagements?stage="+stage.ID.String() && !strings.Contains(body, "Dashboard work "+today)) {
				t.Errorf("destination %s: status %d, missing %q or contains excluded work", tc.href, destination.Code, tc.included)
			}
		}
		otherResponse := request(env.routes, http.MethodGet, "/workspaces/"+other.String(), nil, env.viewerCookies, nil)
		if otherResponse.Code == http.StatusOK {
			t.Error("viewer accessed another workspace dashboard")
		}
	}
	if response := request(env.routes, http.MethodGet, path+"?page=0", nil, env.cookies, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid page: %d", response.Code)
	}
	admin, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, admin, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Panel de gestiones") {
		t.Fatalf("Spanish dashboard: %d", response.Code)
	}
}
