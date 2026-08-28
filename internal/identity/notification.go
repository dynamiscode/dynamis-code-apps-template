package identity

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	maxNotificationTitle = 160
	maxNotificationBody  = 2000
	maxNotifications     = 100
)

func (s *Service) CreateNotification(
	ctx context.Context,
	actor Principal,
	input NotificationInput,
	audit AuditContext,
) (Notification, error) {
	if actor.AuthMethod == "" || input.RecipientUserID == "" ||
		strings.TrimSpace(input.NotificationType) == "" ||
		len(input.NotificationType) > 64 || strings.TrimSpace(input.Title) == "" ||
		len(input.Title) > maxNotificationTitle || len(input.Body) > maxNotificationBody {
		return Notification{}, ErrInvalidProfile
	}
	input.NotificationType = strings.TrimSpace(input.NotificationType)
	input.Title = strings.TrimSpace(input.Title)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Notification{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := s.queryRow(ctx, tx, "SELECT 1 FROM users WHERE id = ?", input.RecipientUserID).Scan(&exists); err != nil {
		return Notification{}, ErrForbidden
	}
	if input.WorkspaceID != "" {
		if err := s.queryRow(ctx, tx, `
			SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?
		`, input.WorkspaceID, input.RecipientUserID).Scan(&exists); err != nil {
			return Notification{}, ErrForbidden
		}
		if actor.AuthMethod != "system" && actor.UserID != input.RecipientUserID {
			if _, err := s.authorize(ctx, tx, actor.UserID, input.WorkspaceID, MembersRead); err != nil {
				return Notification{}, ErrForbidden
			}
		}
	} else if actor.AuthMethod != "system" && actor.UserID != input.RecipientUserID {
		return Notification{}, ErrForbidden
	}
	enabled, err := s.notificationEnabled(ctx, tx, input.RecipientUserID, input.WorkspaceID, input.NotificationType)
	if err != nil {
		return Notification{}, err
	}
	if !enabled {
		return Notification{}, nil
	}
	notificationID, err := id.New()
	if err != nil {
		return Notification{}, err
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		INSERT INTO notifications (id, user_id, workspace_id, notification_type, title, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, notificationID, input.RecipientUserID, nullable(input.WorkspaceID), input.NotificationType,
		input.Title, input.Body, timestamp(now)); err != nil {
		return Notification{}, err
	}
	if actor.AuthMethod != "system" {
		if err := s.audit(ctx, tx, AuditEvent{
			EventType: "notification.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
			WorkspaceID: input.WorkspaceID, TargetType: "notification", TargetID: notificationID,
			Action: "notification.create", Outcome: "success", RequestID: audit.RequestID,
			SourceAddress: audit.SourceAddress,
			Metadata:      metadata(map[string]any{"type": input.NotificationType}), CreatedAt: now,
		}); err != nil {
			return Notification{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Notification{}, err
	}
	return Notification{
		ID: notificationID, UserID: input.RecipientUserID, WorkspaceID: input.WorkspaceID,
		NotificationType: input.NotificationType, Title: input.Title, Body: input.Body, CreatedAt: now,
	}, nil
}

func (s *Service) notificationEnabled(
	ctx context.Context,
	queryer rowQueryer,
	userID, workspaceID, notificationType string,
) (bool, error) {
	var enabled bool
	err := s.queryRow(ctx, queryer, `
		SELECT enabled FROM user_notification_preferences
		WHERE user_id = ? AND notification_type = ?
	`, userID, notificationType).Scan(&enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && !enabled {
		return false, nil
	}
	if workspaceID == "" {
		return true, nil
	}
	err = s.queryRow(ctx, queryer, `
		SELECT enabled FROM workspace_notification_preferences
		WHERE workspace_id = ? AND user_id = ? AND notification_type = ?
	`, workspaceID, userID, notificationType).Scan(&enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return err != nil || enabled, nil
}

func (s *Service) SetNotificationPreference(
	ctx context.Context,
	userID, notificationType string,
	enabled bool,
	audit AuditContext,
) error {
	if err := validateNotificationType(notificationType); err != nil {
		return err
	}
	return s.setNotificationPreference(ctx, userID, "", notificationType, enabled, audit)
}

func (s *Service) SetWorkspaceNotificationPreference(
	ctx context.Context,
	actor Principal,
	notificationType string,
	enabled bool,
	audit AuditContext,
) error {
	if err := validateNotificationType(notificationType); err != nil {
		return err
	}
	if actor.UserID == "" || actor.WorkspaceID == "" {
		return ErrForbidden
	}
	if _, err := s.Authorize(ctx, actor.UserID, actor.WorkspaceID, WorkspaceRead); err != nil {
		return ErrForbidden
	}
	return s.setNotificationPreference(ctx, actor.UserID, actor.WorkspaceID, notificationType, enabled, audit)
}

func (s *Service) setNotificationPreference(
	ctx context.Context,
	userID, workspaceID, notificationType string,
	enabled bool,
	audit AuditContext,
) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := s.queryRow(ctx, tx, "SELECT 1 FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	var query string
	args := []any{userID, notificationType, enabled, timestamp(now)}
	if workspaceID == "" {
		query = `INSERT INTO user_notification_preferences (user_id, notification_type, enabled, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT (user_id, notification_type)
			DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`
	} else {
		if err := s.queryRow(ctx, tx, `
			SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?
		`, workspaceID, userID).Scan(&exists); err != nil {
			return ErrForbidden
		}
		query = `INSERT INTO workspace_notification_preferences
			(workspace_id, user_id, notification_type, enabled, updated_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT (workspace_id, user_id, notification_type)
			DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`
		args = []any{workspaceID, userID, notificationType, enabled, timestamp(now)}
	}
	if _, err := s.exec(ctx, tx, query, args...); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "notification.preference.updated", ActorUserID: userID, AuthMethod: "session",
		WorkspaceID: workspaceID, TargetType: "notification_preference", TargetID: notificationType,
		Action: "notification.preference.update", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"type": notificationType, "enabled": enabled}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func validateNotificationType(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 64 {
		return ErrInvalidProfile
	}
	return nil
}

func (s *Service) GetNotificationPreferences(
	ctx context.Context,
	userID, workspaceID string,
) ([]NotificationPreference, error) {
	if _, err := s.GetUserProfile(ctx, userID); err != nil {
		return nil, err
	}
	if workspaceID != "" {
		if _, err := s.Authorize(ctx, userID, workspaceID, WorkspaceRead); err != nil {
			return nil, ErrForbidden
		}
	}
	preferences := make([]NotificationPreference, 0)
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT notification_type, enabled FROM user_notification_preferences
		WHERE user_id = ? ORDER BY notification_type
	`), userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var preference NotificationPreference
		if err := rows.Scan(&preference.NotificationType, &preference.Enabled); err != nil {
			rows.Close()
			return nil, err
		}
		preference.Scope = "user"
		preferences = append(preferences, preference)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if workspaceID == "" {
		return preferences, nil
	}
	rows, err = s.db.QueryContext(ctx, s.bind(`
		SELECT notification_type, enabled FROM workspace_notification_preferences
		WHERE workspace_id = ? AND user_id = ? ORDER BY notification_type
	`), workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var preference NotificationPreference
		if err := rows.Scan(&preference.NotificationType, &preference.Enabled); err != nil {
			return nil, err
		}
		preference.Scope = "workspace"
		preferences = append(preferences, preference)
	}
	return preferences, rows.Err()
}

func (s *Service) ListNotifications(
	ctx context.Context,
	userID, workspaceID string,
	includeRead bool,
	limit int,
) ([]Notification, error) {
	if limit <= 0 || limit > maxNotifications {
		limit = 50
	}
	if workspaceID != "" {
		if _, err := s.Authorize(ctx, userID, workspaceID, WorkspaceRead); err != nil {
			return nil, ErrForbidden
		}
	}
	query := `SELECT id, user_id, COALESCE(workspace_id, ''), notification_type,
		title, body, created_at, read_at FROM notifications WHERE user_id = ?`
	args := []any{userID}
	if workspaceID != "" {
		query += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	if !includeRead {
		query += " AND read_at IS NULL"
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	return s.scanNotifications(ctx, query, args...)
}

func (s *Service) NotificationsAfter(
	ctx context.Context,
	userID, afterID string,
	limit int,
) ([]Notification, error) {
	if limit <= 0 || limit > maxNotifications {
		limit = maxNotifications
	}
	if afterID == "" {
		return s.scanNotifications(ctx, `
			SELECT id, user_id, COALESCE(workspace_id, ''), notification_type,
				title, body, created_at, read_at FROM notifications
			WHERE user_id = ? ORDER BY created_at, id LIMIT ?
		`, userID, limit)
	}
	var createdAt string
	if err := s.queryRow(ctx, s.db,
		"SELECT created_at FROM notifications WHERE id = ? AND user_id = ?", afterID, userID,
	).Scan(&createdAt); err != nil {
		return nil, ErrInvalidNotification
	}
	return s.scanNotifications(ctx, `
		SELECT id, user_id, COALESCE(workspace_id, ''), notification_type,
			title, body, created_at, read_at FROM notifications
		WHERE user_id = ? AND (created_at > ? OR (created_at = ? AND id > ?))
		ORDER BY created_at, id LIMIT ?
	`, userID, createdAt, createdAt, afterID, limit)
}

func (s *Service) scanNotifications(ctx context.Context, query string, args ...any) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Notification, 0)
	for rows.Next() {
		var notification Notification
		var createdAt string
		var readAt sql.NullString
		if err := rows.Scan(&notification.ID, &notification.UserID, &notification.WorkspaceID,
			&notification.NotificationType, &notification.Title, &notification.Body,
			&createdAt, &readAt); err != nil {
			return nil, err
		}
		notification.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		if readAt.Valid {
			value, err := parseTimestamp(readAt.String)
			if err != nil {
				return nil, err
			}
			notification.ReadAt = &value
		}
		result = append(result, notification)
	}
	return result, rows.Err()
}

func (s *Service) MarkNotificationRead(
	ctx context.Context,
	userID, notificationID string,
	audit AuditContext,
) error {
	now := s.now().UTC()
	result, err := s.exec(ctx, s.db, `
		UPDATE notifications SET read_at = ?
		WHERE id = ? AND user_id = ? AND read_at IS NULL
	`, timestamp(now), notificationID, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var exists int
		if err := s.queryRow(ctx, s.db,
			"SELECT 1 FROM notifications WHERE id = ? AND user_id = ?", notificationID, userID,
		).Scan(&exists); err != nil {
			return ErrForbidden
		}
		return nil
	}
	return s.RecordAudit(ctx, AuditEvent{
		EventType: "notification.read", ActorUserID: userID, AuthMethod: "session",
		TargetType: "notification", TargetID: notificationID, Action: "notification.read",
		Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: "{}", CreatedAt: now,
	})
}

func (s *Service) UnreadNotificationCount(ctx context.Context, userID string) (int, error) {
	var count int
	if err := s.queryRow(ctx, s.db,
		"SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL", userID,
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
