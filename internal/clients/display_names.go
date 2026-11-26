package clients

import (
	"context"
	"database/sql"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// DisplayNamesByID returns names of existing workspace clients, including archived clients.
// Missing and foreign clients are omitted. A request can contain at most 100 IDs.
func (s *Service) DisplayNamesByID(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, ids []ClientID) (map[ClientID]string, error) {
	if len(ids) > 100 {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	names := make(map[ClientID]string, len(ids))
	if len(ids) == 0 {
		return names, tx.Commit()
	}
	args := make([]any, 1, len(ids)+1)
	args[0] = workspaceID
	placeholders := make([]string, len(ids))
	for i, clientID := range ids {
		if clientID <= 0 {
			return nil, ErrInvalidInput
		}
		args = append(args, clientID)
		placeholders[i] = "?"
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver,
		"SELECT id, display_name FROM clients WHERE workspace_id = ? AND id IN ("+strings.Join(placeholders, ",")+")"), args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id ClientID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		names[id] = name
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
	return names, nil
}
