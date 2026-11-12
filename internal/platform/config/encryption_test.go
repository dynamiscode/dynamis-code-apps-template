package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadFromSelectsEncryptionKeysWithoutLeakingThem(t *testing.T) {
	general := []byte("abcdefghijklmnopqrstuvwxyz012345")
	legacy := []byte("01234567890123456789012345678901")
	generalEncoded := base64.StdEncoding.EncodeToString(general)
	legacyEncoded := base64.StdEncoding.EncodeToString(legacy)

	tests := []struct {
		name    string
		env     map[string]string
		primary int
		keys    map[int][]byte
	}{
		{"fallback only", map[string]string{"WEBHOOK_ENCRYPTION_KEY": legacyEncoded}, WebhookKeyVersion, map[int][]byte{WebhookKeyVersion: legacy}},
		{"general only", map[string]string{"ENCRYPTION_KEY": generalEncoded}, GeneralKeyVersion, map[int][]byte{GeneralKeyVersion: general}},
		{"both keep fallback for old rows", map[string]string{"ENCRYPTION_KEY": generalEncoded, "WEBHOOK_ENCRYPTION_KEY": legacyEncoded}, GeneralKeyVersion, map[int][]byte{GeneralKeyVersion: general, WebhookKeyVersion: legacy}},
		{"none", map[string]string{}, 0, map[int][]byte{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := LoadFrom(env(test.env))
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Encryption
			if got.Primary != test.primary || len(got.Keys) != len(test.keys) {
				t.Fatalf("encryption = primary %d, %d keys; want primary %d, %d keys", got.Primary, len(got.Keys), test.primary, len(test.keys))
			}
			for version, key := range test.keys {
				if string(got.Keys[version]) != string(key) {
					t.Fatalf("version %d key mismatch", version)
				}
			}
			if got.Configured() != (test.primary != 0) {
				t.Fatalf("Configured() = %v", got.Configured())
			}
		})
	}

	secret := strings.Repeat("ab", 32)
	for _, name := range []string{"ENCRYPTION_KEY", "WEBHOOK_ENCRYPTION_KEY"} {
		if _, err := LoadFrom(env(map[string]string{name: secret[:62]})); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("invalid %s result = %v", name, err)
		}
	}
}
