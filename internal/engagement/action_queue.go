package engagement

import (
	"context"
	"database/sql"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type ActionQueueFilter string

const (
	ActionQueueUrgent    ActionQueueFilter = ""
	ActionQueueOverdue   ActionQueueFilter = "overdue"
	ActionQueueToday     ActionQueueFilter = "today"
	ActionQueueUpcoming  ActionQueueFilter = "upcoming"
	ActionQueueUndated   ActionQueueFilter = "undated"
	ActionQueueCompleted ActionQueueFilter = "completed"
)

type ActionQueueOptions struct {
	Filter ActionQueueFilter
	// OwnerID nil includes all owners; a pointer to zero selects unassigned actions.
	OwnerID     *identity.UserID
	Page, Limit int
}

type ActionQueueEntry struct {
	ClientID       clients.ClientID
	ClientName     string
	EngagementID   ID
	EngagementName string
	StageID        StageID
	Action         NextAction
	OwnerEmail     string
	HistoryID      int64
	CompletedAt    string
}

type ActionQueuePage struct {
	Entries []ActionQueueEntry
	HasMore bool
}

// ListActionQueue returns a bounded workspace-scoped page. Dates are evaluated in UTC once per call.
func (s *Service) ListActionQueue(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, options ActionQueueOptions) (ActionQueuePage, error) {
	if options.Page < 1 || options.Page > 1000000 || options.Limit < 1 || options.Limit > 100 || (options.OwnerID != nil && *options.OwnerID < 0) {
		return ActionQueuePage{}, ErrInvalidInput
	}
	today := time.Now().UTC().Format("2006-01-02")
	condition := ""
	args := []any{workspaceID}
	switch options.Filter {
	case ActionQueueUrgent:
		condition = "s.kind = 'open' AND a.due_date <> '' AND a.due_date <= ?"
		args = append(args, today)
	case ActionQueueOverdue:
		condition = "s.kind = 'open' AND a.due_date <> '' AND a.due_date < ?"
		args = append(args, today)
	case ActionQueueToday:
		condition = "s.kind = 'open' AND a.due_date = ?"
		args = append(args, today)
	case ActionQueueUpcoming:
		condition = "a.due_date > ?"
		args = append(args, today)
	case ActionQueueUndated:
		condition = "a.due_date = ''"
	case ActionQueueCompleted:
		condition = "a.outcome = 'completed'"
	default:
		return ActionQueuePage{}, ErrInvalidInput
	}
	if options.OwnerID != nil {
		condition += " AND COALESCE(a.owner_id, 0) = ?"
		args = append(args, *options.OwnerID)
	}
	table, extra, order := "engagement_next_actions", "a.version, 0, ''", "a.due_date, e.id"
	if options.Filter == ActionQueueCompleted {
		table, extra, order = "engagement_action_history", "e.action_version, a.id, a.created_at", "a.created_at DESC, a.id DESC"
	}
	args = append(args, options.Limit+1, (options.Page-1)*options.Limit)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return ActionQueuePage{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return ActionQueuePage{}, identity.ErrForbidden
	}
	query := `SELECT c.id, c.display_name, e.id, e.name, e.stage_id, a.action_text, a.due_date, COALESCE(a.owner_id, 0), COALESCE(u.email, ''), ` + extra + ` FROM ` + table + ` a JOIN engagements e ON e.workspace_id = a.workspace_id AND e.id = a.engagement_id JOIN clients c ON c.workspace_id = e.workspace_id AND c.id = e.client_id JOIN engagement_stages s ON s.workspace_id = e.workspace_id AND s.id = e.stage_id LEFT JOIN users u ON u.id = a.owner_id WHERE a.workspace_id = ? AND ` + condition + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), args...)
	if err != nil {
		return ActionQueuePage{}, err
	}
	page := ActionQueuePage{Entries: []ActionQueueEntry{}}
	for rows.Next() {
		var entry ActionQueueEntry
		if err := rows.Scan(&entry.ClientID, &entry.ClientName, &entry.EngagementID, &entry.EngagementName, &entry.StageID, &entry.Action.Text, &entry.Action.DueDate, &entry.Action.OwnerID, &entry.OwnerEmail, &entry.Action.Version, &entry.HistoryID, &entry.CompletedAt); err != nil {
			rows.Close()
			return ActionQueuePage{}, err
		}
		page.Entries = append(page.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ActionQueuePage{}, err
	}
	if err := rows.Close(); err != nil {
		return ActionQueuePage{}, err
	}
	if len(page.Entries) > options.Limit {
		page.HasMore = true
		page.Entries = page.Entries[:options.Limit]
	}
	if err := tx.Commit(); err != nil {
		return ActionQueuePage{}, err
	}
	return page, nil
}
