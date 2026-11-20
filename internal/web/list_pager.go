package web

import (
	"net/url"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// listPagerWindow is how many pages either side of the current page the pager shows.
const listPagerWindow = 2

// listPageNumber reads the 1-based page from a list's query. A missing or invalid page is the first page.
func listPageNumber(query url.Values) int {
	value, err := strconv.Atoi(query.Get("page"))
	if err != nil || value < 1 {
		return 1
	}
	return value
}

// listStart is the position of a page's first row in the sorted list.
func listStart(page, limit int) int {
	return (page-1)*limit + 1
}

// newListPager builds a list's page navigation. Each link keeps the list's query and sets page, and drops the
// A list with no rows still has one page.
func newListPager(path string, query url.Values, page, limit, total int) viewmodel.ListPager {
	if limit < 1 {
		// Without a limit the whole list is one page.
		limit = max(total, 1)
	}
	pageCount := (total + limit - 1) / limit
	if pageCount < 1 {
		pageCount = 1
	}
	href := func(number int) string {
		values := url.Values{}
		for key, value := range query {
			if key != "page" {
				values[key] = value
			}
		}
		values.Set("page", strconv.Itoa(number))
		return path + "?" + values.Encode()
	}
	pager := viewmodel.ListPager{Page: page, PageCount: pageCount}
	if page > 1 {
		pager.Prev = href(page - 1)
	}
	if page < pageCount {
		pager.Next = href(page + 1)
	}
	previous := 0
	for number := 1; number <= pageCount; number++ {
		if number != 1 && number != pageCount && (number < page-listPagerWindow || number > page+listPagerWindow) {
			continue
		}
		if previous != 0 && number-previous > 1 {
			pager.Links = append(pager.Links, viewmodel.ListPageLink{})
		}
		pager.Links = append(pager.Links, viewmodel.ListPageLink{Number: number, Href: href(number), Current: number == page})
		previous = number
	}
	return pager
}
