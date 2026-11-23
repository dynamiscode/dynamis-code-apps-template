// Package catalog owns the workspace catalog.
package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid catalog entry")
var decimalAmount = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

type PriceID int64

func (v PriceID) String() string               { return id.EncodeKey(id.TypeCatalogPrice, int64(v)) }
func (v PriceID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParsePriceID(s string) PriceID            { v, _ := id.DecodeKey(id.TypeCatalogPrice, s); return PriceID(v) }

type PriceOption struct {
	ID          PriceID `json:"id"`
	Mode        string  `json:"mode"`
	Amount      *string `json:"amount"`
	Currency    string  `json:"currency"`
	Variant     string  `json:"variant"`
	Eligibility string  `json:"eligibility"`
}

var ErrNotFound = errors.New("catalog entry not found")
var ErrInUse = errors.New("catalog entry is in use; archive it instead")
var ErrLastPrice = errors.New("catalog entry must have at least one price")

type EntryID int64

func (v EntryID) String() string               { return id.EncodeKey(id.TypeCatalogEntry, int64(v)) }
func (v EntryID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseEntryID(s string) EntryID            { v, _ := id.DecodeKey(id.TypeCatalogEntry, s); return EntryID(v) }

type Entry struct {
	ID              EntryID       `json:"id"`
	Name            string        `json:"name"`
	DurationMinutes *int          `json:"durationMinutes"`
	Archived        bool          `json:"archived"`
	Prices          []PriceOption `json:"prices"`
}

type Service struct {
	db    *sql.DB
	auth  *identity.Service
	inUse func(context.Context, *sql.Tx, identity.WorkspaceID, EntryID) (bool, error)
}

// A nil inUse checker is valid only while no sale integration exists.
func NewService(db *sql.DB, auth *identity.Service, inUse func(context.Context, *sql.Tx, identity.WorkspaceID, EntryID) (bool, error)) *Service {
	return &Service{db: db, auth: auth, inUse: inUse}
}

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, includeArchived bool) ([]Entry, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	query := `SELECT e.id, e.name, e.duration_minutes, e.archived_at, p.id, p.mode, p.amount, p.currency, p.variant, p.eligibility FROM catalog_entries e LEFT JOIN catalog_prices p ON p.entry_id = e.id WHERE e.workspace_id = $1`
	if !includeArchived {
		query += ` AND e.archived_at IS NULL`
	}
	query += ` ORDER BY e.id, p.id`
	rows, err := s.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	var last int64
	for rows.Next() {
		var key int64
		var name string
		var duration *int
		var priceID sql.NullInt64
		var archived, mode, amount, currency, variant, eligibility sql.NullString
		if err := rows.Scan(&key, &name, &duration, &archived, &priceID, &mode, &amount, &currency, &variant, &eligibility); err != nil {
			return nil, err
		}
		if key != last {
			entries = append(entries, Entry{ID: EntryID(key), Name: name, DurationMinutes: duration, Archived: archived.Valid, Prices: []PriceOption{}})
			last = key
		}
		if mode.Valid {
			p := PriceOption{ID: PriceID(priceID.Int64), Mode: mode.String, Currency: currency.String, Variant: variant.String, Eligibility: eligibility.String}
			if amount.Valid {
				p.Amount = &amount.String
			}
			entries[len(entries)-1].Prices = append(entries[len(entries)-1].Prices, p)
		}
	}
	return entries, rows.Err()
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, name string, duration *int, prices []PriceOption) (Entry, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return Entry{}, err
	}
	if !identity.CanWriteCatalog(principal) {
		return Entry{}, identity.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" || (duration != nil && *duration <= 0) || len(prices) == 0 {
		return Entry{}, ErrInvalidInput
	}
	prices = append([]PriceOption(nil), prices...)
	for i := range prices {
		if err := validatePrice(&prices[i]); err != nil {
			return Entry{}, err
		}
	}
	var entryID EntryID
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_entries (workspace_id, name, duration_minutes) VALUES ($1, $2, $3) RETURNING id`, workspaceID, name, duration).Scan(&entryID)
	if err != nil {
		return Entry{}, err
	}
	for i := range prices {
		p := &prices[i]
		if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_prices (entry_id, mode, amount, currency, variant, eligibility) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, entryID, p.Mode, p.Amount, p.Currency, p.Variant, p.Eligibility).Scan(&p.ID); err != nil {
			return Entry{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return Entry{ID: entryID, Name: name, DurationMinutes: duration, Prices: prices}, nil
}

func validatePrice(option *PriceOption) error {
	option.Currency = strings.TrimSpace(option.Currency)
	option.Variant = strings.TrimSpace(option.Variant)
	option.Eligibility = strings.TrimSpace(option.Eligibility)
	if option.Currency == "" || option.Variant == "" || (option.Mode != "fixed" && option.Mode != "manual") || (option.Mode == "fixed" && (option.Amount == nil || !decimalAmount.MatchString(*option.Amount))) || (option.Mode == "manual" && option.Amount != nil) {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) Update(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, name string, duration *int) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		name = strings.TrimSpace(name)
		if name == "" || (duration != nil && *duration <= 0) {
			return nil, ErrInvalidInput
		}
		return tx.ExecContext(ctx, `UPDATE catalog_entries SET name = $1, duration_minutes = $2 WHERE workspace_id = $3 AND id = $4`, name, duration, workspaceID, entryID)
	})
}

func (s *Service) UpdatePrice(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, priceID PriceID, option PriceOption) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		if err := validatePrice(&option); err != nil {
			return nil, err
		}
		return tx.ExecContext(ctx, `UPDATE catalog_prices SET mode = $1, amount = $2, currency = $3, variant = $4, eligibility = $5 WHERE id = $6 AND entry_id = $7 AND EXISTS (SELECT 1 FROM catalog_entries WHERE id = $7 AND workspace_id = $8)`, option.Mode, option.Amount, option.Currency, option.Variant, option.Eligibility, priceID, entryID, workspaceID)
	})
}

func (s *Service) DeletePrice(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, priceID PriceID) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM catalog_prices WHERE entry_id = $1 AND EXISTS (SELECT 1 FROM catalog_entries WHERE id = $1 AND workspace_id = $2)`, entryID, workspaceID).Scan(&count); err != nil {
			return nil, err
		}
		if count <= 1 {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM catalog_prices WHERE id = $1 AND entry_id = $2 AND EXISTS (SELECT 1 FROM catalog_entries WHERE id = $2 AND workspace_id = $3))`, priceID, entryID, workspaceID).Scan(&exists); err != nil {
				return nil, err
			}
			if exists {
				return nil, ErrLastPrice
			}
		}
		return tx.ExecContext(ctx, `DELETE FROM catalog_prices WHERE id = $1 AND entry_id = $2 AND EXISTS (SELECT 1 FROM catalog_entries WHERE id = $2 AND workspace_id = $3)`, priceID, entryID, workspaceID)
	})
}

func (s *Service) SetArchived(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, archived bool) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		var at any
		if archived {
			at = time.Now().UTC().Format(time.RFC3339Nano)
		}
		return tx.ExecContext(ctx, `UPDATE catalog_entries SET archived_at = $1 WHERE workspace_id = $2 AND id = $3`, at, workspaceID, entryID)
	})
}
func (s *Service) Delete(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		var found EntryID
		if err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_entries WHERE workspace_id = $1 AND id = $2`, workspaceID, entryID).Scan(&found); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, err
		}
		if s.inUse != nil {
			used, err := s.inUse(ctx, tx, workspaceID, entryID)
			if err != nil {
				return nil, err
			}
			if used {
				return nil, ErrInUse
			}
		}
		return tx.ExecContext(ctx, `DELETE FROM catalog_entries WHERE workspace_id = $1 AND id = $2`, workspaceID, entryID)
	})
}
func (s *Service) mutate(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, operation func(*sql.Tx) (sql.Result, error)) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return err
	}
	if !identity.CanWriteCatalog(principal) {
		return identity.ErrForbidden
	}
	result, err := operation(tx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Service) AddPrice(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID string, option PriceOption) (PriceOption, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return PriceOption{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return PriceOption{}, err
	}
	if !identity.CanWriteCatalog(principal) {
		return PriceOption{}, identity.ErrForbidden
	}
	if err := validatePrice(&option); err != nil {
		return PriceOption{}, err
	}
	key, err := id.DecodeKey(id.TypeCatalogEntry, entryID)
	if err != nil {
		return PriceOption{}, ErrInvalidInput
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_prices (entry_id, mode, amount, currency, variant, eligibility) SELECT id, $1, $2, $3, $4, $5 FROM catalog_entries WHERE id = $6 AND workspace_id = $7 AND archived_at IS NULL RETURNING id`, option.Mode, option.Amount, option.Currency, option.Variant, option.Eligibility, key, workspaceID).Scan(&option.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return PriceOption{}, ErrInvalidInput
	}
	if err != nil {
		return PriceOption{}, err
	}
	if err := tx.Commit(); err != nil {
		return PriceOption{}, err
	}
	return option, nil
}
