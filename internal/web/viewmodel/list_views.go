package viewmodel

// ListViewOption is one saved view as a list's view menu shows it.
type ListViewOption struct {
	ID       int64
	Name     string
	Shared   bool
	Default  bool
	Editable bool
	// Current is set on the view whose saved query matches the list on screen.
	Current bool
}
