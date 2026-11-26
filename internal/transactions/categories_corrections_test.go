package transactions

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestCategoryWithRefundReplacementAndMethodRename(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "category-correction@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	category, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Refunds", false)
	if err != nil {
		t.Fatal(err)
	}
	method, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, "", "Mobile payment", true)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Receipt", Direction: "in", AmountMinor: 500, Currency: "USD", PaymentMethod: method.ID})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Refund", Direction: "out", AmountMinor: 200, Currency: "USD", PaymentMethod: method.ID, CategoryID: &category.ID, RefundOfEntryID: receipt.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Void(ctx, actor, owner.WorkspaceID, refund.ID, "Wrong amount"); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Replacement", Direction: "out", AmountMinor: 150, Currency: "USD", PaymentMethod: method.ID, CategoryID: &category.ID, ReplacesEntryID: refund.ID, RefundOfEntryID: receipt.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, category.ID, "Customer refunds", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, category.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, method.ID, "Mobile", false); err != nil {
		t.Fatal(err)
	}
	counts, err := s.RefundCounts(ctx, actor, owner.WorkspaceID, []EntryID{receipt.ID})
	if err != nil || counts[receipt.ID] != 2 {
		t.Fatalf("refund count includes voided: %v %v", counts, err)
	}
	linked, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, RefundOfEntryID: receipt.ID})
	if err != nil || len(linked) != 2 || linked[0].ID != replacement.ID || linked[1].ID != refund.ID || linked[1].VoidedAt == "" {
		t.Fatalf("linked refunds: %+v %v", linked, err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	counts, err = s.RefundCounts(ctx, otherActor, other, []EntryID{receipt.ID})
	if err != nil || len(counts) != 0 {
		t.Fatalf("foreign receipt counts: %v %v", counts, err)
	}
	linked, err = s.List(ctx, otherActor, other, ListFilter{Page: 1, RefundOfEntryID: receipt.ID})
	if err != nil || len(linked) != 0 {
		t.Fatalf("foreign receipt refunds: %+v %v", linked, err)
	}
	if _, err := s.RefundCounts(ctx, otherActor, owner.WorkspaceID, []EntryID{receipt.ID}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("foreign principal: %v", err)
	}
	if _, err := s.RefundCounts(ctx, actor, owner.WorkspaceID, make([]EntryID, 101)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unbounded receipt count: %v", err)
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	for _, entry := range entries {
		if entry.ID == refund.ID || entry.ID == replacement.ID {
			if entry.CategoryID == nil || *entry.CategoryID != category.ID || entry.CategoryName != "Customer refunds" || entry.PaymentMethodName != "Mobile" || entry.RefundOfEntryID != receipt.ID {
				t.Fatalf("historical correction: %+v", entry)
			}
		}
	}
	if entries[0].ID != replacement.ID || entries[0].ReplacesEntryID != refund.ID {
		t.Fatalf("replacement link: %+v", entries[0])
	}
	for range 101 {
		if _, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Refund", Direction: "out", AmountMinor: 1, Currency: "USD", PaymentMethod: "cash", RefundOfEntryID: receipt.ID}); err != nil {
			t.Fatal(err)
		}
	}
	counts, err = s.RefundCounts(ctx, actor, owner.WorkspaceID, []EntryID{receipt.ID})
	if err != nil || counts[receipt.ID] != 103 {
		t.Fatalf("all refund counts: %v %v", counts, err)
	}
	first, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, RefundOfEntryID: receipt.ID})
	if err != nil || len(first) != 100 {
		t.Fatalf("first linked page: %d %v", len(first), err)
	}
	second, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 2, RefundOfEntryID: receipt.ID})
	if err != nil || len(second) != 3 || second[2].ID != refund.ID {
		t.Fatalf("second linked page: %+v %v", second, err)
	}
}
