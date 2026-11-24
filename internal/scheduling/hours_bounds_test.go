package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestWorkingHoursBoundsTransitions(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, start, end, min, max string }{
		{"fall back", "2024-11-03T05:30:00Z", "2024-11-03T06:30:00Z", "01:00:00.000000000", "01:59:59.999999999"},
		{"spring forward", "2024-03-10T06:30:00Z", "2024-03-10T07:30:00Z", "01:30:00.000000000", "03:30:00.000000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, _ := time.Parse(time.RFC3339, tc.start)
			end, _ := time.Parse(time.RFC3339, tc.end)
			min, max, valid := workingHoursBounds(start, end, location)
			if !valid || min.Format("15:04:05.000000000") != tc.min || max.Format("15:04:05.000000000") != tc.max {
				t.Fatalf("bounds = %v, %v, %v", min, max, valid)
			}
		})
	}
}

func TestBookingAndMoveRejectFallBackOutsideHours(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "dst@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	if err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Create(ctx, actor, owner.WorkspaceID, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, r.ID, []WorkingHours{{Weekday: 0, Start: "01:30", End: "02:30"}}); err != nil {
		t.Fatal(err)
	}
	duration := 60
	entry, err := catalog.NewService(db, auth, nil).Create(ctx, actor, owner.WorkspaceID, "Visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	fall := time.Date(2024, 11, 3, 5, 30, 0, 0, time.UTC)
	if _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, fall); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("booking: %v", err)
	}
	booking, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, fall.Add((7*24+1)*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, fall); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("move: %v", err)
	}
}
