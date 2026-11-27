package viewmodel

import (
	"example.com/dynamis-code/apps-template/internal/clients"
)

// clientFieldRow is a custom field with its choice options. Options are set only
// for single-choice fields.
type ClientFieldRow struct {
	clients.Field
	Options        []clients.Option
	OptionDraft    string
	OptionErrorKey string
}

// clientValueRow is a field's value with the choices a form offers. ValueLabel is
// the selected choice's label, for read-only pages.
type ClientValueRow struct {
	clients.FieldValue
	Choices    []ChoiceView
	ValueLabel string
}

// choiceView is one choice in a select.
type ChoiceView struct {
	ID       string
	Label    string
	Selected bool
}
