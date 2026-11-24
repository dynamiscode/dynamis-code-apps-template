package portability

import (
	"context"
	"database/sql"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

type EngagementStage struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Position   int64      `json:"position"`
	ArchivedAt *time.Time `json:"archivedAt"`
}

type EngagementRecord struct {
	ID                string           `json:"id"`
	ClientID          string           `json:"clientId"`
	StageID           string           `json:"stageId"`
	Name              *string          `json:"name"`
	Notes             string           `json:"notes"`
	QuoteStatus       string           `json:"quoteStatus"`
	QuoteAmount       string           `json:"quoteAmount"`
	QuoteDateSent     string           `json:"quoteDateSent"`
	AssigneeID        *identity.UserID `json:"assigneeId"`
	AgreementRecorded bool             `json:"agreementRecorded"`
	AgreementFileID   *string          `json:"agreementFileId"`
}

func (s *Service) engagementStages(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID) ([]EngagementStage, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`SELECT id, name, kind, position, archived_at FROM engagement_stages WHERE workspace_id = ? ORDER BY position, id LIMIT ?`), workspaceID, s.maxRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []EngagementStage{}
	for rows.Next() {
		var stage EngagementStage
		var key int64
		var archived sql.NullString
		if err := rows.Scan(&key, &stage.Name, &stage.Kind, &stage.Position, &archived); err != nil {
			return nil, err
		}
		stage.ID = id.EncodeKey(id.TypeEngagementStage, key)
		if archived.Valid {
			value, err := parseTime(archived.String)
			if err != nil {
				return nil, err
			}
			stage.ArchivedAt = &value
		}
		result = append(result, stage)
	}
	return result, rows.Err()
}

func (s *Service) engagements(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID) ([]EngagementRecord, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`SELECT id, client_id, stage_id, name, notes, quote_status, quote_amount, quote_date_sent, assignee_id, agreement_recorded, agreement_file_id FROM engagements WHERE workspace_id = ? ORDER BY id LIMIT ?`), workspaceID, s.maxRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []EngagementRecord{}
	for rows.Next() {
		var value EngagementRecord
		var key, client, stage int64
		var name sql.NullString
		var assignee, file sql.NullInt64
		if err := rows.Scan(&key, &client, &stage, &name, &value.Notes, &value.QuoteStatus, &value.QuoteAmount, &value.QuoteDateSent, &assignee, &value.AgreementRecorded, &file); err != nil {
			return nil, err
		}
		value.ID = id.EncodeKey(id.TypeEngagement, key)
		value.ClientID = id.EncodeKey(id.TypeClient, client)
		value.StageID = id.EncodeKey(id.TypeEngagementStage, stage)
		if name.Valid {
			value.Name = &name.String
		}
		if assignee.Valid {
			user := identity.UserID(assignee.Int64)
			value.AssigneeID = &user
		}
		if file.Valid {
			fileID := id.EncodeKey(id.TypeFile, file.Int64)
			value.AgreementFileID = &fileID
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
