package metrics

import (
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// TestMetrics_EveryQuotaAccountHasAccountInfo is the invariant behind the join
// this repository documents:
//
//	loomwatch_quota_utilization_percent
//	  * on (provider, account_id) group_left(account_name) loomwatch_account_info
//
// group_left drops left-hand series that find no match, and it does so without
// an error. So an account with quota series and no account_info series does not
// produce a gap in that query - it produces a query that silently answers for a
// subset and says nothing about the rest.
//
// account_info was emitted for the three providers that keep account rows and
// for nobody else, which meant the documented join returned a fraction of the
// data on any deployment running the other providers. It could not be seen on a
// stand configured with only multi-account providers, which is why it survived.
func TestMetrics_EveryQuotaAccountHasAccountInfo(t *testing.T) {
	s := seededStore(t)
	defer s.Close()

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	type key struct{ provider, account string }

	quotaAccounts := map[key]bool{}
	for _, mm := range metricsOf(families, "loomwatch_quota_utilization_percent") {
		l := labelsOf(mm)
		quotaAccounts[key{l["provider"], l["account_id"]}] = true
	}
	if len(quotaAccounts) == 0 {
		t.Fatal("no quota series were exported at all; the fixture proves nothing")
	}

	known := map[key]bool{}
	for _, mm := range metricsOf(families, "loomwatch_account_info") {
		l := labelsOf(mm)
		known[key{l["provider"], l["account_id"]}] = true
	}

	for k := range quotaAccounts {
		if !known[k] {
			t.Errorf("provider %q account %q reports a quota but has no account_info series: "+
				"the documented join drops it without saying so", k.provider, k.account)
		}
	}
}

// TestMetrics_AccountInfoExcludesDeletedAccounts covers the other half.
//
// The metrics layer was the only consumer in the repository reading accounts
// without filtering deleted_at, so an account removed through the panel kept
// exporting for as long as it still had snapshots - showing up in dashboards
// and, once ownership alerting exists, being reported as an account nobody
// owns.
func TestMetrics_AccountInfoExcludesDeletedAccounts(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	live, err := s.GetOrCreateProviderAccount("zai", "live")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount(live): %v", err)
	}
	gone, err := s.GetOrCreateProviderAccount("zai", "gone")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount(gone): %v", err)
	}

	now := time.Now().UTC()
	for _, id := range []int64{live.ID, gone.ID} {
		if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
			CapturedAt:       now,
			TokensLimit:      6,
			TokensPercentage: 40,
		}, id); err != nil {
			t.Fatalf("InsertZaiSnapshot(%d): %v", id, err)
		}
	}

	if err := s.MarkProviderAccountDeleted("zai", "gone"); err != nil {
		t.Fatalf("MarkProviderAccountDeleted: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	for _, mm := range metricsOf(families, "loomwatch_account_info") {
		if labelsOf(mm)["account_name"] == "gone" {
			t.Error("a deleted account is still exported; removing it in the panel " +
				"has to remove it from the metrics too, or the removal is cosmetic")
		}
	}
}

// TestMetrics_ApiIntegrationsIsNotAnAccount keeps the fix above from
// degenerating into "emit account_info for everything that reports".
//
// api_integrations is the collector's own ingestion path. It reports under a
// provider/account_id pair like everything else and there is nobody to own it,
// so an ownership check that treats it as an account fires forever against a
// mapping that is in fact complete.
func TestMetrics_ApiIntegrationsIsNotAnAccount(t *testing.T) {
	s := seededStore(t)
	defer s.Close()

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	for _, mm := range metricsOf(families, "loomwatch_account_info") {
		if labelsOf(mm)["provider"] == "api_integrations" {
			t.Error("api_integrations has an account_info series; it is an ingestion " +
				"path, not a subscription anybody can own")
		}
	}
}

// seededStore fills a store with one snapshot for a spread of providers: two
// that keep account rows and several that report under the default account.
func seededStore(t *testing.T) *store.Store {
	t.Helper()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	now := time.Now().UTC()

	if _, err := s.InsertCopilotSnapshot(&api.CopilotSnapshot{
		CapturedAt:  now,
		CopilotPlan: "individual_pro",
		Quotas: []api.CopilotQuota{{
			Name: "premium_interactions", Entitlement: 100, Remaining: 25, PercentRemaining: 25,
		}},
	}); err != nil {
		t.Fatalf("InsertCopilotSnapshot: %v", err)
	}

	if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{
		CapturedAt: now,
		PlanType:   "pro",
		Quotas:     []api.CodexQuota{{Name: "five_hour", Utilization: 35}},
	}); err != nil {
		t.Fatalf("InsertCodexSnapshot: %v", err)
	}

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now, TokensLimit: 6, TokensPercentage: 68,
	}, 0); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	return s
}

func metricsOf(families []*dto.MetricFamily, name string) []*dto.Metric {
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()
		}
	}
	return nil
}

func labelsOf(m *dto.Metric) map[string]string {
	out := map[string]string{}
	for _, l := range m.GetLabel() {
		out[l.GetName()] = l.GetValue()
	}
	return out
}

// scrapeInto runs one scrape and returns what the registry holds afterwards.
func scrapeInto(t *testing.T, s *store.Store) []*dto.MetricFamily {
	t.Helper()
	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	return families
}

func accountLabel(id int64) string {
	return strconv.FormatInt(id, 10)
}
