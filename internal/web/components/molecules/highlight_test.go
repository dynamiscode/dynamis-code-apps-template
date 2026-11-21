package molecules

import (
	"reflect"
	"testing"
)

// TestListHighlightPartsMarksEveryCaseInsensitiveMatch checks that each match is marked in its original
// case, that the text around matches is kept, and that an empty or unmatched term leaves the text whole.
func TestListHighlightPartsMarksEveryCaseInsensitiveMatch(t *testing.T) {
	got := listHighlightParts("Ada Lovelace, ada@example.test", "ADA")
	want := []highlightPart{
		{Text: "Ada", Match: true},
		{Text: " Lovelace, "},
		{Text: "ada", Match: true},
		{Text: "@example.test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parts = %+v, want %+v", got, want)
	}
	if got := listHighlightParts("Notes", ""); !reflect.DeepEqual(got, []highlightPart{{Text: "Notes"}}) {
		t.Fatalf("empty term parts = %+v", got)
	}
	if got := listHighlightParts("a.b", "a+"); !reflect.DeepEqual(got, []highlightPart{{Text: "a.b"}}) {
		t.Fatalf("regex characters in the term must be literal, parts = %+v", got)
	}
}
