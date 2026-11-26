package actioncheck

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Output protocols are separate review roles. This diagnostic checks the
// inventory of direct raw-output calls in the web package. It is not proof
// that unrecognized calls, other packages or indirect output are safe.
func TestWebRawEffectInventory(t *testing.T) {
	found, err := packages.Load(&packages.Config{
		Dir: "../..", Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedCompiledGoFiles,
	}, "./internal/web", "net/http")
	if err != nil || len(found) != 2 {
		t.Fatalf("load raw effect inventory: %v", err)
	}
	var web *packages.Package
	var writer *types.Interface
	for _, pkg := range found {
		if len(pkg.Errors) != 0 {
			t.Fatalf("type check %s: %v", pkg.PkgPath, pkg.Errors)
		}
		if pkg.PkgPath == "net/http" {
			writer = pkg.Types.Scope().Lookup("ResponseWriter").Type().Underlying().(*types.Interface)
		} else {
			web = pkg
		}
	}
	if web == nil || writer == nil {
		t.Fatal("missing package or HTTP writer capability")
	}
	// Fixed producers only. Files and client stream code are profile-prunable.
	want := map[string]map[string]int{
		"files.go:fileDownload":                      {"io.Copy": 1},
		"engagement_notes.go:previewEngagementNotes": {"writer.Write": 1}, // Goldmark renders HTML without raw HTML or unsafe links.
		"management.go:exportWorkspace":              {"writer.Write": 1},
		"handler.go:serveChangeStream":               {"fmt.Fprint": 2, "encoding/json.NewEncoder": 1},
		"notifications.go:notificationEvents":        {"fmt.Fprint": 3},
		"notifications.go:sendNotifications":         {"fmt.Fprint": 1, "fmt.Fprintf": 2},
		"clients_events.go:sendClientChanges":        {"fmt.Fprint": 1, "fmt.Fprintf": 2},
	}
	const closeFrame = "event: close\ndata: {\"reason\":\"lifetime\"}\n\n"
	const heartbeatFrame = ": heartbeat\n\n"
	const resyncFrame = "event: resync\ndata: {\"reason\":\"cursor\"}\n\n"
	const idFrame = "id: %s\n"
	formats := map[string][]string{
		"handler.go:serveChangeStream": {closeFrame, heartbeatFrame},
		"notifications.go:notificationEvents": {
			`{"type":"urn:dynamis-code:problem:stream-limit","title":"Too Many Requests","status":429,"code":"stream-limit"}`,
			closeFrame, heartbeatFrame,
		},
		"notifications.go:sendNotifications":  {idFrame, resyncFrame, "id: %s\nevent: notification.created\ndata: %s\n\n"},
		"clients_events.go:sendClientChanges": {idFrame, resyncFrame, "id: %s\nevent: client.changed\ndata: %s\n\n"},
	}
	streamTypes := map[string]map[string]bool{
		"handler.go:serveChangeStream":        {"application/problem+json": true, "text/event-stream": true},
		"notifications.go:notificationEvents": {"application/problem+json": true, "text/event-stream": true},
		"notifications.go:sendNotifications":  {},
		"clients_events.go:sendClientChanges": {},
	}
	for i, source := range web.Syntax {
		path := filepath.Base(web.CompiledGoFiles[i])
		if strings.HasSuffix(path, "_templ.go") {
			continue
		}
		for _, declaration := range source.Decls {
			owner, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := path + ":" + owner.Name.Name
			index := indexIngressAliases(owner.Body, web.TypesInfo)
			knownEncoders := map[*ast.CallExpr]bool{}
			knownHeaders := map[*ast.CallExpr]bool{}
			ast.Inspect(owner.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				effect := rawWebEffect(call, index, writer)
				if media, stream := streamTypes[key]; stream {
					if header := checkStreamHeader(t, web, index, writer, call, media); header != nil {
						knownHeaders[header] = true
					}
					if writerHeaderCall(call, web.TypesInfo, writer) && !knownHeaders[call] {
						t.Errorf("unreviewed stream header capability use in %s at %s", key, web.Fset.Position(call.Pos()))
					}
				}
				if key == "handler.go:serveChangeStream" {
					selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
					if method := index.resolve(call.Fun); selector != nil && method != nil && method.FullName() == "(*encoding/json.Encoder).Encode" {
						if encoder, ok := ast.Unparen(selector.X).(*ast.CallExpr); ok &&
							rawWebEffect(encoder, index, writer) == "encoding/json.NewEncoder" &&
							len(call.Args) == 1 && matchesFixedWireJSON(call.Args[0], web.TypesInfo, map[string]string{
							"type": `"urn:dynamis-code:problem:stream-limit"`, "title": `"Too Many Requests"`,
							"status": "429", "detail": `"The realtime connection limit was reached."`, "code": `"stream-limit"`,
						}) {
							knownEncoders[encoder] = true
						}
					}
				}
				if effect == "encoding/json.NewEncoder" && !knownEncoders[call] {
					t.Errorf("unknown encoding/json sink data in %s at %s", key, web.Fset.Position(call.Pos()))
				}
				if effect == "fmt.Fprint" || effect == "fmt.Fprintf" {
					matched := false
					for i, format := range formats[key] {
						if len(call.Args) == 2+strings.Count(format, "%s") &&
							matchesWireLiteral(call.Args[1], web.TypesInfo, format) {
							formats[key] = append(formats[key][:i], formats[key][i+1:]...)
							matched = true
							break
						}
					}
					if !matched {
						t.Errorf("unapproved wire framing in %s at %s", key, web.Fset.Position(call.Pos()))
					}
					if matched && len(call.Args) == 4 {
						payloadPackage, payloadType := web.PkgPath, "notificationEvent"
						if key == "clients_events.go:sendClientChanges" {
							payloadPackage = strings.TrimSuffix(web.PkgPath, "/web") + "/clients"
							payloadType = "Change"
						}
						if !matchesWireMarshal(call.Args[3], owner.Body, index, payloadPackage, payloadType) {
							t.Errorf("unapproved JSON event producer in %s at %s", key, web.Fset.Position(call.Pos()))
						}
					}
				}
				if effect != "" {
					remaining := want[key][effect]
					if remaining == 0 {
						t.Errorf("unreviewed direct web effect %q in %s", effect, key)
					} else {
						want[key][effect] = remaining - 1
					}
				}
				return true
			})
			if len(formats[key]) != 0 {
				t.Errorf("missing reviewed wire format(s) in %s", key)
			}
			if len(streamTypes[key]) != 0 {
				t.Errorf("missing reviewed stream MIME declaration(s) in %s", key)
			}
			for effect, remaining := range want[key] {
				if remaining > 0 {
					t.Errorf("missing expected direct web effect %q in %s", effect, key)
				}
			}
		}
	}
}

func checkStreamHeader(t *testing.T, web *packages.Package, index ingressAliases, writer *types.Interface, call *ast.CallExpr, want map[string]bool) *ast.CallExpr {
	t.Helper()
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	method := index.resolve(call.Fun)
	if !ok || method == nil || method.FullName() != "(net/http.Header).Set" || len(call.Args) != 2 {
		return nil
	}
	header, ok := ast.Unparen(selector.X).(*ast.CallExpr)
	if !ok || !writerHeaderCall(header, web.TypesInfo, writer) {
		t.Errorf("unreviewed header destination at %s", web.Fset.Position(call.Pos()))
		return nil
	}
	key := web.TypesInfo.Types[call.Args[0]].Value
	if key == nil || key.Kind() != constant.String {
		t.Errorf("unreviewed dynamic stream header at %s", web.Fset.Position(call.Pos()))
		return nil
	}
	name := strings.ToLower(constant.StringVal(key))
	if name != "content-type" {
		expected := map[string]string{"retry-after": "30", "cache-control": "no-store", "x-accel-buffering": "no"}
		if approved, ok := expected[name]; !ok || !matchesWireLiteral(call.Args[1], web.TypesInfo, approved) {
			t.Errorf("unreviewed stream metadata %q at %s", name, web.Fset.Position(call.Pos()))
			return nil
		}
		return header
	}
	for media := range want {
		if matchesWireLiteral(call.Args[1], web.TypesInfo, media) {
			delete(want, media)
			return header
		}
	}
	t.Errorf("unreviewed stream MIME declaration at %s", web.Fset.Position(call.Pos()))
	return nil
}

func writerHeaderCall(call *ast.CallExpr, info *types.Info, writer *types.Interface) bool {
	receiver, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selection := info.Selections[receiver]
	return selection != nil && selection.Kind() == types.MethodVal && selection.Obj().Name() == "Header" &&
		types.Implements(selection.Recv(), writer)
}

func rawWebEffect(call *ast.CallExpr, index ingressAliases, writer *types.Interface) string {
	info := index.info
	if node, ok := call.Fun.(*ast.SelectorExpr); ok {
		if selection := info.Selections[node]; selection != nil && node.Sel.Name == "Write" && types.Implements(selection.Recv(), writer) {
			return "writer.Write"
		}
	}
	fn := index.resolve(call.Fun)
	if fn == nil || fn.Pkg() == nil {
		return ""
	}
	name := fn.Pkg().Path() + "." + fn.Name()
	switch name {
	case "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln", "io.Copy", "io.CopyBuffer", "io.WriteString", "encoding/json.NewEncoder":
		if len(call.Args) != 0 && types.Implements(info.TypeOf(call.Args[0]), writer) {
			return name
		}
	}
	return ""
}
