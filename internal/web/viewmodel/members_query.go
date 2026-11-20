package viewmodel

// memberListQuery is a parsed members list query. Conditions come from the shared filter model.
type MemberListQuery struct {
	Filter ListFilterInput
	Search string
	Sort   string
	Limit  int
	Page   int
}
