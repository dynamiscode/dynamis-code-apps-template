package items

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const maximumRetainedChanges = 1000

// ChangeID is a change event's integer key. Only its short ID leaves the process.
type ChangeID int64

// ErrInvalidChangeID is returned for a cursor that is malformed or does not
// decode to an event key. Adapters treat it as an unknown cursor and resync.
var ErrInvalidChangeID = errors.New("invalid change id")

var changeCodec = mustChangeCodec()

// changeOrigin is the cursor before the first event: the short ID of key 0.
// Decoding rejects key 0, so the origin is matched by value, never parsed.
var changeOrigin, _ = changeCodec.Encode(0)

func mustChangeCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeItemEvent, "")
	if err != nil {
		panic("items: item event short-id codec: " + err.Error())
	}
	return codec
}

// String returns the event's short ID.
func (c ChangeID) String() string {
	if c == 0 {
		return ""
	}
	// Keys are positive, so the uint64 conversion is exact and Encode cannot fail.
	short, _ := changeCodec.Encode(uint64(c))
	return short
}

// MarshalJSON emits the short ID, never the integer key.
func (c ChangeID) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func parseChangeID(short string) (int64, error) {
	value, err := changeCodec.Decode(short)
	if err != nil || value == 0 || value > math.MaxInt64 {
		return 0, ErrInvalidChangeID
	}
	return int64(value), nil
}

type Change struct {
	ID            ChangeID             `json:"id"`
	SchemaVersion int                  `json:"schemaVersion"`
	OccurredAt    time.Time            `json:"occurredAt"`
	WorkspaceID   identity.WorkspaceID `json:"workspaceId"`
	ItemID        ItemID               `json:"itemId"`
	Action        string               `json:"action"`
	ItemVersion   int64                `json:"itemVersion"`
}

type ChangePage struct {
	Changes []Change
	Next    string
	Resync  bool
}

func (s *Service) Changes(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
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
		return ChangePage{Next: latest, Resync: true}, nil
	}
	var afterKey int64
	var occurredAt string
	if after != changeOrigin {
		afterKey, err = parseChangeID(after)
		if err != nil {
			return ChangePage{}, err
		}
		err = s.queryRow(ctx, s.db, `
			SELECT occurred_at FROM item_events
			WHERE workspace_id = ? AND id = ?
		`, workspaceID, afterKey).Scan(&occurredAt)
		if errors.Is(err, sql.ErrNoRows) {
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
	if after != changeOrigin {
		query += " AND (occurred_at > ? OR (occurred_at = ? AND id > ?))"
		args = append(args, occurredAt, occurredAt, afterKey)
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
		page.Next = change.ID.String()
	}
	return page, rows.Err()
}

func (s *Service) latestChangeID(ctx context.Context, workspaceID identity.WorkspaceID) (string, error) {
	var latest int64
	err := s.queryRow(ctx, s.db, `
		SELECT id FROM item_events WHERE workspace_id = ?
		ORDER BY occurred_at DESC, id DESC LIMIT 1
	`, workspaceID).Scan(&latest)
	if errors.Is(err, sql.ErrNoRows) {
		return changeOrigin, nil
	}
	if err != nil {
		return "", err
	}
	return ChangeID(latest).String(), nil
}

func (s *Service) recordChange(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	itemID ItemID,
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
	var eventID int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO item_events (
			workspace_id, item_id, event_type, item_version, occurred_at
		) VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, itemID, action, version, formatTime(occurredAt)).Scan(&eventID); err != nil {
		return fmt.Errorf("record item change: %w", err)
	}
	return nil
}
