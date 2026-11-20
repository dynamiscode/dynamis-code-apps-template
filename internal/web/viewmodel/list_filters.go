package viewmodel

const (
	ListFilterMaxConditions = 20
	ListMatchAll            = "all"
	ListMatchAny            = "any"
	ListOpIsEmpty           = "is_empty"
	ListOpBetween           = "between"
)

// listFilter is one condition: a field, an operator, and a value.
type ListFilter struct {
	Field string
	Op    string
	Value string
}

// listFilterInput is a parsed list query: the conditions, the match mode, and an add step that is
// open but not yet applied.
type ListFilterInput struct {
	Filters    []ListFilter
	Match      string
	AddField   string
	AddOp      string
	AddValues  []string
	AddValueTo string
	// AddOpen is true when the add menu should be open, even before a field is chosen.
	AddOpen bool
}

// listFilterChip is one applied condition as the chip row shows it.
type ListFilterChip struct {
	FieldLabelKey string
	FieldLabel    string
	OpLabelKey    string
	Value         string
	ValueKey      string
	ValueLabel    string
	EditHref      string
	RemoveHref    string
}

// listFieldLink picks a field in the first add step.
type ListFieldLink struct {
	LabelKey string
	Label    string
	Href     string
}

// listOpOption is one condition choice in the second add step.
type ListOpOption struct {
	Key      string
	LabelKey string
	Checked  bool
}

// listChoiceOption is one value choice in the second add step.
type ListChoiceOption struct {
	Value    string
	LabelKey string
	Label    string
	Selected bool
}

// listFilterStep is the second add step: the chosen field, its conditions, and its value.
type ListFilterStep struct {
	FieldKey      string
	FieldLabelKey string
	FieldLabel    string
	Ops           []ListOpOption
	IsChoice      bool
	Choices       []ListChoiceOption
	InputType     string
	Value         string
	Range         bool
	ValueTo       string
}

// listFilterView is everything the shared filter bar renders for one list.
type ListFilterView struct {
	Path      string
	Preserved []ClientHiddenField
	Match     string
	ShowMatch bool
	Chips     []ListFilterChip
	CanAdd    bool
	AddOpen   bool
	Fields    []ListFieldLink
	Step      *ListFilterStep
	ClearHref string
	BackHref  string
}
