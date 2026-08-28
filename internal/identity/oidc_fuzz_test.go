package identity

import "testing"

func FuzzValidatePublicHTTPSURL(f *testing.F) {
	for _, seed := range []string{
		"https://id.example.com/tenant",
		"http://id.example.com",
		"https://localhost",
		"https://169.254.169.254/latest/meta-data",
		"https://user:pass@id.example.com",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, raw string) {
		if len(raw) > 4096 {
			raw = raw[:4096]
		}
		_ = validatePublicHTTPSURL(raw)
	})
}
