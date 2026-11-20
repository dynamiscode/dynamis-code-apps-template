package components

import "github.com/a-h/templ"

// ListAction is one item in a row's actions menu. A link sets Href. A post sets PostPath and runs as a form with the
// workspace CSRF token; Confirm, when set, asks before the post is sent. Icon names the glyph shown with the label.
// Danger marks a destructive action, which the menu sets apart from the others.
type ListAction struct {
	Label    string
	Icon     string
	Href     string
	PostPath string
	Confirm  string
	Danger   bool
}

// ListActionConfirm is the confirmation attribute of a post action. Only an action that names a prompt gets one,
// so the page's confirm handler sees just the buttons that ask.
func ListActionConfirm(action ListAction) templ.Attributes {
	if action.Confirm == "" {
		return nil
	}
	return templ.Attributes{"data-confirm": action.Confirm}
}
