package listprefs

import (
	"context"
	"database/sql"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// GetOrder returns the member's saved column order for one list: the keys of the known columns in the saved
// sequence. Columns without a saved position are not returned, so the caller keeps their declared order.
func (s *Service) GetOrder(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	defaults map[string]bool,
) ([]string, error) {
	if err := validInput(listKey, "", defaults); err != nil {
		return nil, err
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT column_key FROM list_column_orders
		WHERE workspace_id = ? AND user_id = ? AND list_key = ?
		ORDER BY sort_position, column_key
	`), workspaceID, actor.UserID, listKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var order []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		if _, known := defaults[key]; known {
			order = append(order, key)
		}
	}
	return order, rows.Err()
}

// SetOrder replaces the member's saved column order for one list. Every key must be a known column, once.
func (s *Service) SetOrder(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	order []string,
	defaults map[string]bool,
) error {
	if err := validInput(listKey, "", defaults); err != nil {
		return err
	}
	return s.apply(ctx, actor, workspaceID, listKey, nil, order, true, defaults)
}

// writeOrderInTx replaces the member's saved column order for one list. Every key must be a known column, once.
func writeOrderInTx(ctx context.Context, tx *sql.Tx, driver config.DatabaseDriver, workspaceID identity.WorkspaceID, userID identity.UserID, listKey string, order []string, defaults map[string]bool) error {
	seen := make(map[string]bool, len(order))
	for _, key := range order {
		if _, known := defaults[key]; !known || seen[key] {
			return ErrInvalidInput
		}
		seen[key] = true
	}
	if err := resetOrderInTx(ctx, tx, driver, workspaceID, userID, listKey); err != nil {
		return err
	}
	for position, key := range order {
		if _, err := tx.ExecContext(ctx, database.Rebind(driver, `
			INSERT INTO list_column_orders (workspace_id, user_id, list_key, column_key, sort_position)
			VALUES (?, ?, ?, ?, ?)
		`), workspaceID, userID, listKey, key, position); err != nil {
			return err
		}
	}
	return nil
}

// resetOrderInTx removes the member's saved column order for one list. Reset calls it with its own transaction.
func resetOrderInTx(ctx context.Context, tx *sql.Tx, driver config.DatabaseDriver, workspaceID identity.WorkspaceID, userID identity.UserID, listKey string) error {
	_, err := tx.ExecContext(ctx, database.Rebind(driver, `
		DELETE FROM list_column_orders WHERE workspace_id = ? AND user_id = ? AND list_key = ?
	`), workspaceID, userID, listKey)
	return err
}
