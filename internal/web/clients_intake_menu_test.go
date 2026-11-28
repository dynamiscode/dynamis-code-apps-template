package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestClientIntakeMenuLoadsAuthorizedLink(t *testing.T) {
	env := newClientsTestEnv(t)
	settings := "/workspaces/" + env.workspaceID.String() + "/intake"
	response := request(env.routes, http.MethodPost, settings, url.Values{
		"csrf": {env.csrf}, "action": {"create"}, "consent": {"I agree"},
	}, env.cookies, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("create link: status %d", response.Code)
	}
	response = request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "until you create an open engagement stage") || strings.Contains(response.Body.String(), `id="client-intake-link"`) {
		t.Fatalf("owner: missing readiness guidance or exposed unusable link, status %d", response.Code)
	}
	response = request(env.routes, http.MethodGet, env.listPath(), nil, env.viewerCookies, nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `id="client-intake-link"`) {
		t.Fatalf("viewer: link exposed or request failed, status %d", response.Code)
	}
}

func TestClientIntakeActionsHaveBoundaries(t *testing.T) {
	data := viewmodel.PageData{CurrentPath: "/workspaces/1/clients", IntakeActive: true, IntakeReady: true, IntakeLink: "/intake/example"}
	var out bytes.Buffer
	if err := clientIntakeMenu(data).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<summary>`, `class="btn btn-primary btn-sm" data-copy-target=`, `class="btn btn-outline btn-sm" href="/intake/example"`, `class="btn btn-outline btn-sm" href="/workspaces/1/intake"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing bounded action %q", want)
		}
	}
}

func TestClientIntakeMenuStates(t *testing.T) {
	const link = "/intake/private-token"
	cases := []struct {
		name   string
		data   viewmodel.PageData
		want   []string
		absent []string
	}{
		{"unauthorized", viewmodel.PageData{IntakeEnabled: true, IntakeActive: true, IntakeLink: link}, nil, []string{"client-intake-link", link}},
		{"disabled feature", viewmodel.PageData{CanManageFields: true, IntakeActive: true, IntakeLink: link}, nil, []string{"client-intake-link", link}},
		{"inactive", viewmodel.PageData{CanManageFields: true, IntakeEnabled: true, IntakeLink: link}, []string{"/workspaces/1/intake"}, []string{"client-intake-link", link}},
		{"active", viewmodel.PageData{CanManageFields: true, IntakeEnabled: true, IntakeActive: true, IntakeReady: true, IntakeLink: link}, []string{`id="client-intake-link"`, `readonly`, `data-copy-target="client-intake-link"`, `href="/intake/private-token"`, `href="/workspaces/1/intake"`}, nil},
		{"unavailable", viewmodel.PageData{CanManageFields: true, IntakeEnabled: true, IntakeActive: true, IntakeReady: true, IntakeUnavailable: true, IntakeLink: link}, []string{"/workspaces/1/intake"}, []string{"client-intake-link", link}},
		{"missing link", viewmodel.PageData{CanManageFields: true, IntakeEnabled: true, IntakeActive: true, IntakeReady: true}, []string{"/workspaces/1/intake"}, []string{"client-intake-link"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.data.CurrentPath = "/workspaces/1/clients"
			var out bytes.Buffer
			if err := clientCreateMenu(tc.data).Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("unexpected %q", absent)
				}
			}
		})
	}
}

func TestClientManageMenuAuthorizationAndPlacement(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada")
	owner := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil).Body.String()
	newClient := strings.Index(owner, `data-client-create`)
	manage := strings.Index(owner, `aria-label="Manage"`)
	if newClient < 0 || manage < newClient || !strings.Contains(owner, ">Manage form</a>") || !strings.Contains(owner, ">Manage custom fields</a>") {
		t.Fatal("owner must have the management menu after New client")
	}
	menuStart := strings.LastIndex(owner[:manage], "<summary")
	menuEnd := strings.Index(owner[manage:], `id="client-results"`)
	if menuStart < 0 || menuEnd < 0 {
		t.Fatal("management menu must precede the client results")
	}
	menu := owner[menuStart : manage+menuEnd]
	for _, want := range []string{
		`btn-outline btn-square`, `&#8942;`, `menu dropdown-content`,
		`href="/workspaces/` + env.workspaceID.String() + `/engagements/due"`,
		`<summary>Share the form</summary>`,
		`href="/workspaces/` + env.workspaceID.String() + `/intake"`,
		`href="/workspaces/` + env.workspaceID.String() + `/client-fields"`,
		`class="border border-base-300"`,
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("management menu missing %q", want)
		}
	}
	if strings.Contains(menu, `href="/workspaces/`+env.workspaceID.String()+`/engagements"`) {
		t.Fatal("clients menu must not link to the bulleted list")
	}
	viewer := request(env.routes, http.MethodGet, env.listPath(), nil, env.viewerCookies, nil).Body.String()
	if strings.Contains(viewer, ">Manage form</a>") || strings.Contains(viewer, ">Manage custom fields</a>") || strings.Contains(viewer, "<summary>Share the form</summary>") || !strings.Contains(viewer, `/engagements/due"`) {
		t.Fatal("viewer must not see management actions")
	}
}

func TestClientManageMenuWithoutIntake(t *testing.T) {
	data := viewmodel.PageData{CurrentPath: "/workspaces/1/clients", FieldsPath: "/workspaces/1/client-fields", CanManageFields: true}
	var out bytes.Buffer
	if err := clientCreateMenu(data).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `href="/workspaces/1/intake"`) || !strings.Contains(out.String(), `href="/workspaces/1/client-fields"`) {
		t.Fatal("disabled intake must hide its action while preserving custom fields")
	}
}
