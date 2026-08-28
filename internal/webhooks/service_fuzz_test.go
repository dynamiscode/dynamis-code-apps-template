package webhooks

import "testing"

func FuzzValidateURL(f *testing.F) {
	for _, seed := range []string{
		"https://example.com/hook",
		"http://127.0.0.1:8080/hook",
		"https://10.0.0.1/hook",
		"https://user:pass@example.com/hook",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, raw string) {
		_, _ = validateURL(raw)
	})
}
