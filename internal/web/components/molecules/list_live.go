package molecules

import (
	"net/url"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// ListLiveLinkAttributes makes a sort link swap the results region in place, as live search does. The link keeps its
// href, so without JavaScript it still loads the full page.
func ListLiveLinkAttributes(data viewmodel.PageData, href string) templ.Attributes {
	if !data.ListLive || href == "" {
		return nil
	}
	return templ.Attributes{
		"hx-get":      listResultsHref(href),
		"hx-target":   "#list-results",
		"hx-swap":     "outerHTML",
		"hx-push-url": "true",
		"hx-sync":     "this:replace",
	}
}

// ListLivePageSizeAttributes makes the page-size form swap the results region in place.
func ListLivePageSizeAttributes(data viewmodel.PageData) templ.Attributes {
	if !data.ListLive {
		return nil
	}
	return templ.Attributes{
		"hx-get":      ListResultsURL(data.CurrentPath, ""),
		"hx-target":   "#list-results",
		"hx-swap":     "outerHTML",
		"hx-push-url": "true",
		"hx-sync":     "this:replace",
	}
}

// listResultsHref adds the results fragment marker to a list address and keeps its query.
func listResultsHref(href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return href
	}
	query := parsed.Query()
	query.Set("part", "results")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// ListResultsURL is the address of a list's live change: the preference route with the list query and the results
// fragment marker. The results fragment is what htmx swaps into the list's results region.
func ListResultsURL(path, suffix string) string {
	return listResultsHref(path + suffix)
}

// ListOOB marks an element as an out-of-band swap when the results fragment refreshes it in place.
func ListOOB(oob bool) templ.Attributes {
	if !oob {
		return nil
	}
	return templ.Attributes{"hx-swap-oob": "true"}
}

// ListColumnOrderHref is the order route of a list, with the query kept.
func ListColumnOrderHref(path, suffix string) string {
	return ListResultsURL(path+"/preferences/columns/order", suffix)
}
