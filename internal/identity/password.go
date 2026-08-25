package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

type passwordParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultPasswordParams = passwordParams{
	memory:      19 * 1024,
	iterations:  2,
	parallelism: 1,
	saltLength:  16,
	keyLength:   32,
}

func hashPassword(password string, params passwordParams) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, params.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		params.keyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.memory,
		params.iterations,
		params.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(password, encoded string) bool {
	params, salt, want, err := parsePasswordHash(encoded)
	if err != nil {
		return false
	}
	got := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		uint32(len(want)),
	)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func parsePasswordHash(encoded string) (passwordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return passwordParams{}, nil, nil, errors.New("invalid password hash")
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if !strings.HasPrefix(parts[2], "v=") || err != nil || version != argon2.Version {
		return passwordParams{}, nil, nil, errors.New("invalid Argon2 version")
	}
	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 {
		return passwordParams{}, nil, nil, errors.New("invalid Argon2 parameters")
	}
	memory64, memoryErr := strconv.ParseUint(strings.TrimPrefix(parameters[0], "m="), 10, 32)
	iterations64, iterationsErr := strconv.ParseUint(strings.TrimPrefix(parameters[1], "t="), 10, 32)
	parallelism64, parallelismErr := strconv.ParseUint(strings.TrimPrefix(parameters[2], "p="), 10, 8)
	if !strings.HasPrefix(parameters[0], "m=") ||
		!strings.HasPrefix(parameters[1], "t=") ||
		!strings.HasPrefix(parameters[2], "p=") ||
		memoryErr != nil || iterationsErr != nil || parallelismErr != nil {
		return passwordParams{}, nil, nil, errors.New("invalid Argon2 parameters")
	}
	memory := uint32(memory64)
	iterations := uint32(iterations64)
	parallelism := uint8(parallelism64)
	if memory == 0 || memory > 1024*1024 ||
		iterations == 0 || iterations > 10 ||
		parallelism == 0 || parallelism > 16 {
		return passwordParams{}, nil, nil, errors.New("invalid Argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return passwordParams{}, nil, nil, errors.New("invalid password salt")
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return passwordParams{}, nil, nil, errors.New("invalid password key")
	}
	return passwordParams{
		memory:      memory,
		iterations:  iterations,
		parallelism: parallelism,
		saltLength:  uint32(len(salt)),
		keyLength:   uint32(len(key)),
	}, salt, key, nil
}
