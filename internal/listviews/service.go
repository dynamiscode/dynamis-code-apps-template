// Package listviews stores saved views: named, prefiltered ways of looking at a list. A personal view belongs to its
// creator. A shared view belongs to the workspace; only its creator and workspace admins change it.
package listviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var (
	ErrInvalidInput = errors.New("saved view input is invalid")
	ErrNotFound     = errors.New("saved view was not found")
	ErrLimit        = errors.New("saved view limit reached")
)

// Keys of the lists that can hold saved views. They match the list preference keys.
const (
	ListClients      = "clients"
	ListItems        = "items"
	ListFiles        = "files"
	ListMembers      = "members"
	ListTransactions = "transactions"
)

// Limits keep the view menu usable: per list, per person, and per workspace for shared views.
const (
	MaxPersonalPerList = 20
	MaxSharedPerList   = 30
	maxNameLength      = 60
)

// State is what a view applies: the list's query (filters, match, sort and order, without the search and rows per
// page) and the column settings.
type State struct {
	Query   string          `json:"query"`
	Visible map[string]bool `json:"visible,omitempty"`
	Order   []string        `json:"order,omitempty"`
}

// View is one saved view as the list shows it.
type View struct {
	ID        int64
	ListKey   string
	Name      string
	Shared    bool
	CreatedBy identity.UserID
	State     State
}

// Service stores saved views in the list_views table and each person's default in list_view_defaults.
type Service struct {
	db     *sql.DB
	driver config.DatabaseDriver
	auth   *identity.Service
	now    func() time.Time
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service) *Service {
	return &Service{db: db, driver: driver, auth: auth, now: time.Now}
}

// List returns the views the actor can see in one list, personal views first and then shared, with the id of the
// actor's default view (zero when the list opens in Active).
func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, listKey string) ([]View, int64, error) {
	if err := validListKey(listKey); err != nil {
		return nil, 0, err
	}
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead)
	if err != nil || !canReadList(principal, listKey) {
		return nil, 0, identity.ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, list_key, name, shared, created_by_user_id, state FROM list_views
		WHERE workspace_id = ? AND list_key = ? AND (shared = ? OR created_by_user_id = ?)
		ORDER BY shared, id
	`), workspaceID, listKey, true, actor.UserID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var views []View
	for rows.Next() {
		view, err := scanView(rows)
		if err != nil {
			return nil, 0, err
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var defaultID int64
	err = s.db.QueryRowContext(ctx, database.Rebind(s.driver, `
		SELECT view_id FROM list_view_defaults WHERE workspace_id = ? AND user_id = ? AND list_key = ?
	`), workspaceID, actor.UserID, listKey).Scan(&defaultID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, 0, err
	}
	return views, defaultID, nil
}

// Create saves a new view. A shared view needs the workspace-update permission, which owners and admins have.
func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, listKey, name string, shared bool, state State) (View, error) {
	name, encoded, err := validate(listKey, name, state)
	if err != nil {
		return View{}, err
	}
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead)
	if err != nil || !canReadList(principal, listKey) {
		return View{}, identity.ErrForbidden
	}
	if shared {
		if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
			return View{}, identity.ErrForbidden
		}
	}
	// Serializable, so two creates cannot both pass the count and exceed the per-list limit.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return View{}, err
	}
	defer tx.Rollback()
	if err := s.checkLimit(ctx, tx, actor, workspaceID, listKey, shared); err != nil {
		return View{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	var id int64
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `
		INSERT INTO list_views (workspace_id, created_by_user_id, list_key, name, shared, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`), workspaceID, actor.UserID, listKey, name, shared, encoded, now, now).Scan(&id); err != nil {
		return View{}, err
	}
	if err := tx.Commit(); err != nil {
		return View{}, err
	}
	return View{ID: id, ListKey: listKey, Name: name, Shared: shared, CreatedBy: actor.UserID, State: state}, nil
}

// Update renames a view and replaces its state. The creator changes a personal view. A shared view can be changed
// by its creator and by workspace admins.
func (s *Service) Update(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, id int64, name string, state State) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	view, err := s.loadForChange(ctx, tx, actor, workspaceID, id)
	if err != nil {
		return err
	}
	name, encoded, err := validate(view.ListKey, name, state)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
		UPDATE list_views SET name = ?, state = ?, updated_at = ? WHERE id = ? AND workspace_id = ?
	`), name, encoded, s.now().UTC().Format(time.RFC3339Nano), id, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a view and any member's default that pointed at it. The change rules are the same as Update's.
func (s *Service) Delete(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.loadForChange(ctx, tx, actor, workspaceID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
		DELETE FROM list_views WHERE id = ? AND workspace_id = ?
	`), id, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetDefault makes a view the actor's default for a list. Zero clears the default, so the list opens in Active.
// The actor can only pick a view they can see.
func (s *Service) SetDefault(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, listKey string, id int64) error {
	if err := validListKey(listKey); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead)
	if err != nil || !canReadList(principal, listKey) {
		return identity.ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
		DELETE FROM list_view_defaults WHERE workspace_id = ? AND user_id = ? AND list_key = ?
	`), workspaceID, actor.UserID, listKey); err != nil {
		return err
	}
	if id != 0 {
		view, err := s.visibleTo(ctx, tx, actor, workspaceID, id)
		if err != nil {
			return err
		}
		if view.ListKey != listKey {
			return ErrInvalidInput
		}
		if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
			INSERT INTO list_view_defaults (workspace_id, user_id, list_key, view_id) VALUES (?, ?, ?, ?)
		`), workspaceID, actor.UserID, listKey, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Get returns one view the actor can see.
func (s *Service) Get(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, id int64) (View, error) {
	principal, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead)
	if err != nil {
		return View{}, identity.ErrForbidden
	}
	view, err := s.visibleTo(ctx, s.db, actor, workspaceID, id)
	if err != nil {
		return View{}, err
	}
	if !canReadList(principal, view.ListKey) {
		return View{}, identity.ErrForbidden
	}
	return view, nil
}

// visibleTo reads a view the actor can see, on a database or a transaction.
func (s *Service) visibleTo(ctx context.Context, q queryer, actor identity.Principal, workspaceID identity.WorkspaceID, id int64) (View, error) {
	row := q.QueryRowContext(ctx, database.Rebind(s.driver, `
		SELECT id, list_key, name, shared, created_by_user_id, state FROM list_views
		WHERE id = ? AND workspace_id = ? AND (shared = ? OR created_by_user_id = ?)
	`), id, workspaceID, true, actor.UserID)
	view, err := scanView(row)
	if errors.Is(err, sql.ErrNoRows) {
		return View{}, ErrNotFound
	}
	return view, err
}

// loadForChange reads a view the actor may change: their own personal view, or a shared view they created or may
// manage as a workspace admin.
func (s *Service) loadForChange(ctx context.Context, tx *sql.Tx, actor identity.Principal, workspaceID identity.WorkspaceID, id int64) (View, error) {
	// A removed member keeps no rights to change views, even their own personal ones.
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead)
	if err != nil {
		return View{}, identity.ErrForbidden
	}
	view, err := s.visibleTo(ctx, tx, actor, workspaceID, id)
	if err != nil {
		return View{}, err
	}
	if !canReadList(principal, view.ListKey) {
		return View{}, identity.ErrForbidden
	}
	if view.Shared {
		if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
			return View{}, identity.ErrForbidden
		}
		return view, nil
	}
	if view.CreatedBy == actor.UserID {
		return view, nil
	}
	if view.Shared {
		if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err == nil {
			return view, nil
		}
	}
	return View{}, identity.ErrForbidden
}

// checkLimit refuses a view past the per-list limit: personal views count per person, shared ones per workspace.
func (s *Service) checkLimit(ctx context.Context, tx *sql.Tx, actor identity.Principal, workspaceID identity.WorkspaceID, listKey string, shared bool) error {
	var count int
	query := `SELECT COUNT(*) FROM list_views WHERE workspace_id = ? AND list_key = ? AND shared = ? AND created_by_user_id = ?`
	args := []any{workspaceID, listKey, false, actor.UserID}
	limit := MaxPersonalPerList
	if shared {
		query = `SELECT COUNT(*) FROM list_views WHERE workspace_id = ? AND list_key = ? AND shared = ?`
		args = []any{workspaceID, listKey, true}
		limit = MaxSharedPerList
	}
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, query), args...).Scan(&count); err != nil {
		return err
	}
	if count >= limit {
		return ErrLimit
	}
	return nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type scanner interface {
	Scan(dest ...any) error
}

func scanView(row scanner) (View, error) {
	var view View
	var state string
	if err := row.Scan(&view.ID, &view.ListKey, &view.Name, &view.Shared, &view.CreatedBy, &state); err != nil {
		return View{}, err
	}
	if err := json.Unmarshal([]byte(state), &view.State); err != nil {
		return View{}, err
	}
	return view, nil
}

// validate checks the list, the name and the state, and returns the trimmed name and the stored state.
func validate(listKey, name string, state State) (string, string, error) {
	if err := validListKey(listKey); err != nil {
		return "", "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxNameLength {
		return "", "", ErrInvalidInput
	}
	if len(state.Query) > 2000 || len(state.Order) > 200 || len(state.Visible) > 200 {
		return "", "", ErrInvalidInput
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", "", err
	}
	return name, string(encoded), nil
}

func canReadList(principal identity.Principal, listKey string) bool {
	return listKey != ListTransactions || identity.CanReadTransactions(principal)
}

func validListKey(listKey string) error {
	switch listKey {
	case ListClients, ListItems, ListFiles, ListMembers, ListTransactions:
		return nil
	default:
		return ErrInvalidInput
	}
}
