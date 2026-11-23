// Package catalog owns the workspace catalog.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid catalog entry")
var decimalAmount = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

type PriceOption struct {
	Mode        string  `json:"mode"`
	Amount      *string `json:"amount"`
	Currency    string  `json:"currency"`
	Variant     string  `json:"variant"`
	Eligibility string  `json:"eligibility"`
}

type Entry struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	DurationMinutes *int          `json:"durationMinutes"`
	Prices          []PriceOption `json:"prices"`
}

type Service struct {
	db   *sql.DB
	auth *identity.Service
}

func NewService(db *sql.DB, auth *identity.Service) *Service { return &Service{db: db, auth: auth} }

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Entry, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.name, e.duration_minutes, p.mode, p.amount, p.currency, p.variant, p.eligibility FROM catalog_entries e LEFT JOIN catalog_prices p ON p.entry_id = e.id WHERE e.workspace_id = $1 AND e.archived_at IS NULL ORDER BY e.id, p.id`, workspaceID)
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
		var mode, amount, currency, variant, eligibility sql.NullString
		if err := rows.Scan(&key, &name, &duration, &mode, &amount, &currency, &variant, &eligibility); err != nil {
			return nil, err
		}
		if key != last {
			entries = append(entries, Entry{ID: id.EncodeKey(id.TypeCatalogEntry, key), Name: name, DurationMinutes: duration, Prices: []PriceOption{}})
			last = key
		}
		if mode.Valid {
			p := PriceOption{Mode: mode.String, Currency: currency.String, Variant: variant.String, Eligibility: eligibility.String}
			if amount.Valid {
				p.Amount = &amount.String
			}
			entries[len(entries)-1].Prices = append(entries[len(entries)-1].Prices, p)
		}
	}
	return entries, rows.Err()
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, name string, duration *int) (Entry, error) {
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
	if name == "" || (duration != nil && *duration <= 0) {
		return Entry{}, ErrInvalidInput
	}
	var key int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_entries (workspace_id, name, duration_minutes) VALUES ($1, $2, $3) RETURNING id`, workspaceID, name, duration).Scan(&key); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return Entry{ID: id.EncodeKey(id.TypeCatalogEntry, key), Name: name, DurationMinutes: duration, Prices: []PriceOption{}}, nil
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
	option.Currency = strings.TrimSpace(option.Currency)
	option.Variant = strings.TrimSpace(option.Variant)
	option.Eligibility = strings.TrimSpace(option.Eligibility)
	if option.Currency == "" || option.Variant == "" || (option.Mode != "fixed" && option.Mode != "manual") || (option.Mode == "fixed" && (option.Amount == nil || !decimalAmount.MatchString(*option.Amount))) || (option.Mode == "manual" && option.Amount != nil) {
		return PriceOption{}, ErrInvalidInput
	}
	key, err := id.DecodeKey(id.TypeCatalogEntry, entryID)
	if err != nil {
		return PriceOption{}, ErrInvalidInput
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO catalog_prices (entry_id, mode, amount, currency, variant, eligibility) SELECT id, $1, $2, $3, $4, $5 FROM catalog_entries WHERE id = $6 AND workspace_id = $7 AND archived_at IS NULL`, option.Mode, option.Amount, option.Currency, option.Variant, option.Eligibility, key, workspaceID)
	if err != nil {
		return PriceOption{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return PriceOption{}, err
	}
	if n != 1 {
		return PriceOption{}, ErrInvalidInput
	}
	if err := tx.Commit(); err != nil {
		return PriceOption{}, err
	}
	return option, nil
}
