package scheduling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/notifications"
	"example.com/dynamis-code/apps-template/internal/platform/mail"
)

// ReconcileReminders requests reminders for upcoming bookings created before upgrade.
func (s *Service) ReconcileReminders(ctx context.Context) error {
	var after BookingID
	for {
		rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id FROM scheduling_bookings WHERE id > $1 AND status = 'booked' AND starts_at > $2 ORDER BY id LIMIT 100`, after, time.Now().UTC().Format(bookingTimeLayout))
		if err != nil {
			return err
		}
		type candidate struct {
			id        BookingID
			workspace identity.WorkspaceID
		}
		var batch []candidate
		for rows.Next() {
			var c candidate
			if err := rows.Scan(&c.id, &c.workspace); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, c)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, c := range batch {
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET status = status WHERE workspace_id = $1 AND id = $2`, c.workspace, c.id)
			if err != nil {
				tx.Rollback()
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				tx.Rollback()
				return err
			}
			if n != 0 {
				if err := refreshReminder(ctx, tx, c.workspace, c.id, false); err != nil {
					tx.Rollback()
					return err
				}
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			after = c.id
		}
	}
}

func refreshReminder(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, booking BookingID, force bool) error {
	var start, status, email string
	var engagement int64
	err := tx.QueryRowContext(ctx, `SELECT b.starts_at, b.status, COALESCE(c.email, ''), COALESCE(b.engagement_id, 0) FROM scheduling_bookings b LEFT JOIN engagements e ON e.id = b.engagement_id AND e.workspace_id = b.workspace_id LEFT JOIN clients c ON c.id = e.client_id AND c.workspace_id = b.workspace_id WHERE b.workspace_id = $1 AND b.id = $2`, workspace, booking).Scan(&start, &status, &email, &engagement)
	if err != nil {
		return err
	}
	when, err := time.Parse(bookingTimeLayout, start)
	if err != nil {
		return err
	}
	if status != string(BookingBooked) || strings.TrimSpace(email) == "" || !when.After(time.Now()) {
		return notifications.CancelBookingReminderTx(ctx, tx, workspace, int64(booking))
	}
	return notifications.RequestBookingReminderTx(ctx, tx, workspace, int64(booking), engagement, when, force)
}

// RefreshClientRemindersTx updates upcoming reminders with the client's email change.
func (s *Service) RefreshClientRemindersTx(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, client clients.ClientID) error {
	rows, err := tx.QueryContext(ctx, `SELECT b.id FROM scheduling_bookings b JOIN engagements e ON e.id = b.engagement_id AND e.workspace_id = b.workspace_id WHERE b.workspace_id = $1 AND e.client_id = $2 AND b.status = 'booked' AND b.starts_at > $3 ORDER BY b.id`, workspace, client, time.Now().UTC().Format(bookingTimeLayout))
	if err != nil {
		return err
	}
	var bookings []BookingID
	for rows.Next() {
		var id BookingID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		bookings = append(bookings, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, booking := range bookings {
		if _, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET status = status WHERE workspace_id = $1 AND id = $2`, workspace, booking); err != nil {
			return err
		}
		if err := refreshReminder(ctx, tx, workspace, booking, true); err != nil {
			return err
		}
	}
	return nil
}

// ReminderHandler validates the booking again before sending the queued notification.
func (s *Service) ReminderHandler(sender mail.Sender) jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		var request notifications.BookingReminder
		if err := json.Unmarshal([]byte(job.Payload), &request); err != nil || request.BookingID <= 0 {
			return jobs.Failure{Category: "invalid-reminder"}
		}
		delivery := sender
		if resolver, ok := sender.(interface {
			SenderFor(context.Context, string) (mail.Sender, error)
		}); ok {
			var err error
			delivery, err = resolver.SenderFor(ctx, job.WorkspaceID.String())
			if err != nil {
				return jobs.Failure{Category: "email-delivery"}
			}
		}
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Serialize booking changes against delivery, including on SQLite.
		result, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET status = status WHERE workspace_id = $1 AND id = $2`, job.WorkspaceID, request.BookingID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		result, err = tx.ExecContext(ctx, `UPDATE background_jobs SET status = status WHERE id = $1 AND status = 'running' AND lease_token = $2`, job.ID, job.LeaseToken)
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		var start, status, email, zone string
		err = tx.QueryRowContext(ctx, `SELECT b.starts_at, b.status, COALESCE(c.email, ''), w.time_zone FROM scheduling_bookings b JOIN workspaces w ON w.id = b.workspace_id LEFT JOIN engagements e ON e.id = b.engagement_id AND e.workspace_id = b.workspace_id LEFT JOIN clients c ON c.id = e.client_id AND c.workspace_id = b.workspace_id WHERE b.workspace_id = $1 AND b.id = $2`, job.WorkspaceID, request.BookingID).Scan(&start, &status, &email, &zone)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		when, err := time.Parse(bookingTimeLayout, start)
		if err != nil {
			return err
		}
		if status != string(BookingBooked) || !when.Equal(request.Start) || !when.After(time.Now()) {
			return nil
		}
		payload := job.Payload
		if strings.TrimSpace(email) == "" {
			request.Suppressed = true
			encoded, err := json.Marshal(request)
			if err != nil {
				return err
			}
			payload = string(encoded)
		} else {
			location, err := time.LoadLocation(zone)
			if err != nil {
				return err
			}
			if err := mail.SendFor(ctx, delivery, job.WorkspaceID.String(), email, "Booking reminder", "Your booking starts at "+when.In(location).Format(time.RFC3339)+" ("+zone+")."); err != nil {
				return jobs.Failure{Category: "email-delivery"}
			}
		}
		outcome := jobs.EvaluateJobOutcome(job.AttemptCount, jobs.MaxAttempts, nil, time.Now().UTC())
		if _, err := tx.ExecContext(ctx, `UPDATE background_jobs SET status = $1, completed_at = $2, lease_token = NULL, leased_until = NULL, last_error = NULL, payload = $5 WHERE id = $3 AND lease_token = $4`, string(outcome.Status), time.Now().UTC().Format(time.RFC3339Nano), job.ID, job.LeaseToken, payload); err != nil {
			return err
		}
		return tx.Commit()
	}
}
