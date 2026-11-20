package identity

import (
	"slices"
	"strings"
)

// MemberSearchMatches reports whether a member's email or role contains the search text, ignoring case. Search is
// the members list's one free-text box.
func MemberSearchMatches(member MemberSummary, search string) bool {
	needle := strings.ToLower(search)
	return strings.Contains(strings.ToLower(member.Email), needle) || strings.Contains(strings.ToLower(string(member.Role)), needle)
}

// SortMembers orders members in place by sort: "email" (the default), "role", or "created_at". A leading "-" reverses
// the order. Ties break on the member ID in the same direction, so pages are stable.
func SortMembers(members []MemberSummary, sort string) []MemberSummary {
	descending := strings.HasPrefix(sort, "-")
	field := strings.TrimPrefix(sort, "-")
	slices.SortFunc(members, func(a, b MemberSummary) int {
		var order int
		switch field {
		case "role":
			order = strings.Compare(string(a.Role), string(b.Role))
		case "created_at":
			order = a.CreatedAt.Compare(b.CreatedAt)
		default:
			order = strings.Compare(strings.ToLower(a.Email), strings.ToLower(b.Email))
		}
		if order == 0 {
			order = strings.Compare(a.ID.String(), b.ID.String())
		}
		if descending {
			return -order
		}
		return order
	})
	return members
}
