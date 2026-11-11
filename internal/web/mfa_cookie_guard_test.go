package web

import (
	"os"
	"strings"
	"testing"
)

// MFA challenge cookie names must only appear in mfa.go, which owns the
// set/clear lifecycle helpers. Literals elsewhere let cookie drift return.
func TestMFACookieLiteralsOnlyInLifecycleHelpers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	literals := []string{`"mfa_challenge"`, `"mfa_csrf"`, `"mfa_return_to"`}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "mfa.go" {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, literal := range literals {
			if strings.Contains(string(source), literal) {
				t.Fatalf("%s contains MFA cookie literal %s; use setMFAChallengeCookies/clearMFAChallengeCookies in mfa.go", name, literal)
			}
		}
	}
}
