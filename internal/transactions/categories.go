package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrDuplicateCategory = fmt.Errorf("%w: expense category already exists", ErrInvalidInput)

type CategoryID int64

func (v CategoryID) String() string               { return id.EncodeKey(id.TypeTransactionsCategory, int64(v)) }
func (v CategoryID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v *CategoryID) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseCategoryID(value)
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}
func ParseCategoryID(value string) (CategoryID, error) {
	key, err := id.DecodeKey(id.TypeTransactionsCategory, value)
	return CategoryID(key), err
}

type Category struct {
	ID       CategoryID `json:"id"`
	Name     string     `json:"name"`
	Archived bool       `json:"archived"`
}

func (s *Service) Categories(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]Category, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, archived FROM transactions_expense_categories WHERE workspace_id = $1 ORDER BY name, id`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := []Category{}
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name, &category.Archived); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (s *Service) ChangeCategory(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, categoryID CategoryID, name string, archive bool) (Category, error) {
	return retrySerialization(ctx, func() (Category, error) {
		return s.changeCategory(ctx, actor, workspace, categoryID, name, archive)
	})
}

func (s *Service) changeCategory(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, categoryID CategoryID, name string, archive bool) (Category, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Category{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite)
	if err != nil {
		return Category{}, err
	}
	if !identity.CanWriteCatalog(principal) {
		return Category{}, identity.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if !archive && (name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64) {
		return Category{}, ErrInvalidInput
	}
	// Serialize category changes in this workspace, including concurrent archives.
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET name = name WHERE id = $1`, workspace); err != nil {
		return Category{}, err
	}
	if !archive {
		var duplicate bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM transactions_expense_categories WHERE workspace_id = $1 AND name = $2 AND archived = FALSE AND id <> $3)`, workspace, name, categoryID).Scan(&duplicate); err != nil {
			return Category{}, err
		}
		if duplicate {
			return Category{}, ErrDuplicateCategory
		}
	}
	var category Category
	var action string
	var oldName string
	switch {
	case categoryID == 0 && !archive:
		action = "create"
		err = tx.QueryRowContext(ctx, `INSERT INTO transactions_expense_categories (workspace_id, name) VALUES ($1,$2) RETURNING id, name, archived`, workspace, name).Scan(&category.ID, &category.Name, &category.Archived)
	case categoryID > 0 && archive:
		action = "archive"
		err = tx.QueryRowContext(ctx, `UPDATE transactions_expense_categories SET archived = TRUE WHERE workspace_id = $1 AND id = $2 AND archived = FALSE RETURNING id, name, archived`, workspace, categoryID).Scan(&category.ID, &category.Name, &category.Archived)
	case categoryID > 0:
		action = "rename"
		err = tx.QueryRowContext(ctx, `UPDATE transactions_expense_categories SET name = name WHERE workspace_id = $1 AND id = $2 AND archived = FALSE RETURNING name`, workspace, categoryID).Scan(&oldName)
		if err == sql.ErrNoRows {
			return Category{}, ErrInvalidInput
		}
		if err != nil {
			return Category{}, err
		}
		err = tx.QueryRowContext(ctx, `UPDATE transactions_expense_categories SET name = $1 WHERE workspace_id = $2 AND id = $3 AND archived = FALSE RETURNING id, name, archived`, name, workspace, categoryID).Scan(&category.ID, &category.Name, &category.Archived)
	default:
		return Category{}, ErrInvalidInput
	}
	if err == sql.ErrNoRows {
		return Category{}, ErrInvalidInput
	}
	if err != nil {
		return Category{}, err
	}
	details := struct {
		Name   string `json:"name"`
		Before string `json:"before,omitempty"`
		After  string `json:"after,omitempty"`
	}{Name: category.Name}
	if action == "rename" {
		details.Before, details.After = oldName, category.Name
	}
	metadata, err := json.Marshal(details)
	if err != nil {
		return Category{}, err
	}
	now := time.Now().UTC()
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "transactions.category." + action, ActorUserID: principal.UserID, AuthMethod: principal.AuthMethod, WorkspaceID: workspace, TargetType: "transactions_category", TargetKey: int64(category.ID), Action: "transactions.category." + action, Outcome: "success", Metadata: string(metadata), CreatedAt: now}); err != nil {
		return Category{}, err
	}
	if err := tx.Commit(); err != nil {
		return Category{}, err
	}
	return category, nil
}
