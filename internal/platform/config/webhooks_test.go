package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadFromParsesWebhookEncryptionKeyWithoutLeakingIt(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	encoded := base64.StdEncoding.EncodeToString(key)
	cfg, err := LoadFrom(env(map[string]string{"WEBHOOK_ENCRYPTION_KEY": encoded}))
	if err != nil || string(cfg.Webhooks.SecretKey) != string(key) {
		t.Fatalf("webhook key = %x, err = %v", cfg.Webhooks.SecretKey, err)
	}
	secret := strings.Repeat("ab", 32)
	if _, err := LoadFrom(env(map[string]string{"WEBHOOK_ENCRYPTION_KEY": secret[:62]})); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("invalid webhook key result = %v", err)
	}
}
