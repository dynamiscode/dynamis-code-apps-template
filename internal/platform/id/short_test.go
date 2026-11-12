package id

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestTypeRegistryCodesAreUniqueAndNonEmpty(t *testing.T) {
	t.Parallel()

	seen := make(map[TypeCode]bool)
	for _, code := range TypeCodes() {
		if code == "" {
			t.Fatal("registry contains an empty type code")
		}
		if seen[code] {
			t.Fatalf("registry contains duplicate type code %q", code)
		}
		seen[code] = true
	}
	if len(seen) == 0 {
		t.Fatal("registry is empty")
	}
}

func TestCodecRoundTripsEachTypeCode(t *testing.T) {
	t.Parallel()

	values := []uint64{0, 1, 42, 1 << 20, math.MaxUint64}
	for _, code := range TypeCodes() {
		codec, err := NewCodec(code, "")
		if err != nil {
			t.Fatalf("NewCodec(%q) error = %v", code, err)
		}
		for _, want := range values {
			encoded, err := codec.Encode(want)
			if err != nil {
				t.Fatalf("Encode(%d) for %q error = %v", want, code, err)
			}
			if len(encoded) < minLength {
				t.Fatalf("Encode(%d) for %q = %q, shorter than %d", want, code, encoded, minLength)
			}
			got, err := codec.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode(%q) for %q error = %v", encoded, code, err)
			}
			if got != want {
				t.Fatalf("Decode(Encode(%d)) for %q = %d", want, code, got)
			}
		}
	}
}

func TestSameIntegerDiffersAcrossTypeCodes(t *testing.T) {
	t.Parallel()

	codes := TypeCodes()
	for n := uint64(0); n <= 50; n++ {
		seen := make(map[string]TypeCode)
		for _, code := range codes {
			codec, err := NewCodec(code, "")
			if err != nil {
				t.Fatalf("NewCodec(%q) error = %v", code, err)
			}
			encoded, err := codec.Encode(n)
			if err != nil {
				t.Fatalf("Encode(%d) for %q error = %v", n, code, err)
			}
			if other, dup := seen[encoded]; dup {
				t.Fatalf("Encode(%d) = %q for both %q and %q", n, encoded, other, code)
			}
			seen[encoded] = code
		}
	}
}

func TestDifferentSeedChangesEncodingForSameTypeAndInteger(t *testing.T) {
	t.Parallel()

	for _, code := range TypeCodes() {
		a, err := NewCodec(code, "alpha")
		if err != nil {
			t.Fatalf("NewCodec(%q, alpha) error = %v", code, err)
		}
		b, err := NewCodec(code, "beta")
		if err != nil {
			t.Fatalf("NewCodec(%q, beta) error = %v", code, err)
		}
		for n := uint64(0); n <= 50; n++ {
			ea, _ := a.Encode(n)
			eb, _ := b.Encode(n)
			if ea == eb {
				t.Fatalf("seeds alpha and beta both encode %d as %q for %q", n, ea, code)
			}
		}
	}
}

func TestUnsetSeedEncodingIsPinned(t *testing.T) {
	t.Parallel()

	codec, err := NewCodec(TypeItem, "")
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	got, err := codec.Encode(1)
	if err != nil {
		t.Fatalf("Encode(1) error = %v", err)
	}
	// Regression pin of the wire format: changing the alphabet derivation or
	// the Sqids options changes every issued ID and must be a deliberate change.
	const want = "knyy"
	if got != want {
		t.Fatalf("Encode(1) for item without seed = %q, want %q", got, want)
	}
}

func TestDecodeAcceptsValidAllDigitInput(t *testing.T) {
	t.Parallel()

	codec, err := NewCodec(TypeItem, "")
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	found := false
	for n := uint64(0); n < 100000 && !found; n++ {
		encoded, err := codec.Encode(n)
		if err != nil {
			t.Fatalf("Encode(%d) error = %v", n, err)
		}
		if strings.Trim(encoded, "23456789") != "" {
			continue
		}
		found = true
		got, err := codec.Decode(encoded)
		if err != nil || got != n {
			t.Fatalf("Decode(%q) = %d, %v; want %d", encoded, got, err, n)
		}
	}
	if !found {
		t.Fatal("no all-digit encoding found in range")
	}
}

func TestDecodeRejectsSubMinimumAndInvalidCharacters(t *testing.T) {
	t.Parallel()

	codec, err := NewCodec(TypeItem, "")
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	for _, input := range []string{"", "1", "22", "222", "0000", "iiii", "ABCD", "abc!"} {
		if _, err := codec.Decode(input); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Decode(%q) error = %v, want ErrInvalidID", input, err)
		}
	}
}

func TestDecodeRejectsNonCanonicalInput(t *testing.T) {
	t.Parallel()

	codec, err := NewCodec(TypeItem, "")
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	valid, err := codec.Encode(1)
	if err != nil {
		t.Fatalf("Encode(1) error = %v", err)
	}
	rejected := 0
	for i := 0; i < len(valid); i++ {
		for _, c := range baseAlphabet {
			mutated := valid[:i] + string(c) + valid[i+1:]
			if mutated == valid {
				continue
			}
			got, err := codec.Decode(mutated)
			if err != nil {
				rejected++
				continue
			}
			again, err := codec.Encode(got)
			if err != nil || again != mutated {
				t.Fatalf("Decode(%q) = %d accepted but re-encodes to %q", mutated, got, again)
			}
		}
	}
	if rejected == 0 {
		t.Fatal("no single-character mutation was rejected")
	}
	if _, err := codec.Decode(valid + "a"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Decode(%q) error = %v, want ErrInvalidID", valid+"a", err)
	}
}

func TestSwappedTypeCodeAndSeedDoNotCollide(t *testing.T) {
	t.Parallel()

	a, err := NewCodec(TypeItem, string(TypeWorkspace))
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	b, err := NewCodec(TypeWorkspace, string(TypeItem))
	if err != nil {
		t.Fatalf("NewCodec error = %v", err)
	}
	if a.alphabet == b.alphabet {
		t.Fatalf("type %q seeded %q shares an alphabet with type %q seeded %q",
			TypeItem, TypeWorkspace, TypeWorkspace, TypeItem)
	}
}

func TestSeededCodecRoundTripsAndRejectsNonCanonicalInput(t *testing.T) {
	t.Parallel()

	for _, code := range TypeCodes() {
		codec, err := NewCodec(code, "rotate-me")
		if err != nil {
			t.Fatalf("NewCodec(%q, seed) error = %v", code, err)
		}
		for _, want := range []uint64{0, 7, 1 << 40, math.MaxUint64} {
			encoded, err := codec.Encode(want)
			if err != nil {
				t.Fatalf("Encode(%d) for %q error = %v", want, code, err)
			}
			got, err := codec.Decode(encoded)
			if err != nil || got != want {
				t.Fatalf("Decode(%q) for %q = %d, %v; want %d", encoded, code, got, err, want)
			}
		}
		valid, err := codec.Encode(1)
		if err != nil {
			t.Fatalf("Encode(1) for %q error = %v", code, err)
		}
		for i := 0; i < len(valid); i++ {
			for _, c := range codec.alphabet {
				mutated := valid[:i] + string(c) + valid[i+1:]
				got, err := codec.Decode(mutated)
				if err != nil {
					continue
				}
				if again, _ := codec.Encode(got); again != mutated {
					t.Fatalf("seeded %q accepted %q as %d, re-encodes to %q", code, mutated, got, again)
				}
			}
		}
	}
}
