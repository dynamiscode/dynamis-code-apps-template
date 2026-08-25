package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func newSecret() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func SecretHash(value string) string {
	return hashSecret(value)
}

func equalSecretHash(value, encodedHash string) bool {
	want, err := hex.DecodeString(encodedHash)
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(value))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

func EqualSecretHash(value, encodedHash string) bool {
	return equalSecretHash(value, encodedHash)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
