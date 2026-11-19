package web

func currentPage(active bool) string {
	if active {
		return "page"
	}
	return "false"
}
