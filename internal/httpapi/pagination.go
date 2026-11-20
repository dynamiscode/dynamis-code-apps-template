package httpapi

import (
	"net/url"
	"strconv"
)

// maxPage bounds the page number so that the row offset stays a small integer.
const maxPage = 1000000

// parsePage reads the optional 1-based page parameter. A missing page is the first page.
func parsePage(query url.Values) (int, bool) {
	raw := query.Get("page")
	if raw == "" {
		return 1, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maxPage {
		return 0, false
	}
	return value, true
}
