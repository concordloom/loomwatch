package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Fork change: при переименовании бренда экспортёр обязан какое-то время
// публиковать оба имени ряда. Иначе правила Grafana, отобранные по onwatch_*,
// замолчали бы в момент выката — а именно они сторожат исчерпание квоты.
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
			t.Fatalf("ряд %s отсутствует — при переименовании публикуются оба имени", want)
		}
	}

	// У устаревшего ряда должна быть пометка, иначе непонятно, какой снимать.
	for _, f := range families {
		if f.GetName() == "onwatch_quota_utilization_percent" {
			if !strings.Contains(f.GetHelp(), "deprecated") {
				t.Fatalf("устаревший ряд без пометки: %q", f.GetHelp())
			}
		}
	}
}
