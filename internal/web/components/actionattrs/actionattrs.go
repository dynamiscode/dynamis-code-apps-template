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
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/a-h/templ"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// allowed lists the only attribute keys a Safe value may carry. Class and
// style attributes are excluded because callers cannot override component
// styling through Attrs. A key alone is not enough for href and onclick,
// whose values are additionally constrained by valueCheckers below.
// "aria-*", "data-*", and "hx-*" are admitted by Allowed below as
// namespaces. All three are inert: aria-* and data-* carry accessibility
// semantics and custom data, while hx-* configures declarative htmx behavior,
// never arbitrary script. The namespaces themselves are the safety boundary.
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
	if !isAttributeScalar(value) {
		return fmt.Errorf("actionattrs: %s attribute value has unsupported type %T", key, value)
	}
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

func isAttributeScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64, complex64, complex128, templ.KeyValue[string, bool], templ.KeyValue[bool, bool],
		templ.SafeURL, templ.SafeCSS, *string, *bool, *int, *int8, *int16, *int32, *int64, *uint, *uint8, *uint16,
		*uint32, *uint64, *uintptr, *float32, *float64, *complex64, *complex128:
		return true
	}
	return false
}

// ScalarAttributes preserves native attributes on non-action elements while
// rejecting values that can render HTML, scripts, or callbacks.
func ScalarAttributes(attrs templ.Attributes) templ.Attributes {
	for key, value := range attrs {
		if ExecutableAttribute(key) || !isAttributeScalar(value) {
			panic(fmt.Sprintf("actionattrs: unverified %s attribute value of type %T", key, value))
		}
	}
	return attrs
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
	if ExecutableAttribute(key) {
		return false
	}
	// "class" and "style" must never match here: both would need to start
	// with "class-"/"style-" to do so, which they do not, so the prefix
	// check below cannot accidentally let either through.
	return strings.HasPrefix(key, "aria-") || strings.HasPrefix(key, "data-") || strings.HasPrefix(key, "hx-")
}

// ExecutableAttribute identifies native and framework code-bearing attributes.
func ExecutableAttribute(key string) bool {
	key = strings.ToLower(key)
	return key == "srcdoc" || strings.HasPrefix(key, "x-") || strings.HasPrefix(key, "@") ||
		strings.HasPrefix(key, ":") || strings.HasPrefix(key, "on") ||
		strings.HasPrefix(key, "hx-on") || strings.HasPrefix(key, "data-hx-on")
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

// SafeClass accepts any visual class token.
func SafeClass(value string) bool { return true }

func normalizeClassToken(tok string) string {
	base := tok[strings.LastIndex(tok, ":")+1:]
	return strings.TrimSuffix(strings.TrimPrefix(base, "!"), "!")
}

func htmlClassTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return strings.ContainsRune("\t\n\f\r ", r) })
}

// HasBoundary reports whether value contains an unconditional btn/border token.
// Retained for compatibility; action-control validation does not require it.
func HasBoundary(value string) bool {
	for _, tok := range htmlClassTokens(value) {
		if strings.ContainsRune(tok, ':') {
			continue
		}
		if base := normalizeClassToken(tok); base == "btn" || base == "border" {
			return true
		}
	}
	return false
}

// ButtonClass marks a caller-supplied class fragment as a bounded class source.
func ButtonClass(value string) string { return value }

// LinkClass marks a caller-supplied class fragment as a bounded class source.
func LinkClass(value string) string { return value }

// Scanned wraps a component slot so its fully rendered HTML is validated
// before emission. It rejects executable attributes, inline style on actions,
// and malformed or context-sensitive raw HTML. Visual class tokens are
// unrestricted.
//
// A slot's declared type is ordinarily templ.Component, which anyone --
// including a hand-written templ.ComponentFunc, a templ.Raw call embedding
// an arbitrary HTML string, or a function in a package this module does
// not even have source for -- can satisfy with absolutely anything.
// scripts/actioncheck's AST analysis cannot see across a package or module
// boundary like that at all. Scanned can, because it runs the real render
// and inspects what actually comes out: the one thing that is always true
// regardless of where or how the component was built.
//
// scripts/actioncheck requires every bare component-slot reference (a
// render with no call syntax at all) to be wrapped in a call to Scanned;
// see safeComponent's doc comment.
//
// Scanned fully buffers the slot's rendered output before validating or
// writing any of it. An earlier, streaming version of this function
// (reading one HTML token at a time through a goroutine and an io.Pipe)
// was reviewed and found to have several real, separately-discovered
// concurrency bugs: a panic inside the render goroutine crashed the whole
// process instead of propagating to the caller; a cancelled context could
// not actually interrupt a blocked tokenizer.Next() call (the renderer
// goroutine has no reason to stop, since ordinary templ-generated code
// does not check ctx between every write, so cancellation silently did
// nothing and the render could still "succeed"); a token cut off mid-way
// by an error was partly consumed by the tokenizer but never written out
// (neither validated nor forwarded, nor accounted for); and a short write
// (an io.Writer returning fewer bytes than requested with a nil error)
// went undetected. Each fix for one of these introduced a new way to get
// another one wrong. None of that complexity is load-bearing: a plain,
// synchronous, non-concurrent implementation has none of these failure
// modes by construction. It has no goroutine, so a panic propagates to
// the caller normally. It checks ctx at its two real decision points
// (before rendering, and before writing) instead of pretending it can
// interrupt code it does not control mid-flight. It only tokenizes a
// complete, fully-rendered buffer, so there is no such thing as a token
// cut off mid-stream. And it checks the byte count from its one Write
// call for a short write.
//
// Scanned holds the entire slot output in memory. Nested wrappers hold
// multiple copies. The legacy list_screen.templ slots remain buffered but
// the client, member, and file journeys no longer use them. Replacing this
// with streaming validation must preserve HTML tokenizer state across writes
// and reject incomplete fragments before they can join adjacent markup.
func Scanned(c templ.Component) templ.Component {
	if c == nil {
		return c
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := c.Render(ctx, &buf); err != nil {
			// Deliberate: nothing is written, not even whatever prefix
			// rendered before the error. The slot never finished, so this
			// check never saw the complete output and cannot rule out that
			// the unseen rest would have contained something unsafe; writing
			// a partial, unvalidated prefix would defeat the point of this
			// guard.
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := scanRenderedHTML(buf.Bytes()); err != nil {
			panic("actionattrs: " + err.Error())
		}
		n, err := w.Write(buf.Bytes())
		if err != nil {
			return err
		}
		if n < buf.Len() {
			return io.ErrShortWrite
		}
		return nil
	})
}

// scanRenderedHTML parses the fully-rendered buffer and checks each action
// element for unsafe attributes. Reaching tokenizer end is always a clean,
// complete parse, never a stream being cut off partway through a token.
func scanRenderedHTML(rendered []byte) error {
	tokenizer := html.NewTokenizer(bytes.NewReader(rendered))
	contextSensitive := false
	var tokenViolation error
	for {
		tt := tokenizer.Next()
		if tt == html.CommentToken {
			raw := tokenizer.Raw()
			if bytes.HasPrefix(raw, []byte("<!--")) && !bytes.HasSuffix(raw, []byte("-->")) &&
				!bytes.HasSuffix(raw, []byte("--!>")) && !bytes.Equal(raw, []byte("<!-->")) && !bytes.Equal(raw, []byte("<!--->")) {
				return fmt.Errorf("rendered output contains an unterminated HTML comment")
			}
		}
		if tt == html.ErrorToken {
			// html.Tokenizer reports a dangling fragment it could not form
			// into a complete token (an unterminated "<button", a tag cut off
			// mid-attribute, ...) the same way it reports a clean end: an
			// ErrorToken with Err() == io.EOF. The one difference is Raw():
			// for every well-formed input, the final ErrorToken's Raw() is
			// empty; a dangling fragment leaves its unconsumed bytes there
			// instead. That distinction matters here specifically because a
			// slot's output does not stay isolated once it reaches the
			// browser: HTML is parsed as one continuous stream, so "<button"
			// from one slot followed immediately by
			// " onclick=evil()>x</button>" from unrelated, adjacent markup
			// forms one real, live <button> neither half looked like alone.
			// A validated slot's own output must never end on a dangling
			// fragment for that reason, not only a genuine parse error.
			if tokenizer.Err() != io.EOF || len(tokenizer.Raw()) > 0 {
				return fmt.Errorf("rendered output did not end as well-formed, complete HTML (dangling fragment %q)", tokenizer.Raw())
			}
			if !contextSensitive {
				return tokenViolation
			}
			return checkContextSensitiveHTML(rendered)
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, hasAttrs := tokenizer.TagName()
			tag := string(name)
			contextSensitive = contextSensitive || tag == "svg" || tag == "math" || tag == "noscript"
			if err := checkTagToken(tokenizer, tag, hasAttrs); err != nil && tokenViolation == nil {
				tokenViolation = err
			}
		}
	}
}

// checkTagToken checks an element for executable attributes and action
// elements for inline style.
func checkTagToken(tokenizer *html.Tokenizer, tag string, hasAttr bool) error {
	isAction := tag == "a" || tag == "button"
	for hasAttr {
		key, _, next := tokenizer.TagAttr()
		hasAttr = next
		if ExecutableAttribute(string(key)) {
			return fmt.Errorf("rendered a <%s> with executable attribute %q", tag, key)
		}
		if isAction && string(key) == "style" {
			return fmt.Errorf("rendered a <%s> with unverified inline style", tag)
		}
	}
	return nil
}

func checkContextSensitiveHTML(rendered []byte) error {
	for _, scripts := range []bool{false, true} {
		nodes, err := html.ParseFragmentWithOptions(bytes.NewReader(rendered), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}, html.ParseOptionEnableScripting(scripts))
		if err != nil {
			return err
		}
		for _, node := range nodes {
			for element := range node.Descendants() {
				if err := checkContextElement(element); err != nil {
					return err
				}
			}
			if err := checkContextElement(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkContextElement(node *html.Node) error {
	if node.Type != html.ElementNode {
		return nil
	}
	isAction := node.Data == "a" || node.Data == "button"
	for _, attribute := range node.Attr {
		if ExecutableAttribute(attribute.Key) {
			return fmt.Errorf("rendered a <%s> with executable attribute %q", node.Data, attribute.Key)
		}
		if isAction && attribute.Key == "style" {
			return fmt.Errorf("rendered a <%s> with unverified inline style", node.Data)
		}
	}
	if isAction {
		for parent := node.Parent; parent != nil; parent = parent.Parent {
			if parent.Type == html.ElementNode && (parent.Data == "title" || parent.Data == "textarea") {
				return fmt.Errorf("rendered a <%s> in context-sensitive <%s>", node.Data, parent.Data)
			}
		}
	}
	return nil
}
