package molecules

import (
	"regexp"
	"strings"
	"sync"
)

// highlightPart is a run of cell text; Match marks the runs that equal the search.
type highlightPart struct {
	Text  string
	Match bool
}

// highlightCache keeps the pattern for the last search term. A list renders the same term for every cell, so
// one entry avoids compiling a pattern per cell. ponytail: one entry, so alternating terms recompile; add a
// small map if that shows up.
var highlightCache struct {
	sync.Mutex
	term string
	re   *regexp.Regexp
}

// highlightPattern returns the case-insensitive literal pattern for term.
func highlightPattern(term string) *regexp.Regexp {
	highlightCache.Lock()
	defer highlightCache.Unlock()
	if highlightCache.re == nil || highlightCache.term != term {
		highlightCache.term = term
		highlightCache.re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
	}
	return highlightCache.re
}

// listHighlightParts splits text around each case-insensitive match of term. An empty term keeps the text whole.
func listHighlightParts(text, term string) []highlightPart {
	term = strings.TrimSpace(term)
	if term == "" {
		return []highlightPart{{Text: text}}
	}
	matches := highlightPattern(term).FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []highlightPart{{Text: text}}
	}
	parts := make([]highlightPart, 0, 2*len(matches)+1)
	last := 0
	for _, match := range matches {
		if match[0] > last {
			parts = append(parts, highlightPart{Text: text[last:match[0]]})
		}
		parts = append(parts, highlightPart{Text: text[match[0]:match[1]], Match: true})
		last = match[1]
	}
	if last < len(text) {
		parts = append(parts, highlightPart{Text: text[last:]})
	}
	return parts
}
