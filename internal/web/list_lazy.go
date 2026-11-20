package web

import (
	"net/http"
	"net/url"
)

// A list page loads its results in two steps. A normal navigation renders the controls and a skeleton, and
// forms.js then fetches the results fragment, which carries part=results. The all=1 parameter renders the whole
// list in the page, for clients without JavaScript.
const (
	listPartKey     = "part"
	listPartResults = "results"
	listFullKey     = "all"
)

// listLazyRequest reports whether a list page should send the skeleton instead of the results. Htmx requests,
// such as realtime refreshes, and the full-list fallback get the results directly.
func listLazyRequest(request *http.Request) bool {
	return request.Header.Get("HX-Request") != "true" && request.URL.Query().Get(listFullKey) != "1"
}

// listResultsFragment reports whether the request asks only for the results fragment.
func listResultsFragment(request *http.Request) bool {
	return request.Header.Get("HX-Request") == "true" && request.URL.Query().Get(listPartKey) == listPartResults
}

// listResultsURLs returns the address of a list's results fragment and the address of its full list, both for
// the page's current query.
func listResultsURLs(path string, query url.Values) (fragment, full string) {
	fragmentQuery := cloneClientQuery(query)
	fragmentQuery.Set(listPartKey, listPartResults)
	fragmentQuery.Del(listFullKey)
	fullQuery := cloneClientQuery(query)
	fullQuery.Del(listPartKey)
	fullQuery.Set(listFullKey, "1")
	return path + "?" + fragmentQuery.Encode(), path + "?" + fullQuery.Encode()
}
