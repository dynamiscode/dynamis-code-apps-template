package clients

import (
	"context"
	"database/sql"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// CountArchived returns how many clients in the workspace are archived. The archived footer link shows it, so a
// member can see that archived clients exist before opening them.
func (s *Service) CountArchived(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) (int, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return 0, identity.ErrForbidden
	}
	var count int
	if err := s.queryRow(ctx, tx,
		`SELECT COUNT(*) FROM clients WHERE workspace_id = ? AND archived_at IS NOT NULL`, workspaceID,
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}
