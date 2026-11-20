package components

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ClientColumnLabelText(data viewmodel.PageData, column viewmodel.ListColumn) string {
	if column.LabelKey != "" {
		return data.T(column.LabelKey)
	}
	return column.Label
}

func ClientColumnVisibilityValue(visible bool) string {
	return strconv.FormatBool(!visible)
}

// listColumnCounts returns the values for the "N of M shown" line of the column settings.
func ListColumnCounts(columns []viewmodel.ListColumn) map[string]any {
	shown := 0
	for _, column := range columns {
		if column.Visible {
			shown++
		}
	}
	return map[string]any{"shown": shown, "total": len(columns)}
}

// listBadgeColumn reports whether a column shows its value as a colored badge.
func ListBadgeColumn(key string) bool {
	return key == "role" || key == "status"
}

// listInitials returns up to two initials of a name, used in the identity circle beside a row's link.
func ListInitials(name string) string {
	fields := strings.Fields(name)
	initials := make([]rune, 0, 2)
	for _, field := range fields {
		if len(initials) == 2 {
			break
		}
		r, _ := utf8.DecodeRuneInString(field)
		initials = append(initials, unicode.ToUpper(r))
	}
	return string(initials)
}
