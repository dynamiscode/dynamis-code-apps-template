package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// webhookPayload is the data of a client webhook event. Custom values are keyed
// by field short ID. An unset field has no key, so an absent key means unset.
type webhookPayload struct {
	ID           ClientID          `json:"id"`
	DisplayName  string            `json:"displayName"`
	Email        string            `json:"email"`
	Phone        string            `json:"phone"`
	Notes        string            `json:"notes"`
	ArchivedAt   *time.Time        `json:"archivedAt"`
	Version      int64             `json:"version"`
	CustomValues map[string]string `json:"customValues"`
}

// publishEvent writes one client event to the outbox inside tx, so the event
// commits or rolls back with its change. It is a no-op without a publisher.
func (s *Service) publishEvent(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	eventType string,
	client Client,
	occurredAt time.Time,
) error {
	if s.publisher == nil {
		return nil
	}
	values, err := s.eventValues(ctx, tx, workspaceID, client.ID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(webhookPayload{
		ID: client.ID, DisplayName: client.DisplayName, Email: client.Email,
		Phone: client.Phone, Notes: client.Notes, ArchivedAt: client.ArchivedAt,
		Version: client.Version, CustomValues: values,
	})
	if err != nil {
		return err
	}
	return s.publisher.PublishInTx(ctx, tx, workspaceID, eventType, data, occurredAt)
}

// eventValues maps the client's stored values to field short IDs. It covers
// archived fields too, because archive keeps their values.
func (s *Service) eventValues(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
) (map[string]string, error) {
	stored, err := s.rawValues(ctx, tx, workspaceID, clientID)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(stored))
	if len(stored) == 0 {
		return values, nil
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver,
		`SELECT id, slot FROM client_fields WHERE workspace_id = ?`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fieldID FieldID
		var slot int
		if err := rows.Scan(&fieldID, &slot); err != nil {
			return nil, err
		}
		if value, ok := stored[slotKey(slot)]; ok {
			values[fieldID.String()] = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return values, nil
}
