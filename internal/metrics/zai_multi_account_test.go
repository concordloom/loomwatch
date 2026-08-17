package metrics

import (
	"strconv"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// TestMetrics_ZaiExportsEveryAccount pins the reason the Z.ai multi-account work
// exists: with one key watched, a second subscription burned unobserved. The
// exporter has to publish a series per account, each under its own id.
func TestMetrics_ZaiExportsEveryAccount(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	primary, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("DefaultZaiAccountID: %v", err)
	}
	spare, err := s.CreateOrRestoreProviderAccount("zai", "spare")
	if err != nil {
		t.Fatalf("CreateOrRestoreProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	reset := now.Add(3 * time.Hour)
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now, TokensUsage: 100, TokensPercentage: 89,
		TokensNextResetTime: &reset,
	}, primary); err != nil {
		t.Fatalf("insert primary: %v", err)
	}
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: now, TokensUsage: 100, TokensPercentage: 12,
	}, spare.ID); err != nil {
		t.Fatalf("insert spare: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "zai",
		"quota_type": "tokens",
		"account_id": strconv.FormatInt(primary, 10),
	}, 89)
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", map[string]string{
		"provider":   "zai",
		"quota_type": "tokens",
		"account_id": strconv.FormatInt(spare.ID, 10),
	}, 12)
}
