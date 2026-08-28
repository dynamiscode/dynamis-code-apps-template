package portability

import (
	"bytes"
	"testing"
)

func FuzzParseImport(f *testing.F) {
	f.Add("application/json", []byte(`{"formatVersion":"dynamis-code.workspace/v1","items":[{"title":"item","status":"active"}]}`))
	f.Add("text/csv", []byte("title,status\nitem,active\n"))
	f.Add("application/json", []byte("{"))

	service := &Service{maxImportRecords: 100, maxImportBytes: 1 << 20}
	f.Fuzz(func(_ *testing.T, format string, data []byte) {
		_, _ = service.parseImport(ImportInput{Format: format, Reader: bytes.NewReader(data)})
	})
}
