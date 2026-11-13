package id

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"strings"

	"github.com/sqids/sqids-go"
)

// TypeCode names an entity type. Each code is never placed in an encoded ID;
// it only seeds that type's alphabet.
type TypeCode string

// Type-code registry. Add every new code to TypeCodes; the registry test
// enforces that codes are non-empty and unique.
const (
	TypeItem            TypeCode = "item"
	TypeWorkspace       TypeCode = "workspace"
	TypeFile            TypeCode = "file"
	TypeWebhook         TypeCode = "webhook"
	TypeWebhookDelivery TypeCode = "webhook_delivery"
	TypeWebhookEvent    TypeCode = "webhook_event"
	TypeJob             TypeCode = "job"
	TypeInvitation      TypeCode = "invitation"
	TypeMember          TypeCode = "member"
	TypeToken           TypeCode = "token"
	TypePublicLink      TypeCode = "public_link"
	TypeSCIMToken       TypeCode = "scim_token"
	TypeOAuthClient     TypeCode = "oauth_client"
	TypeOAuthGrant      TypeCode = "oauth_grant"
	TypeItemEvent       TypeCode = "item_event"
	TypeNotification    TypeCode = "notification"
	TypeMFAPasskey      TypeCode = "mfa_passkey"
	TypeAuditEvent      TypeCode = "audit_event"
	TypeUser            TypeCode = "user"
)

// TypeCodes returns every registered type code.
func TypeCodes() []TypeCode {
	return []TypeCode{
		TypeItem, TypeWorkspace, TypeFile, TypeWebhook, TypeWebhookDelivery,
		TypeWebhookEvent, TypeJob, TypeInvitation, TypeMember, TypeToken,
		TypePublicLink, TypeSCIMToken, TypeOAuthClient, TypeOAuthGrant, TypeItemEvent,
		TypeNotification, TypeMFAPasskey, TypeAuditEvent, TypeUser,
	}
}

// entityCodecs holds one codec per registered type code, built once at init.
var entityCodecs = mustEntityCodecs()

func mustEntityCodecs() map[TypeCode]*Codec {
	codecs := make(map[TypeCode]*Codec)
	for _, code := range TypeCodes() {
		codec, err := NewCodec(code, "")
		if err != nil {
			panic("id: short-id codec for " + string(code) + ": " + err.Error())
		}
		codecs[code] = codec
	}
	return codecs
}

// EncodeKey returns the short ID for a positive integer key of type code.
// Callers pass only keys from the database; a non-positive key yields "".
func EncodeKey(code TypeCode, key int64) string {
	codec, ok := entityCodecs[code]
	if !ok || key <= 0 {
		return "" // an unregistered code has no short-ID alphabet
	}
	short, _ := codec.Encode(uint64(key))
	return short
}

// DecodeKey strictly decodes a short ID of type code to its integer key.
// Malformed input and IDs of another type return ErrInvalidID.
func DecodeKey(code TypeCode, short string) (int64, error) {
	value, err := entityCodecs[code].Decode(short)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidID
	}
	return int64(value), nil
}

// ShortIDAlphabet is the alphabet every short ID uses; the CLI validates against it.
const ShortIDAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

const (
	baseAlphabet = ShortIDAlphabet
	minLength    = 4
)

// ErrInvalidID is returned when a short ID fails strict decoding.
var ErrInvalidID = errors.New("invalid short id")

// Codec encodes integer IDs to short strings for one type code, using an
// alphabet shuffled from the type code and an optional seed.
type Codec struct {
	alphabet  string
	sq        *sqids.Sqids
	maxLength int
}

// NewCodec returns a codec for code. An empty seed derives the alphabet from
// the type code alone.
func NewCodec(code TypeCode, seed string) (*Codec, error) {
	// The seed is domain-separated from the type code: XOR would let a seed
	// equal to another type's name collide with that type. Type codes never
	// contain NUL, so the separator keeps the two inputs unambiguous.
	key := hashToUint64(string(code))
	if seed != "" {
		seedInput := sha256.Sum256([]byte(seed))
		key = hashToUint64(string(code) + "\x00" + string(seedInput[:8]))
	}
	alphabet := shuffle(baseAlphabet, key)
	// ponytail: no Sqids blocklist (passing options disables the default), so
	// random 4-char IDs could spell a banned word. Add the default list when that matters.
	sq, err := sqids.New(sqids.Options{Alphabet: alphabet, MinLength: minLength})
	if err != nil {
		return nil, err
	}
	c := &Codec{alphabet: alphabet, sq: sq}
	widest, err := c.Encode(math.MaxUint64)
	if err != nil {
		return nil, err
	}
	c.maxLength = len(widest)
	return c, nil
}

// Encode returns the short string for n.
func (c *Codec) Encode(n uint64) (string, error) {
	return c.sq.Encode([]uint64{n})
}

// Decode returns the integer for s. It rejects bad length, characters outside
// the codec alphabet, and any input that does not re-encode to exactly s.
func (c *Codec) Decode(s string) (uint64, error) {
	if len(s) < minLength || len(s) > c.maxLength {
		return 0, ErrInvalidID
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(c.alphabet, s[i]) < 0 {
			return 0, ErrInvalidID
		}
	}
	values := c.sq.Decode(s)
	if len(values) != 1 {
		return 0, ErrInvalidID
	}
	canonical, err := c.Encode(values[0])
	if err != nil || canonical != s {
		return 0, ErrInvalidID
	}
	return values[0], nil
}

func hashToUint64(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(sum[:8])
}

// shuffle is a deterministic Fisher-Yates permutation driven by splitmix64.
func shuffle(in string, key uint64) string {
	b := []byte(in)
	state := key
	for i := len(b) - 1; i > 0; i-- {
		state += 0x9e3779b97f4a7c15
		z := state
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		z ^= z >> 31
		j := int(z % uint64(i+1))
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
