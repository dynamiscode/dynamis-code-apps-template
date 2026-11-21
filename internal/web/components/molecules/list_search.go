package molecules

import (
	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// ListSearchLive returns the htmx attributes that let a list's search form update its results region as the
// user types. The form still submits as a normal GET without JavaScript. Pages without a results region get
// no attributes, so the search never targets a missing element.
func ListSearchLive(data viewmodel.PageData) templ.Attributes {
	if !data.ListLive {
		return nil
	}
	return templ.Attributes{
		"hx-get":      data.CurrentPath + "?part=results",
		"hx-trigger":  "submit, input changed delay:300ms from:#list-search",
		"hx-target":   "#list-results",
		"hx-swap":     "outerHTML",
		"hx-sync":     "this:replace",
		"hx-push-url": "true",
	}
}
