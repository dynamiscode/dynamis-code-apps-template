package components

import "example.com/dynamis-code/apps-template/internal/web/viewmodel"

// ListFooterCount is the footer's row summary. When the page knows its first row, it names the range shown:
// "Showing 26–50 of 412 rows". Otherwise it gives the page's row count and the total.
func ListFooterCount(data viewmodel.PageData) string {
	rows := len(data.ListRows)
	if rows == 0 && data.ListTotal > 0 {
		// A page past the last one: say so, and that the rows exist on other pages.
		return data.T("ui.rows_past_end", map[string]any{"total": data.ListTotal})
	}
	if data.ListStart > 0 && rows > 0 {
		return data.T("ui.rows_range", map[string]any{
			"start": data.ListStart, "end": data.ListStart + rows - 1, "total": data.ListTotal,
		})
	}
	return data.T("ui.rows_shown", map[string]any{"count": rows, "total": data.ListTotal})
}
