package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Fork change: during the brand rename the exporter has to publish both series
// names for a while. Otherwise the Grafana rules that select on onwatch_* would
// have gone silent at the moment of the rollout - and those are exactly the
// rules that guard quota exhaustion.
func TestMetrics_PublishesBothNamesDuringRename(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acc, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: time.Now().UTC(), TokensUsage: 100, TokensPercentage: 91,
	}, acc); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	names := map[string]bool{}
	for _, f := range families {
		names[f.GetName()] = true
	}

	for _, want := range []string{
		"loomwatch_quota_utilization_percent",
		"onwatch_quota_utilization_percent",
	} {
		if !names[want] {
			t.Fatalf("series %s is missing - both names are published during the rename", want)
		}
	}

	// The deprecated series must be marked, otherwise it is unclear which one to remove.
	for _, f := range families {
		if f.GetName() == "onwatch_quota_utilization_percent" {
			if !strings.Contains(f.GetHelp(), "deprecated") {
				t.Fatalf("deprecated series carries no marker: %q", f.GetHelp())
			}
		}
	}
}
