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
	"fmt"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// allowed lists the only attribute keys a Safe value may carry. Anything
// that controls layout or styling ("class", "style") is excluded on
// purpose: those must stay with the component, not the caller. A key alone
// is not enough for href and onclick, whose VALUE also controls behavior;
// valueCheckers below additionally constrains those two.
// "class" and "style" are deliberately absent and always panic: they are
// exactly the two attributes that can hide or restyle an action control the
// same way an unbounded Class string can (style="display:none" is
// equivalent to removing the "btn"/"border" boundary class). This is not an
// oversight; no caller should route either through Attrs. A caller that
// needs a visual variant has Class for exactly that purpose, checked
// separately by scripts/actioncheck.
// allowed lists the individual, non-namespaced keys this guard permits.
// "aria-*", "data-*", and "hx-*" are not enumerated here one at a time;
// Allowed below admits the whole namespace for each. All three are
// HTML/htmx-spec guaranteed inert: an aria-* or data-* attribute carries no
// browser-interpreted behavior at all (that is the point of both
// namespaces -- accessibility semantics and custom data hooks,
// respectively), and an hx-* attribute only ever configures htmx's
// declarative request/swap behavior, never arbitrary script. Enumerating
// every key in these namespaces one at a time, as this list once did,
// means every new legitimate key is a new panic in production until
// someone notices and adds it; the namespaces themselves are the actual
// safety boundary, not their individual member names.
var allowed = map[string]bool{
	// href is allowed, value-checked below, because this codebase routes
	// some anchor targets through Attrs instead of a dedicated Href field
	// (DropdownItem, FooterNav's AnchorProps).
	"href":     true,
	"id":       true,
	"name":     true,
	"type":     true,
	"value":    true,
	"disabled": true,
	// title and download are plain, non-executable HTML attribute values
	// (a tooltip, a save-as filename hint); templ escapes attribute values,
	// so neither carries a script-injection or style-override risk the way
	// class/style/onclick/href do.
	"title":    true,
	"download": true,
	// target and rel are standard anchor-navigation attributes with no
	// browser-executed content (rel is, if anything, a security hardening
	// attribute: "noopener noreferrer" is what prevents target="_blank"
	// tabnapping).
	"target": true,
	"rel":    true,
	// onclick is allowed, value-checked below to the one fixed shape this
	// codebase uses (the time-slot picker's modal trigger).
	"onclick": true,
}

// showModalPattern is the onclick shapes this codebase uses: the time-slot
// picker's "<id>.showModal()" native-dialog open trigger and "<id>.close()"
// close trigger. Anything else is rejected; onclick's allow-list entry is
// not a general license to pass script text through.
var showModalPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.(?:showModal|close)\(\)$`)

// hrefSchemePattern matches a leading URL scheme ("javascript:", "http:",
// ...). Anchored at the start, since RFC 3986 only treats a leading
// "word:" as a scheme; a colon elsewhere in a relative reference is not one.
var hrefSchemePattern = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)

// validHref reports whether s is safe to spread onto an href attribute
// without templ.SafeURL's sanitization (Attrs bypasses that entirely).
// Allowed: empty, a fragment or query, a single-slash path-absolute
// reference, a scheme-less relative reference, or plain http(s). Rejected:
// any other scheme (javascript:, data:, vbscript:, ...), and anything that
// could be interpreted as protocol-relative by a browser -- "//evil.com"
// (same scheme, host evil.com) or a leading backslash in that position
// ("/\evil.com", "\\evil.com"), which several browsers normalize to the
// same thing. A backslash anywhere else in the value is also rejected,
// since it serves no legitimate purpose in a path this codebase builds.
// normalizeHref mirrors the WHATWG URL parser's own input-cleanup steps
// (https://url.spec.whatwg.org/#concept-basic-url-parser): remove every
// ASCII tab or newline anywhere in the string, then trim leading and
// trailing C0 controls (U+0000-U+001F) and space. Browsers apply this
// before looking at the scheme, so "jav\tascript:" and " //evil.com" (a
// leading space) are indistinguishable from "javascript:" and "//evil.com"
// at the point a browser actually navigates; validating the raw string
// without this step lets either hide a scheme or a protocol-relative
// prefix from the checks below.
func normalizeHref(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	isTrimmed := func(r rune) bool { return r <= ' ' }
	return strings.TrimFunc(cleaned, isTrimmed)
}

func validHref(raw string) bool {
	s := normalizeHref(raw)
	if s == "" {
		return true
	}
	// Any remaining C0 control character is rejected outright: it serves no
	// legitimate purpose in a path/URL this codebase builds, and is exactly
	// the kind of byte real-world scheme/host obfuscation tricks rely on.
	for _, r := range s {
		if r < 0x20 {
			return false
		}
	}
	if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "?") {
		return true
	}
	if strings.HasPrefix(s, "/") {
		rest := s[1:]
		if strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "\\") {
			return false
		}
		return !strings.Contains(rest, "\\")
	}
	if strings.HasPrefix(s, "\\") {
		return false
	}
	if scheme := hrefSchemePattern.FindString(s); scheme != "" {
		lower := strings.ToLower(scheme)
		return (lower == "http:" || lower == "https:") && !strings.Contains(s, "\\")
	}
	// No scheme, no leading slash: a plain relative reference ("settings",
	// "export/download"). It resolves against the current page and cannot
	// change host, so it is safe provided it carries no backslash trick.
	return !strings.Contains(s, "\\")
}

// ValidateValue rejects a value that does not match the one shape this
// codebase's real callers need for key, closing off the rest of that key's
// space (script injection via onclick, open-redirect/script schemes via
// href). Exported so scripts/actioncheck can apply the identical rule to a
// literal templ.Attributes{...} composite, instead of maintaining a
// second, divergent copy of this logic.
func ValidateValue(key string, value any) error {
	switch key {
	case "onclick":
		s, ok := value.(string)
		if !ok || !showModalPattern.MatchString(s) {
			return fmt.Errorf("actionattrs: onclick value not allowed: %v", value)
		}
	case "href":
		s, ok := value.(string)
		if !ok || !validHref(s) {
			return fmt.Errorf("actionattrs: href value not allowed: %v", value)
		}
	}
	return nil
}

// Allowed reports whether key is on the attribute allow-list. Exported so
// scripts/actioncheck can require every key of a literal
// templ.Attributes{...} composite to pass the same check a runtime
// actionattrs.From/With call would, instead of only restricting "class"
// there as it historically did.
func Allowed(key string) bool {
	if allowed[key] {
		return true
	}
	// "class" and "style" must never match here: both would need to start
	// with "class-"/"style-" to do so, which they do not, so the prefix
	// check below cannot accidentally let either through.
	return strings.HasPrefix(key, "aria-") || strings.HasPrefix(key, "data-") || strings.HasPrefix(key, "hx-")
}

func checkValue(key string, value any) {
	if err := ValidateValue(key, value); err != nil {
		panic(err.Error())
	}
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
	if !Allowed(key) {
		panic("actionattrs: key not allowed: " + key)
	}
	checkValue(key, value)
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
	for key, value := range m {
		if !Allowed(key) {
			panic("actionattrs: key not allowed: " + key)
		}
		checkValue(key, value)
	}
	return m
}

var forbiddenClassTokens = map[string]bool{
	"btn-ghost": true,
	"btn-link":  true,
	"border-0":  true,
}

func normalizeClassToken(tok string) string {
	base := tok[strings.LastIndex(tok, ":")+1:]
	// Tailwind's "important" modifier marks the utility with "!": a leading
	// "!" in v3 ("!border-0", "md:!border-0") or a trailing "!" in v4
	// ("border-0!"). Strip either before comparing so it cannot smuggle a
	// forbidden override past the bare-token check.
	base = strings.TrimPrefix(base, "!")
	base = strings.TrimSuffix(base, "!")
	return base
}

func checkClassTokens(value string) {
	for _, tok := range strings.Fields(value) {
		base := normalizeClassToken(tok)
		if forbiddenClassTokens[base] {
			panic("actionattrs: forbidden class override: " + tok)
		}
	}
}

func hasBoundaryToken(value string) bool {
	for _, tok := range strings.Fields(value) {
		if normalizeClassToken(tok) == "btn" || normalizeClassToken(tok) == "border" {
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
