package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var (
	ErrTimelineTextRequired = fmt.Errorf("%w: timeline text is required", ErrInvalidInput)
	ErrTimelineTextTooLong  = fmt.Errorf("%w: timeline text exceeds 2000 characters", ErrInvalidInput)
	ErrTimelineType         = fmt.Errorf("%w: unknown timeline type", ErrInvalidInput)
	ErrTimelineChannel      = fmt.Errorf("%w: unknown timeline channel", ErrInvalidInput)
	ErrTimelineCorrection   = fmt.Errorf("%w: correction must reference an entry of this client", ErrInvalidInput)
)

type TimelineEntryID int64

func (v TimelineEntryID) String() string               { return id.EncodeKey(id.TypeClientTimelineEntry, int64(v)) }
func (v TimelineEntryID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseTimelineEntryID(value string) (TimelineEntryID, error) {
	key, err := id.DecodeKey(id.TypeClientTimelineEntry, value)
	return TimelineEntryID(key), err
}

type TimelineInput struct {
	Type       string
	Channel    string
	Text       string
	CorrectsID *TimelineEntryID
}

type TimelineEntry struct {
	ID         TimelineEntryID  `json:"id"`
	ClientID   ClientID         `json:"clientId"`
	AuthorID   *identity.UserID `json:"authorId"`
	Type       string           `json:"type"`
	Channel    string           `json:"channel"`
	Text       string           `json:"text"`
	CorrectsID *TimelineEntryID `json:"correctsId"`
	CreatedAt  time.Time        `json:"createdAt"`
}

func (s *Service) AddTimelineEntry(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, clientID ClientID, input TimelineInput, audit identity.AuditContext) (TimelineEntry, error) {
	switch input.Type {
	case "note", "call", "visit", "document", "update", "change", "delay":
	default:
		return TimelineEntry{}, ErrTimelineType
	}
	switch input.Channel {
	case "", "phone", "whatsapp", "email", "in_person", "other":
	default:
		return TimelineEntry{}, ErrTimelineChannel
	}
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" {
		return TimelineEntry{}, ErrTimelineTextRequired
	}
	if !utf8.ValidString(input.Text) {
		return TimelineEntry{}, ErrInvalidInput
	}
	if utf8.RuneCountInString(input.Text) > 2000 {
		return TimelineEntry{}, ErrTimelineTextTooLong
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return TimelineEntry{}, err
	}
	defer tx.Rollback()
	if _, err = s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return TimelineEntry{}, identity.ErrForbidden
	}
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return TimelineEntry{}, err
	}
	if input.CorrectsID != nil {
		var exists int
		err = s.queryRow(ctx, tx, `SELECT 1 FROM client_timeline_entries WHERE workspace_id = ? AND client_id = ? AND id = ?`, workspaceID, clientID, *input.CorrectsID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return TimelineEntry{}, ErrTimelineCorrection
		}
		if err != nil {
			return TimelineEntry{}, err
		}
	}
	now := s.now().UTC()
	entry := TimelineEntry{ClientID: clientID, AuthorID: &actor.UserID, Type: input.Type, Channel: input.Channel, Text: input.Text, CorrectsID: input.CorrectsID, CreatedAt: now}
	if err = s.queryRow(ctx, tx, `INSERT INTO client_timeline_entries (workspace_id, client_id, author_id, entry_type, channel, text, corrects_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, workspaceID, clientID, actor.UserID, input.Type, input.Channel, input.Text, input.CorrectsID, formatTime(now)).Scan(&entry.ID); err != nil {
		return TimelineEntry{}, err
	}
	if err = s.recordChange(ctx, tx, workspaceID, clientID, "timeline_added", client.Version, now); err != nil {
		return TimelineEntry{}, err
	}
	if err = s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{eventType: "client.timeline_added", action: "client.timeline.add", targetType: "client", targetKey: int64(clientID), metadata: "{}", at: now}); err != nil {
		return TimelineEntry{}, err
	}
	if err = tx.Commit(); err != nil {
		return TimelineEntry{}, err
	}
	return entry, nil
}

func (s *Service) ListTimelineEntries(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, clientID ClientID) ([]TimelineEntry, error) {
	return s.listTimelineEntries(ctx, actor, workspaceID, clientID, 0, 0)
}

// ListTimelineEntriesPage returns a bounded, 1-based page in timeline order.
func (s *Service) ListTimelineEntriesPage(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, clientID ClientID, limit, page int) ([]TimelineEntry, error) {
	if limit < 1 || limit > 100 || page < 1 || page > 1000000 {
		return nil, ErrInvalidInput
	}
	return s.listTimelineEntries(ctx, actor, workspaceID, clientID, limit, (page-1)*limit)
}

func (s *Service) listTimelineEntries(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, clientID ClientID, limit, offset int) ([]TimelineEntry, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, ErrNotFound
	}
	if _, err = s.get(ctx, tx, workspaceID, clientID); err != nil {
		return nil, err
	}
	query := `SELECT id, author_id, entry_type, channel, text, corrects_id, created_at FROM client_timeline_entries WHERE workspace_id = ? AND client_id = ? ORDER BY created_at, id`
	args := []any{workspaceID, clientID}
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]TimelineEntry, 0)
	for rows.Next() {
		var entry TimelineEntry
		var author, corrects sql.NullInt64
		var created string
		if err = rows.Scan(&entry.ID, &author, &entry.Type, &entry.Channel, &entry.Text, &corrects, &created); err != nil {
			return nil, err
		}
		entry.ClientID = clientID
		if author.Valid {
			value := identity.UserID(author.Int64)
			entry.AuthorID = &value
		}
		if corrects.Valid {
			value := TimelineEntryID(corrects.Int64)
			entry.CorrectsID = &value
		}
		entry.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}
