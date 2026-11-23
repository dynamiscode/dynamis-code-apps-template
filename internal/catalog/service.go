// Package catalog owns the workspace catalog.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
)

var ErrInvalidInput = errors.New("invalid catalog entry")

type Entry struct {
	Name            string `json:"name"`
	DurationMinutes *int   `json:"durationMinutes"`
}

type Service struct {
	db   *sql.DB
	auth *identity.Service
}

func NewService(db *sql.DB, auth *identity.Service) *Service { return &Service{db: db, auth: auth} }

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Entry, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name, duration_minutes FROM catalog_entries WHERE workspace_id = $1 AND archived_at IS NULL ORDER BY id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.Name, &entry.DurationMinutes); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, name string, duration *int) (Entry, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return Entry{}, err
	}
	if !identity.CanWriteCatalog(principal) {
		return Entry{}, identity.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" || (duration != nil && *duration <= 0) {
		return Entry{}, ErrInvalidInput
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO catalog_entries (workspace_id, name, duration_minutes) VALUES ($1, $2, $3)`, workspaceID, name, duration)
	if err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return Entry{Name: name, DurationMinutes: duration}, nil
}
