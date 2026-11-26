package engagement

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// ListInput selects a 1-based page. A nil assignee includes all assignees;
// a pointer to zero selects unassigned engagements.
type ListInput struct {
	Search     string
	StageID    StageID
	AssigneeID *identity.UserID
	Page       int
	Limit      int
	Sort       string
	Direction  string
}

type ListItem struct {
	ID         ID
	Name       string
	ClientID   clients.ClientID
	ClientName string
	StageID    StageID
	AssigneeID identity.UserID
}

type Page struct {
	Engagements []ListItem
	Total       int
	Page        int
}

// List returns engagements in descending ID order by default, including archived stages.
// Search matches literal substrings in engagement and client names, folding ASCII letters only.
func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, input ListInput) (Page, error) {
	if input.Page < 1 || input.Page > clients.MaxListPage || input.Limit < 1 || input.Limit > 100 || input.StageID < 0 || (input.AssigneeID != nil && *input.AssigneeID < 0) || !utf8.ValidString(input.Search) || utf8.RuneCountInString(input.Search) > 200 {
		return Page{}, ErrInvalidInput
	}
	sortColumns := map[string]string{
		"name": "e.name", "client": "c.display_name", "stage": "s.name", "assignee": "COALESCE(u.email, '')",
	}
	order := "e.id DESC"
	if input.Sort != "" {
		column, ok := sortColumns[input.Sort]
		if !ok || (input.Direction != "asc" && input.Direction != "desc") {
			return Page{}, ErrInvalidInput
		}
		direction := strings.ToUpper(input.Direction)
		order = column + " " + direction + ", e.id " + direction
	} else if input.Direction != "" {
		return Page{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return Page{}, identity.ErrForbidden
	}
	where := ` FROM engagements e JOIN clients c ON c.workspace_id = e.workspace_id AND c.id = e.client_id JOIN engagement_stages s ON s.workspace_id = e.workspace_id AND s.id = e.stage_id LEFT JOIN users u ON u.id = e.assignee_id WHERE e.workspace_id = ?`
	args := []any{workspaceID}
	if search := strings.TrimSpace(input.Search); search != "" {
		needle := database.ASCIILowerExpr(s.driver, "?")
		where += ` AND (` + database.ContainsExpr(s.driver, database.ASCIILowerExpr(s.driver, "e.name"), needle) + ` OR ` + database.ContainsExpr(s.driver, database.ASCIILowerExpr(s.driver, "c.display_name"), needle) + `)`
		args = append(args, search, search)
	}
	if input.StageID != 0 {
		where += ` AND e.stage_id = ?`
		args = append(args, input.StageID)
	}
	if input.AssigneeID != nil {
		if *input.AssigneeID == 0 {
			where += ` AND e.assignee_id IS NULL`
		} else {
			where += ` AND e.assignee_id = ?`
			args = append(args, *input.AssigneeID)
		}
	}
	var result Page
	result.Page = input.Page
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT COUNT(*)`+where), args...).Scan(&result.Total); err != nil {
		return Page{}, err
	}
	query := `SELECT e.id, e.name, e.client_id, c.display_name, e.stage_id, COALESCE(e.assignee_id, 0)` + where + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), append(args, input.Limit, (input.Page-1)*input.Limit)...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	result.Engagements = make([]ListItem, 0, input.Limit)
	for rows.Next() {
		var item ListItem
		if err := rows.Scan(&item.ID, &item.Name, &item.ClientID, &item.ClientName, &item.StageID, &item.AssigneeID); err != nil {
			return Page{}, err
		}
		result.Engagements = append(result.Engagements, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return result, nil
}
