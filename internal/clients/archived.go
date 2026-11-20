package clients

// ValidArchivedSelector reports whether an archived selector names a list the service can read. An empty
// selector reads the active clients. The web layer uses it to refuse a bad selector before a lazy list
// loads its rows.
func ValidArchivedSelector(value string) bool {
	switch archivedScope(value) {
	case ArchivedActive, ArchivedOnly, ArchivedAll:
		return true
	}
	return false
}
