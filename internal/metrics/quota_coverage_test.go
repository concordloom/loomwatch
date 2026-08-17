package metrics

import (
	"strconv"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// TestMetrics_ZaiTokenQuotaExportedWhenUsageIsZero reproduces issue #112.
//
// The Z.ai scrape guarded both quota series on Usage > 0. Usage is the amount
// consumed, so the guard suppressed the series exactly when nothing had been
// consumed yet - and, more importantly, in plans where the provider reports a
// meaningful Percentage while leaving Usage at zero. The observed production
// case had tokens_usage=0 alongside tokens_percentage=68: the one number that
// actually moves was never exported, and no reset timestamp was emitted either,
// so burn-rate prediction was impossible.
//
// A quota series must exist when the provider declares the quota. Zero
// consumption is a legitimate value, not an absence of data.
func TestMetrics_ZaiTokenQuotaExportedWhenUsageIsZero(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	futureReset := now.Add(3 * time.Hour)

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now,
		// The plan declares both limits...
		TimeLimit:   5,
		TokensLimit: 6,
		// ...but nothing has been consumed in the Usage sense.
		TimeUsage:   0,
		TokensUsage: 0,
		// The percentages are what the dashboard shows and what we alert on.
		TimePercentage:      0,
		TokensPercentage:    68,
		TokensNextResetTime: &futureReset,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	// Fork change: Z.ai series are labelled with the real provider account id
	// now that the provider is multi-account, so the expectation resolves the
	// default account instead of the old single-account sentinel.
	zaiAccount, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	zaiAccountLabel := strconv.FormatInt(zaiAccount, 10)

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	tokenLabels := map[string]string{
		"provider":   "zai",
		"quota_type": "tokens",
		"account_id": zaiAccountLabel,
	}
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", tokenLabels, 68)
	assertGaugeValue(t, families, "onwatch_quota_reset_timestamp_seconds", tokenLabels, float64(futureReset.Unix()))

	// The time quota is declared too, so its series must exist at its real
	// value rather than vanish because consumption is zero.
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "zai",
		"quota_type": "time",
		"account_id": zaiAccountLabel,
	}, 0)
}

// TestMetrics_ZaiUndeclaredQuotaIsNotExported is the counterpart: a quota the
// plan does not declare must stay absent. Without this the fix above would
// degenerate into "always emit", and a plan without a token limit would report
// a permanent, meaningless zero that alerts would read as healthy.
func TestMetrics_ZaiUndeclaredQuotaIsNotExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: time.Now().UTC(),
		TimeLimit:  5,
		// No TOKENS_LIMIT in the provider response at all.
		TokensLimit:      0,
		TokensPercentage: 0,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	zaiAccount, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	zaiAccountLabel := strconv.FormatInt(zaiAccount, 10)

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	if hasGaugeMetric(families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "zai",
		"quota_type": "tokens",
		"account_id": zaiAccountLabel,
	}) {
		t.Fatal("token quota series exported although the plan declares no token limit")
	}
}

// TestMetrics_MiniMaxWeeklyQuotaExported covers the second gap: MiniMax plans
// carry a weekly quota alongside the rolling five-hour one, the store already
// reads it, and the scrape dropped it because it iterated only the per-model
// values. The weekly window is the one that burns over days, so its absence
// left long-horizon exhaustion unobservable.
func TestMetrics_MiniMaxWeeklyQuotaExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("minimax", "primary")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}
	accountID := strconv.FormatInt(acct.ID, 10)

	now := time.Now().UTC()
	cycleReset := now.Add(2 * time.Hour)
	weeklyReset := now.Add(40 * time.Hour)

	if _, err := s.InsertMiniMaxSnapshot(&api.MiniMaxSnapshot{
		CapturedAt: now,
		Models: []api.MiniMaxModelQuota{{
			ModelName:   "general",
			Total:       100,
			Remain:      88,
			Used:        12,
			UsedPercent: 12,
			ResetAt:     &cycleReset,

			WeeklyTotal:       100,
			WeeklyRemain:      20,
			WeeklyUsed:        80,
			WeeklyUsedPercent: 80,
			WeeklyResetAt:     &weeklyReset,
			HasWeeklyQuota:    true,
		}},
	}, acct.ID); err != nil {
		t.Fatalf("InsertMiniMaxSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	// The existing five-hour series must keep its name and value.
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "minimax",
		"quota_type": "general",
		"account_id": accountID,
	}, 12)

	weeklyLabels := map[string]string{
		"provider":   "minimax",
		"quota_type": "weekly_general",
		"account_id": accountID,
	}
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", weeklyLabels, 80)
	assertGaugeValue(t, families, "onwatch_quota_reset_timestamp_seconds", weeklyLabels, float64(weeklyReset.Unix()))
}

// TestMetrics_MiniMaxWithoutWeeklyQuotaHasNoWeeklySeries guards the other
// direction. Accounts created before the weekly window existed report zeros,
// and emitting a flat zero series for them would be worse than silence: an
// alert on utilization would read it as a healthy account forever.
func TestMetrics_MiniMaxWithoutWeeklyQuotaHasNoWeeklySeries(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("minimax", "legacy")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	cycleReset := now.Add(time.Hour)

	if _, err := s.InsertMiniMaxSnapshot(&api.MiniMaxSnapshot{
		CapturedAt: now,
		Models: []api.MiniMaxModelQuota{{
			ModelName:   "general",
			Total:       100,
			Remain:      70,
			Used:        30,
			UsedPercent: 30,
			ResetAt:     &cycleReset,
			// No weekly window on this account.
		}},
	}, acct.ID); err != nil {
		t.Fatalf("InsertMiniMaxSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	if hasGaugeMetric(families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "minimax",
		"quota_type": "weekly_general",
		"account_id": strconv.FormatInt(acct.ID, 10),
	}) {
		t.Fatal("weekly series exported for an account that has no weekly quota")
	}
}
