package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
)

var ErrConflict = errors.New("transaction revision conflict")

type UpdateInput struct {
	ExpectedRevision int64
	Field            string
	Value            string
}

// Update changes exactly one field, preserving the transaction identity and links.
func (s *Service) Update(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, entryID EntryID, input UpdateInput) (Entry, error) {
	return retrySerialization(ctx, func() (Entry, error) {
		return s.update(ctx, actor, workspace, entryID, input)
	})
}

func (s *Service) update(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, entryID EntryID, input UpdateInput) (Entry, error) {
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
	if entryID <= 0 || input.ExpectedRevision <= 0 {
		return Entry{}, ErrInvalidInput
	}

	var entry Entry
	var client, category sql.NullInt64
	var created string
	err = tx.QueryRowContext(ctx, `SELECT e.id, e.description, e.direction, e.amount_minor, e.currency, e.payment_method, m.display_name, e.occurred_on, e.client_id, e.category_id, COALESCE(c.name, ''), e.note, e.created_by, e.created_at, COALESCE(e.replaces_entry_id, 0), COALESCE(e.refund_of_entry_id, 0), e.revision FROM transactions_entries e JOIN transactions_payment_methods m ON m.workspace_id = e.workspace_id AND m.name = e.payment_method LEFT JOIN transactions_expense_categories c ON c.workspace_id = e.workspace_id AND c.id = e.category_id WHERE e.workspace_id = $1 AND e.id = $2 AND e.voided_at IS NULL`, workspace, entryID).Scan(&entry.ID, &entry.Description, &entry.Direction, &entry.AmountMinor, &entry.Currency, &entry.PaymentMethod, &entry.PaymentMethodName, &entry.OccurredOn, &client, &category, &entry.CategoryName, &entry.Note, &entry.CreatedBy, &created, &entry.ReplacesEntryID, &entry.RefundOfEntryID, &entry.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrInvalidInput
	}
	if err != nil {
		return Entry{}, err
	}
	if entry.Revision != input.ExpectedRevision {
		return Entry{}, ErrConflict
	}
	entry.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
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
	before := entry

	value := strings.TrimSpace(input.Value)
	changed := false
	switch input.Field {
	case "description":
		if value == entry.Description {
			break
		}
		if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 200 {
			return Entry{}, ErrInvalidInput
		}
		entry.Description, changed = value, true
	case "direction":
		if value == entry.Direction {
			break
		}
		if value != "in" && value != "out" {
			return Entry{}, ErrInvalidInput
		}
		entry.Direction, changed = value, true
	case "amount_minor":
		amount, err := strconv.ParseInt(value, 10, 64)
		if err != nil || amount <= 0 {
			return Entry{}, ErrInvalidInput
		}
		changed = amount != entry.AmountMinor
		entry.AmountMinor = amount
	case "currency":
		value = strings.ToUpper(value)
		if value == entry.Currency {
			break
		}
		currencies, err := currency.List(ctx, tx, workspace)
		if err != nil {
			return Entry{}, err
		}
		enabled := false
		for _, item := range currencies {
			if item.Code == value && CurrencyAvailable(item) {
				enabled = true
				break
			}
		}
		if !enabled {
			return Entry{}, ErrInvalidInput
		}
		oldScale, oldOK := CurrencyScale(entry.Currency)
		newScale, newOK := CurrencyScale(value)
		if !oldOK || !newOK {
			return Entry{}, ErrInvalidInput
		}
		for _, item := range currencies {
			if item.Code == entry.Currency && item.Decimals != oldScale {
				return Entry{}, ErrInvalidInput
			}
		}
		amount := entry.AmountMinor
		for oldScale < newScale {
			if amount > math.MaxInt64/10 {
				return Entry{}, ErrInvalidInput
			}
			amount *= 10
			oldScale++
		}
		for oldScale > newScale {
			if amount%10 != 0 {
				return Entry{}, ErrInvalidInput
			}
			amount /= 10
			oldScale--
		}
		entry.Currency, entry.AmountMinor, changed = value, amount, true
	case "payment_method":
		if value == entry.PaymentMethod {
			break
		}
		if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 64 {
			return Entry{}, ErrInvalidInput
		}
		var name string
		err := tx.QueryRowContext(ctx, `SELECT display_name FROM transactions_payment_methods WHERE workspace_id = $1 AND name = $2 AND enabled = TRUE`, workspace, value).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrInvalidInput
		}
		if err != nil {
			return Entry{}, err
		}
		entry.PaymentMethod, entry.PaymentMethodName, changed = value, name, true
	case "occurred_on":
		if value == entry.OccurredOn {
			break
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return Entry{}, ErrInvalidInput
		}
		entry.OccurredOn, changed = value, true
	case "client_id":
		var id clients.ClientID
		if value != "" {
			id, err = clients.ParseClientID(value)
			if err != nil || id <= 0 {
				return Entry{}, ErrInvalidInput
			}
		}
		if (entry.ClientID == nil && id == 0) || (entry.ClientID != nil && *entry.ClientID == id) {
			break
		}
		if id == 0 {
			entry.ClientID = nil
		} else {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM clients WHERE workspace_id = $1 AND id = $2)`, workspace, id).Scan(&exists); err != nil {
				return Entry{}, err
			}
			if !exists {
				return Entry{}, ErrInvalidInput
			}
			entry.ClientID = &id
		}
		changed = true
	case "category_id":
		var id CategoryID
		if value != "" {
			id, err = ParseCategoryID(value)
			if err != nil || id <= 0 {
				return Entry{}, ErrInvalidInput
			}
		}
		if (entry.CategoryID == nil && id == 0) || (entry.CategoryID != nil && *entry.CategoryID == id) {
			break
		}
		if id == 0 {
			entry.CategoryID, entry.CategoryName = nil, ""
		} else {
			var name string
			err := tx.QueryRowContext(ctx, `UPDATE transactions_expense_categories SET name = name WHERE workspace_id = $1 AND id = $2 AND archived = FALSE RETURNING name`, workspace, id).Scan(&name)
			if errors.Is(err, sql.ErrNoRows) {
				return Entry{}, ErrInvalidInput
			}
			if err != nil {
				return Entry{}, err
			}
			entry.CategoryID, entry.CategoryName = &id, name
		}
		changed = true
	case "note":
		if value == entry.Note {
			break
		}
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1000 {
			return Entry{}, ErrInvalidInput
		}
		entry.Note, changed = value, true
	default:
		return Entry{}, ErrInvalidInput
	}
	if !changed {
		return entry, nil
	}
	if entry.CategoryID != nil && entry.Direction != "out" {
		return Entry{}, ErrInvalidInput
	}

	// Lock the shared receipt before checking active refunds, as creation does.
	if entry.RefundOfEntryID != 0 {
		if entry.Direction != "out" {
			return Entry{}, ErrInvalidInput
		}
		result, err := tx.ExecContext(ctx, `UPDATE transactions_entries SET amount_minor = amount_minor WHERE workspace_id = $1 AND id = $2 AND direction = 'in' AND currency = $3 AND voided_at IS NULL`, workspace, entry.RefundOfEntryID, entry.Currency)
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
		if err := tx.QueryRowContext(ctx, `SELECT amount_minor - COALESCE((SELECT SUM(amount_minor) FROM transactions_entries r WHERE r.workspace_id = $1 AND r.refund_of_entry_id = $2 AND r.voided_at IS NULL AND r.id <> $3), 0) FROM transactions_entries WHERE workspace_id = $1 AND id = $2`, workspace, entry.RefundOfEntryID, entryID).Scan(&remaining); err != nil {
			return Entry{}, err
		}
		if entry.AmountMinor > remaining {
			return Entry{}, &RefundLimitError{Remaining: remaining, Currency: entry.Currency}
		}
	}
	var column string
	var fieldValue, oldValue, newValue any
	switch input.Field {
	case "description":
		column, fieldValue = "description", entry.Description
	case "note":
		column, fieldValue = "note", entry.Note
	case "client_id":
		column, fieldValue = "client_id", entry.ClientID
	case "direction":
		column, fieldValue, oldValue, newValue = "direction", entry.Direction, before.Direction, entry.Direction
	case "amount_minor":
		column, fieldValue, oldValue, newValue = "amount_minor", entry.AmountMinor, before.AmountMinor, entry.AmountMinor
	case "payment_method":
		column, fieldValue, oldValue, newValue = "payment_method", entry.PaymentMethod, before.PaymentMethod, entry.PaymentMethod
	case "occurred_on":
		column, fieldValue, oldValue, newValue = "occurred_on", entry.OccurredOn, before.OccurredOn, entry.OccurredOn
	case "category_id":
		column, fieldValue, oldValue, newValue = "category_id", entry.CategoryID, before.CategoryID, entry.CategoryID
	case "currency":
		column, fieldValue = "currency", entry.Currency
		oldValue = map[string]any{"currency": before.Currency, "amountMinor": before.AmountMinor}
		newValue = map[string]any{"currency": entry.Currency, "amountMinor": entry.AmountMinor}
	}
	query := `UPDATE transactions_entries SET ` + column + ` = $1, revision = revision + 1 WHERE workspace_id = $2 AND id = $3 AND revision = $4 AND voided_at IS NULL`
	args := []any{fieldValue, workspace, entryID, input.ExpectedRevision}
	if input.Field == "currency" {
		query = `UPDATE transactions_entries SET currency = $1, amount_minor = $2, revision = revision + 1 WHERE workspace_id = $3 AND id = $4 AND revision = $5 AND voided_at IS NULL`
		args = []any{entry.Currency, entry.AmountMinor, workspace, entryID, input.ExpectedRevision}
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return Entry{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Entry{}, err
	}
	if count != 1 {
		return Entry{}, ErrConflict
	}
	// The update locks the receipt; validate refunds before committing it.
	if entry.RefundOfEntryID == 0 {
		var countRefunds int64
		var total sql.NullInt64
		var incompatible bool
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), SUM(amount_minor), COALESCE(MAX(CASE WHEN currency <> $3 THEN 1 ELSE 0 END), 0) <> 0 FROM transactions_entries WHERE workspace_id = $1 AND refund_of_entry_id = $2 AND voided_at IS NULL`, workspace, entryID, entry.Currency).Scan(&countRefunds, &total, &incompatible); err != nil {
			return Entry{}, err
		}
		if countRefunds > 0 && (entry.Direction != "in" || incompatible || !total.Valid || total.Int64 > entry.AmountMinor) {
			return Entry{}, ErrInvalidInput
		}
	}
	entry.Revision++
	metadata := map[string]any{"field": input.Field, "revision": entry.Revision, "changed": true}
	if input.Field != "description" && input.Field != "note" && input.Field != "client_id" {
		metadata["before"], metadata["after"] = oldValue, newValue
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return Entry{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "transactions.entry.updated", ActorUserID: principal.UserID, AuthMethod: principal.AuthMethod, WorkspaceID: workspace, TargetType: "transactions_entry", TargetKey: int64(entry.ID), Action: "transactions.entry.update", Outcome: "success", Metadata: string(encoded), CreatedAt: time.Now().UTC()}); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}
