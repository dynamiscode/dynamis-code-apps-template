package items

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const maximumRetainedChanges = 1000
const changeOriginCursor = "0"

type Change struct {
	ID            string    `json:"id"`
	SchemaVersion int       `json:"schemaVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	WorkspaceID   string    `json:"workspaceId"`
	ItemID        string    `json:"itemId"`
	Action        string    `json:"action"`
	ItemVersion   int64     `json:"itemVersion"`
}

type ChangePage struct {
	Changes []Change
	Next    string
	Resync  bool
}

func (s *Service) Changes(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	after string,
	limit int,
) (ChangePage, error) {
	if limit < 1 || limit > 100 {
		return ChangePage{}, ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(
		ctx, actor, workspaceID, identity.ResourcesRead,
	); err != nil {
		return ChangePage{}, identity.ErrForbidden
	}
	latest, err := s.latestChangeID(ctx, workspaceID)
	if err != nil {
		return ChangePage{}, err
	}
	if after == "" {
		if latest == "" {
			latest = changeOriginCursor
		}
		return ChangePage{Next: latest, Resync: true}, nil
	}
	var occurredAt string
	if after != changeOriginCursor {
		err = s.queryRow(ctx, s.db, `
			SELECT occurred_at FROM item_events
			WHERE workspace_id = ? AND id = ?
		`, workspaceID, after).Scan(&occurredAt)
		if errors.Is(err, sql.ErrNoRows) {
			if latest == "" {
				latest = changeOriginCursor
			}
			return ChangePage{Next: latest, Resync: true}, nil
		}
		if err != nil {
			return ChangePage{}, err
		}
	}
	query := `
		SELECT id, workspace_id, item_id, event_type, item_version, occurred_at
		FROM item_events WHERE workspace_id = ?
	`
	args := []any{workspaceID}
	if after != changeOriginCursor {
		query += " AND (occurred_at > ? OR (occurred_at = ? AND id > ?))"
		args = append(args, occurredAt, occurredAt, after)
	}
	query += " ORDER BY occurred_at, id LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, database.Rebind(s.driver, query), args...)
	if err != nil {
		return ChangePage{}, err
	}
	defer rows.Close()
	page := ChangePage{Changes: make([]Change, 0, limit), Next: after}
	for rows.Next() {
		var change Change
		var timestamp string
		if err := rows.Scan(
			&change.ID, &change.WorkspaceID, &change.ItemID, &change.Action,
			&change.ItemVersion, &timestamp,
		); err != nil {
			return ChangePage{}, err
		}
		change.SchemaVersion = 1
		change.OccurredAt, err = parseTime(timestamp)
		if err != nil {
			return ChangePage{}, err
		}
		page.Changes = append(page.Changes, change)
		page.Next = change.ID
	}
	return page, rows.Err()
}

func (s *Service) latestChangeID(ctx context.Context, workspaceID string) (string, error) {
	var latest string
	err := s.queryRow(ctx, s.db, `
		SELECT id FROM item_events WHERE workspace_id = ?
		ORDER BY occurred_at DESC, id DESC LIMIT 1
	`, workspaceID).Scan(&latest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return latest, err
}

func (s *Service) recordChange(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID string,
	itemID string,
	action string,
	version int64,
	occurredAt time.Time,
) error {
	if _, err := s.exec(ctx, tx, `
		DELETE FROM item_events WHERE workspace_id = ? AND id NOT IN (
			SELECT id FROM item_events WHERE workspace_id = ?
			ORDER BY occurred_at DESC, id DESC LIMIT ?
		)
	`, workspaceID, workspaceID, maximumRetainedChanges-1); err != nil {
		return err
	}
	eventID, err := id.New()
	if err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO item_events (
			id, workspace_id, item_id, event_type, item_version, occurred_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, eventID, workspaceID, itemID, action, version, formatTime(occurredAt)); err != nil {
		return fmt.Errorf("record item change: %w", err)
	}
	return nil
}
