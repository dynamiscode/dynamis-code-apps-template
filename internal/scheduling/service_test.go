package scheduling

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestBookingTransitions(t *testing.T) {
	for _, target := range []BookingStatus{BookingDone, BookingCancelled, BookingNoShow} {
		if got, err := EvaluateBookingTransition(BookingBooked, target); err != nil || got != target {
			t.Fatalf("booked to %s: %s %v", target, got, err)
		}
		if _, err := EvaluateBookingTransition(target, BookingDone); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("terminal %s: %v", target, err)
		}
	}
}

func TestConcurrentBookingsSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "bookings.db"), MaxOpenConns: 4, MaxIdleConns: 4})
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "concurrent@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	checkConcurrentBookings(t, NewService(db, auth), actor, owner.WorkspaceID)
}

func checkConcurrentBookings(t *testing.T, s *Service, actor identity.Principal, workspace identity.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	r, err := s.Create(ctx, actor, workspace, "Concurrent room", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkingHours(ctx, actor, workspace, r.ID, []WorkingHours{{Weekday: 1, Start: "09:00", End: "17:00"}}); err != nil {
		t.Fatal(err)
	}
	duration := 60
	e, err := catalog.NewService(s.db, s.auth, nil).Create(ctx, actor, workspace, "Concurrent visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-ready
			_, err := s.Book(ctx, actor, workspace, r.ID, e.ID, time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC))
			results <- err
		}()
	}
	close(ready)
	a, b := <-results, <-results
	if (a == nil) == (b == nil) {
		t.Fatalf("expected one success: %v, %v", a, b)
	}
	if a != nil && !errors.Is(a, ErrOverlap) || b != nil && !errors.Is(b, ErrOverlap) {
		t.Fatalf("expected overlap: %v, %v", a, b)
	}
	bookings, _, err := s.Schedule(ctx, actor, workspace, r.ID)
	if err != nil || len(bookings) != 1 {
		t.Fatalf("schedule: %+v %v", bookings, err)
	}
}

func TestBookingRequiresAvailableWorkingHours(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "booking@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	r, err := s.Create(ctx, actor, owner.WorkspaceID, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, r.ID, []WorkingHours{{Weekday: 1, Start: "09:00", End: "17:00"}}); err != nil {
		t.Fatal(err)
	}
	duration := 60
	price := catalog.PriceOption{Mode: "manual", Currency: "USD", Variant: "Standard"}
	c := catalog.NewService(db, auth, nil)
	entry, err := c.Create(ctx, actor, owner.WorkspaceID, "Visit", &duration, []catalog.PriceOption{price})
	if err != nil {
		t.Fatal(err)
	}
	untimed, err := c.Create(ctx, actor, owner.WorkspaceID, "Untimed", nil, []catalog.PriceOption{price})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	hugeDuration := 307445735
	huge, err := c.Create(ctx, actor, owner.WorkspaceID, "Too long", &hugeDuration, []catalog.PriceOption{price})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, huge.ID, start.Add(3*time.Hour)); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("overflow duration: %v", err)
	}
	booking, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start)
	if err != nil || !booking.End.Equal(start.Add(time.Hour)) || booking.Status != "booked" {
		t.Fatalf("booking: %+v %v", booking, err)
	}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	history, err := s.BookingHistory(ctx, actor, owner.WorkspaceID, r.ID, booking.ID)
	if err != nil || len(history) != 1 || !history[0].OldStart.Equal(start) || !history[0].NewStart.Equal(start.Add(time.Hour)) || history[0].ActorID != owner.UserID {
		t.Fatalf("history: %+v %v", history, err)
	}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, start); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, start.Add(-time.Hour)); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("move outside hours: %v", err)
	}
	history, err = s.BookingHistory(ctx, actor, owner.WorkspaceID, r.ID, booking.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("refused move changed history: %+v %v", history, err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	es := engagement.NewService(db, config.SQLite, auth)
	stage, err := es.CreateStage(ctx, actor, owner.WorkspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := es.Create(ctx, actor, owner.WorkspaceID, engagement.Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, booking.ID, &e.ID); err != nil {
		t.Fatal(err)
	}
	linked, _, err := s.Schedule(ctx, actor, owner.WorkspaceID, r.ID)
	if err != nil || len(linked) != 1 || linked[0].EngagementID == nil || *linked[0].EngagementID != e.ID {
		t.Fatalf("linked: %+v %v", linked, err)
	}
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, booking.ID, nil); err != nil {
		t.Fatal(err)
	}
	created, err := s.BookWithEngagement(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(time.Hour), &e.ID)
	if err != nil || created.EngagementID == nil || *created.EngagementID != e.ID {
		t.Fatalf("created with engagement: %+v %v", created, err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	otherClient, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, otherActor, other, clients.Input{DisplayName: "Other client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherStage, err := es.CreateStage(ctx, otherActor, other, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := es.Create(ctx, otherActor, other, engagement.Input{Name: "Foreign", ClientID: otherClient.ID, StageID: otherStage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, booking.ID, &foreign.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign engagement: %v", err)
	}
	if _, err := s.BookWithEngagement(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(2*time.Hour), &foreign.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign booking: %v", err)
	}
	if _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(30*time.Minute)); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlap: %v", err)
	}
	if _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(-time.Hour)); !errors.Is(err, ErrOutsideHours) {
		t.Fatalf("hours: %v", err)
	}
	if _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, untimed.ID, start); !errors.Is(err, ErrNoDuration) {
		t.Fatalf("duration: %v", err)
	}
	bookings, zone, err := s.Schedule(ctx, actor, owner.WorkspaceID, r.ID)
	if err != nil || zone != "UTC" || len(bookings) != 2 || bookings[0].EntryName != "Visit" {
		t.Fatalf("schedule: %+v %q %v", bookings, zone, err)
	}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, start.Add(time.Hour)); !errors.Is(err, ErrOverlap) {
		t.Fatalf("move overlap: %v", err)
	}
	history, err = s.BookingHistory(ctx, actor, owner.WorkspaceID, r.ID, booking.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("overlap changed history: %+v %v", history, err)
	}
	bookings, _, err = s.Schedule(ctx, actor, owner.WorkspaceID, r.ID)
	if err != nil || len(bookings) != 2 || bookings[0].ID != booking.ID || bookings[0].ResourceID != r.ID || bookings[0].EntryID != entry.ID || !bookings[0].Start.Equal(start) {
		t.Fatalf("refused move changed booking: %+v %v", bookings, err)
	}
	if err := clients.NewService(db, config.SQLite, auth, 50).Delete(ctx, actor, owner.WorkspaceID, client.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	bookings, _, err = s.Schedule(ctx, actor, owner.WorkspaceID, r.ID)
	if err != nil || len(bookings) != 2 || bookings[1].EngagementID != nil {
		t.Fatalf("client deletion: %+v %v", bookings, err)
	}
	for _, outcome := range []BookingStatus{BookingCancelled, BookingNoShow, BookingDone} {
		if err := s.SetBookingStatus(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, outcome); err != nil {
			t.Fatal(err)
		}
		if outcome != BookingDone {
			if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, start.Add(time.Hour)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("move %s: %v", outcome, err)
			}
		}
		if err := s.SetBookingStatus(ctx, actor, owner.WorkspaceID, r.ID, booking.ID, BookingDone); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("terminal: %v", err)
		}
		booking, err = s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start)
		if outcome == BookingDone {
			if !errors.Is(err, ErrOverlap) {
				t.Fatalf("done must block: %v", err)
			}
		} else if err != nil {
			t.Fatalf("%s must free slot: %v", outcome, err)
		}
	}
}

func TestResourceAndWorkspaceTimeZone(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	resource, err := s.Create(ctx, actor, owner.WorkspaceID, " Room ", nil)
	if err != nil || resource.Name != "Room" || resource.UserID != nil {
		t.Fatalf("create: %+v, %v", resource, err)
	}
	hours := []WorkingHours{{Weekday: 1, Start: "09:00", End: "12:00"}, {Weekday: 1, Start: "13:00", End: "17:00"}}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, resource.ID, hours); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, resource.ID, []WorkingHours{{Weekday: 1, Start: "09:00", End: "12:00"}, {Weekday: 1, Start: "11:00", End: "13:00"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlap: %v", err)
	}
	if err := s.Update(ctx, actor, owner.WorkspaceID, resource.ID, "Person", &owner.UserID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, "America/Bogota"); err != nil {
		t.Fatal(err)
	}
	resources, zone, err := s.List(ctx, actor, owner.WorkspaceID)
	if err == nil && (len(resources) != 1 || len(resources[0].WorkingHours) != 2) {
		t.Fatalf("hours: %+v", resources)
	}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, resource.ID, []WorkingHours{{Weekday: 2, Start: "10:00", End: "11:00"}}); err != nil {
		t.Fatal(err)
	}
	replaced, _, err := s.List(ctx, actor, owner.WorkspaceID)
	if err != nil || len(replaced) != 1 || len(replaced[0].WorkingHours) != 1 || replaced[0].WorkingHours[0].Weekday != 2 {
		t.Fatalf("replacement: %+v, %v", replaced, err)
	}
	if err != nil || zone != "America/Bogota" || len(resources) != 1 || resources[0].Name != "Person" || resources[0].UserID == nil || *resources[0].UserID != owner.UserID {
		t.Fatalf("list: %+v, %q, %v", resources, zone, err)
	}
}

func TestAuthorizationValidationAndIsolation(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "A"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	resource, err := s.Create(ctx, actor, owner.WorkspaceID, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	duration := 60
	entry, err := catalog.NewService(db, auth, nil).Create(ctx, actor, owner.WorkspaceID, "Visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []identity.Role{identity.Admin, identity.Member, identity.Viewer} {
		inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		user, _, err := auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := auth.Authorize(ctx, user, owner.WorkspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		if list, _, err := s.List(ctx, principal, owner.WorkspaceID); err != nil || len(list) < 1 {
			t.Fatalf("%s list: %v %v", role, list, err)
		}
		_, err = s.Create(ctx, principal, owner.WorkspaceID, "New", nil)
		if role == identity.Admin && err != nil || role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s create: %v", role, err)
		}
		if err := s.SetTimeZone(ctx, principal, owner.WorkspaceID, "UTC"); role == identity.Admin && err != nil || role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s zone: %v", role, err)
		}
		if err := s.Update(ctx, principal, owner.WorkspaceID, resource.ID, "Room", nil); role == identity.Admin && err != nil || role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s update: %v", role, err)
		}
		if err := s.SetWorkingHours(ctx, principal, owner.WorkspaceID, resource.ID, []WorkingHours{{Weekday: 1, Start: "09:00", End: "17:00"}}); role == identity.Admin && err != nil || role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s hours: %v", role, err)
		}
		if role == identity.Member || role == identity.Viewer {
			_, err := s.Book(ctx, principal, owner.WorkspaceID, resource.ID, entry.ID, time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC))
			if role == identity.Member && err != nil || role == identity.Viewer && !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("%s booking: %v", role, err)
			}
			if bookings, _, err := s.Schedule(ctx, principal, owner.WorkspaceID, resource.ID); err != nil || len(bookings) != 1 {
				t.Fatalf("%s schedule: %+v %v", role, bookings, err)
			} else if err := s.SetEngagement(ctx, principal, owner.WorkspaceID, bookings[0].ID, nil); role == identity.Member && err != nil || role == identity.Viewer && !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("%s link: %v", role, err)
			} else if role == identity.Viewer {
				if err := s.MoveBooking(ctx, principal, owner.WorkspaceID, resource.ID, bookings[0].ID, time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)); !errors.Is(err, identity.ErrForbidden) {
					t.Fatalf("viewer move: %v", err)
				}
				if _, err := s.BookingHistory(ctx, principal, owner.WorkspaceID, resource.ID, bookings[0].ID); err != nil {
					t.Fatalf("viewer history: %v", err)
				}
				if err := s.SetBookingStatus(ctx, principal, owner.WorkspaceID, resource.ID, bookings[0].ID, BookingDone); !errors.Is(err, identity.ErrForbidden) {
					t.Fatalf("viewer status: %v", err)
				}
			}
		}
		if role == identity.Member {
			if err := s.Update(ctx, actor, owner.WorkspaceID, resource.ID, "Room", &user); err != nil {
				t.Fatal(err)
			}
			members, err := auth.ListMembers(ctx, actor)
			if err != nil {
				t.Fatal(err)
			}
			for _, member := range members {
				if member.UserID == user {
					if err := auth.RemoveMember(ctx, actor, member.ID, identity.AuditContext{}); err != nil {
						t.Fatal(err)
					}
				}
			}
			list, _, err := s.List(ctx, actor, owner.WorkspaceID)
			if err != nil || list[0].UserID != nil {
				t.Fatalf("removed member link: %v %v", list, err)
			}
		}
	}
	if err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, "Local"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid zone: %v", err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, "  ", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank: %v", err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "B"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if list, _, err := s.List(ctx, otherActor, other); err != nil || len(list) != 0 {
		t.Fatalf("foreign list: %v %v", list, err)
	}
	if err := s.Update(ctx, otherActor, other, resource.ID, "Stolen", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign update: %v", err)
	}
	if err := s.SetWorkingHours(ctx, otherActor, other, resource.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign hours: %v", err)
	}
}
