package i18n

import (
	"encoding/json"
	"testing"
	"testing/fstest"
)

func TestCatalogsHaveExactKeyParity(t *testing.T) {
	catalogs := make([]map[string]string, 0, len(supported))
	for _, locale := range supported {
		contents, err := localeFiles.ReadFile("locales/" + string(locale) + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(contents, &messages); err != nil {
			t.Fatal(err)
		}
		catalogs = append(catalogs, messages)
	}
	if len(catalogs) != 2 || len(catalogs[0]) != len(catalogs[1]) {
		t.Fatalf("catalog key counts = %d and %d", len(catalogs[0]), len(catalogs[1]))
	}
	for key := range catalogs[0] {
		if _, ok := catalogs[1][key]; !ok {
			t.Fatalf("Spanish catalog missing key %q", key)
		}
	}
}

func TestCatalogRejectsMalformedTemplate(t *testing.T) {
	_, err := newCatalog(fstest.MapFS{
		"locales/en.json": {Data: []byte(`{"common.test":"{{.name"}`)},
		"locales/es.json": {Data: []byte(`{"common.test":"Prueba"}`)},
	})
	if err == nil {
		t.Fatal("newCatalog() error = nil")
	}
}

func TestCatalogFallbackAndLocaleMatching(t *testing.T) {
	catalog, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Translate(Spanish, "common.product_name", nil); got != "Dynamis Code" {
		t.Fatalf("Translate() fallback = %q", got)
	}
	if got := catalog.Translate(Spanish, "workspace.items_title", map[string]any{"name": "Equipo"}); got != "Elementos de Equipo" {
		t.Fatalf("Translate() interpolation = %q", got)
	}
	if got := catalog.Resolve("", "", "", "es-MX,es;q=0.9"); got != Spanish {
		t.Fatalf("Resolve(es-MX) = %q", got)
	}
	if got := catalog.Resolve("", "", "", "fr-FR"); got != English {
		t.Fatalf("Resolve(unsupported) = %q", got)
	}
	if got := catalog.Resolve("", "es", "en", "en"); got != Spanish {
		t.Fatalf("Resolve(cookie) = %q", got)
	}
	if got := catalog.Resolve("en", "es", "es", "es"); got != English {
		t.Fatalf("Resolve(user) = %q", got)
	}
	if got := catalog.Resolve("", "", "es", "en"); got != Spanish {
		t.Fatalf("Resolve(workspace) = %q", got)
	}
}

func TestCatalogFallsBackWhenSpanishKeyIsMissing(t *testing.T) {
	catalog, err := newCatalog(fstest.MapFS{
		"locales/en.json": {Data: []byte(`{"common.test":"English {{.name}}"}`)},
		"locales/es.json": {Data: []byte(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Translate(Spanish, "common.test", map[string]any{"name": "value"}); got != "English value" {
		t.Fatalf("Translate() missing Spanish key = %q", got)
	}
}
