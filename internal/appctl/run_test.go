package appctl

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunRejectsRemovedItemsCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exit := Run([]string{"items", "list"}, func(string) string { return "" }, &stdout, &stderr); exit != 2 || stdout.Len() != 0 {
		t.Fatalf("removed items command: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
}

func TestRunRejectsUnsafeConfigurationAndRedirects(t *testing.T) {
	secret := "do-not-print-this-token"
	var stdout, stderr bytes.Buffer
	exit := Run([]string{"clients", "list", "--workspace", "0123456789abcdef0123456789abcdef",
		"--client", "0123456789abcdef0123456789abcdef"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			return ""
		}, &stdout, &stderr)
	if exit != 2 || !strings.Contains(stderr.String(), "not valid for this command") {
		t.Fatalf("irrelevant flag exit=%d stderr=%q", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit = Run([]string{"clients", "list", "--workspace", "0123456789abcdef0123456789abcdef",
		"--base-url", "http://user:" + secret + "@example.com"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			return ""
		}, &stdout, &stderr)
	if exit != 2 || strings.Contains(stderr.String(), secret) {
		t.Fatalf("unsafe configuration exit=%d stderr=%q", exit, stderr.String())
	}

	followed := false
	var redirectRequests int
	var redirectAuth string
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectRequests++
		redirectAuth = request.Header.Get("Authorization")
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirect.Close)
	stdout.Reset()
	stderr.Reset()
	exit = Run([]string{"clients", "list", "--workspace", "23456789abcd"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			if key == "APPCTL_BASE_URL" {
				return redirect.URL
			}
			return ""
		}, &stdout, &stderr)
	if exit != 1 || followed || redirectRequests != 1 || redirectAuth != "Bearer "+secret || strings.Contains(stderr.String(), secret) {
		t.Fatalf("redirect exit=%d followed=%v requests=%d authenticated=%t stderr=%q", exit, followed, redirectRequests, redirectAuth == "Bearer "+secret, stderr.String())
	}
}

func runOK(t *testing.T, env func(string) string, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if exit := Run(args, env, &stdout, &stderr); exit != 0 {
		t.Fatalf("Run(%v) exit=%d stderr=%s", args, exit, stderr.String())
	}
	if stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("Run(%v) stdout=%q stderr=%q", args, stdout.String(), stderr.String())
	}
	return stdout.String()
}
