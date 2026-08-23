package metrics

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Five providers were collected, stored and shown on the dashboard while
// exporting nothing at all: Scrape called eleven functions and none of them was
// theirs. Nothing in the chart could alert on them, and the front page counted
// them among the providers this fork watches.
//
// One test per provider rather than a loop over a table: each has its own
// storage shape, and a loop that skipped one silently would look identical to a
// loop that covered it.

func TestMetrics_SyntheticQuotasExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	renews := now.Add(2 * time.Hour)
	if _, err := s.InsertSnapshot(&api.Snapshot{
		CapturedAt: now,
		Sub:        api.QuotaInfo{Limit: 200, Requests: 50, RenewsAt: renews},
		Search:     api.QuotaInfo{Limit: 100, Requests: 0, RenewsAt: renews},
		ToolCall:   api.QuotaInfo{Limit: 400, Requests: 100, RenewsAt: renews},
	}); err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}

	families := scrapeInto(t, s)

	// 50 of 200 is a quarter of the plan's own limit.
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", map[string]string{
		"provider": "synthetic", "quota_type": "sub", "account_id": "default",
	}, 25)
	// Nothing consumed is a reading, not an absence: the series has to exist.
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", map[string]string{
		"provider": "synthetic", "quota_type": "search", "account_id": "default",
	}, 0)
	assertGaugeValue(t, families, "loomwatch_quota_reset_timestamp_seconds", map[string]string{
		"provider": "synthetic", "quota_type": "tool", "account_id": "default",
	}, float64(renews.Unix()))
}

func TestMetrics_CursorQuotasExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	resets := now.Add(90 * time.Minute)
	if _, err := s.InsertCursorSnapshot(&api.CursorSnapshot{
		CapturedAt: now,
		PlanName:   "pro",
		Quotas: []api.CursorQuota{
			{Name: "requests", Used: 30, Limit: 100, Utilization: 30, ResetsAt: &resets},
		},
	}); err != nil {
		t.Fatalf("InsertCursorSnapshot: %v", err)
	}

	families := scrapeInto(t, s)
	labels := map[string]string{"provider": "cursor", "quota_type": "requests", "account_id": "default"}
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", labels, 30)
	assertGaugeValue(t, families, "loomwatch_quota_reset_timestamp_seconds", labels, float64(resets.Unix()))
}

func TestMetrics_OpenCodeQuotasExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	if _, err := s.InsertOpenCodeSnapshot(&api.OpenCodeSnapshot{
		CapturedAt: now,
		PlanName:   "team",
		Quotas: []api.OpenCodeQuota{
			{Name: "credits", Used: 8, Limit: 10, Utilization: 80},
		},
	}); err != nil {
		t.Fatalf("InsertOpenCodeSnapshot: %v", err)
	}

	families := scrapeInto(t, s)
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", map[string]string{
		"provider": "opencode", "quota_type": "credits", "account_id": "default",
	}, 80)
}

func TestMetrics_KimiQuotasExportedPerAccount(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("kimi", "work")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	if _, err := s.InsertKimiSnapshot(&api.KimiSnapshot{
		CapturedAt: now,
		AccountID:  acct.ID,
		Quotas:     []api.KimiQuota{{Name: "weekly", Utilization: 45, Limit: 100, Used: 45}},
	}); err != nil {
		t.Fatalf("InsertKimiSnapshot: %v", err)
	}

	families := scrapeInto(t, s)
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", map[string]string{
		"provider": "kimi", "quota_type": "weekly", "account_id": accountLabel(acct.ID),
	}, 45)
}

func TestMetrics_GrokQuotasExportedPerAccount(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("grok", "personal")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	if _, err := s.InsertGrokSnapshot(&api.GrokSnapshot{
		CapturedAt: now,
		AccountID:  acct.ID,
		Quotas:     []api.GrokQuota{{Name: "messages", Utilization: 12}},
	}); err != nil {
		t.Fatalf("InsertGrokSnapshot: %v", err)
	}

	families := scrapeInto(t, s)
	assertGaugeValue(t, families, "loomwatch_quota_utilization_percent", map[string]string{
		"provider": "grok", "quota_type": "messages", "account_id": accountLabel(acct.ID),
	}, 12)
}
