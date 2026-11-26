// Package transactions records immutable workspace money entries.
package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid transactions entry")
var ErrActiveRefunds = errors.New("receipt has active refunds")

// RefundLimitError reports the remainder observed in the refund transaction.
type RefundLimitError struct {
	Remaining int64
	Currency  string
}

func (e *RefundLimitError) Error() string { return "refund exceeds remaining receipt amount" }
func (e *RefundLimitError) Unwrap() error { return ErrInvalidInput }

type EntryID int64

func (v EntryID) String() string               { return id.EncodeKey(id.TypeTransactionsEntry, int64(v)) }
func (v EntryID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }

func ParseEntryID(value string) (EntryID, error) {
	key, err := id.DecodeKey(id.TypeTransactionsEntry, value)
	return EntryID(key), err
}

type Entry struct {
	ID                EntryID           `json:"id"`
	Description       string            `json:"description"`
	Direction         string            `json:"direction"`
	AmountMinor       int64             `json:"amountMinor"`
	Currency          string            `json:"currency"`
	PaymentMethod     string            `json:"paymentMethod"`
	PaymentMethodName string            `json:"paymentMethodName"`
	OccurredOn        string            `json:"occurredOn"`
	ClientID          *clients.ClientID `json:"clientId"`
	CategoryID        *CategoryID       `json:"categoryId"`
	CategoryName      string            `json:"categoryName"`
	Note              string            `json:"note"`
	CreatedBy         identity.UserID   `json:"createdBy"`
	CreatedAt         time.Time         `json:"createdAt"`
	VoidedBy          identity.UserID   `json:"voidedBy,omitempty"`
	VoidedAt          string            `json:"voidedAt,omitempty"`
	VoidReason        string            `json:"voidReason,omitempty"`
	ReplacesEntryID   EntryID           `json:"replacesEntryId,omitempty"`
	RefundOfEntryID   EntryID           `json:"refundOfEntryId,omitempty"`
}

type Input struct {
	Description     string            `json:"description"`
	Direction       string            `json:"direction"`
	AmountMinor     int64             `json:"amountMinor"`
	Currency        string            `json:"currency"`
	PaymentMethod   string            `json:"paymentMethod"`
	OccurredOn      string            `json:"occurredOn"`
	ClientID        *clients.ClientID `json:"clientId"`
	CategoryID      *CategoryID       `json:"categoryId"`
	Note            string            `json:"note"`
	ReplacesEntryID EntryID           `json:"replacesEntryId,omitempty"`
	RefundOfEntryID EntryID           `json:"refundOfEntryId,omitempty"`
}

type ListFilter struct {
	Page            int
	Limit           int
	EntryID         EntryID
	ClientID        clients.ClientID
	RefundOfEntryID EntryID
	CategoryID      CategoryID
	PaymentMethod   string
	Direction       string
	From            string
	To              string
	Conditions      []ListCondition
	Match           string
	Search          string
	Sort            string
	Order           string
}

type ListCondition struct {
	Field, Op, Value string
}

type Service struct {
	db   *sql.DB
	auth *identity.Service
}

func NewService(db *sql.DB, auth *identity.Service) *Service { return &Service{db: db, auth: auth} }

func retrySerialization[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		value, err := operation()
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "40001" || attempt == 2 {
			return value, err
		}
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
	}
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, input Input) (Entry, error) {
	return retrySerialization(ctx, func() (Entry, error) {
		return s.create(ctx, actor, workspace, input)
	})
}

func (s *Service) create(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, input Input) (Entry, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite)
	if err != nil {
		return Entry{}, err
	}
	if !identity.CanWriteTransactions(principal) {
		return Entry{}, identity.ErrForbidden
	}
	if input.ReplacesEntryID != 0 && !identity.CanVoidTransactions(principal) {
		return Entry{}, identity.ErrForbidden
	}

	input.Description = strings.TrimSpace(input.Description)
	input.Direction = strings.TrimSpace(input.Direction)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
	input.OccurredOn = strings.TrimSpace(input.OccurredOn)
	input.Note = strings.TrimSpace(input.Note)
	if input.Description == "" || !utf8.ValidString(input.Description) || utf8.RuneCountInString(input.Description) > 200 {
		return Entry{}, ErrInvalidInput
	}
	if (input.Direction != "in" && input.Direction != "out") || input.AmountMinor <= 0 || len(input.Currency) != 3 || input.Currency[0] < 'A' || input.Currency[0] > 'Z' || input.Currency[1] < 'A' || input.Currency[1] > 'Z' || input.Currency[2] < 'A' || input.Currency[2] > 'Z' || input.PaymentMethod == "" || utf8.RuneCountInString(input.PaymentMethod) > 64 || utf8.RuneCountInString(input.Note) > 1000 || !utf8.ValidString(input.Note) || !utf8.ValidString(input.PaymentMethod) {
		return Entry{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	if input.OccurredOn == "" {
		input.OccurredOn, err = today(ctx, tx, workspace, now)
		if err != nil {
			return Entry{}, err
		}
	} else if _, err := time.Parse("2006-01-02", input.OccurredOn); err != nil {
		return Entry{}, ErrInvalidInput
	}
	if err := seedMethods(ctx, tx, workspace); err != nil {
		return Entry{}, err
	}
	var methodName string
	if err := tx.QueryRowContext(ctx, `SELECT display_name FROM transactions_payment_methods WHERE workspace_id = $1 AND name = $2 AND enabled = TRUE`, workspace, input.PaymentMethod).Scan(&methodName); errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrInvalidInput
	} else if err != nil {
		return Entry{}, err
	}
	var valid bool
	if input.ClientID != nil {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM clients WHERE workspace_id = $1 AND id = $2)`, workspace, *input.ClientID).Scan(&valid); err != nil {
			return Entry{}, err
		}
		if !valid {
			return Entry{}, ErrInvalidInput
		}
	}
	var categoryName string
	if input.CategoryID != nil {
		if input.Direction != "out" {
			return Entry{}, ErrInvalidInput
		}
		err := tx.QueryRowContext(ctx, `UPDATE transactions_expense_categories SET name = name WHERE workspace_id = $1 AND id = $2 AND archived = FALSE RETURNING name`, workspace, *input.CategoryID).Scan(&categoryName)
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrInvalidInput
		}
		if err != nil {
			return Entry{}, err
		}
	}
	if input.ReplacesEntryID < 0 || input.RefundOfEntryID < 0 {
		return Entry{}, ErrInvalidInput
	}
	if input.ReplacesEntryID != 0 {
		var refundOf EntryID
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(refund_of_entry_id, 0) FROM transactions_entries WHERE workspace_id = $1 AND id = $2 AND voided_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM transactions_entries replacement WHERE replacement.workspace_id = $1 AND replacement.replaces_entry_id = $2)`, workspace, input.ReplacesEntryID).Scan(&refundOf)
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrInvalidInput
		}
		if err != nil {
			return Entry{}, err
		}
		if refundOf != 0 && refundOf != input.RefundOfEntryID {
			return Entry{}, ErrInvalidInput
		}
	}
	if input.RefundOfEntryID != 0 {
		if input.Direction != "out" {
			return Entry{}, ErrInvalidInput
		}
		// Lock the receipt before reading the remainder; serializable transactions
		// that observed an older snapshot fail rather than overspend the receipt.
		result, err := tx.ExecContext(ctx, `UPDATE transactions_entries SET amount_minor = amount_minor WHERE workspace_id = $1 AND id = $2 AND direction = 'in' AND currency = $3 AND voided_at IS NULL`, workspace, input.RefundOfEntryID, input.Currency)
		if err != nil {
			return Entry{}, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return Entry{}, err
		}
		if count != 1 {
			return Entry{}, ErrInvalidInput
		}
		var remaining int64
		if err := tx.QueryRowContext(ctx, `SELECT amount_minor - COALESCE((SELECT SUM(amount_minor) FROM transactions_entries refunds WHERE refunds.workspace_id = $1 AND refunds.refund_of_entry_id = $2 AND refunds.voided_at IS NULL), 0) FROM transactions_entries WHERE workspace_id = $1 AND id = $2`, workspace, input.RefundOfEntryID).Scan(&remaining); err != nil {
			return Entry{}, err
		}
		if input.AmountMinor > remaining {
			return Entry{}, &RefundLimitError{Remaining: remaining, Currency: input.Currency}
		}
	}
	entry := Entry{Description: input.Description, Direction: input.Direction, AmountMinor: input.AmountMinor, Currency: input.Currency, PaymentMethod: input.PaymentMethod, PaymentMethodName: methodName, OccurredOn: input.OccurredOn, ClientID: input.ClientID, CategoryID: input.CategoryID, CategoryName: categoryName, Note: input.Note, CreatedBy: principal.UserID, CreatedAt: now, ReplacesEntryID: input.ReplacesEntryID, RefundOfEntryID: input.RefundOfEntryID}
	err = tx.QueryRowContext(ctx, `INSERT INTO transactions_entries (workspace_id, direction, amount_minor, currency, payment_method, occurred_on, client_id, note, created_by, created_at, replaces_entry_id, refund_of_entry_id, category_id, description) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,0),NULLIF($12,0),$13,$14) RETURNING id`, workspace, entry.Direction, entry.AmountMinor, entry.Currency, entry.PaymentMethod, entry.OccurredOn, entry.ClientID, entry.Note, entry.CreatedBy, now.Format(time.RFC3339Nano), entry.ReplacesEntryID, entry.RefundOfEntryID, entry.CategoryID, entry.Description).Scan(&entry.ID)
	if err != nil {
		return Entry{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "transactions.entry.created", ActorUserID: principal.UserID, AuthMethod: principal.AuthMethod, WorkspaceID: workspace, TargetType: "transactions_entry", TargetKey: int64(entry.ID), Action: "transactions.entry.create", Outcome: "success", Metadata: "{}", CreatedAt: now}); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Void preserves the original entry and records the authorized correction.
func (s *Service) Void(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, entryID EntryID, reason string) (Entry, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite)
	if err != nil {
		return Entry{}, err
	}
	if !identity.CanVoidTransactions(principal) {
		return Entry{}, identity.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if entryID <= 0 || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 1000 {
		return Entry{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	var entry Entry
	var client, category sql.NullInt64
	var created string
	entry.VoidedBy, entry.VoidedAt, entry.VoidReason = principal.UserID, now.Format(time.RFC3339Nano), reason
	err = tx.QueryRowContext(ctx, `UPDATE transactions_entries SET voided_by = $1, voided_at = $2, void_reason = $3 WHERE workspace_id = $4 AND id = $5 AND voided_at IS NULL RETURNING id, direction, amount_minor, currency, payment_method, occurred_on, client_id, note, created_by, created_at, COALESCE(replaces_entry_id, 0), COALESCE(refund_of_entry_id, 0), category_id, description`, principal.UserID, entry.VoidedAt, reason, workspace, entryID).Scan(&entry.ID, &entry.Direction, &entry.AmountMinor, &entry.Currency, &entry.PaymentMethod, &entry.OccurredOn, &client, &entry.Note, &entry.CreatedBy, &created, &entry.ReplacesEntryID, &entry.RefundOfEntryID, &category, &entry.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrInvalidInput
	}
	if err != nil {
		return Entry{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT display_name FROM transactions_payment_methods WHERE workspace_id = $1 AND name = $2`, workspace, entry.PaymentMethod).Scan(&entry.PaymentMethodName); err != nil {
		return Entry{}, err
	}
	if category.Valid {
		value := CategoryID(category.Int64)
		entry.CategoryID = &value
		if err := tx.QueryRowContext(ctx, `SELECT name FROM transactions_expense_categories WHERE workspace_id = $1 AND id = $2`, workspace, value).Scan(&entry.CategoryName); err != nil {
			return Entry{}, err
		}
	}
	if entry.Direction == "in" {
		var hasRefunds bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM transactions_entries WHERE workspace_id = $1 AND refund_of_entry_id = $2 AND voided_at IS NULL)`, workspace, entryID).Scan(&hasRefunds); err != nil {
			return Entry{}, err
		}
		if hasRefunds {
			return Entry{}, errors.Join(ErrInvalidInput, ErrActiveRefunds)
		}
	}
	if client.Valid {
		value := clients.ClientID(client.Int64)
		entry.ClientID = &value
	}
	entry.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Entry{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "transactions.entry.voided", ActorUserID: principal.UserID, AuthMethod: principal.AuthMethod, WorkspaceID: workspace, TargetType: "transactions_entry", TargetKey: int64(entry.ID), Action: "transactions.entry.void", Outcome: "success", Metadata: "{}", CreatedAt: now}); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// List returns one page of at most 100 entries, ordered by creation by default.
func (s *Service) List(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, filter ListFilter) ([]Entry, error) {
	return s.list(ctx, actor, workspace, filter, nil)
}

// ListPage returns the page and the number of entries matching the same filters.
func (s *Service) ListPage(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, filter ListFilter) ([]Entry, int, error) {
	var total int
	entries, err := s.list(ctx, actor, workspace, filter, &total)
	return entries, total, err
}

func (s *Service) list(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, filter ListFilter, total *int) ([]Entry, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 100
	}
	if filter.Page < 1 || filter.Page > 1000000 || limit < 1 || limit > 100 || filter.EntryID < 0 || filter.ClientID < 0 || filter.CategoryID < 0 || filter.RefundOfEntryID < 0 ||
		(filter.Direction != "" && filter.Direction != "in" && filter.Direction != "out") ||
		len(filter.PaymentMethod) > 128 || len(filter.Search) > 200 || len(filter.Conditions) > 20 ||
		(filter.Match != "" && filter.Match != "all" && filter.Match != "any") ||
		(filter.Order != "" && filter.Order != "asc" && filter.Order != "desc") ||
		(filter.From != "" && filter.To != "" && filter.From > filter.To) {
		return nil, ErrInvalidInput
	}
	for _, date := range []string{filter.From, filter.To} {
		if date != "" {
			parsed, err := time.Parse("2006-01-02", date)
			if err != nil || parsed.Format("2006-01-02") != date {
				return nil, ErrInvalidInput
			}
		}
	}
	args := []any{workspace, (filter.Page - 1) * limit, filter.ClientID, filter.CategoryID, filter.PaymentMethod, filter.Direction, filter.From, filter.To, filter.RefundOfEntryID, filter.EntryID}
	query := `SELECT e.id, e.direction, e.amount_minor, e.currency, e.payment_method, m.display_name, e.occurred_on, e.client_id, e.note, e.created_by, e.created_at, COALESCE(e.voided_by, 0), COALESCE(e.voided_at, ''), e.void_reason, COALESCE(e.replaces_entry_id, 0), COALESCE(e.refund_of_entry_id, 0), e.category_id, COALESCE(c.name, ''), e.description FROM transactions_entries e JOIN transactions_payment_methods m ON m.workspace_id = e.workspace_id AND m.name = e.payment_method LEFT JOIN transactions_expense_categories c ON c.workspace_id = e.workspace_id AND c.id = e.category_id WHERE e.workspace_id = $1 AND (CAST($3 AS BIGINT) = 0 OR e.client_id = $3) AND (CAST($4 AS BIGINT) = 0 OR e.category_id = $4) AND ($5 = '' OR e.payment_method = $5) AND ($6 = '' OR e.direction = $6) AND ($7 = '' OR e.occurred_on >= $7) AND ($8 = '' OR e.occurred_on <= $8) AND (CAST($9 AS BIGINT) = 0 OR e.refund_of_entry_id = $9) AND (CAST($10 AS BIGINT) = 0 OR e.id = $10)`
	addArg := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	if filter.Search != "" {
		value := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(strings.TrimSpace(filter.Search)))
		p := addArg("%" + value + "%")
		query += ` AND (LOWER(e.description) LIKE ` + p + ` ESCAPE '\' OR LOWER(e.note) LIKE ` + p + ` ESCAPE '\' OR LOWER(m.display_name) LIKE ` + p + ` ESCAPE '\' OR LOWER(COALESCE(c.name, '')) LIKE ` + p + ` ESCAPE '\' OR EXISTS (SELECT 1 FROM clients lc WHERE lc.workspace_id = e.workspace_id AND lc.id = e.client_id AND LOWER(lc.display_name) LIKE ` + p + ` ESCAPE '\'))`
	}
	conditions := make([]string, 0, len(filter.Conditions))
	for _, condition := range filter.Conditions {
		column := map[string]string{"client_id": "e.client_id", "entry_id": "e.id", "refund_of_entry_id": "e.refund_of_entry_id", "category_id": "e.category_id", "payment_method": "e.payment_method", "direction": "e.direction", "occurred_on": "e.occurred_on"}[condition.Field]
		if column == "" {
			return nil, ErrInvalidInput
		}
		values := strings.Split(condition.Value, ",")
		if condition.Field == "occurred_on" {
			if (condition.Op == "between" && len(values) != 2) || (condition.Op != "between" && len(values) != 1) {
				return nil, ErrInvalidInput
			}
			for _, value := range values {
				parsed, err := time.Parse("2006-01-02", value)
				if err != nil || parsed.Format("2006-01-02") != value {
					return nil, ErrInvalidInput
				}
			}
			switch condition.Op {
			case "equals":
				conditions = append(conditions, column+" = "+addArg(values[0]))
			case "gte":
				conditions = append(conditions, column+" >= "+addArg(values[0]))
			case "lte":
				conditions = append(conditions, column+" <= "+addArg(values[0]))
			case "between":
				if values[0] > values[1] {
					return nil, ErrInvalidInput
				}
				conditions = append(conditions, "("+column+" >= "+addArg(values[0])+" AND "+column+" <= "+addArg(values[1])+")")
			default:
				return nil, ErrInvalidInput
			}
			continue
		}
		if condition.Op != "equals" || len(values) == 0 || len(values) > 100 {
			return nil, ErrInvalidInput
		}
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			var arg any = value
			switch condition.Field {
			case "client_id":
				id, err := clients.ParseClientID(value)
				if err != nil || id <= 0 {
					return nil, ErrInvalidInput
				}
				arg = id
			case "entry_id", "refund_of_entry_id":
				id, err := ParseEntryID(value)
				if err != nil || id <= 0 {
					return nil, ErrInvalidInput
				}
				arg = id
			case "category_id":
				id, err := ParseCategoryID(value)
				if err != nil || id <= 0 {
					return nil, ErrInvalidInput
				}
				arg = id
			case "direction":
				if value != "in" && value != "out" {
					return nil, ErrInvalidInput
				}
			case "payment_method":
				if value == "" || len(value) > 128 {
					return nil, ErrInvalidInput
				}
			}
			placeholders = append(placeholders, addArg(arg))
		}
		conditions = append(conditions, column+" IN ("+strings.Join(placeholders, ", ")+")")
	}
	if len(conditions) > 0 {
		join := " AND "
		if filter.Match == "any" {
			join = " OR "
		}
		query += " AND (" + strings.Join(conditions, join) + ")"
	}
	sortColumn := map[string]string{"": "e.id", "date": "e.occurred_on", "direction": "e.direction", "amount": "e.currency", "method": "m.display_name", "category": "c.name", "client": "(SELECT lc.display_name FROM clients lc WHERE lc.workspace_id = e.workspace_id AND lc.id = e.client_id)", "note": "e.note", "status": "e.voided_at"}[filter.Sort]
	if sortColumn == "" {
		return nil, ErrInvalidInput
	}
	if total != nil {
		countQuery := "SELECT COUNT(*)" + query[strings.Index(query, " FROM "):] + " AND CAST($2 AS BIGINT) >= 0"
		if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(total); err != nil {
			return nil, err
		}
	}
	order := " DESC"
	if filter.Order == "asc" {
		order = " ASC"
	}
	query += " ORDER BY " + sortColumn + order
	if filter.Sort == "amount" {
		query += ", e.amount_minor" + order
	}
	query += ", e.id DESC LIMIT " + addArg(limit) + " OFFSET $2"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanEntry(rows *sql.Rows) (Entry, error) {
	var entry Entry
	var client, category sql.NullInt64
	var created string
	if err := rows.Scan(&entry.ID, &entry.Direction, &entry.AmountMinor, &entry.Currency, &entry.PaymentMethod, &entry.PaymentMethodName, &entry.OccurredOn, &client, &entry.Note, &entry.CreatedBy, &created, &entry.VoidedBy, &entry.VoidedAt, &entry.VoidReason, &entry.ReplacesEntryID, &entry.RefundOfEntryID, &category, &entry.CategoryName, &entry.Description); err != nil {
		return Entry{}, err
	}
	if client.Valid {
		value := clients.ClientID(client.Int64)
		entry.ClientID = &value
	}
	if category.Valid {
		value := CategoryID(category.Int64)
		entry.CategoryID = &value
	}
	var err error
	entry.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return entry, err
}

// EntriesByID returns entries from one workspace for at most 100 IDs.
func (s *Service) EntriesByID(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, ids []EntryID) (map[EntryID]Entry, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	if len(ids) > 100 {
		return nil, ErrInvalidInput
	}
	entries := make(map[EntryID]Entry, len(ids))
	if len(ids) == 0 {
		return entries, nil
	}
	args := make([]any, 1, len(ids)+1)
	args[0] = workspace
	placeholders := make([]string, 0, len(ids))
	for _, entryID := range ids {
		if entryID <= 0 {
			return nil, ErrInvalidInput
		}
		args = append(args, entryID)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.direction, e.amount_minor, e.currency, e.payment_method, m.display_name, e.occurred_on, e.client_id, e.note, e.created_by, e.created_at, COALESCE(e.voided_by, 0), COALESCE(e.voided_at, ''), e.void_reason, COALESCE(e.replaces_entry_id, 0), COALESCE(e.refund_of_entry_id, 0), e.category_id, COALESCE(c.name, ''), e.description FROM transactions_entries e JOIN transactions_payment_methods m ON m.workspace_id = e.workspace_id AND m.name = e.payment_method LEFT JOIN transactions_expense_categories c ON c.workspace_id = e.workspace_id AND c.id = e.category_id WHERE e.workspace_id = $1 AND e.id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries[entry.ID] = entry
	}
	return entries, rows.Err()
}

// RefundCounts returns counts including voided refunds for up to one page of receipts.
func (s *Service) RefundCounts(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, receipts []EntryID) (map[EntryID]int, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	if len(receipts) > 100 {
		return nil, ErrInvalidInput
	}
	counts := make(map[EntryID]int)
	if len(receipts) == 0 {
		return counts, nil
	}
	args := make([]any, 1, len(receipts)+1)
	args[0] = workspace
	placeholders := make([]string, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt <= 0 {
			return nil, ErrInvalidInput
		}
		args = append(args, receipt)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT refund_of_entry_id, COUNT(*) FROM transactions_entries WHERE workspace_id = $1 AND refund_of_entry_id IN (`+strings.Join(placeholders, ",")+`) GROUP BY refund_of_entry_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var receipt EntryID
		var count int
		if err := rows.Scan(&receipt, &count); err != nil {
			return nil, err
		}
		counts[receipt] = count
	}
	return counts, rows.Err()
}

func (s *Service) Methods(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	if err := seedMethods(ctx, tx, workspace); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT name FROM transactions_payment_methods WHERE workspace_id = $1 AND enabled = TRUE ORDER BY name`, workspace)
	if err != nil {
		return nil, err
	}
	methods := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		methods = append(methods, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return methods, nil
}

func seedMethods(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID) error {
	for _, method := range []string{"check", "transfer", "cash", "card"} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO transactions_payment_methods (workspace_id, name, display_name) VALUES ($1, $2, $2) ON CONFLICT (workspace_id, name) DO NOTHING`, workspace, method); err != nil {
			return err
		}
	}
	return nil
}

// Today returns the current date in the workspace time zone.
func (s *Service) Today(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) (string, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return "", err
	}
	if !identity.CanReadTransactions(principal) {
		return "", identity.ErrForbidden
	}
	return today(ctx, s.db, workspace, time.Now())
}

func today(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, workspace identity.WorkspaceID, now time.Time) (string, error) {
	var zone string
	if err := query.QueryRowContext(ctx, `SELECT time_zone FROM workspaces WHERE id = $1`, workspace).Scan(&zone); err != nil {
		return "", err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("2006-01-02"), nil
}
