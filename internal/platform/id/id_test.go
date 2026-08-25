package id

import (
	"encoding/hex"
	"testing"
)

func TestNewReturnsOpaque128BitID(t *testing.T) {
	t.Parallel()

	first, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if first == second {
		t.Fatal("New() returned duplicate IDs")
	}
	decoded, err := hex.DecodeString(first)
	if err != nil {
		t.Fatalf("ID is not hexadecimal: %v", err)
	}
	if len(decoded) != 16 {
		t.Fatalf("ID bytes = %d, want 16", len(decoded))
	}
}
