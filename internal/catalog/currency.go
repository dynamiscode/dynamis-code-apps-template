package catalog

import (
	"context"
	"database/sql"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type Currency struct {
	Code     string
	Decimals int
	Default  bool
}

// ListCurrencies returns the currencies enabled for a workspace.
func (s *Service) ListCurrencies(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Currency, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	return listCurrencies(ctx, s.db, workspaceID)
}

type currencyQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listCurrencies(ctx context.Context, db currencyQuerier, workspaceID identity.WorkspaceID) ([]Currency, error) {
	rows, err := db.QueryContext(ctx, `SELECT code, decimals, is_default FROM catalog_currencies WHERE workspace_id = $1 ORDER BY is_default DESC, code`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var currencies []Currency
	for rows.Next() {
		var currency Currency
		if err := rows.Scan(&currency.Code, &currency.Decimals, &currency.Default); err != nil {
			return nil, err
		}
		currencies = append(currencies, currency)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(currencies) == 0 {
		return []Currency{{Code: "USD", Decimals: 2, Default: true}}, nil
	}
	return currencies, nil
}

func validateCurrency(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID, option PriceOption) error {
	currencies, err := listCurrencies(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	for _, currency := range currencies {
		if currency.Code != option.Currency {
			continue
		}
		if option.Amount != nil {
			_, fraction, found := strings.Cut(*option.Amount, ".")
			if found && len(fraction) > currency.Decimals {
				return ErrInvalidInput
			}
		}
		return nil
	}
	return ErrInvalidInput
}

// SetCurrencies replaces the workspace currency configuration atomically.
func (s *Service) SetCurrencies(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, currencies []Currency) error {
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
	defaults := 0
	seen := map[string]bool{}
	for i := range currencies {
		c := &currencies[i]
		c.Code = strings.TrimSpace(c.Code)
		if c.Code == "" || c.Decimals < 0 || c.Decimals > 18 || seen[c.Code] {
			return ErrInvalidInput
		}
		seen[c.Code] = true
		if c.Default {
			defaults++
		}
	}
	if defaults != 1 {
		return ErrInvalidInput
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT p.currency FROM catalog_prices p JOIN catalog_entries e ON e.id = p.entry_id WHERE e.workspace_id = $1`, workspaceID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return err
		}
		if !seen[code] {
			rows.Close()
			return ErrInUse
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_currencies WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	for _, c := range currencies {
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_currencies (workspace_id, code, decimals, is_default) VALUES ($1, $2, $3, $4)`, workspaceID, c.Code, c.Decimals, c.Default); err != nil {
			return err
		}
	}
	return tx.Commit()
}
