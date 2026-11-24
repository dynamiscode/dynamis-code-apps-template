package engagement

import (
	"context"
	"database/sql"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type DashboardStage struct {
	Stage Stage
	Count int
}

type DashboardPage struct {
	Overdue int
	Today   int
	Stages  []DashboardStage
	HasMore bool
}

// Dashboard counts urgent actions and pages open stages, including archived stages.
func (s *Service) Dashboard(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, page, limit int) (DashboardPage, error) {
	if page < 1 || page > clients.MaxListPage || limit < 1 || limit > 100 {
		return DashboardPage{}, ErrInvalidInput
	}
	today := time.Now().UTC().Format("2006-01-02")
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return DashboardPage{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return DashboardPage{}, identity.ErrForbidden
	}
	var result DashboardPage
	query := `SELECT COALESCE(SUM(CASE WHEN a.due_date <> '' AND a.due_date < ? THEN 1 ELSE 0 END), 0), COALESCE(SUM(CASE WHEN a.due_date = ? THEN 1 ELSE 0 END), 0) FROM engagement_next_actions a JOIN engagements e ON e.workspace_id = a.workspace_id AND e.id = a.engagement_id JOIN engagement_stages s ON s.workspace_id = e.workspace_id AND s.id = e.stage_id WHERE a.workspace_id = ? AND s.kind = 'open'`
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, query), today, today, workspaceID).Scan(&result.Overdue, &result.Today); err != nil {
		return DashboardPage{}, err
	}
	query = `SELECT s.id, s.name, s.kind, s.archived_at IS NOT NULL, COUNT(e.id) FROM engagement_stages s LEFT JOIN engagements e ON e.workspace_id = s.workspace_id AND e.stage_id = s.id WHERE s.workspace_id = ? AND s.kind = 'open' GROUP BY s.id, s.name, s.kind, s.archived_at, s.position ORDER BY s.position, s.id LIMIT ? OFFSET ?`
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), workspaceID, limit+1, (page-1)*limit)
	if err != nil {
		return DashboardPage{}, err
	}
	result.Stages = make([]DashboardStage, 0, limit)
	for rows.Next() {
		var stage DashboardStage
		if err := rows.Scan(&stage.Stage.ID, &stage.Stage.Name, &stage.Stage.Kind, &stage.Stage.Archived, &stage.Count); err != nil {
			rows.Close()
			return DashboardPage{}, err
		}
		result.Stages = append(result.Stages, stage)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return DashboardPage{}, err
	}
	if err := rows.Close(); err != nil {
		return DashboardPage{}, err
	}
	if len(result.Stages) > limit {
		result.HasMore = true
		result.Stages = result.Stages[:limit]
	}
	if err := tx.Commit(); err != nil {
		return DashboardPage{}, err
	}
	return result, nil
}
