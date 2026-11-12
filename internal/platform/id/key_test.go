package id

import "testing"

func TestNotificationAndAccountTokenTypesAreRegistered(t *testing.T) {
	t.Parallel()

	registered := make(map[TypeCode]bool)
	for _, code := range TypeCodes() {
		registered[code] = true
	}
	for _, code := range []TypeCode{TypeNotification, TypeEmailVerification, TypePasswordReset} {
		if !registered[code] {
			t.Fatalf("type code %q is not registered", code)
		}
		if got, err := DecodeKey(code, EncodeKey(code, 42)); err != nil || got != 42 {
			t.Fatalf("DecodeKey(%q, EncodeKey) = %d, %v; want 42", code, got, err)
		}
	}
}

func TestEntityKeysRoundTripForSharingAndOAuthTypes(t *testing.T) {
	t.Parallel()

	for _, code := range []TypeCode{TypePublicLink, TypeSCIMToken, TypeOAuthClient, TypeOAuthGrant, TypeMFAChallenge, TypeMFARecoveryCode, TypeMFAPasskey, TypeOAuthToken, TypeSession, TypeAuditEvent} {
		for _, key := range []int64{1, 2, 42, 1 << 40} {
			short := EncodeKey(code, key)
			got, err := DecodeKey(code, short)
			if err != nil || got != key {
				t.Fatalf("DecodeKey(%q, EncodeKey(%q, %d)) = %d, %v", code, code, key, got, err)
			}
		}
	}
}

func TestEntityKeysDoNotDecodeAcrossTypes(t *testing.T) {
	t.Parallel()

	short := EncodeKey(TypeSCIMToken, 42)
	got, err := DecodeKey(TypePublicLink, short)
	if err == nil && got == 42 {
		t.Fatalf("DecodeKey(public_link, scim token %q) = 42; want a different key or an error", short)
	}
}

func TestEntityKeysRejectMalformedShortIDs(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "1", "NOT-VALID", "0000"} {
		if _, err := DecodeKey(TypeOAuthGrant, input); err == nil {
			t.Fatalf("DecodeKey(%q) error = nil, want an error", input)
		}
	}
}
