package web

import (
	"slices"
)

// listPageSizes are the rows-per-page choices every list offers. A size of 0 means all rows.
var listPageSizes = []int{10, 30, 50, 100}

// listPageSizeOptions returns the dropdown choices for a list. The current size is always included,
// so a size chosen in an old link still shows as selected.
func listPageSizeOptions(current int, base []int) []int {
	sizes := slices.Clone(base)
	if !slices.Contains(sizes, current) {
		sizes = append(sizes, current)
		slices.Sort(sizes)
	}
	return sizes
}

// memberPageSizes adds all rows: members are not paged unless a size is chosen.
var memberPageSizes = []int{0, 10, 30, 50, 100}
