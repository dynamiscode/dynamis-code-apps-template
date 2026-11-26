package transactions

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestPostgresCategoryLifecycleAndConcurrency(t *testing.T) {
	ctx := context.Background()
	db := testdb.Postgres(t)
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "transactions-categories@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Office", false)
			results <- err
		}()
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrDuplicateCategory)) || (second == nil && errors.Is(first, ErrDuplicateCategory))) {
		t.Fatalf("concurrent category creation: %v, %v", first, second)
	}
	categories, err := s.Categories(ctx, actor, owner.WorkspaceID)
	if err != nil || len(categories) != 1 || categories[0].Name != "Office" {
		t.Fatalf("categories after concurrent creation: %+v %v", categories, err)
	}

	for i := range 5 {
		category, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, fmt.Sprintf("Race %d", i), false)
		if err != nil {
			t.Fatal(err)
		}
		input := Input{Description: "Entry", Direction: "out", AmountMinor: 100, Currency: "USD", PaymentMethod: "cash", CategoryID: &category.ID}
		start := make(chan struct{})
		archiveResult := make(chan error, 1)
		entryResult := make(chan error, 1)
		go func() {
			<-start
			_, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, category.ID, "", true)
			archiveResult <- err
		}()
		go func() { <-start; _, err := s.Create(ctx, actor, owner.WorkspaceID, input); entryResult <- err }()
		close(start)
		archiveErr, entryErr := <-archiveResult, <-entryResult
		if archiveErr != nil || (entryErr != nil && !errors.Is(entryErr, ErrInvalidInput)) {
			t.Fatalf("archive/create race: archive=%v entry=%v", archiveErr, entryErr)
		}
		if _, err := s.Create(ctx, actor, owner.WorkspaceID, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("new entry using archived category: %v", err)
		}
		if entryErr == nil {
			entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range entries {
				if entry.CategoryID != nil && *entry.CategoryID == category.ID && entry.CategoryName == category.Name {
					found = true
				}
			}
			if !found {
				t.Fatalf("successful entry lost archived category %q", category.Name)
			}
		}
	}
}

func TestPostgresConcurrentRefundCap(t *testing.T) {
	ctx := context.Background()
	db := testdb.Postgres(t)
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "transactions-refund@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	actor.AuthMethod = "test"
	service := NewService(db, auth)
	receipt, err := service.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Receipt", Direction: "in", AmountMinor: 100, Currency: "USD", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := service.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Refund", Direction: "out", AmountMinor: 60, Currency: "USD", PaymentMethod: "cash", RefundOfEntryID: receipt.ID})
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent refunds = %d, want 1", successes)
	}
	entries, err := service.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 2 || entries[0].Description != "Refund" || entries[1].Description != "Receipt" || entries[0].RefundOfEntryID != receipt.ID || entries[0].AmountMinor != 60 {
		t.Fatalf("entries after concurrent refunds: %+v, %v", entries, err)
	}
}

func TestPostgresFilterLargeIDs(t *testing.T) {
	ctx := context.Background()
	db := testdb.Postgres(t)
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "transactions-filter@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	for _, filter := range []ListFilter{{Page: 1, ClientID: 2147483648}, {Page: 1, CategoryID: 2147483648}} {
		entries, err := s.List(ctx, actor, owner.WorkspaceID, filter)
		if err != nil || len(entries) != 0 {
			t.Fatalf("large ID filter: %+v, %v", entries, err)
		}
	}
}
