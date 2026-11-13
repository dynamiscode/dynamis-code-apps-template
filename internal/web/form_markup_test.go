package web

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestFormMarkupRules checks every template in templates/ against the form
// markup rules in the form-markup skill. Rules that the current templates break
// are pinned as known findings: the test fails when a new break appears and
// when a known finding is fixed, so the list only shrinks on purpose.
func TestFormMarkupRules(t *testing.T) {
	var (
		control     = regexp.MustCompile(`<(input|select|textarea)\b[^>]*>`)
		skipControl = regexp.MustCompile(`type="(hidden|submit|button)"`)
		idAttr      = regexp.MustCompile(`\bid="([^"]+)"`)
		labelFor    = regexp.MustCompile(`<label[^>]*\bfor="([^"]+)"`)
		alert       = regexp.MustCompile(`<[a-z]+\b[^>]*role="alert"[^>]*>`)
		button      = regexp.MustCompile(`<button\b[^>]*>`)
		destructive = regexp.MustCompile(`value="(delete|remove|revoke|transfer)"`)
		textField   = regexp.MustCompile(`<(input|textarea)\b[^>]*>`)
		nonText     = regexp.MustCompile(`type="(hidden|submit|button|checkbox|radio|file)"`)
		required    = regexp.MustCompile(`\srequired[\s>]`)
		disclosed   = regexp.MustCompile(`(?s)<fieldset data-show-when="[^"]+">.*?</fieldset>`)
	)
	knownNoConfirm := map[string]bool{}
	knownUnmarkedOptional := map[string]bool{}

	names, err := fs.Glob(files, "templates/*.html")
	if err != nil || len(names) == 0 {
		t.Fatalf("templates = %v, %v", names, err)
	}
	var noConfirm, unmarked []string
	for _, name := range names {
		raw, err := fs.ReadFile(files, name)
		if err != nil {
			t.Fatal(err)
		}
		page, base := string(raw), strings.TrimPrefix(name, "templates/")
		labels := map[string]string{}
		for _, m := range regexp.MustCompile(`<label[^>]*\bfor="([^"]+)"[^>]*>(.*?)</label>`).FindAllStringSubmatch(page, -1) {
			labels[m[1]] = m[2]
		}
		for _, m := range labelFor.FindAllStringSubmatch(page, -1) {
			if _, ok := labels[m[1]]; !ok {
				labels[m[1]] = ""
			}
		}

		// Rule 1, persistent labels: every visible control has a label that stays
		// on screen (label for=id, a wrapping label, or an aria-label).
		for _, loc := range control.FindAllStringIndex(page, -1) {
			tag := page[loc[0]:loc[1]]
			if skipControl.MatchString(tag) || strings.Contains(tag, "aria-label=") {
				continue
			}
			if id := idAttr.FindStringSubmatch(tag); id != nil {
				if _, ok := labels[id[1]]; ok {
					continue
				}
			}
			open, close := strings.LastIndex(page[:loc[0]], "<label"), strings.LastIndex(page[:loc[0]], "</label>")
			if open > close {
				continue
			}
			t.Errorf("%s: control without a persistent label: %s", base, tag)
		}

		// Rule 2, error auto-focus: an alert that htmx swaps in must take focus,
		// so app.js can move focus to it after htmx:after:swap.
		if strings.Contains(page, "hx-target") {
			for _, tag := range alert.FindAllString(page, -1) {
				if !strings.Contains(tag, `tabindex="-1"`) {
					t.Errorf("%s: htmx-swapped alert cannot take focus: %s", base, tag)
				}
			}
		}

		// Rule 3, confirmation: a destructive button asks before it acts.
		for _, tag := range button.FindAllString(page, -1) {
			if m := destructive.FindStringSubmatch(tag); m != nil && !strings.Contains(tag, "data-confirm") {
				noConfirm = append(noConfirm, base+" "+m[1])
			}
		}

		// Rule 4, explicit optional marking: a text field that is not required
		// says so in its label. A select always has a value, so it is skipped.
		// Fields in a progressive-disclosure section are required only for the
		// option that shows the section, so they are left out.
		for _, tag := range textField.FindAllString(disclosed.ReplaceAllString(page, ""), -1) {
			if nonText.MatchString(tag) || required.MatchString(tag) || strings.Contains(tag, "readonly") {
				continue
			}
			id := idAttr.FindStringSubmatch(tag)
			if id == nil || strings.Contains(labels[id[1]], "optional") {
				continue
			}
			unmarked = append(unmarked, base+" "+id[1])
		}
	}
	compare(t, "destructive button without data-confirm", noConfirm, knownNoConfirm)
	compare(t, "optional field not marked optional", unmarked, knownUnmarkedOptional)
}

func compare(t *testing.T, rule string, found []string, known map[string]bool) {
	t.Helper()
	seen := map[string]bool{}
	for _, key := range found {
		seen[key] = true
		if !known[key] {
			t.Errorf("new break of %q: %s", rule, key)
		}
	}
	var fixed []string
	for key := range known {
		if !seen[key] {
			fixed = append(fixed, key)
		}
	}
	sort.Strings(fixed)
	for _, key := range fixed {
		t.Errorf("known finding fixed, remove it from the list for %q: %s", rule, key)
	}
}
