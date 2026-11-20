package web

import "net/http"

// A list page renders its whole results in the page, with or without JavaScript. An htmx request for the results
// fragment, part=results, gets only the region that the request replaces, for example after a sort or a search.
// The all parameter is still accepted so existing links keep working; it no longer changes the page.
const (
	listPartKey     = "part"
	listPartResults = "results"
	listFullKey     = "all"
)

// listResultsFragment reports whether the request asks only for the results fragment.
func listResultsFragment(request *http.Request) bool {
	return request.Header.Get("HX-Request") == "true" && request.URL.Query().Get(listPartKey) == listPartResults
}
