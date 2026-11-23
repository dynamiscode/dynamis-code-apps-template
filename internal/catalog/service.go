// Package catalog owns the workspace catalog.
package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid catalog entry")
var ErrNotFound = errors.New("catalog entry not found")
var ErrInUse = errors.New("catalog entry is in use; archive it instead")

type EntryID int64

func (v EntryID) String() string               { return id.EncodeKey(id.TypeCatalogEntry, int64(v)) }
func (v EntryID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseEntryID(s string) EntryID            { v, _ := id.DecodeKey(id.TypeCatalogEntry, s); return EntryID(v) }

type Entry struct {
	ID              EntryID `json:"id"`
	Name            string  `json:"name"`
	DurationMinutes *int    `json:"durationMinutes"`
	Archived        bool    `json:"archived"`
}

type Service struct {
	db    *sql.DB
	auth  *identity.Service
	inUse func(context.Context, *sql.Tx, identity.WorkspaceID, EntryID) (bool, error)
}

// A nil inUse checker is valid only while no sale integration exists.
func NewService(db *sql.DB, auth *identity.Service, inUse func(context.Context, *sql.Tx, identity.WorkspaceID, EntryID) (bool, error)) *Service {
	return &Service{db: db, auth: auth, inUse: inUse}
}

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, includeArchived bool) ([]Entry, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, err
	}
	query := `SELECT id, name, duration_minutes, archived_at FROM catalog_entries WHERE workspace_id = $1`
	if !includeArchived {
		query += ` AND archived_at IS NULL`
	}
	query += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		var entry Entry
		var archived sql.NullString
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.DurationMinutes, &archived); err != nil {
			return nil, err
		}
		entry.Archived = archived.Valid
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
	var entryID EntryID
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_entries (workspace_id, name, duration_minutes) VALUES ($1, $2, $3) RETURNING id`, workspaceID, name, duration).Scan(&entryID)
	if err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return Entry{ID: entryID, Name: name, DurationMinutes: duration}, nil
}

func (s *Service) SetArchived(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, archived bool) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		var at any
		if archived {
			at = time.Now().UTC().Format(time.RFC3339Nano)
		}
		return tx.ExecContext(ctx, `UPDATE catalog_entries SET archived_at = $1 WHERE workspace_id = $2 AND id = $3`, at, workspaceID, entryID)
	})
}
func (s *Service) Delete(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID) error {
	return s.mutate(ctx, actor, workspaceID, entryID, func(tx *sql.Tx) (sql.Result, error) {
		var found EntryID
		if err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_entries WHERE workspace_id = $1 AND id = $2`, workspaceID, entryID).Scan(&found); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, err
		}
		if s.inUse != nil {
			used, err := s.inUse(ctx, tx, workspaceID, entryID)
			if err != nil {
				return nil, err
			}
			if used {
				return nil, ErrInUse
			}
		}
		return tx.ExecContext(ctx, `DELETE FROM catalog_entries WHERE workspace_id = $1 AND id = $2`, workspaceID, entryID)
	})
}
func (s *Service) mutate(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, entryID EntryID, operation func(*sql.Tx) (sql.Result, error)) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	principal, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return err
	}
	if !identity.CanWriteCatalog(principal) {
		return identity.ErrForbidden
	}
	result, err := operation(tx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}
