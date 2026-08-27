package i18n

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFiles embed.FS

type Locale string

const (
	English Locale = "en"
	Spanish Locale = "es"
)

var supported = []Locale{English, Spanish}

type Catalog struct {
	messages map[Locale]map[string]*template.Template
}

func New() (*Catalog, error) {
	return newCatalog(localeFiles)
}

func newCatalog(source fs.FS) (*Catalog, error) {
	catalog := &Catalog{messages: make(map[Locale]map[string]*template.Template)}
	for _, locale := range supported {
		path := "locales/" + string(locale) + ".json"
		contents, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, fmt.Errorf("read locale %q: %w", locale, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(contents, &messages); err != nil {
			return nil, fmt.Errorf("parse locale %q: %w", locale, err)
		}
		compiled := make(map[string]*template.Template, len(messages))
		for key, message := range messages {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(message) == "" {
				return nil, errors.New("locale keys and messages must not be empty")
			}
			parsed, err := template.New(key).Parse(message)
			if err != nil {
				return nil, fmt.Errorf("parse locale %q key %q: %w", locale, key, err)
			}
			compiled[key] = parsed
		}
		catalog.messages[locale] = compiled
	}
	return catalog, nil
}

func Supported() []Locale {
	return append([]Locale(nil), supported...)
}

func ParseLocale(raw string) (Locale, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(English):
		return English, true
	case string(Spanish):
		return Spanish, true
	default:
		return "", false
	}
}

func (c *Catalog) Translate(locale Locale, key string, values map[string]any) string {
	if c == nil {
		return key
	}
	message := c.messages[locale][key]
	if message == nil {
		message = c.messages[English][key]
	}
	if message == nil {
		return key
	}
	var output bytes.Buffer
	if err := message.Execute(&output, values); err != nil {
		return key
	}
	return output.String()
}

func (c *Catalog) Resolve(user, cookie, workspace, acceptLanguage string) Locale {
	for _, raw := range []string{user, cookie, workspace} {
		if locale, ok := ParseLocale(raw); ok {
			return locale
		}
	}
	matcher := language.NewMatcher([]language.Tag{language.English, language.Spanish})
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err == nil && len(tags) > 0 {
		_, index, _ := matcher.Match(tags...)
		if index >= 0 && index < len(supported) {
			return supported[index]
		}
	}
	return English
}

func FormatDate(locale Locale, value time.Time) string {
	value = value.UTC()
	if locale == Spanish {
		months := [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
		return fmt.Sprintf("%d de %s de %d", value.Day(), months[value.Month()-1], value.Year())
	}
	return value.Format("Jan 2, 2006")
}

func FormatDateTime(locale Locale, value time.Time) string {
	value = value.UTC()
	if locale == Spanish {
		return FormatDate(locale, value) + " " + value.Format("15:04")
	}
	return value.Format("Jan 2, 2006 3:04 PM")
}
