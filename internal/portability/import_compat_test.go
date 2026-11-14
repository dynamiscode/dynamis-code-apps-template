package portability

import (
	"errors"
	"strings"
	"testing"
)

// The importer reads v1 documents with and without the additive client keys in
// every build, so a Clients-off app still imports a Clients-on export.
func TestParseImportAcceptsExportsWithAndWithoutClientKeys(t *testing.T) {
	service := &Service{maxImportRecords: 100, maxImportBytes: 1 << 20}
	const header = `"formatVersion":"dynamis-code.workspace/v1","exportedAt":"2026-08-25T20:00:00Z",` +
		`"workspace":{"id":"1","name":"Compat","locale":"en","createdAt":"2026-08-01T00:00:00Z"},` +
		`"members":[],"items":[{"title":"Compat item","status":"active"}]`
	legacy := `{` + header + `,"auditEvents":[],"excluded":[]}`
	withClients := `{` + header + `,` +
		`"clients":[{"id":"c1","displayName":"Client","email":"","phone":"","notes":"","archivedAt":null,` +
		`"version":1,"createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-01T00:00:00Z",` +
		`"customValues":{"f7k2":"Partner"}}],` +
		`"clientFields":[{"id":"f1","label":"Referral","type":"text","archivedAt":null,` +
		`"createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-01T00:00:00Z"}],` +
		`"clientFieldOptions":[],"auditEvents":[],"excluded":[]}`
	unknown := `{` + header + `,"bogus":true,"auditEvents":[],"excluded":[]}`
	for name, document := range map[string]string{"legacy": legacy, "with client keys": withClients} {
		records, err := service.parseImport(ImportInput{Format: "application/json", Reader: strings.NewReader(document)})
		if err != nil || len(records) != 1 || records[0].Title != "Compat item" {
			t.Fatalf("%s export = %+v, %v; want its one item", name, records, err)
		}
	}
	// Unknown top-level keys are still rejected; only the declared client keys are new.
	if _, err := service.parseImport(ImportInput{Format: "application/json", Reader: strings.NewReader(unknown)}); !errors.Is(err, ErrInvalidImport) {
		t.Fatalf("unknown key import error = %v; want ErrInvalidImport", err)
	}
}
