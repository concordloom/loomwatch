package metrics

import (
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// The window length a quota runs on, exported rather than left to be guessed.
//
// The collector already decodes it. api.ZaiWindowLabel turns Z.ai's
// {unit, number} descriptor into "5h" / "weekly" / "monthly", is checked
// against live responses, and is already used to name the short series
// tokens_5h. The long window's descriptor sits in the same snapshot and was
// thrown away at the metric boundary: the gauge carries only
// {provider, quota_type, account_id}, so nothing downstream could tell a
// five-hour window from a week.
//
// Downstream then guessed. The dashboard derived the length statistically, as
// the longest time-to-reset seen over seven days, and on a stand with 2.08 days
// of history it reported "7 day" for a window it had never once seen reset -
// the number was the distance from the start of the series to the furthest
// reset among the provider's accounts, and with twelve hours of history the
// same column would have said "5 day" with equal confidence.
//
// The series name cannot stand in for this either. `tokens` is not the
// provider's word for the weekly window; it is a literal in this repository
// (internal/tracker/zai_tracker.go, internal/agent/zai_agent.go).

func TestMetrics_ZaiWindowLengthIsExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	reset := now.Add(3 * time.Hour)
	shortReset := now.Add(90 * time.Minute)

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now,
		// The long window: unit 6 number 1 is one week.
		TokensLimit:         6,
		TokensPercentage:    42,
		TokensNextResetTime: &reset,
		TokensUnit:          6,
		TokensNumber:        1,
		// The short window: unit 3 number 5 is five hours.
		TokensShortHasWindow:     true,
		TokensShortLimit:         15,
		TokensShortPercentage:    7,
		TokensShortNextResetTime: &shortReset,
		TokensShortUnit:          3,
		TokensShortNumber:        5,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	account, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	id := strconv.FormatInt(account, 10)

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	assertGaugeValue(t, families, "onwatch_quota_window_seconds", map[string]string{
		"provider": "zai", "quota_type": "tokens", "account_id": id,
	}, 7*24*3600)

	assertGaugeValue(t, families, "onwatch_quota_window_seconds", map[string]string{
		"provider": "zai", "quota_type": "tokens_5h", "account_id": id,
	}, 5*3600)
}

// The counterpart, and the reason this is a separate series rather than a
// label on the utilisation gauge: a window the provider did not describe must
// be ABSENT, not zero. Zero seconds is a quantity, and a consumer that reads it
// as one draws a window of no length - which is how the dashboard came to
// display a confident, invented number in the first place. Absence is the only
// value that says "not known".
func TestMetrics_ZaiUndescribedWindowIsAbsent(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	reset := now.Add(3 * time.Hour)

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt:          now,
		TokensLimit:         6,
		TokensPercentage:    42,
		TokensNextResetTime: &reset,
		// Unit 0 is not a code the provider documents, and zaiUnitHours
		// deliberately returns zero for it.
		TokensUnit:   0,
		TokensNumber: 1,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	account, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	id := strconv.FormatInt(account, 10)

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	// The quota itself is still exported - only its window is unknown.
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", map[string]string{
		"provider": "zai", "quota_type": "tokens", "account_id": id,
	}, 42)

	assertSeriesAbsent(t, families, "onwatch_quota_window_seconds", map[string]string{
		"provider": "zai", "quota_type": "tokens", "account_id": id,
	})
}

// MiniMax describes its windows by their two ends rather than by a code, so the
// length is the difference. Both windows are checked in one snapshot because
// the weekly one is the companion of the rolling one and they are emitted from
// the same loop: a change that labels one and forgets the other is the failure
// this covers.
func TestMetrics_MiniMaxWindowLengthIsExported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	windowStart := now.Add(-2 * time.Hour)
	windowEnd := now.Add(3 * time.Hour)
	weeklyStart := now.Add(-24 * time.Hour)
	weeklyEnd := now.Add(6 * 24 * time.Hour)

	acct, err := s.GetOrCreateProviderAccount("minimax", "default")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	if _, err := s.InsertMiniMaxSnapshot(&api.MiniMaxSnapshot{
		CapturedAt: now,
		Models: []api.MiniMaxModelQuota{{
			ModelName:   "general",
			Total:       100,
			Remain:      60,
			Used:        40,
			UsedPercent: 40,
			ResetAt:     &windowEnd,
			WindowStart: &windowStart,
			WindowEnd:   &windowEnd,

			HasWeeklyQuota:    true,
			WeeklyTotal:       100,
			WeeklyRemain:      89,
			WeeklyUsed:        11,
			WeeklyUsedPercent: 11,
			WeeklyResetAt:     &weeklyEnd,
			WeeklyWindowStart: &weeklyStart,
			WeeklyWindowEnd:   &weeklyEnd,
		}},
	}, acct.ID); err != nil {
		t.Fatalf("InsertMiniMaxSnapshot: %v", err)
	}

	id := strconv.FormatInt(acct.ID, 10)

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	assertGaugeValue(t, families, "onwatch_quota_window_seconds", map[string]string{
		"provider": "minimax", "quota_type": "general", "account_id": id,
	}, 5*3600)

	assertGaugeValue(t, families, "onwatch_quota_window_seconds", map[string]string{
		"provider": "minimax", "quota_type": "weekly_general", "account_id": id,
	}, 7*24*3600)
}

// assertSeriesAbsent fails when a series with exactly these labels exists. It
// is not assertMetricFamilyMissing: the family is expected to be there for
// other quotas, and the claim is about one series inside it.
func assertSeriesAbsent(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) {
	t.Helper()
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelsEqual(metric, labels) {
				t.Fatalf("metric %s with labels %v exists (value %v), want absent",
					name, labels, metric.GetGauge().GetValue())
			}
		}
	}
}
