// Package listprefs stores explicit per-member column visibility overrides.
package listprefs

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var ErrInvalidInput = errors.New("list preference input is invalid")

const (
	ListClients = "clients"
	ListItems   = "items"
	ListFiles   = "files"
	ListMembers = "members"
)

type Service struct {
	db     *sql.DB
	driver config.DatabaseDriver
	auth   *identity.Service
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service) *Service {
	return &Service{db: db, driver: driver, auth: auth}
}

// Get merges the caller's declared defaults with their saved deviations.
func (s *Service) Get(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	defaults map[string]bool,
) (map[string]bool, error) {
	if err := validInput(listKey, "", defaults); err != nil {
		return nil, err
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	result := make(map[string]bool, len(defaults))
	for key, visible := range defaults {
		result[key] = visible
	}
	rows, err := s.db.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT column_key, visible FROM list_column_preferences
		WHERE workspace_id = ? AND user_id = ? AND list_key = ?
	`), workspaceID, actor.UserID, listKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var visible bool
		if err := rows.Scan(&key, &visible); err != nil {
			return nil, err
		}
		if _, known := defaults[key]; known {
			result[key] = visible
		}
	}
	return result, rows.Err()
}

// Set changes one known column and leaves every other override untouched.
func (s *Service) Set(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey, columnKey string,
	visible bool,
	defaults map[string]bool,
) error {
	if err := validInput(listKey, columnKey, defaults); err != nil {
		return err
	}
	return s.apply(ctx, actor, workspaceID, listKey, map[string]bool{columnKey: visible}, nil, false, defaults)
}

// ApplyView writes a saved view's column visibility and, when order is non-nil, its column order, in one
// transaction. A column the list no longer has is skipped, so one stale column does not refuse the view.
func (s *Service) ApplyView(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	visible map[string]bool,
	order []string,
	defaults map[string]bool,
) error {
	if err := validInput(listKey, "", defaults); err != nil {
		return err
	}
	return s.apply(ctx, actor, workspaceID, listKey, visible, order, order != nil, defaults)
}

// apply writes visibility overrides and, when saveOrder is set, the column order, all in one transaction.
func (s *Service) apply(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	visible map[string]bool,
	order []string,
	saveOrder bool,
	defaults map[string]bool,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return identity.ErrForbidden
	}
	for columnKey, value := range visible {
		if _, known := defaults[columnKey]; !known {
			continue
		}
		if err := writeVisibleInTx(ctx, tx, s.driver, workspaceID, actor.UserID, listKey, columnKey, value, defaults[columnKey]); err != nil {
			return err
		}
	}
	if saveOrder {
		if err := writeOrderInTx(ctx, tx, s.driver, workspaceID, actor.UserID, listKey, order, defaults); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// writeVisibleInTx stores one column's visibility, or removes the override when it matches the default.
func writeVisibleInTx(ctx context.Context, tx *sql.Tx, driver config.DatabaseDriver, workspaceID identity.WorkspaceID, userID identity.UserID, listKey, columnKey string, visible, defaultVisible bool) error {
	if visible == defaultVisible {
		_, err := tx.ExecContext(ctx, database.Rebind(driver, `DELETE FROM list_column_preferences WHERE workspace_id = ? AND user_id = ? AND list_key = ? AND column_key = ?`), workspaceID, userID, listKey, columnKey)
		return err
	}
	_, err := tx.ExecContext(ctx, database.Rebind(driver, `INSERT INTO list_column_preferences (workspace_id, user_id, list_key, column_key, visible)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT (workspace_id, user_id, list_key, column_key)
		DO UPDATE SET visible = excluded.visible`), workspaceID, userID, listKey, columnKey, visible)
	return err
}

// Reset atomically removes this user's overrides for one list in one workspace.
func (s *Service) Reset(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
) error {
	if err := validListKey(listKey); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return identity.ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
		DELETE FROM list_column_preferences WHERE workspace_id = ? AND user_id = ? AND list_key = ?
	`), workspaceID, actor.UserID, listKey); err != nil {
		return err
	}
	if err := resetOrderInTx(ctx, tx, s.driver, workspaceID, actor.UserID, listKey); err != nil {
		return err
	}
	return tx.Commit()
}

func validInput(listKey, columnKey string, defaults map[string]bool) error {
	if validListKey(listKey) != nil || len(defaults) == 0 {
		return ErrInvalidInput
	}
	for key := range defaults {
		if strings.TrimSpace(key) == "" || len(key) > 128 {
			return ErrInvalidInput
		}
	}
	if columnKey != "" {
		if _, ok := defaults[columnKey]; !ok {
			return ErrInvalidInput
		}
	}
	return nil
}

func validListKey(listKey string) error {
	switch listKey {
	case ListClients, ListItems, ListFiles, ListMembers:
		return nil
	default:
		return ErrInvalidInput
	}
}
