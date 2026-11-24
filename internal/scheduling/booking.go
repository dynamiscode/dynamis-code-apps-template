package scheduling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrNoDuration = errors.New("catalog entry has no duration")
var ErrOverlap = errors.New("booking overlaps another booking")
var ErrOutsideHours = errors.New("booking is outside working hours")
var ErrInvalidTransition = errors.New("invalid booking transition")

type BookingStatus string

const (
	BookingBooked    BookingStatus = "booked"
	BookingDone      BookingStatus = "done"
	BookingCancelled BookingStatus = "cancelled"
	BookingNoShow    BookingStatus = "no-show"
)

func EvaluateBookingTransition(from, to BookingStatus) (BookingStatus, error) {
	if from != BookingBooked || (to != BookingDone && to != BookingCancelled && to != BookingNoShow) {
		return from, ErrInvalidTransition
	}
	return to, nil
}

type BookingID int64

func (v BookingID) String() string               { return id.EncodeKey(id.TypeSchedulingBooking, int64(v)) }
func (v BookingID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseBookingID(value string) BookingID {
	key, _ := id.DecodeKey(id.TypeSchedulingBooking, value)
	return BookingID(key)
}

type Booking struct {
	ID           BookingID       `json:"id"`
	ResourceID   ResourceID      `json:"resourceId"`
	EntryID      catalog.EntryID `json:"entryId"`
	EntryName    string          `json:"entryName"`
	Start        time.Time       `json:"start"`
	End          time.Time       `json:"end"`
	Status       BookingStatus   `json:"status"`
	EngagementID *engagement.ID  `json:"engagementId"`
}

type EngagementOption struct {
	ID       engagement.ID
	ClientID clients.ClientID
	Label    string
}

func (s *Service) EngagementOptions(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) ([]EngagementOption, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, c.id, c.display_name, st.name FROM engagements e JOIN clients c ON c.id = e.client_id AND c.workspace_id = e.workspace_id JOIN engagement_stages st ON st.id = e.stage_id AND st.workspace_id = e.workspace_id WHERE e.workspace_id = $1 ORDER BY c.display_name, e.id`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []EngagementOption
	for rows.Next() {
		var option EngagementOption
		var client, stage string
		if err := rows.Scan(&option.ID, &option.ClientID, &client, &stage); err != nil {
			return nil, err
		}
		option.Label = client + " — " + stage + " (" + option.ID.String() + ")"
		options = append(options, option)
	}
	return options, rows.Err()
}

func (s *Service) SetBookingStatus(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, booking BookingID, target BookingStatus) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite); err != nil {
		return err
	}
	var current BookingStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM scheduling_bookings WHERE workspace_id = $1 AND resource_id = $2 AND id = $3`, workspace, resource, booking).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := EvaluateBookingTransition(current, target); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET status = $1 WHERE workspace_id = $2 AND resource_id = $3 AND id = $4 AND status = $5`, target, workspace, resource, booking, current)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrInvalidTransition
	}
	if err := refreshReminder(ctx, tx, workspace, booking, false); err != nil {
		return err
	}
	return tx.Commit()
}

const bookingTimeLayout = "2006-01-02T15:04:05.000000000Z"

func workingHoursBounds(start, end time.Time, location *time.Location) (time.Time, time.Time, bool) {
	first := start.In(location)
	min, max := first, first
	check := func(t time.Time) bool {
		local := t.In(location)
		if local.Year() != first.Year() || local.YearDay() != first.YearDay() {
			return false
		}
		clock := local.Format("15:04:05.000000000")
		if clock < min.Format("15:04:05.000000000") {
			min = local
		}
		if clock > max.Format("15:04:05.000000000") {
			max = local
		}
		return true
	}
	for cursor := start; ; {
		_, boundary := cursor.In(location).ZoneBounds()
		if boundary.IsZero() || boundary.After(end) {
			break
		}
		if !check(boundary.Add(-time.Nanosecond)) || !check(boundary) {
			return min, max, false
		}
		cursor = boundary
	}
	valid := check(end)
	return min, max, valid
}

func CatalogEntryInUse(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, entry catalog.EntryID) (bool, error) {
	var used bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_bookings WHERE workspace_id = $1 AND entry_id = $2)`, workspace, entry).Scan(&used)
	return used, err
}

func (s *Service) Schedule(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID) ([]Booking, string, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspace, identity.ResourcesRead); err != nil {
		return nil, "", err
	}
	var zone string
	if err := s.db.QueryRowContext(ctx, `SELECT w.time_zone FROM workspaces w JOIN scheduling_resources r ON r.workspace_id = w.id WHERE w.id = $1 AND r.id = $2`, workspace, resource).Scan(&zone); errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	} else if err != nil {
		return nil, "", err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, b.entry_id, e.name, b.starts_at, b.ends_at, b.status, b.engagement_id FROM scheduling_bookings b JOIN catalog_entries e ON e.id = b.entry_id WHERE b.workspace_id = $1 AND b.resource_id = $2 ORDER BY b.starts_at, b.id`, workspace, resource)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	bookings := []Booking{}
	for rows.Next() {
		var b Booking
		var start, end string
		b.ResourceID = resource
		if err := rows.Scan(&b.ID, &b.EntryID, &b.EntryName, &start, &end, &b.Status, &b.EngagementID); err != nil {
			return nil, "", err
		}
		b.Start, err = time.Parse(bookingTimeLayout, start)
		if err != nil {
			return nil, "", err
		}
		b.End, err = time.Parse(bookingTimeLayout, end)
		if err != nil {
			return nil, "", err
		}
		b.Start, b.End = b.Start.In(location), b.End.In(location)
		bookings = append(bookings, b)
	}
	return bookings, zone, rows.Err()
}

func (s *Service) Book(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, entry catalog.EntryID, start time.Time) (Booking, error) {
	return s.BookWithEngagement(ctx, actor, workspace, resource, entry, start, nil)
}

func (s *Service) BookWithEngagement(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, entry catalog.EntryID, start time.Time, link *engagement.ID) (Booking, error) {
	for attempt := 0; ; attempt++ {
		booking, err := s.book(ctx, actor, workspace, resource, entry, start, link)
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "40001" || attempt == 2 {
			return booking, err
		}
		if err := ctx.Err(); err != nil {
			return Booking{}, err
		}
	}
}

func (s *Service) book(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, entry catalog.EntryID, start time.Time, link *engagement.ID) (Booking, error) {
	b := Booking{ResourceID: resource, EntryID: entry, Start: start.UTC(), Status: "booked", EngagementID: link}
	if start.IsZero() {
		return Booking{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite); err != nil {
		return Booking{}, err
	}
	if err := validateEngagement(ctx, tx, workspace, link); err != nil {
		return Booking{}, err
	}
	// Lock the resource before checking the slot, including on SQLite.
	result, err := tx.ExecContext(ctx, `UPDATE scheduling_resources SET name = name WHERE workspace_id = $1 AND id = $2`, workspace, resource)
	if err != nil {
		return Booking{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Booking{}, err
	}
	if count == 0 {
		return Booking{}, ErrNotFound
	}
	var duration sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT name, duration_minutes FROM catalog_entries WHERE workspace_id = $1 AND id = $2 AND archived_at IS NULL`, workspace, entry).Scan(&b.EntryName, &duration); errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotFound
	} else if err != nil {
		return Booking{}, err
	}
	if !duration.Valid {
		return Booking{}, ErrNoDuration
	}
	if duration.Int64 <= 0 {
		return Booking{}, ErrInvalidInput
	}
	if duration.Int64 > math.MaxInt64/int64(time.Minute) {
		return Booking{}, ErrOutsideHours
	}
	b.End = b.Start.Add(time.Duration(duration.Int64) * time.Minute)
	var zone string
	if err := tx.QueryRowContext(ctx, `SELECT time_zone FROM workspaces WHERE id = $1`, workspace).Scan(&zone); err != nil {
		return Booking{}, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return Booking{}, err
	}
	localStart, localEnd, sameDay := workingHoursBounds(b.Start, b.End, location)
	if !sameDay {
		return Booking{}, ErrOutsideHours
	}
	var within bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_working_hours WHERE resource_id = $1 AND weekday = $2 AND start_time || ':00.000000000' <= $3 AND end_time || ':00.000000000' >= $4)`, resource, int(localStart.Weekday()), localStart.Format("15:04:05.000000000"), localEnd.Format("15:04:05.000000000")).Scan(&within); err != nil {
		return Booking{}, err
	}
	if !within {
		return Booking{}, ErrOutsideHours
	}
	var overlap bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scheduling_bookings WHERE workspace_id = $1 AND resource_id = $2 AND status IN ('booked', 'done') AND starts_at < $3 AND ends_at > $4)`, workspace, resource, b.End.Format(bookingTimeLayout), b.Start.Format(bookingTimeLayout)).Scan(&overlap); err != nil {
		return Booking{}, err
	}
	if overlap {
		return Booking{}, ErrOverlap
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO scheduling_bookings (workspace_id, resource_id, entry_id, starts_at, ends_at, status, engagement_id) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, workspace, resource, entry, b.Start.Format(bookingTimeLayout), b.End.Format(bookingTimeLayout), b.Status, link).Scan(&b.ID); err != nil {
		return Booking{}, err
	}
	if err := refreshReminder(ctx, tx, workspace, b.ID, false); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return b, nil
}

func validateEngagement(ctx context.Context, tx *sql.Tx, workspace identity.WorkspaceID, link *engagement.ID) error {
	if link == nil {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM engagements WHERE workspace_id = $1 AND id = $2)`, workspace, *link).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) SetEngagement(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, booking BookingID, link *engagement.ID) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.ResourcesWrite); err != nil {
		return err
	}
	if err := validateEngagement(ctx, tx, workspace, link); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE scheduling_bookings SET engagement_id = $1 WHERE workspace_id = $2 AND id = $3`, link, workspace, booking)
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
	if err := refreshReminder(ctx, tx, workspace, booking, false); err != nil {
		return err
	}
	return tx.Commit()
}
