package viewmodel

// itemListQuery is a parsed items list query. Its conditions come from the shared filter model.
type ItemListQuery struct {
	Search string
	Status string
	Sort   string
	Limit  int
	Match  string
	Filter ListFilterInput
}
