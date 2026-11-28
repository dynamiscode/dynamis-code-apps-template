package actioncheck

import (
	"crypto/sha256"
	"fmt"

	templparser "github.com/a-h/templ/parser/v2"
)

// Bind exceptional template scripts to each exact opening tag and body.
var reviewedTemplateScripts = map[string]map[string]string{
	"internal/web/components/atoms/countdown.templ": {
		"45b9d45dfb1d535edf99240432a04c210e07e4c38d839bcf098e865e5fa366e5": "Updates existing countdown values",
	},
	"internal/web/components/atoms/date_picker.templ": {
		"b9e1b12df9ecbf8dfef549b3c26867412392eee9c5477e8f2407dd26f9d644f6": "Dispatches the dropdown close event",
		"6a4f29c7121afb4af74e9fe222379ce8b9cc51044d034a277c97694b82ec8f24": "Highlights the selected day on existing bounded day buttons",
	},
	"internal/web/components/atoms/resizable.templ": {
		"15c99785ecfaa568bc2964d9aa4a17a23d6b563fc8a496ef5c3c30a25e1ca7d3": "Adjusts flex size of an existing panel",
	},
	"internal/web/components/atoms/time_slot_picker.templ": {
		"96f6eedf3aadf05d8389230c28f5d0337350b905f7b0aa85da3d348f463efc0c": "Closes the native dialog after a successful reservation",
	},
	"internal/web/components/templates/head.templ": {
		"8374d4789b13adab479e10c48e15a1c2b7f79206665d4f9e40ecfb6d8273efe6": "Loads the reviewed notes-editor.js asset; its contents are pinned in scripts/reviewed-browser-scripts.json",
		"d0891fedd35de3513f48836c56a24ba25eca7423aa6b264d8a180b621d671b2a": "Empty script body carries localized data attributes",
	},
}

// These fixed Alpine bindings only call the reviewed first-party script.
var reviewedExecutableAttributes = map[string]map[string]int{
	"internal/web/auth.templ":                                  {`x-data="pageBehaviors"`: 1, `x-data="passkeyLogin"`: 1},
	"internal/web/catalog.templ":                               {`x-data="listScreen"`: 1},
	"internal/web/clients_list.templ":                          {`x-data="listScreen"`: 1},
	"internal/web/engagement_list.templ":                       {`x-data="listScreen"`: 1},
	"internal/web/public_intake.templ":                         {`x-data="pageBehaviors"`: 1},
	"internal/web/transactions.templ":                          {`x-data="listScreen"`: 1},
	"internal/web/engagement_new.templ":                        {`x-data="engagementClientPicker"`: 1, `x-data="engagementNotesEditor"`: 1},
	"internal/web/security.templ":                              {`x-data="passkeyEnroll"`: 1},
	"internal/web/notifications.templ":                         {`x-data="notificationsLive"`: 1},
	"internal/web/components/templates/workspace.templ":        {`x-data="pageBehaviors"`: 1, `@click="toggleDrawer"`: 1},
	"internal/web/components/organisms/dashboard_header.templ": {`@click="toggleDrawer"`: 2},
}

// Opaque component slots require path-, expression- and parent-scoped review.
var reviewedScannedSlots = map[string]map[string]int{
	"internal/web/components/atoms/anchor.templ": {
		"a @ actionattrs.Scanned(props.LeftIcon)":  1,
		"a @ actionattrs.Scanned(props.RightIcon)": 1,
	},
	"internal/web/components/atoms/banner.templ": {
		"section/div/div @ actionattrs.Scanned(props.Title)": 1,
	},
	"internal/web/components/atoms/checkbox.templ": {
		"span @ actionattrs.Scanned(props.Content)": 1,
	},
	"internal/web/components/atoms/dock.templ": {
		"div/button @ actionattrs.Scanned(item.Icon)": 1,
	},
	"internal/web/components/atoms/drawer.templ": {
		"div/div/label @ actionattrs.Scanned(props.Toggle)": 1,
		"div/div/ul @ actionattrs.Scanned(props.Sidebar)":   1,
	},
	"internal/web/components/atoms/dropdown.templ": {
		"details/summary @ actionattrs.Scanned(props.Summary)": 1,
		"details/div @ actionattrs.Scanned(props.Panel)":       1,
		"details/ul/li @ actionattrs.Scanned(di.Item)":         1,
	},
	"internal/web/components/atoms/fab.templ": {
		"div/div @ actionattrs.Scanned(props.Toggle.(templ.Component))": 1,
		"div/div @ actionattrs.Scanned(props.Close)":                    1,
		"div/div @ actionattrs.Scanned(props.MainAction)":               1,
	},
	"internal/web/components/atoms/features.templ": {
		"div @ actionattrs.Scanned(feature.Icon)": 1,
	},
	"internal/web/components/atoms/footer.templ": {
		"footer/aside/div @ actionattrs.Scanned(props.Icon)":       1,
		"footer/nav/div/a @ actionattrs.Scanned(anchor.LeftIcon)":  1,
		"footer/nav/div/a @ actionattrs.Scanned(anchor.RightIcon)": 1,
	},
	"internal/web/components/atoms/indicator.templ": {
		"div/span @ actionattrs.Scanned(props.Item)": 1,
	},
	"internal/web/components/atoms/infinite_scroll.templ": {
		" @ actionattrs.Scanned(r)": 1,
	},
	"internal/web/components/atoms/input.templ": {
		"label @ actionattrs.Scanned(props.Icon)": 1,
	},
	"internal/web/components/atoms/megamenu.templ": {
		"div/div/ul @ actionattrs.Scanned(popover.Component)": 1,
	},
	"internal/web/components/atoms/menu.templ": {
		"li/a @ actionattrs.Scanned(props.Icon)":               2,
		"li/details/summary @ actionattrs.Scanned(props.Icon)": 2,
	},
	"internal/web/components/atoms/modal.templ": {
		" @ actionattrs.Scanned(c)": 1,
	},
	"internal/web/components/atoms/navbar.templ": {
		"div/div @ actionattrs.Scanned(props.Start)":  1,
		"div/div @ actionattrs.Scanned(props.Center)": 1,
		"div/div @ actionattrs.Scanned(props.End)":    1,
	},
	"internal/web/components/atoms/popover.templ": {
		"details/summary @ actionattrs.Scanned(props.Trigger)": 1,
	},
	"internal/web/components/atoms/resizable.templ": {
		"div/div @ actionattrs.Scanned(props.First)":  1,
		"div/div @ actionattrs.Scanned(props.Second)": 1,
	},
	"internal/web/components/atoms/swap.templ": {
		"label/div @ actionattrs.Scanned(props.On)":  1,
		"label/div @ actionattrs.Scanned(props.Off)": 1,
	},
	"internal/web/components/atoms/table.templ": {
		"div/table/thead/tr/th @ actionattrs.Scanned(header)": 1,
		"div/table/tbody @ actionattrs.Scanned(trow)":         1,
	},
	"internal/web/components/atoms/tabs.templ": {
		"div/div @ actionattrs.Scanned(tab.Content)": 1,
	},
	"internal/web/components/atoms/testimonial.templ": {
		"section/div/div/div/blockquote/div @ actionattrs.Scanned(props[i].Avatar)": 1,
	},
	"internal/web/components/atoms/timeline.templ": {
		"ul/li/div @ actionattrs.Scanned(prop.Middle)": 1,
	},
	"internal/web/components/molecules/empty_state.templ": {
		" @ actionattrs.Scanned(props.CTA)": 1,
	},
	"internal/web/components/organisms/list_screen.templ": {
		"div @ actionattrs.Scanned(tools)":  1,
		" @ actionattrs.Scanned(results)":   1,
		"div @ actionattrs.Scanned(body)":   1,
		"div @ actionattrs.Scanned(extras)": 1,
	},
}

func checkTemplateScript(source, path string, element *templparser.ScriptElement, seen map[string]bool) string {
	from, to := element.Range.From.Index, element.Range.To.Index
	if from < 0 || to <= from || to > int64(len(source)) {
		return "unverified browser script range"
	}
	raw := source[from:to]
	switch raw {
	case `<script defer src="/assets/alpine-components.js"></script>`,
		`<script defer src="/assets/js/alpine.min.js"></script>`,
		`<script defer src="/assets/htmx.min.js"></script>`:
		return ""
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	if reviewedTemplateScripts[path][digest] != "" {
		if seen[digest] {
			return "duplicate reviewed inline browser script"
		}
		seen[digest] = true
		return ""
	}
	return "unreviewed inline or external browser script (SHA-256 " + digest + ")"
}
