package metrics

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// The product carried two definitions of "stale" that disagreed.
//
// agent_healthy went to zero after two poll intervals; the
// LoomwatchCollectorNotPolling rule in the chart fires after five. So the health
// flag - and every panel drawn from it - went red three intervals before the
// alert had an opinion, and recovered on its own before anyone could act.
//
// Two intervals also means one missed poll is a sick collector. Against a
// third-party API that occasionally refuses a request, that is not a diagnosis,
// it is a flap: measured on a stand polling Z.ai, the accounts reported healthy
// for 80% of the day and stale for the rest with nothing wrong.
func TestMetrics_HealthyAfterOneMissedPoll(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	const pollInterval = time.Minute

	// Three intervals: one poll was missed and the next one landed. A collector
	// that skipped a beat is not a collector that stopped.
	captured := time.Now().UTC().Add(-3 * pollInterval)
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: captured, TokensLimit: 6, TokensPercentage: 40,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, pollInterval)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	acct, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	assertGaugeValue(t, families, "loomwatch_agent_healthy", map[string]string{
		"provider": "zai", "account_id": accountLabel(acct),
	}, 1)
}

// The counterpart: past the threshold the flag has to fall, or it reports
// nothing at all. The boundary is the same one the alert uses, so the panel and
// the alert cannot disagree about whether a collector has stopped.
func TestMetrics_StaleAfterTheAlertsOwnThreshold(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	const pollInterval = time.Minute

	captured := time.Now().UTC().Add(-(staleIntervals + 1) * pollInterval)
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: captured, TokensLimit: 6, TokensPercentage: 40,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, pollInterval)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	acct, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	assertGaugeValue(t, families, "loomwatch_agent_healthy", map[string]string{
		"provider": "zai", "account_id": accountLabel(acct),
	}, 0)
}

// staleIntervals is pinned here as well as at its definition, so that changing
// it on one side of the product and not the other fails a test rather than
// producing a panel and an alert that quietly mean different things.
func TestStaleIntervalsMatchesTheChartRule(t *testing.T) {
	if staleIntervals != 5 {
		t.Errorf("staleIntervals is %d; the chart's LoomwatchCollectorNotPolling rule "+
			"uses pollInterval * 5, and the two have to agree or the panel goes red "+
			"before the alert has an opinion", staleIntervals)
	}
}
