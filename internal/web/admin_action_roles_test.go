package web

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestUnavailableInvitationHasBoundedHomeLink(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", ErrorKey: "invitations.unavailable"}
	for name, render := range map[string]func(*bytes.Buffer) error{
		"invitation": func(b *bytes.Buffer) error { return invitationView(data).Render(context.Background(), b) },
		"continue":   func(b *bytes.Buffer) error { return invitationContinueView(data).Render(context.Background(), b) },
	} {
		var body bytes.Buffer
		if err := render(&body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body.String(), `<a class="btn btn-outline btn-sm" href="/">`) {
			t.Errorf("%s: missing bounded native Home link", name)
		}
	}
}

func TestWorkspaceCreationDisclosureRespectsPermission(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []bool{false, true} {
		data := viewmodel.PageData{Catalog: catalog, Locale: "en", CanCreateWorkspace: allowed}
		var body bytes.Buffer
		if err := workspaceSelection(data).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.String(), `class="collapse-title font-semibold rounded-field border border-base-300"`) != allowed {
			t.Errorf("bounded workspace creation disclosure visibility differs from canCreate=%v", allowed)
		}
	}
}
