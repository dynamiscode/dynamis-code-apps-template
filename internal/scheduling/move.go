package scheduling

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type BookingMove struct {
	OldStart time.Time       `json:"oldStart"`
	OldEnd   time.Time       `json:"oldEnd"`
	NewStart time.Time       `json:"newStart"`
	NewEnd   time.Time       `json:"newEnd"`
	ActorID  identity.UserID `json:"actorId"`
	MovedAt  time.Time       `json:"movedAt"`
}

func (s *Service) BookingHistory(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, booking BookingID) ([]BookingMove, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead); err != nil {
		return nil, err
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_bookings WHERE workspace_id = $1 AND resource_id = $2 AND id = $3)`, workspace, resource, booking).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT old_start, old_end, new_start, new_end, actor_id, moved_at FROM scheduling_booking_moves WHERE workspace_id = $1 AND booking_id = $2 ORDER BY id`, workspace, booking)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []BookingMove{}
	for rows.Next() {
		var m BookingMove
		var oldStart, oldEnd, newStart, newEnd, movedAt string
		if err := rows.Scan(&oldStart, &oldEnd, &newStart, &newEnd, &m.ActorID, &movedAt); err != nil {
			return nil, err
		}
		for i, value := range []string{oldStart, oldEnd, newStart, newEnd, movedAt} {
			parsed, err := time.Parse(bookingTimeLayout, value)
			if err != nil {
				return nil, err
			}
			*([]*time.Time{&m.OldStart, &m.OldEnd, &m.NewStart, &m.NewEnd, &m.MovedAt}[i]) = parsed
		}
		history = append(history, m)
	}
	return history, rows.Err()
}

func (s *Service) MoveBooking(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, booking BookingID, start time.Time) error {
	for attempt := 0; ; attempt++ {
		err := s.moveBooking(ctx, actor, workspace, resource, booking, start)
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "40001" || attempt == 2 {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *Service) moveBooking(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, booking BookingID, start time.Time) error {
	if start.IsZero() {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE scheduling_resources SET name = name WHERE workspace_id = $1 AND id = $2`, workspace, resource)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	var oldStart, oldEnd string
	var status BookingStatus
	if err := tx.QueryRowContext(ctx, `SELECT starts_at, ends_at, status FROM scheduling_bookings WHERE workspace_id = $1 AND resource_id = $2 AND id = $3`, workspace, resource, booking).Scan(&oldStart, &oldEnd, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status == BookingCancelled || status == BookingNoShow {
		return ErrInvalidTransition
	}
	previousStart, err := time.Parse(bookingTimeLayout, oldStart)
	if err != nil {
		return err
	}
	previousEnd, err := time.Parse(bookingTimeLayout, oldEnd)
	if err != nil {
		return err
	}
	start = start.UTC()
	end := start.Add(previousEnd.Sub(previousStart))
	var zone string
	if err := tx.QueryRowContext(ctx, `SELECT time_zone FROM workspaces WHERE id = $1`, workspace).Scan(&zone); err != nil {
		return err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	localStart, localEnd, sameDay := workingHoursBounds(start, end, location)
	if !sameDay {
		return ErrOutsideHours
	}
	var within bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_working_hours WHERE resource_id = $1 AND weekday = $2 AND start_time || ':00.000000000' <= $3 AND end_time || ':00.000000000' >= $4)`, resource, int(localStart.Weekday()), localStart.Format("15:04:05.000000000"), localEnd.Format("15:04:05.000000000")).Scan(&within); err != nil {
		return err
	}
	if !within {
		return ErrOutsideHours
	}
	var overlap bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_bookings WHERE workspace_id = $1 AND resource_id = $2 AND id <> $3 AND status IN ('booked', 'done') AND starts_at < $4 AND ends_at > $5)`, workspace, resource, booking, end.Format(bookingTimeLayout), start.Format(bookingTimeLayout)).Scan(&overlap); err != nil {
		return err
	}
	if overlap {
		return ErrOverlap
	}
	newStart, newEnd := start.Format(bookingTimeLayout), end.Format(bookingTimeLayout)
	if _, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET starts_at = $1, ends_at = $2 WHERE workspace_id = $3 AND resource_id = $4 AND id = $5`, newStart, newEnd, workspace, resource, booking); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scheduling_booking_moves (workspace_id, booking_id, actor_id, old_start, old_end, new_start, new_end, moved_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, workspace, booking, actor.UserID, oldStart, oldEnd, newStart, newEnd, time.Now().UTC().Format(bookingTimeLayout)); err != nil {
		return err
	}
	if err := refreshReminder(ctx, tx, workspace, booking, false); err != nil {
		return err
	}
	return tx.Commit()
}
