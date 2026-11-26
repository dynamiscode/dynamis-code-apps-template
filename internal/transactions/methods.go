package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
)

var ErrLastMethod = errors.New("cannot disable the last enabled payment method")

type Method struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ListMethods returns disabled methods only to workspace administrators.
func (s *Service) ListMethods(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]Method, error) {
	return s.listMethods(ctx, actor, workspace, false, false, false, "")
}

// ListFilterMethods includes disabled methods that appear in workspace entries.
func (s *Service) ListFilterMethods(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]Method, error) {
	return s.listMethods(ctx, actor, workspace, true, false, false, "")
}

// ListMethodPage returns at most 101 methods after cursor, ordered by stable ID.
// The extra method signals that another page is available.
func (s *Service) ListMethodPage(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, cursor string, enabledOnly bool) ([]Method, error) {
	return s.listMethods(ctx, actor, workspace, false, true, enabledOnly, cursor)
}

func (s *Service) listMethods(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, filterChoices, paged, enabledOnly bool, cursor string) ([]Method, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesRead)
	if err != nil {
		return nil, err
	}
	if !identity.CanReadTransactions(principal) {
		return nil, identity.ErrForbidden
	}
	if err := seedMethods(ctx, tx, workspace); err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if paged {
		rows, err = tx.QueryContext(ctx, `SELECT m.name, m.display_name, m.enabled FROM transactions_payment_methods m WHERE m.workspace_id = $1 AND m.name > $2 AND (m.enabled = TRUE OR $3 = TRUE) ORDER BY m.name LIMIT 101`, workspace, cursor, !enabledOnly && (principal.Role == identity.Owner || principal.Role == identity.Admin))
	} else {
		rows, err = tx.QueryContext(ctx, `SELECT m.name, m.display_name, m.enabled FROM transactions_payment_methods m WHERE m.workspace_id = $1 AND (m.enabled = TRUE OR $2 = TRUE OR ($3 = TRUE AND EXISTS (SELECT 1 FROM transactions_entries e WHERE e.workspace_id = m.workspace_id AND e.payment_method = m.name))) ORDER BY m.display_name, m.name`, workspace, !filterChoices && (principal.Role == identity.Owner || principal.Role == identity.Admin), filterChoices)
	}
	if err != nil {
		return nil, err
	}
	methods := []Method{}
	for rows.Next() {
		var method Method
		if err := rows.Scan(&method.ID, &method.Name, &method.Enabled); err != nil {
			rows.Close()
			return nil, err
		}
		methods = append(methods, method)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return methods, nil
}

// SaveMethod creates a method when id is empty, otherwise updates its name and state.
func (s *Service) SaveMethod(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, id, name string, enabled bool) (Method, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
		return Method{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Method{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite)
	if err != nil {
		return Method{}, err
	}
	if !identity.CanManageTransactions(principal) {
		return Method{}, identity.ErrForbidden
	}
	// Serialize all method writes within a workspace on both supported databases.
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET name = name WHERE id = $1`, workspace); err != nil {
		return Method{}, err
	}
	if err := seedMethods(ctx, tx, workspace); err != nil {
		return Method{}, err
	}
	var duplicate bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transactions_payment_methods WHERE workspace_id = $1 AND display_name = $2 AND name <> $3)`, workspace, name, id).Scan(&duplicate); err != nil {
		return Method{}, err
	}
	if duplicate {
		return Method{}, ErrInvalidInput
	}
	action := "transactions.method.updated"
	var before *Method
	if id == "" {
		if !enabled {
			return Method{}, ErrInvalidInput
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions_payment_methods WHERE workspace_id = $1`, workspace).Scan(&count); err != nil {
			return Method{}, err
		}
		if count >= 100 {
			return Method{}, ErrInvalidInput
		}
		for n := count + 1; ; n++ {
			id = "method-" + strconv.Itoa(n)
			result, err := tx.ExecContext(ctx, `INSERT INTO transactions_payment_methods (workspace_id, name, display_name, enabled) VALUES ($1, $2, $3, TRUE) ON CONFLICT (workspace_id, name) DO NOTHING`, workspace, id, name)
			if err != nil {
				return Method{}, err
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return Method{}, err
			}
			if inserted == 1 {
				break
			}
		}
		action = "transactions.method.created"
	} else {
		before = &Method{ID: id}
		if err := tx.QueryRowContext(ctx, `SELECT display_name, enabled FROM transactions_payment_methods WHERE workspace_id = $1 AND name = $2`, workspace, id).Scan(&before.Name, &before.Enabled); errors.Is(err, sql.ErrNoRows) {
			return Method{}, ErrInvalidInput
		} else if err != nil {
			return Method{}, err
		}
		if before.Enabled && !enabled {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions_payment_methods WHERE workspace_id = $1 AND enabled = TRUE`, workspace).Scan(&count); err != nil {
				return Method{}, err
			}
			if count <= 1 {
				return Method{}, ErrLastMethod
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE transactions_payment_methods SET display_name = $1, enabled = $2 WHERE workspace_id = $3 AND name = $4`, name, enabled, workspace, id); err != nil {
			return Method{}, err
		}
	}
	after := Method{ID: id, Name: name, Enabled: enabled}
	metadata, err := json.Marshal(struct {
		MethodID string  `json:"methodId"`
		Before   *Method `json:"before"`
		After    Method  `json:"after"`
	}{id, before, after})
	if err != nil {
		return Method{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: action, ActorUserID: principal.UserID, AuthMethod: principal.AuthMethod, WorkspaceID: workspace, TargetType: "transactions_method", Action: action, Outcome: "success", Metadata: string(metadata), CreatedAt: time.Now().UTC()}); err != nil {
		return Method{}, err
	}
	if err := tx.Commit(); err != nil {
		return Method{}, err
	}
	return after, nil
}
