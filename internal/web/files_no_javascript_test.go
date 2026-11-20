package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Ported from scripts/playwright/files.spec.mjs (javaScriptEnabled:false block).

func TestNoJavaScriptFilesUploadFilterAndLocalizedValidation(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "native-upload.txt", "native upload", env.cookies, nil)
	if upload.Code != http.StatusSeeOther {
		t.Fatalf("native upload = %d: %s", upload.Code, upload.Body.String())
	}
	list := request(env.routes, http.MethodGet, env.filesPath(), nil, env.cookies, nil)
	if !strings.Contains(list.Body.String(), "native-upload.txt") {
		t.Fatalf("uploaded file missing from list: %s", list.Body.String())
	}
	filtered := request(env.routes, http.MethodGet, env.filesPath()+"?search=missing-file", nil, env.cookies, nil)
	if !strings.Contains(filtered.Body.String(), "No files yet.") {
		t.Fatalf("filter with no matches missing empty state: %s", filtered.Body.String())
	}
	spanish := append(append([]*http.Cookie(nil), env.cookies...), &http.Cookie{Name: "locale", Value: "es"})
	invalid := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "unsupported.exe", "unsupported", spanish, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "El archivo seleccionado no es válido.") {
		t.Fatalf("Spanish invalid upload = %d: %s", invalid.Code, invalid.Body.String())
	}
}
