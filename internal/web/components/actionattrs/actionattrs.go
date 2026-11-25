// Package actionattrs builds attribute sets for action controls (anchors and
// buttons) that can only ever contain an allow-listed set of keys.
//
// scripts/actioncheck trusts any value reaching an action control's Attrs
// spread through a call of the form `<expr>.Templ()` because, in this
// module, only actionattrs.Safe defines that method (actionattrs_test.go
// enforces that uniqueness). Do not add a Templ() method to any other type;
// doing so would let unreviewed attributes bypass the action-control check.
package actionattrs

import (
	"strings"

	"github.com/a-h/templ"
)

// allowed lists the only attribute keys a Safe value may carry. Anything
// that controls layout or styling ("class", "style") is excluded on
// purpose: those must stay with the component, not the caller.
var allowed = map[string]bool{
	"id":                   true,
	"name":                 true,
	"type":                 true,
	"value":                true,
	"disabled":             true,
	"aria-label":           true,
	"aria-current":         true,
	"aria-expanded":        true,
	"aria-haspopup":        true,
	"aria-controls":        true,
	"data-confirm":         true,
	"data-testid":          true,
	"data-requires-change": true,
	"hx-get":               true,
	"hx-post":              true,
	"hx-put":               true,
	"hx-delete":            true,
	"hx-target":            true,
	"hx-swap":              true,
	"hx-trigger":           true,
	"hx-include":           true,
	"hx-confirm":           true,
	"hx-indicator":         true,
	"hx-vals":              true,
	"hx-push-url":          true,
	"hx-boost":             true,
}

// Safe is an action-control attribute set. Build one with New and With;
// there is no way to put an arbitrary key into it.
type Safe struct {
	attrs templ.Attributes
}

// New returns an empty Safe set.
func New() Safe {
	return Safe{}
}

// With returns a copy of s with key set to value. It panics if key is not
// on the allow list, so an unreviewed key fails at construction, not later.
func (s Safe) With(key string, value any) Safe {
	if !allowed[key] {
		panic("actionattrs: key not allowed: " + key)
	}
	out := Safe{attrs: make(templ.Attributes, len(s.attrs)+1)}
	for k, v := range s.attrs {
		out.attrs[k] = v
	}
	out.attrs[key] = value
	return out
}

// Merge returns a copy of s with every key from other applied on top.
func (s Safe) Merge(other Safe) Safe {
	out := s
	for k, v := range other.attrs {
		out = out.With(k, v)
	}
	return out
}

// Templ converts s to templ.Attributes for spreading onto an element. This
// is the only way to get templ.Attributes out of a Safe value, and the
// only producer scripts/actioncheck recognizes as bounded; see the package
// doc comment.
func (s Safe) Templ() templ.Attributes {
	out := make(templ.Attributes, len(s.attrs))
	for k, v := range s.attrs {
		out[k] = v
	}
	return out
}

// From validates an arbitrary templ.Attributes map against the same
// allow-list as With and returns it unchanged. It panics on the first
// disallowed key, so a caller-assembled attribute map is checked once at
// this boundary instead of trusted blindly. scripts/actioncheck treats any
// `actionattrs.From(expr)` call as a bounded attribute source for the same
// reason it trusts Templ(): the panic happens before render.
func From(m templ.Attributes) templ.Attributes {
	for key := range m {
		if !allowed[key] {
			panic("actionattrs: key not allowed: " + key)
		}
	}
	return m
}

var forbiddenClassTokens = map[string]bool{
	"btn-ghost": true,
	"btn-link":  true,
	"border-0":  true,
}

func checkClassTokens(value string) {
	for _, tok := range strings.Fields(value) {
		base := tok[strings.LastIndex(tok, ":")+1:]
		if forbiddenClassTokens[base] {
			panic("actionattrs: forbidden class override: " + tok)
		}
	}
}

func hasBoundaryToken(value string) bool {
	for _, tok := range strings.Fields(value) {
		if tok == "btn" || tok == "border" {
			return true
		}
	}
	return false
}

// ButtonClass validates a caller-supplied class fragment destined for a
// component that itself guarantees the "btn" boundary class (atoms.Button).
// It panics if value carries a forbidden override token (btn-ghost,
// btn-link, border-0). scripts/actioncheck treats
// `actionattrs.ButtonClass(expr)` as a bounded class source, trusting this
// runtime guard in place of a compile-time literal.
func ButtonClass(value string) string {
	checkClassTokens(value)
	return value
}

// LinkClass validates a caller-supplied class fragment destined for a
// component that does not itself guarantee a boundary class (atoms.Anchor
// and similar). In addition to ButtonClass's override check, it requires
// value to carry the "btn" or "border" boundary token itself, and panics if
// either check fails. scripts/actioncheck treats `actionattrs.LinkClass(expr)`
// as a bounded class source.
func LinkClass(value string) string {
	checkClassTokens(value)
	if !hasBoundaryToken(value) {
		panic("actionattrs: class missing boundary token (btn or border): " + value)
	}
	return value
}
