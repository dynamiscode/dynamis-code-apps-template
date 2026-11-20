package components

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"strconv"
)

// listPageSizeLabel is the dropdown text for one size: "All" for 0, otherwise the number.
func ListPageSizeLabel(data viewmodel.PageData, size int) string {
	if size == 0 {
		return data.T("ui.all")
	}
	return strconv.Itoa(size)
}
