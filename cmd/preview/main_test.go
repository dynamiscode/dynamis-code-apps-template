package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildPreview(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "preview")
	output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build preview: %v\n%s", err, output)
	}
	return binary
}

func startPreview(t *testing.T, binary, scenario string) string {
	t.Helper()
	cmd := exec.Command(binary, "-scenario", scenario)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
		} else {
			lines <- ""
		}
	}()
	select {
	case address := <-lines:
		parsed, err := url.Parse(address)
		if err != nil || parsed.Scheme != "http" || net.ParseIP(parsed.Hostname()) == nil || !net.ParseIP(parsed.Hostname()).IsLoopback() || parsed.Port() == "" {
			t.Fatalf("invalid preview address %q", address)
		}
		return strings.TrimSuffix(address, "/")
	case <-time.After(30 * time.Second):
		t.Fatal("preview did not start")
		return ""
	}
}

func TestPreviewProcessBoundaries(t *testing.T) {
	binary := buildPreview(t)
	a := startPreview(t, binary, "empty")
	b := startPreview(t, binary, "empty")
	if a == b {
		t.Fatal("preview instances share an address")
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := http.NewRequest(http.MethodGet, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = "attacker.example"
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected host status: %d", res.StatusCode)
	}
	r, err = http.NewRequest(http.MethodPost, a+"/workspaces", strings.NewReader(url.Values{"name": {"Isolated studio"}, "locale": {"en"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", a)
	res, err = client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status: %d", res.StatusCode)
	}
	for _, tc := range []struct {
		address string
		want    bool
	}{{a, true}, {b, false}} {
		res, err := client.Get(tc.address)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "Isolated studio") != tc.want {
			t.Fatalf("unexpected state at %s", tc.address)
		}
	}
}

func TestIndependentScenarios(t *testing.T) {
	a, err := newPreview("empty")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newPreview("error")
	if err != nil {
		t.Fatal(err)
	}
	create := func(p *preview, name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/workspaces", strings.NewReader(url.Values{"name": {name}, "locale": {"en"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://localhost")
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		return w
	}
	if got := create(a, "New studio").Code; got != http.StatusSeeOther {
		t.Fatalf("create status = %d", got)
	}
	if got := create(b, "Rejected"); !strings.Contains(got.Body.String(), "Simulated save failure") {
		t.Fatal("missing simulated failure")
	}
	for _, tc := range []struct {
		p            *preview
		want, absent string
	}{{a, "New studio", "Acme Studio"}, {b, "Acme Studio", "New studio"}} {
		w := httptest.NewRecorder()
		tc.p.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
		if !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.absent) {
			t.Fatalf("unexpected workspace state: %s", w.Body.String())
		}
	}
}

func TestComponentIndexNavigation(t *testing.T) {
	p, err := newPreview("empty")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/components", nil))
	for _, href := range []string{"/components/alert", "/components/alert?state=error"} {
		if !strings.Contains(w.Body.String(), `class="btn btn-sm" href="`+href+`"`) {
			t.Errorf("fixture link %q is missing a visible boundary", href)
		}
	}
}

func TestRejectsCrossOriginMutation(t *testing.T) {
	p, err := newPreview("empty")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://localhost/workspaces", strings.NewReader(url.Values{"name": {"Forbidden studio"}, "locale": {"en"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://other.example")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	if strings.Contains(w.Body.String(), "Forbidden studio") {
		t.Fatal("cross-origin request changed workspace state")
	}
}
