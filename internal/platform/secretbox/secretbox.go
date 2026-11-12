// Package secretbox seals and opens small secrets with versioned AES-256-GCM
// keys. Ciphertext is base64 (raw standard) of nonce || sealed bytes, the
// format webhook secrets already use, so existing rows keep decrypting.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

var (
	ErrNoPrimaryKey = errors.New("secretbox: primary key is not configured")
	ErrUnknownKey   = errors.New("secretbox: key version is not configured")
	ErrInvalidValue = errors.New("secretbox: value cannot be opened")
)

// Keyring holds key material by version. Primary is the version used for new
// values; other versions are kept only to open rows sealed before a change.
type Keyring struct {
	keys    map[int][]byte
	primary int
}

func New(keys map[int][]byte, primary int) Keyring {
	copied := make(map[int][]byte, len(keys))
	for version, key := range keys {
		copied[version] = append([]byte(nil), key...)
	}
	return Keyring{keys: copied, primary: primary}
}

// Configured reports whether a usable primary key exists.
func (k Keyring) Configured() bool {
	return len(k.keys[k.primary]) == 32
}

// Seal encrypts plain under the primary key and returns its version.
func (k Keyring) Seal(plain string) (string, int, error) {
	if !k.Configured() {
		return "", 0, ErrNoPrimaryKey
	}
	gcm, err := newGCM(k.keys[k.primary])
	if err != nil {
		return "", 0, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", 0, err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), k.primary, nil
}

// Open decrypts a value that was sealed under the given key version.
func (k Keyring) Open(version int, value string) (string, error) {
	key, ok := k.keys[version]
	if !ok {
		return "", ErrUnknownKey
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	encoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return "", ErrInvalidValue
	}
	plain, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrInvalidValue
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
