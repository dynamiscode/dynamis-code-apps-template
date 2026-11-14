package clients

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

// ChangeID is a client change event's integer key. Only its short ID leaves the process.
type ChangeID int64

// ErrInvalidChangeID is returned for a cursor that is malformed or does not
// decode to an event key. Adapters treat it as an unknown cursor and resync.
var ErrInvalidChangeID = errors.New("invalid change id")

var changeCodec = mustChangeCodec()

// changeOrigin is the cursor before the first event: the short ID of key 0.
// Decoding rejects key 0, so the origin is matched by value, never parsed.
var changeOrigin, _ = changeCodec.Encode(0)

func mustChangeCodec() *id.Codec {
	codec, err := id.NewCodec(id.TypeClientEvent, "")
	if err != nil {
		panic("clients: client event short-id codec: " + err.Error())
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

// Change is one refresh hint for the client list. It carries the client's short
// ID and action only, never the client's fields.
type Change struct {
	ID            ChangeID             `json:"id"`
	SchemaVersion int                  `json:"schemaVersion"`
	OccurredAt    time.Time            `json:"occurredAt"`
	WorkspaceID   identity.WorkspaceID `json:"workspaceId"`
	ClientID      ClientID             `json:"clientId"`
	Action        string               `json:"action"`
	ClientVersion int64                `json:"clientVersion"`
}

type ChangePage struct {
	Changes []Change
	Next    string
	Resync  bool
}

// Changes returns the client changes after the cursor in one workspace. An empty
// or unknown cursor, or one whose event was pruned, returns Resync with the
// latest cursor so the caller refreshes the list before it replays.
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
			SELECT occurred_at FROM client_events
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
		SELECT id, workspace_id, client_id, event_type, client_version, occurred_at
		FROM client_events WHERE workspace_id = ?
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
			&change.ID, &change.WorkspaceID, &change.ClientID, &change.Action,
			&change.ClientVersion, &timestamp,
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
		SELECT id FROM client_events WHERE workspace_id = ?
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

// recordChange appends one event in the caller's transaction, so a change and its
// refresh hint commit together. Each workspace keeps at most maximumRetainedChanges.
func (s *Service) recordChange(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	action string,
	version int64,
	occurredAt time.Time,
) error {
	if _, err := s.exec(ctx, tx, `
		DELETE FROM client_events WHERE workspace_id = ? AND id NOT IN (
			SELECT id FROM client_events WHERE workspace_id = ?
			ORDER BY occurred_at DESC, id DESC LIMIT ?
		)
	`, workspaceID, workspaceID, maximumRetainedChanges-1); err != nil {
		return err
	}
	var eventID int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO client_events (
			workspace_id, client_id, event_type, client_version, occurred_at
		) VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, clientID, action, version, formatTime(occurredAt)).Scan(&eventID); err != nil {
		return fmt.Errorf("record client change: %w", err)
	}
	return nil
}
