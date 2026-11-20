package viewmodel

import (
	"example.com/dynamis-code/apps-template/internal/clients"
)

// clientQueryForm holds the parsed Clients list query. The conditions come from the shared filter
// model; Filters is the same list in the form the service takes.
type ClientQueryForm struct {
	Sort     string
	Order    string
	Limit    int
	Archived string
	Search   string
	Match    string
	Filters  []clients.FilterInput
	Filter   ListFilterInput
}
