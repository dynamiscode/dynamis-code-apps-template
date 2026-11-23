package clients

import (
	"context"
	"database/sql"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// Archived selects which clients a list shows. The zero value means active only.
const (
	ArchivedActive = "active"
	ArchivedOnly   = "archived"
	ArchivedAll    = "all"
)

// maxBulkClients bounds one bulk request, so a single transaction stays small.
const maxBulkClients = 100

// Archive sets archived_at. Archiving an archived client changes nothing.
// Members and above may archive; viewers are refused.
func (s *Service) Archive(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
) (Client, error) {
	return s.setArchived(ctx, actor, workspaceID, clientID, audit, true)
}

// Unarchive clears archived_at. Unarchiving an active client changes nothing.
// Members and above may unarchive; viewers are refused.
func (s *Service) Unarchive(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
) (Client, error) {
	return s.setArchived(ctx, actor, workspaceID, clientID, audit, false)
}

// BulkArchive archives or unarchives every listed client in one transaction. Any failure, including a missing
// client, changes nothing. The rules are the single-client rules: members and above may do either.
func (s *Service) BulkArchive(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientIDs []ClientID,
	audit identity.AuditContext,
	archive bool,
) error {
	if len(clientIDs) > maxBulkClients {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return identity.ErrForbidden
	}
	for _, clientID := range uniqueClientIDs(clientIDs) {
		if _, err := s.setArchivedInTx(ctx, tx, actor, workspaceID, clientID, audit, archive); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) setArchived(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
	archive bool,
) (Client, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Client{}, identity.ErrForbidden
	}
	client, err := s.setArchivedInTx(ctx, tx, actor, workspaceID, clientID, audit, archive)
	if err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

// setArchivedInTx changes one client inside a transaction whose caller has already authorized the actor and
// will commit it.
func (s *Service) setArchivedInTx(
	ctx context.Context,
	tx *sql.Tx,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
	archive bool,
) (Client, error) {
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return Client{}, err
	}
	if (client.ArchivedAt != nil) == archive {
		return client, nil
	}
	now := s.now().UTC()
	var archivedAt any
	action, eventType, auditAction := "unarchived", "client.unarchived", "client.unarchive"
	// Unarchive is an update to the client; archive is its own webhook event.
	webhookType := "client.updated"
	client.ArchivedAt = nil
	if archive {
		archivedAt = formatTime(now)
		action, eventType, auditAction = "archived", "client.archived", "client.archive"
		webhookType = "client.archived"
		client.ArchivedAt = &now
	}
	client.Version++
	client.UpdatedAt = now
	if _, err := s.exec(ctx, tx, `
		UPDATE clients SET archived_at = ?, version = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?
	`, archivedAt, client.Version, formatTime(now), workspaceID, clientID); err != nil {
		return Client{}, err
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, action, client.Version, now); err != nil {
		return Client{}, err
	}
	if err := s.publishEvent(ctx, tx, workspaceID, webhookType, client, now); err != nil {
		return Client{}, err
	}
	if err := s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{
		eventType: eventType, action: auditAction, targetType: "client",
		targetKey: int64(client.ID), metadata: "{}", at: now,
	}); err != nil {
		return Client{}, err
	}
	return client, nil
}

// Delete permanently removes a client and its custom values, which live in the
// same row. Owners and admins only.
func (s *Service) Delete(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return identity.ErrForbidden
	}
	if err := s.deleteInTx(ctx, tx, actor, workspaceID, clientID, audit); err != nil {
		return err
	}
	return tx.Commit()
}

// BulkDelete permanently removes every listed client in one transaction. Any failure, including a missing client,
// removes nothing. Owners and admins only, as for a single client.
func (s *Service) BulkDelete(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientIDs []ClientID,
	audit identity.AuditContext,
) error {
	if len(clientIDs) > maxBulkClients {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return identity.ErrForbidden
	}
	for _, clientID := range uniqueClientIDs(clientIDs) {
		if err := s.deleteInTx(ctx, tx, actor, workspaceID, clientID, audit); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteInTx removes one client inside a transaction whose caller has already authorized the actor and will commit.
func (s *Service) deleteInTx(
	ctx context.Context,
	tx *sql.Tx,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	audit identity.AuditContext,
) error {
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return err
	}
	var count int
	if err := s.queryRow(ctx, tx, `SELECT COUNT(*) FROM engagements WHERE workspace_id = ? AND client_id = ?`, workspaceID, clientID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrHasEngagements
	}
	now := s.now().UTC()
	// The payload reads the client's values, so it is written before the row goes.
	if err := s.publishEvent(ctx, tx, workspaceID, "client.deleted", client, now); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM clients WHERE workspace_id = ? AND id = ?
	`, workspaceID, clientID); err != nil {
		return err
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, "deleted", client.Version, now); err != nil {
		return err
	}
	return s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{
		eventType: "client.deleted", action: "client.delete", targetType: "client",
		targetKey: int64(client.ID), metadata: "{}", at: now,
	})
}

// uniqueClientIDs keeps the first occurrence of each ID, so a repeated ID in a bulk request is not an error.
func uniqueClientIDs(ids []ClientID) []ClientID {
	seen := make(map[ClientID]bool, len(ids))
	unique := make([]ClientID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	return unique
}
