package identity

import (
	"context"
	"database/sql"
	"errors"

	"example.com/dynamis-code/apps-template/internal/i18n"
)

func normalizeLocale(raw string, allowAutomatic bool) (string, error) {
	if raw == "" && allowAutomatic {
		return "", nil
	}
	locale, ok := i18n.ParseLocale(raw)
	if !ok {
		return "", ErrInvalidLocale
	}
	return string(locale), nil
}

func defaultLocale(raw string) (string, error) {
	if raw == "" {
		return string(i18n.English), nil
	}
	return normalizeLocale(raw, false)
}

func (s *Service) GetUserLocale(ctx context.Context, userID string) (string, error) {
	var locale sql.NullString
	if err := s.queryRow(ctx, s.db, "SELECT locale FROM users WHERE id = ?", userID).Scan(&locale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrForbidden
		}
		return "", err
	}
	return locale.String, nil
}

func (s *Service) SetUserLocale(ctx context.Context, userID, locale string, audit AuditContext) error {
	normalized, err := normalizeLocale(locale, true)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := s.queryRow(ctx, tx, "SELECT 1 FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrForbidden
		}
		return err
	}
	if _, err := s.exec(ctx, tx, "UPDATE users SET locale = ? WHERE id = ?", nullable(normalized), userID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.locale.updated", ActorUserID: userID, AuthMethod: "session",
		TargetType: "user", TargetID: userID, Action: "user.locale.update", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"locale": normalized}), CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) GetWorkspaceLocale(ctx context.Context, workspaceID string) (string, error) {
	var locale string
	if err := s.queryRow(ctx, s.db, "SELECT locale FROM workspaces WHERE id = ?", workspaceID).Scan(&locale); err != nil {
		return "", err
	}
	return locale, nil
}

func (s *Service) UpdateWorkspaceLocale(ctx context.Context, actor Principal, locale string, audit AuditContext) error {
	normalized, err := defaultLocale(locale)
	if err != nil {
		return err
	}
	if s.require(ctx, actor, WorkspaceUpdate) != nil {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, WorkspaceUpdate) != nil {
		return ErrForbidden
	}
	if _, err := s.exec(ctx, tx, "UPDATE workspaces SET locale = ? WHERE id = ?", normalized, actor.WorkspaceID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.locale.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: actor.WorkspaceID, TargetType: "workspace", TargetID: actor.WorkspaceID,
		Action: "workspace.locale.update", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"locale": normalized}), CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}
