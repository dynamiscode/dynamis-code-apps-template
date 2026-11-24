// Package notifications schedules notification delivery.
package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

const BookingReminderKind = "booking-reminder"

type BookingReminder struct {
	BookingID    int64     `json:"bookingId"`
	EngagementID int64     `json:"engagementId"`
	Start        time.Time `json:"start"`
	Suppressed   bool      `json:"suppressed,omitempty"`
}

// RequestBookingReminderTx replaces the pending request atomically with the booking.
func RequestBookingReminderTx(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, booking, engagement int64, start time.Time, force bool) error {
	payload, err := json.Marshal(BookingReminder{BookingID: booking, EngagementID: engagement, Start: start.UTC()})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	due := start.Add(-24 * time.Hour)
	if due.Before(now) {
		due = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO background_jobs (workspace_id, kind, deduplication_key, payload, status, attempt_count, available_at, created_at)
		VALUES ($1, $2, $3, $4, 'pending', 0, $5, $6)
		ON CONFLICT (workspace_id, kind, deduplication_key) DO UPDATE SET payload = excluded.payload, status = 'pending', attempt_count = 0, available_at = excluded.available_at, lease_token = NULL, leased_until = NULL, started_at = NULL, completed_at = NULL, last_error = NULL WHERE background_jobs.payload <> excluded.payload OR ($7 AND background_jobs.status <> 'succeeded')`, workspace, BookingReminderKind, booking, string(payload), due.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), force)
	return err
}

func CancelBookingReminderTx(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, booking int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM background_jobs WHERE workspace_id = $1 AND kind = $2 AND deduplication_key = $3 AND status IN ('pending', 'running')`, workspace, BookingReminderKind, booking)
	return err
}
