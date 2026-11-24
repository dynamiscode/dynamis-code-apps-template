// Command preview serves synthetic browser fixtures on localhost.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web"
	"example.com/dynamis-code/apps-template/internal/web/assets"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
)

type preview struct {
	mu         sync.Mutex
	workspaces []identity.WorkspaceSummary
	fail       bool
}

func newPreview(scenario string) (*preview, error) {
	p := &preview{}
	switch scenario {
	case "empty":
	case "populated", "error":
		p.workspaces = []identity.WorkspaceSummary{{ID: 1, Name: "Acme Studio", Role: identity.Owner}}
		p.fail = scenario == "error"
	default:
		return nil, fmt.Errorf("unknown scenario %q", scenario)
	}
	return p, nil
}

func (p *preview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'")
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		http.StripPrefix("/assets/", http.FileServer(http.FS(assets.Files))).ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/components" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/tokens.css"><link rel="stylesheet" href="/assets/components.css"><link rel="stylesheet" href="/assets/app.css"><title>Component fixtures</title></head><body><main class="p-4"><h1>Component fixtures</h1><ul><li><a href="/components/alert">Info alert</a></li><li><a href="/components/alert?state=error">Error alert</a></li></ul></main></body></html>`)
		return
	}
	if r.URL.Path == "/components/alert" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/tokens.css"><link rel="stylesheet" href="/assets/components.css"><link rel="stylesheet" href="/assets/app.css"><title>Alert fixture</title></head><body><main class="p-4">`)
		component := atoms.AlertInfo("This is a synthetic example.")
		if r.URL.Query().Get("state") == "error" {
			component = atoms.AlertError("This is a simulated error.")
		}
		_ = component.Render(r.Context(), w)
		fmt.Fprint(w, `</main></body></html>`)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/workspaces" {
		http.Error(w, "Not available in preview mode", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	message := ""
	if r.Method == http.MethodPost {
		origin, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || origin.Scheme != "http" || origin.Host != r.Host {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.Form.Get("name"))
		locale := r.Form.Get("locale")
		switch {
		case p.fail:
			message = "Simulated save failure. Try again."
		case name == "" || len(name) > 120:
			message = "Enter a workspace name of at most 120 characters."
		case locale != "en" && locale != "es":
			message = "Select a workspace language."
		default:
			p.workspaces = append(p.workspaces, identity.WorkspaceSummary{ID: identity.WorkspaceID(len(p.workspaces) + 1), Name: name, Locale: locale, Role: identity.Owner})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	component, err := web.PreviewWorkspaces(p.workspaces, message)
	if err != nil {
		http.Error(w, "Cannot render preview", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("render preview: %v", err)
	}
}

func main() {
	scenario := flag.String("scenario", "populated", "fixture scenario: empty, populated, or error")
	flag.Parse()
	p, err := newPreview(*scenario)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("http://%s/\n", listener.Addr())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "Invalid host", http.StatusForbidden)
			return
		}
		p.ServeHTTP(w, r)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.Serve(listener))
}
