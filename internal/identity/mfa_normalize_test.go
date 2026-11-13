package identity

import (
	"strings"
	"testing"
	"time"
)

// TestMFACodeForgivingFormat pins the server half of the forgiving-format rule
// in the form-field-input skill: codes typed or pasted with spaces, dashes, or
// lower case are read the same as the canonical code.
func TestMFACodeForgivingFormat(t *testing.T) {
	for raw, want := range map[string]string{
		"123456": "123456", " 123 456 ": "123456", "123-456": "123456", "12 34\t56": "123456", "12a456": "12a456",
	} {
		if got := normalizeTOTPCode(raw); got != want {
			t.Errorf("normalizeTOTPCode(%q) = %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]string{
		"ABCDE-FGHIJ": "ABCDE-FGHIJ", "abcde-fghij": "ABCDE-FGHIJ", " abcde-fg_ij \n": "ABCDE-FG_IJ", "abc de-fghij": "ABCDE-FGHIJ",
	} {
		if got := normalizeRecoveryCode(raw); got != want {
			t.Errorf("normalizeRecoveryCode(%q) = %q, want %q", raw, got, want)
		}
	}
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code := totpTestCode(secret, now)
	for _, typed := range []string{code, code[:3] + " " + code[3:], code[:3] + "-" + code[3:], " " + code + " "} {
		if !validTOTP(secret, typed, now) {
			t.Errorf("validTOTP rejected %q", typed)
		}
	}
	if validTOTP(secret, strings.Repeat("1", 7), now) {
		t.Error("validTOTP accepted a 7-digit code")
	}
}
