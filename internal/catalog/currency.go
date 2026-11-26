package catalog

import (
	"context"
	"database/sql"
	"strings"

	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func validateCurrency(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID, option PriceOption) error {
	canonical, known := currency.Canonical(option.Currency)
	if !known {
		return ErrInvalidInput
	}
	currencies, err := currency.List(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	for _, currency := range currencies {
		if currency.Code != option.Currency {
			continue
		}
		if currency.Decimals != canonical.Decimals {
			return ErrInvalidInput
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
