// Package scheduling owns workspace bookable resources.
package scheduling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	_ "time/tzdata"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid scheduling resource")
var ErrNotFound = errors.New("scheduling resource not found")

type ResourceID int64

func (v ResourceID) String() string               { return id.EncodeKey(id.TypeSchedulingResource, int64(v)) }
func (v ResourceID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseResourceID(s string) ResourceID {
	v, _ := id.DecodeKey(id.TypeSchedulingResource, s)
	return ResourceID(v)
}

type Resource struct {
	ID           ResourceID       `json:"id"`
	Name         string           `json:"name"`
	UserID       *identity.UserID `json:"userId"`
	WorkingHours []WorkingHours   `json:"workingHours"`
}

type WorkingHours struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type Service struct {
	db   *sql.DB
	auth *identity.Service
}

func NewService(db *sql.DB, auth *identity.Service) *Service { return &Service{db: db, auth: auth} }

func (s *Service) List(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]Resource, string, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead); err != nil {
		return nil, "", err
	}
	var zone string
	if err := s.db.QueryRowContext(ctx, `SELECT time_zone FROM workspaces WHERE id = $1`, workspace).Scan(&zone); err != nil {
		return nil, "", err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, user_id FROM scheduling_resources WHERE workspace_id = $1 ORDER BY id`, workspace)
	if err != nil {
		return nil, "", err
	}
	resources := []Resource{}
	for rows.Next() {
		var r Resource
		if err := rows.Scan(&r.ID, &r.Name, &r.UserID); err != nil {
			rows.Close()
			return nil, "", err
		}
		r.WorkingHours = []WorkingHours{}
		resources = append(resources, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, "", err
	}
	if err := rows.Close(); err != nil {
		return nil, "", err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT h.resource_id, h.weekday, h.start_time, h.end_time FROM scheduling_working_hours h JOIN scheduling_resources r ON r.id = h.resource_id WHERE r.workspace_id = $1 ORDER BY h.weekday, h.start_time`, workspace)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var rid ResourceID
		var h WorkingHours
		if err := rows.Scan(&rid, &h.Weekday, &h.Start, &h.End); err != nil {
			return nil, "", err
		}
		for i := range resources {
			if resources[i].ID == rid {
				resources[i].WorkingHours = append(resources[i].WorkingHours, h)
				break
			}
		}
	}
	return resources, zone, rows.Err()
}

func (s *Service) SetWorkingHours(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, hours []WorkingHours) error {
	return s.write(ctx, actor, workspace, func(tx *sql.Tx) error {
		for i, h := range hours {
			start, e1 := time.Parse("15:04", h.Start)
			end, e2 := time.Parse("15:04", h.End)
			if h.Weekday < 0 || h.Weekday > 6 || e1 != nil || e2 != nil || start.Format("15:04") != h.Start || end.Format("15:04") != h.End || !start.Before(end) {
				return ErrInvalidInput
			}
			for _, other := range hours[:i] {
				if h.Weekday == other.Weekday && h.Start < other.End && other.Start < h.End {
					return ErrInvalidInput
				}
			}
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_resources WHERE workspace_id = $1 AND id = $2)`, workspace, resource).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM scheduling_working_hours WHERE resource_id IN (SELECT id FROM scheduling_resources WHERE workspace_id = $1 AND id = $2)`, workspace, resource); err != nil {
			return err
		}
		for _, h := range hours {
			if _, err := tx.ExecContext(ctx, `INSERT INTO scheduling_working_hours (resource_id, weekday, start_time, end_time) VALUES ($1, $2, $3, $4)`, resource, h.Weekday, h.Start, h.End); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) write(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, op func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite)
	if err != nil {
		return err
	}
	if !identity.CanManageScheduling(p) {
		return identity.ErrForbidden
	}
	if err := op(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func validResource(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, name string, user *identity.UserID) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInput
	}
	if user != nil {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2)`, workspace, *user).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrInvalidInput
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, name string, user *identity.UserID) (Resource, error) {
	r := Resource{Name: strings.TrimSpace(name), UserID: user, WorkingHours: []WorkingHours{}}
	err := s.write(ctx, actor, workspace, func(tx *sql.Tx) error {
		if err := validResource(ctx, tx, workspace, r.Name, user); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `INSERT INTO scheduling_resources (workspace_id, name, user_id) VALUES ($1, $2, $3) RETURNING id`, workspace, r.Name, user).Scan(&r.ID)
	})
	return r, err
}

func (s *Service) Update(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, name string, user *identity.UserID) error {
	return s.write(ctx, actor, workspace, func(tx *sql.Tx) error {
		if err := validResource(ctx, tx, workspace, name, user); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE scheduling_resources SET name = $1, user_id = $2 WHERE workspace_id = $3 AND id = $4`, strings.TrimSpace(name), user, workspace, resource)
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
		return nil
	})
}

func (s *Service) SetTimeZone(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, zone string) error {
	return s.write(ctx, actor, workspace, func(tx *sql.Tx) error {
		if zone == "" || zone == "Local" {
			return ErrInvalidInput
		}
		if _, err := time.LoadLocation(zone); err != nil {
			return ErrInvalidInput
		}
		_, err := tx.ExecContext(ctx, `UPDATE workspaces SET time_zone = $1 WHERE id = $2`, zone, workspace)
		return err
	})
}
