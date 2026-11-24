package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// AuthorizeIntakeInTx validates the active link in the same transaction as the write.
func (s *Service) AuthorizeIntakeInTx(ctx context.Context, tx *sql.Tx, token string, workspace WorkspaceID) error {
	if token == "" || workspace == 0 {
		return ErrForbidden
	}
	hash := sha256.Sum256([]byte(token))
	var found WorkspaceID
	err := tx.QueryRowContext(ctx, s.bind(`SELECT workspace_id FROM public_intake_links WHERE workspace_id = ? AND token_hash = ?`), workspace, hex.EncodeToString(hash[:])).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	return err
}
