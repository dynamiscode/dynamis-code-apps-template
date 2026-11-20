package clients

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// Filter operators. The names are the REST contract (docs/api.md).
const (
	OpEquals   = "equals"
	OpContains = "contains"
	OpBefore   = "before"
	OpAfter    = "after"
	OpBetween  = "between"
	OpIsEmpty  = "is_empty"
)

// Common fields that lists can sort and filter on. Custom fields use their
// short ID instead.
const (
	FieldDisplayName = "displayName"
	FieldEmail       = "email"
	FieldPhone       = "phone"
	FieldNotes       = "notes"
	FieldCreatedAt   = "createdAt"
	FieldUpdatedAt   = "updatedAt"
)

// ErrInvalidQuery is a sort or filter the list cannot run. It is an invalid
// input, so the REST layer answers it as a bad request.
var ErrInvalidQuery = fmt.Errorf("%w: sort or filter is invalid", ErrInvalidInput)

// SortInput orders a list by one field. The zero value keeps newest first.
// Empty values sort last in both directions. Ties break by client ID in the
// sort direction.
type SortInput struct {
	Field      string `json:"field"`
	Descending bool   `json:"descending"`
}

// FilterInput is one condition on one field. Filters combine with AND. Value
// holds one value. Between takes two comma-separated values, and single choice
// takes one or more comma-separated option IDs. Text values are taken as typed.
type FilterInput struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Value kinds decide which operators a field takes and how it orders.
const (
	kindText      = "text"
	kindNumber    = "number"
	kindDate      = "date"
	kindYesNo     = "yes_no"
	kindChoice    = "choice"
	kindTimestamp = "timestamp"
)

// target is a resolved sort or filter field. value is the SQL expression for
// the stored text, which is NULL when unset.
type target struct {
	key        string
	kind       string
	value      string
	filterable bool
	// choiceRanks orders a single-choice field by option label. Only choice
	// targets set it.
	choiceRanks []choiceRank
}

// choiceRank is one option's place among its field's labels. Options with the
// same label share a rank.
type choiceRank struct {
	shortID string
	rank    int
}

var commonFieldColumns = map[string]struct {
	column     string
	kind       string
	filterable bool
}{
	FieldDisplayName: {"display_name", kindText, true},
	FieldEmail:       {"email", kindText, true},
	FieldPhone:       {"phone", kindText, true},
	FieldNotes:       {"notes", kindText, true},
	FieldCreatedAt:   {"created_at", kindTimestamp, true},
	FieldUpdatedAt:   {"updated_at", kindTimestamp, true},
}

// resolveField maps a sort or filter key to its SQL. Common text fields treat
// an empty value as unset, so it sorts last. Custom fields read the slot
// expression that the slot indexes cover.
func (s *Service) resolveField(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	key string,
) (target, error) {
	if common, ok := commonFieldColumns[key]; ok {
		value := common.column
		if common.kind == kindText {
			value = "NULLIF(" + common.column + ", '')"
		}
		return target{key: key, kind: common.kind, value: value, filterable: common.filterable}, nil
	}
	fieldID, err := ParseFieldID(key)
	if err != nil {
		return target{}, ErrInvalidQuery
	}
	ref, err := s.fieldRef(ctx, tx, workspaceID, fieldID)
	if errors.Is(err, ErrFieldNotFound) || (err == nil && ref.archived) {
		return target{}, ErrInvalidQuery
	}
	if err != nil {
		return target{}, err
	}
	t := target{
		key:        fieldID.String(),
		kind:       fieldKind(ref.fieldType),
		value:      database.JSONTextExpr(s.driver, "custom_values", slotKey(ref.slot)),
		filterable: true,
	}
	if t.kind == kindChoice {
		if t.choiceRanks, err = s.choiceRanks(ctx, tx, workspaceID, fieldID); err != nil {
			return target{}, err
		}
	}
	return t, nil
}

// choiceRanks ranks a field's options by label. A value stores the option's
// short ID, which SQL cannot decode to the option row, so the sort maps the
// stored ID to its label's rank. Labels compare as bytes, the order both
// dialects use for text.
// ponytail: the rank CASE lists every option, at most 100 per field (the slot
// limit). Upgrade: store a sort key on the option row when the limit grows.
func (s *Service) choiceRanks(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
) ([]choiceRank, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, label FROM client_field_options
		WHERE workspace_id = ? AND field_id = ?
	`), workspaceID, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type option struct {
		id    OptionID
		label string
	}
	var options []option
	for rows.Next() {
		var o option
		if err := rows.Scan(&o.id, &o.label); err != nil {
			return nil, err
		}
		options = append(options, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(options, func(a, b option) int { return strings.Compare(a.label, b.label) })
	ranks := make([]choiceRank, 0, len(options))
	rank := 0
	for i, o := range options {
		if i == 0 || o.label != options[i-1].label {
			rank++
		}
		ranks = append(ranks, choiceRank{shortID: o.id.String(), rank: rank})
	}
	return ranks, nil
}

// choiceRankExpr maps a stored option short ID, in input, to its label rank.
// Short IDs come from the option codec alphabet, so they need no quoting.
func choiceRankExpr(input string, ranks []choiceRank) string {
	if len(ranks) == 0 {
		return "NULL"
	}
	var b strings.Builder
	b.WriteString("CASE " + input)
	for _, r := range ranks {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", r.shortID, r.rank)
	}
	b.WriteString(" END")
	return b.String()
}

func fieldKind(fieldType string) string {
	switch fieldType {
	case FieldTypeNumber:
		return kindNumber
	case FieldTypeDate:
		return kindDate
	case FieldTypeYesNo:
		return kindYesNo
	case FieldTypeSingleChoice:
		return kindChoice
	}
	return kindText
}

// orderExpr is the expression that orders a target. Numbers order numerically;
// other kinds order by bytes, which PostgreSQL needs its "C" collation for.
// ponytail: a choice sorts by its option's stored short ID, not its label.
// Upgrade: a sort key on client_field_options, written with each option.
func (s *Service) orderExpr(t target) string {
	switch t.kind {
	case kindNumber:
		return database.NumericExpr(s.driver, t.value)
	case kindChoice:
		return choiceRankExpr(t.value, t.choiceRanks)
	}
	return database.TextOrderExpr(s.driver, t.value)
}

// filterPredicate returns one filter's SQL and its arguments. Values are
// validated and canonicalized the same way SetValue stores them, so equality
// on the stored text is exact.
func (s *Service) filterPredicate(t target, f FilterInput) (string, []any, error) {
	if !t.filterable {
		return "", nil, ErrInvalidQuery
	}
	v := t.value
	if f.Op == OpIsEmpty {
		if f.Value != "" {
			return "", nil, ErrInvalidQuery
		}
		return "(" + v + " IS NULL OR " + v + " = '')", nil, nil
	}
	switch t.kind {
	case kindText:
		// ponytail: case folding and contains cannot use the slot index, so they
		// scan the workspace. Upgrade: a folded slot expression index in a migration.
		value := strings.TrimSpace(f.Value)
		if value == "" {
			return "", nil, ErrInvalidQuery
		}
		switch f.Op {
		case OpEquals:
			return database.ASCIILowerExpr(s.driver, v) + " = ?", []any{asciiLower(value)}, nil
		case OpContains:
			return database.ContainsExpr(s.driver, database.ASCIILowerExpr(s.driver, v), "?"), []any{asciiLower(value)}, nil
		}
	case kindNumber:
		values, err := operands(f.Op, f.Value)
		if err != nil {
			return "", nil, err
		}
		canonical := make([]any, len(values))
		for i, value := range values {
			if canonical[i], err = validNumber(value); err != nil {
				return "", nil, ErrInvalidQuery
			}
		}
		if f.Op == OpBetween && !numericOrder(canonical[0].(string), canonical[1].(string)) {
			return "", nil, ErrInvalidQuery
		}
		switch f.Op {
		case OpEquals:
			return v + " = ?", canonical, nil
		case OpBefore:
			return database.NumericExpr(s.driver, v) + " < " + database.NumericExpr(s.driver, "?"), canonical, nil
		case OpAfter:
			return database.NumericExpr(s.driver, v) + " > " + database.NumericExpr(s.driver, "?"), canonical, nil
		case OpBetween:
			return database.NumericExpr(s.driver, v) + " >= " + database.NumericExpr(s.driver, "?") +
				" AND " + database.NumericExpr(s.driver, v) + " <= " + database.NumericExpr(s.driver, "?"), canonical, nil
		}
	case kindDate:
		values, err := operands(f.Op, f.Value)
		if err != nil {
			return "", nil, err
		}
		canonical := make([]any, len(values))
		for i, value := range values {
			date, err := validDate(value)
			if err != nil {
				return "", nil, ErrInvalidQuery
			}
			canonical[i] = date
		}
		if f.Op == OpBetween && canonical[0].(string) > canonical[1].(string) {
			return "", nil, ErrInvalidQuery
		}
		// Dates are fixed-width YYYY-MM-DD, so the column's own collation
		// orders them the same as bytes and the slot index serves the range.
		switch f.Op {
		case OpEquals:
			return v + " = ?", canonical, nil
		case OpBefore:
			return v + " < ?", canonical, nil
		case OpAfter:
			return v + " > ?", canonical, nil
		case OpBetween:
			return v + " >= ? AND " + v + " <= ?", canonical, nil
		}
	case kindTimestamp:
		values, err := operands(f.Op, f.Value)
		if err != nil {
			return "", nil, err
		}
		canonical := make([]any, len(values))
		for i, value := range values {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return "", nil, ErrInvalidQuery
			}
			canonical[i] = parsed.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
		if f.Op == OpBetween && canonical[0].(string) > canonical[1].(string) {
			return "", nil, ErrInvalidQuery
		}
		switch f.Op {
		case OpEquals:
			return v + " = ?", canonical, nil
		case OpBefore:
			return v + " < ?", canonical, nil
		case OpAfter:
			return v + " > ?", canonical, nil
		case OpBetween:
			return v + " >= ? AND " + v + " <= ?", canonical, nil
		}
	case kindYesNo:
		if f.Op != OpEquals {
			return "", nil, ErrInvalidQuery
		}
		value, err := validYesNo(strings.TrimSpace(f.Value))
		if err != nil {
			return "", nil, ErrInvalidQuery
		}
		return v + " = ?", []any{value}, nil
	case kindChoice:
		if f.Op != OpEquals {
			return "", nil, ErrInvalidQuery
		}
		parts := strings.Split(f.Value, ",")
		args := make([]any, 0, len(parts))
		placeholders := make([]string, 0, len(parts))
		for _, part := range parts {
			optionID, err := ParseOptionID(strings.TrimSpace(part))
			if err != nil {
				return "", nil, ErrInvalidQuery
			}
			args = append(args, optionID.String())
			placeholders = append(placeholders, "?")
		}
		return v + " IN (" + strings.Join(placeholders, ", ") + ")", args, nil
	}
	return "", nil, ErrInvalidQuery
}

// operands splits a filter value for its operator. Between takes two values.
// Every other operator takes one.
func operands(op, value string) ([]string, error) {
	if op == OpBetween {
		parts := strings.Split(value, ",")
		if len(parts) != 2 {
			return nil, ErrInvalidQuery
		}
		return []string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])}, nil
	}
	if op != OpEquals && op != OpBefore && op != OpAfter {
		return nil, ErrInvalidQuery
	}
	return []string{strings.TrimSpace(value)}, nil
}

// numericOrder reports whether from is at most to, comparing canonical numbers.
func numericOrder(from, to string) bool {
	a, errA := strconv.ParseFloat(from, 64)
	b, errB := strconv.ParseFloat(to, 64)
	return errA == nil && errB == nil && a <= b
}

// asciiLower folds only ASCII letters, to match ASCIILowerExpr on both dialects.
func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}

// listQuery is one list request's SQL, its arguments, and its sort binding.
type listQuery struct {
	sql       string
	args      []any
	countSQL  string
	countArgs []any
	sortKey   string
	desc      bool
	sorted    bool
	archived  string
}

// archivedScope maps the archived selector to one of its values. An empty
// selector means active, so a request that omits it reads the default list.
func archivedScope(value string) string {
	if value == "" {
		return ArchivedActive
	}
	return value
}

// buildListQuery turns a request into SQL. The archived selector picks active,
// archived, or all clients. Filters AND together. A sorted request orders by the
// sort value and then the ID; pages are numbered from the page's offset.
func (s *Service) buildListQuery(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	input ListInput,
) (listQuery, error) {
	query := listQuery{desc: input.Sort.Descending, sorted: input.Sort.Field != ""}
	if input.Sort.Descending && !query.sorted {
		return listQuery{}, ErrInvalidQuery
	}
	where := []string{"workspace_id = ?"}
	switch query.archived = archivedScope(input.Archived); query.archived {
	case ArchivedActive:
		where = append(where, "archived_at IS NULL")
	case ArchivedOnly:
		where = append(where, "archived_at IS NOT NULL")
	case ArchivedAll:
	default:
		return listQuery{}, ErrInvalidQuery
	}
	args := []any{workspaceID}
	joiner, err := matchJoiner(input.Match)
	if err != nil {
		return listQuery{}, err
	}
	predicates := make([]string, 0, len(input.Filters))
	for _, filter := range input.Filters {
		t, err := s.resolveField(ctx, tx, workspaceID, filter.Field)
		if err != nil {
			return listQuery{}, err
		}
		predicate, predicateArgs, err := s.filterPredicate(t, filter)
		if err != nil {
			return listQuery{}, err
		}
		predicates = append(predicates, "("+predicate+")")
		args = append(args, predicateArgs...)
	}
	if len(predicates) > 0 {
		where = append(where, "("+strings.Join(predicates, joiner)+")")
	}
	// The search always combines with AND, whatever the match mode.
	if input.Search != "" {
		predicate, searchArgs, err := s.searchPredicate(ctx, tx, workspaceID, input.Search)
		if err != nil {
			return listQuery{}, err
		}
		where = append(where, predicate)
		args = append(args, searchArgs...)
	}
	// The total counts every row the filters match, before the page is cut.
	query.countSQL = "SELECT COUNT(*) FROM clients WHERE " + strings.Join(where, " AND ")
	query.countArgs = append([]any(nil), args...)
	sortExpr := "NULL"
	order := "id DESC"
	if query.sorted {
		t, err := s.resolveField(ctx, tx, workspaceID, input.Sort.Field)
		if err != nil {
			return listQuery{}, err
		}
		query.sortKey = t.key
		sortExpr = t.value
		dir := "ASC"
		if query.desc {
			dir = "DESC"
		}
		order = fmt.Sprintf("(%s) IS NULL, %s %s, id %s", t.value, s.orderExpr(t), dir, dir)
	}
	query.sql = "SELECT id, workspace_id, created_by_user_id, display_name, email, phone, notes, " +
		"version, created_at, updated_at, archived_at, " + sortExpr + " AS sort_value " +
		"FROM clients WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	query.args = args
	return query, nil
}

// Match modes for combining filters. MatchAll is the default and joins filters
// with AND. MatchAny joins them with OR.
const (
	MatchAll = "all"
	MatchAny = "any"
)

// normalizeMatch returns the effective match mode, so an empty mode means MatchAll.
func normalizeMatch(match string) string {
	if match == "" {
		return MatchAll
	}
	return match
}

// matchJoiner returns the SQL operator that joins the filter predicates.
func matchJoiner(match string) (string, error) {
	switch normalizeMatch(match) {
	case MatchAll:
		return " AND ", nil
	case MatchAny:
		return " OR ", nil
	default:
		return "", ErrInvalidQuery
	}
}

// listRow scans the extra sort_value column after a client row.
type listRow struct {
	rows      *sql.Rows
	sortValue *sql.NullString
}

func (r listRow) Scan(dest ...any) error {
	return r.rows.Scan(append(dest, r.sortValue)...)
}
