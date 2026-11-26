package sourceaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRejectsAddedMissingRenamedAndChangedInputs(t *testing.T) {
	root := t.TempDir()
	put := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("go.mod", "module example.test/review")
	put("go.sum", "dependency checksums")
	put("cmd/server/main.go", "reviewed source")
	put("internal/web/page_windows.go", "reviewed build-tag source")
	put("internal/web/page_templ.go", "reviewed generated source")
	put("internal/web/embed.go", "package web\n \t//go:embed payload.txt\nvar sample string")
	put("internal/web/payload.txt", "original")
	put("internal/i18n/locales/en.json", "reviewed embedded source")
	nativeInputs := []string{"helper.m", "helper.for", "helper.hxx", "helper.sx"}
	for _, name := range nativeInputs {
		put("internal/web/"+name, "reviewed native input")
	}
	if err := WriteReviewed(root); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err != nil {
		t.Fatal(err)
	}
	put("internal/web/page_windows.go", "changed")
	put("go.sum", "different checksums")
	put("go.work", "alternate workspace")
	put("plugin.go", "new root-level Go input")
	put("internal/web/payload.txt", "new embedded payload")
	for _, name := range nativeInputs {
		put("internal/web/"+name, "changed native input")
	}
	if err := os.Rename(filepath.Join(root, "cmd/server/main.go"), filepath.Join(root, "cmd/server/renamed.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "internal/i18n/locales/en.json")); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil {
		t.Fatal("manifest accepted changed source and dependency inputs")
	}
	for _, name := range nativeInputs {
		if !strings.Contains(err.Error(), "changed internal/web/"+name) {
			t.Errorf("manifest accepted a change to %s", name)
		}
	}
	for _, message := range []string{
		"changed go.sum", "changed internal/web/page_windows.go", "added go.work", "added plugin.go",
		"missing cmd/server/main.go", "added cmd/server/renamed.go", "missing internal/i18n/locales/en.json",
		"changed internal/web/payload.txt",
	} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("missing %q in %v", message, err)
		}
	}
}
