package store

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTripLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u := &User{Email: "a@b.c", Name: "A", Role: "admin"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	tr := &Trip{OwnerID: u.ID, Name: "Test", StartDate: "2026-08-01", EndDate: "2026-08-10", Currency: "EUR", Dawarich: true, Immich: true}
	if err := s.CreateTrip(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if tr.Days != 10 {
		t.Fatalf("days = %d, want 10", tr.Days)
	}
	p := &Place{TripID: tr.ID, Name: "Beach", Category: "beach", DayDate: "2026-08-02"}
	if err := s.CreatePlace(ctx, p); err != nil {
		t.Fatal(err)
	}
	e := &Expense{TripID: tr.ID, Name: "Hotel", Amount: 10000, Currency: "EUR", AmountHome: 10000, Kind: "planned"}
	if err := s.CreateExpense(ctx, e); err != nil {
		t.Fatal(err)
	}
	f := &Expense{TripID: tr.ID, Name: "Dinner", Amount: 4000, Currency: "EUR", AmountHome: 4000, Kind: "actual", Source: "firefly", ExternalID: "12"}
	if created, err := s.UpsertExternalExpense(ctx, f); err != nil || !created {
		t.Fatalf("upsert create: %v %v", created, err)
	}
	f.Amount = 4500
	f.AmountHome = 4500
	if created, err := s.UpsertExternalExpense(ctx, f); err != nil || created {
		t.Fatalf("upsert update: %v %v", created, err)
	}
	c, err := s.TripCounts(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Places != 1 || c.Planned != 10000 || c.Actual != 4500 {
		t.Fatalf("counts = %+v", c)
	}
	trips, err := s.ListTrips(ctx, TripFilter{UserID: u.ID})
	if err != nil || len(trips) != 1 || trips[0].Role != "owner" {
		t.Fatalf("list: %v %+v", err, trips)
	}
	if err := s.DeleteTrip(ctx, tr.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPlace(ctx, tr.ID, p.ID); err != ErrNotFound {
		t.Fatalf("place should cascade, got %v", err)
	}
}

func TestDaysBetween(t *testing.T) {
	if DaysBetween("2026-01-01", "2026-01-01") != 1 {
		t.Fatal("same day = 1")
	}
	if DaysBetween("", "2026-01-01") != 0 {
		t.Fatal("unset = 0")
	}
	if DaysBetween("2026-01-05", "2026-01-01") != 0 {
		t.Fatal("reversed = 0")
	}
}
