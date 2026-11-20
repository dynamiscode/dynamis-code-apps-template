package components

import (
	"strings"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// ListColumnOrderValue is the list's columns in table order, comma-separated. The columns menu sends it when the
// user reorders the columns; the first column, the name, stays first.
func ListColumnOrderValue(data viewmodel.PageData) string {
	keys := make([]string, 0, len(data.ListColumns))
	for _, column := range data.ListColumns {
		keys = append(keys, column.Key)
	}
	return strings.Join(keys, ",")
}

// ListColumnOrderAfterMove is the column order with the column at index swapped one place by delta (-1 up, 1 down).
// The first column is locked, so a move that would reach index 0 or leave the list returns the order unchanged.
func ListColumnOrderAfterMove(data viewmodel.PageData, index, delta int) string {
	keys := strings.Split(ListColumnOrderValue(data), ",")
	target := index + delta
	if index < 1 || target < 1 || target >= len(keys) {
		return strings.Join(keys, ",")
	}
	keys[index], keys[target] = keys[target], keys[index]
	return strings.Join(keys, ",")
}
