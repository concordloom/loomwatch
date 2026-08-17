package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

// Fork change: Z.ai went multi-account. These tests pin the property the change
// exists for — one subscription's numbers must never be read as another's.

func newZaiAccount(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	acc, err := s.CreateOrRestoreProviderAccount("zai", name)
	if err != nil {
		t.Fatalf("CreateOrRestoreProviderAccount(%s): %v", name, err)
	}
	return acc.ID
}

func TestZaiSnapshotsAreIsolatedPerAccount(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	first := newZaiAccount(t, s, "first")
	second := newZaiAccount(t, s, "second")
	now := time.Now().UTC()

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now, TokensPercentage: 89, TokensUsage: 100,
	}, first); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	// Written later, so a query that ignores the account would return this one
	// for both accounts.
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now.Add(time.Minute), TokensPercentage: 3, TokensUsage: 100,
	}, second); err != nil {
		t.Fatalf("insert second: %v", err)
	}

	latestFirst, err := s.QueryLatestZai(first)
	if err != nil || latestFirst == nil {
		t.Fatalf("QueryLatestZai(first): %v", err)
	}
	if latestFirst.TokensPercentage != 89 {
		t.Fatalf("first account read %d%%, want 89%% — accounts are bleeding into each other",
			latestFirst.TokensPercentage)
	}

	latestSecond, err := s.QueryLatestZai(second)
	if err != nil || latestSecond == nil {
		t.Fatalf("QueryLatestZai(second): %v", err)
	}
	if latestSecond.TokensPercentage != 3 {
		t.Fatalf("second account read %d%%, want 3%%", latestSecond.TokensPercentage)
	}

	rows, err := s.QueryZaiRange(now.Add(-time.Hour), now.Add(time.Hour), first)
	if err != nil {
		t.Fatalf("QueryZaiRange: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("range returned %d rows for one account, want 1", len(rows))
	}
}

func TestZaiCyclesAreIsolatedPerAccount(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	first := newZaiAccount(t, s, "first")
	second := newZaiAccount(t, s, "second")
	now := time.Now().UTC()

	if _, err := s.CreateZaiCycle("tokens", now, nil, first); err != nil {
		t.Fatalf("CreateZaiCycle(first): %v", err)
	}

	// The second account has no cycle of its own yet: it must not inherit one.
	active, err := s.QueryActiveZaiCycle("tokens", second)
	if err != nil {
		t.Fatalf("QueryActiveZaiCycle(second): %v", err)
	}
	if active != nil {
		t.Fatal("second account sees the first account's active cycle")
	}

	if _, err := s.CreateZaiCycle("tokens", now, nil, second); err != nil {
		t.Fatalf("CreateZaiCycle(second): %v", err)
	}
	if err := s.UpdateZaiCycle("tokens", 42, 7, second); err != nil {
		t.Fatalf("UpdateZaiCycle(second): %v", err)
	}

	firstCycle, err := s.QueryActiveZaiCycle("tokens", first)
	if err != nil || firstCycle == nil {
		t.Fatalf("QueryActiveZaiCycle(first): %v", err)
	}
	if firstCycle.PeakValue != 0 {
		t.Fatalf("first account cycle peak %d, want 0 — the update leaked across accounts",
			firstCycle.PeakValue)
	}

	if err := s.CloseZaiCycle("tokens", now.Add(time.Hour), 42, 7, second); err != nil {
		t.Fatalf("CloseZaiCycle(second): %v", err)
	}
	stillActive, err := s.QueryActiveZaiCycle("tokens", first)
	if err != nil || stillActive == nil {
		t.Fatal("closing the second account's cycle also closed the first one")
	}
}

func TestZaiAccountResolutionFallsBackToDefault(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	defaultID, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	if defaultID <= 0 {
		t.Fatal("migration did not create the default Z.ai account")
	}

	// A caller with no account of its own (legacy path, fixture) must land on
	// the default account rather than filing rows under id 0, where nothing
	// would ever read them.
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: time.Now().UTC(), TokensPercentage: 55, TokensUsage: 100,
	}, 0); err != nil {
		t.Fatalf("insert with zero account: %v", err)
	}

	latest, err := s.QueryLatestZai(defaultID)
	if err != nil || latest == nil {
		t.Fatalf("QueryLatestZai(default): %v", err)
	}
	if latest.TokensPercentage != 55 {
		t.Fatalf("default account read %d%%, want 55%%", latest.TokensPercentage)
	}
}
