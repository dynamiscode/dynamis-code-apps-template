package viewmodel

import (
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type ListColumn struct {
	Key, Label, LabelKey, SortHref string
	Visible, DefaultVisible        bool
	SortDirection                  string
}

// ListRow is one row of a list. Cells holds the row's values by column key.
type ListRow struct {
	ID             string
	Cells          map[string]ListCell
	Member         identity.MemberSummary
	ActionContext  string
	ActionPosition int
	Archived       bool
}

// CellText returns the stored text of the cell at key, or "" when the row has no such cell.
func (r ListRow) CellText(key string) string { return r.Cells[key].Text }

// CellKind says how a list cell is shown. The row builder picks it from the column's type, so rendering never
// parses a value back out of its text.
type CellKind int

const (
	CellText CellKind = iota
	CellTime
	CellYesNo
	CellRole
)

// ListHasContent reports whether a list shows rows or carries an active
// query; empty-state chrome (search, views, filters, pagination) hides until
// it does, so an empty list teaches instead of offering controls for nothing.
func (p PageData) ListHasContent() bool {
	return p.ListTotal > 0 || p.HasClientFilters || p.ListSearch != ""
}

// ListCell is one cell. Text is the stored value and the badge class suffix. At and On hold the typed value for
// time and yes/no cells.
type ListCell struct {
	Kind CellKind
	Text string
	At   time.Time
	On   bool
}

func TextCell(text string) ListCell        { return ListCell{Kind: CellText, Text: text} }
func TimeCell(at time.Time) ListCell       { return ListCell{Kind: CellTime, At: at} }
func YesNoCell(on bool) ListCell           { return ListCell{Kind: CellYesNo, On: on} }
func RoleCell(role identity.Role) ListCell { return ListCell{Kind: CellRole, Text: string(role)} }

// ClientListLinkKey names the first visible column, which carries the row's open link.
func (p PageData) ClientListLinkKey() string {
	for _, column := range p.ListColumns {
		if column.Visible {
			return column.Key
		}
	}
	return ""
}

// ClientListCellValue returns the text a cell shows. The ID column shows the row's short ID on every list, so no
// list stores it as a cell.
func (p PageData) ClientListCellValue(column ListColumn, row ListRow) string {
	if column.Key == "id" {
		return row.ID
	}
	cell := row.Cells[column.Key]
	switch cell.Kind {
	case CellTime:
		if cell.At.IsZero() {
			return ""
		}
		return p.DateTime(cell.At)
	case CellYesNo:
		if cell.On {
			return p.T("clients.yes")
		}
		return p.T("clients.no")
	case CellRole:
		return p.Role(identity.Role(cell.Text))
	}
	return cell.Text
}

// clientHiddenField is one list parameter that a form on the list keeps when it submits.
type ClientHiddenField struct {
	Name  string
	Value string
}
