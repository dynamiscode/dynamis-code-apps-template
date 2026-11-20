package components

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// listLabel is the text for a field, choice, or operator: its translation when it has a key, otherwise
// its plain label.
func ListLabel(data viewmodel.PageData, key, text string) string {
	if key != "" {
		return data.T(key)
	}
	return text
}

// listChipValue is the value a chip shows: the choice's label, a range, or the text entered.
func ListChipValue(data viewmodel.PageData, chip viewmodel.ListFilterChip) string {
	if chip.ValueKey != "" || chip.ValueLabel != "" {
		return ListLabel(data, chip.ValueKey, chip.ValueLabel)
	}
	return chip.Value
}

// listInputType is the browser input type for a value, text unless the field says otherwise.
func ListInputType(kind string) string {
	if kind == "number" || kind == "date" {
		return kind
	}
	return "text"
}
