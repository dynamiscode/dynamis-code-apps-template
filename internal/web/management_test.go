package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/sharing"
)

func testMultiInstanceWeb(t *testing.T) (http.Handler, http.Handler, *identity.Service, string, identity.BootstrapResult) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	sharingService := sharing.NewService(db, cfg.Database.Driver, auth)
	portabilityService := portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService)

	instance1, err := NewHandlerWithServices(auth, itemService, sharingService, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	instance2, err := NewHandlerWithServices(auth, itemService, sharingService, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return instance1.Routes(), instance2.Routes(), auth, owner.WorkspaceID, owner
}

func TestMultiInstanceImportFlashStateless(t *testing.T) {
	instance1, instance2, auth, workspaceID, owner := testMultiInstanceWeb(t)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	root := "/workspaces/" + workspaceID + "/settings/import"

	// 1. Post import to instance 1
	postResp := multipartRequest(t, instance1, root, url.Values{"csrf": {session.CSRFSecret}, "confirm": {"yes"}}, "items.csv", "title,status\nItem One,active\nItem Two,complete\n", cookies, nil)
	expectedRedirect := root + "?imported=" + url.QueryEscape(fmt.Sprintf("%d", 2))
	if postResp.Code != http.StatusSeeOther || postResp.Header().Get("Location") != expectedRedirect {
		t.Fatalf("expected 303 redirect to %q, got code %d, location %q", expectedRedirect, postResp.Code, postResp.Header().Get("Location"))
	}

	// 2. Follow redirect to instance 2 (verifies statelessness across multi-instance deployment)
	getResp := request(instance2, http.MethodGet, postResp.Header().Get("Location"), nil, cookies, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from instance2, got %d", getResp.Code)
	}
	body := getResp.Body.String()
	if !strings.Contains(body, "Items imported:") || !strings.Contains(body, "2") {
		t.Fatalf("expected instance2 to render import confirmation with 2 items, got:\n%s", body)
	}
	if !strings.Contains(body, `class="success" role="status"`) {
		t.Fatalf("expected instance2 to render success role=status element, got:\n%s", body)
	}

	// 3. Requesting clean import page on instance 2 does not show flash
	cleanResp := request(instance2, http.MethodGet, root, nil, cookies, nil)
	if cleanResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", cleanResp.Code)
	}
	if strings.Contains(cleanResp.Body.String(), "Items imported:") {
		t.Fatalf("expected instance2 clean page not to show import confirmation, got:\n%s", cleanResp.Body.String())
	}

	// 4. Test JSON import on instance 2, follow redirect on instance 1
	jsonExport, err := json.Marshal(portability.Export{
		FormatVersion: portability.FormatVersion,
		Items:         []items.Item{{Title: "Imported JSON", Status: items.Active}},
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonPostResp := multipartRequest(t, instance2, root, url.Values{"csrf": {session.CSRFSecret}, "confirm": {"yes"}}, "items.json", string(jsonExport), cookies, nil)
	expectedJSONRedirect := root + "?imported=" + url.QueryEscape(fmt.Sprintf("%d", 1))
	if jsonPostResp.Code != http.StatusSeeOther || jsonPostResp.Header().Get("Location") != expectedJSONRedirect {
		t.Fatalf("expected 303 redirect to %q, got code %d, location %q", expectedJSONRedirect, jsonPostResp.Code, jsonPostResp.Header().Get("Location"))
	}
	instance1Get := request(instance1, http.MethodGet, jsonPostResp.Header().Get("Location"), nil, cookies, nil)
	if instance1Get.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from instance1, got %d", instance1Get.Code)
	}
	if !strings.Contains(instance1Get.Body.String(), "Items imported:") || !strings.Contains(instance1Get.Body.String(), "1") {
		t.Fatalf("expected instance1 to render import confirmation with 1 item, got:\n%s", instance1Get.Body.String())
	}
}

func TestImportPageQueryParamHandling(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	root := "/workspaces/" + workspaceID + "/settings/import"

	tests := []struct {
		name         string
		target       string
		wantConfirm  bool
		wantCountStr string
	}{
		{
			name:         "valid positive count",
			target:       root + "?imported=5",
			wantConfirm:  true,
			wantCountStr: "5",
		},
		{
			name:         "valid zero count",
			target:       root + "?imported=0",
			wantConfirm:  true,
			wantCountStr: "0",
		},
		{
			name:        "no query param",
			target:      root,
			wantConfirm: false,
		},
		{
			name:        "non-numeric param",
			target:      root + "?imported=abc",
			wantConfirm: false,
		},
		{
			name:        "negative count",
			target:      root + "?imported=-1",
			wantConfirm: false,
		},
		{
			name:        "empty param",
			target:      root + "?imported=",
			wantConfirm: false,
		},
		{
			name:        "whitespace param",
			target:      root + "?imported=%20%20",
			wantConfirm: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := request(handler, http.MethodGet, tc.target, nil, cookies, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.Code)
			}
			body := resp.Body.String()
			hasConfirm := strings.Contains(body, "Items imported:")
			if hasConfirm != tc.wantConfirm {
				t.Fatalf("target %s: wantConfirm=%v, but hasConfirm=%v\nbody: %s", tc.target, tc.wantConfirm, hasConfirm, body)
			}
			if tc.wantConfirm && !strings.Contains(body, tc.wantCountStr) {
				t.Fatalf("target %s: expected count %s in body\nbody: %s", tc.target, tc.wantCountStr, body)
			}
		})
	}
}
