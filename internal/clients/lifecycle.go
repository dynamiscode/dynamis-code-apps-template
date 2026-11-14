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
	client.ArchivedAt = nil
	if archive {
		archivedAt = formatTime(now)
		action, eventType, auditAction = "archived", "client.archived", "client.archive"
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
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: eventType, ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "client", TargetKey: int64(client.ID), Action: auditAction,
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
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
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		DELETE FROM clients WHERE workspace_id = ? AND id = ?
	`, workspaceID, clientID); err != nil {
		return err
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, "deleted", client.Version, now); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "client.deleted", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "client", TargetKey: int64(client.ID), Action: "client.delete",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
