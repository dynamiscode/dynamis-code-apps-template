package viewmodel

// ListPageLink is one numbered page link. A zero Number marks a gap between shown pages.
type ListPageLink struct {
	Number  int
	Href    string
	Current bool
}

// ListPager is a list's page navigation: previous, numbered pages, and next. An empty Prev or Next has no link.
type ListPager struct {
	Page      int
	PageCount int
	Prev      string
	Next      string
	Links     []ListPageLink
}
