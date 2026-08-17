package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Fork change: the Z.ai tab could only ever show one subscription, so a second
// one burned unnoticed. These tests pin that every account reaches the UI and
// that a per-account view really answers for the account it was asked about.

func newZaiTestHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	h := NewHandler(s, nil, nil, nil, &config.Config{ZaiAPIKey: "placeholder"})
	return h, s
}

func zaiSnapshot(capturedAt time.Time, percent int) *api.ZaiSnapshot {
	reset := capturedAt.Add(48 * time.Hour)
	return &api.ZaiSnapshot{
		CapturedAt:          capturedAt,
		TokensUsage:         100,
		TokensCurrentValue:  float64(percent),
		TokensPercentage:    percent,
		TokensNextResetTime: &reset,
	}
}

func TestZaiAccountsUsage_ReturnsEveryAccount(t *testing.T) {
	h, s := newZaiTestHandler(t)

	first, err := s.CreateOrRestoreProviderAccount("zai", "main-max")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := s.CreateOrRestoreProviderAccount("zai", "spare-max")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := s.InsertZaiSnapshot(zaiSnapshot(now, 91), first.ID); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if _, err := s.InsertZaiSnapshot(zaiSnapshot(now, 7), second.ID); err != nil {
		t.Fatalf("insert second: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/zai/accounts/usage", nil)
	w := httptest.NewRecorder()
	h.ZaiAccountsUsage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}

	var resp struct {
		Accounts []struct {
			AccountID   int64  `json:"accountId"`
			AccountName string `json:"accountName"`
			Quotas      []struct {
				Name         string  `json:"name"`
				UsagePercent float64 `json:"usagePercent"`
				ResetAt      string  `json:"resetAt"`
			} `json:"quotas"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byName := map[string]float64{}
	for _, acc := range resp.Accounts {
		for _, q := range acc.Quotas {
			if q.Name == "tokens" {
				byName[acc.AccountName] = q.UsagePercent
			}
		}
	}

	// The default account created by the migration has no snapshots, so it is
	// present with zeros; the two seeded ones must carry their own numbers.
	if got := byName["main-max"]; got != 91 {
		t.Fatalf("main-max reported %v%%, want 91 — accounts are not separated", got)
	}
	if got := byName["spare-max"]; got != 7 {
		t.Fatalf("spare-max reported %v%%, want 7", got)
	}
	if len(resp.Accounts) < 2 {
		t.Fatalf("returned %d accounts, want every one of them", len(resp.Accounts))
	}
}

func TestZaiCurrentHonoursAccountQueryParam(t *testing.T) {
	h, s := newZaiTestHandler(t)

	spare, err := s.CreateOrRestoreProviderAccount("zai", "spare-max")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defaultID, err := s.DefaultZaiAccountID()
	if err != nil {
		t.Fatalf("default account: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := s.InsertZaiSnapshot(zaiSnapshot(now, 91), defaultID); err != nil {
		t.Fatalf("insert default: %v", err)
	}
	if _, err := s.InsertZaiSnapshot(zaiSnapshot(now, 7), spare.ID); err != nil {
		t.Fatalf("insert spare: %v", err)
	}

	percentFor := func(query string) float64 {
		t.Helper()
		current := h.buildZaiCurrent(h.parseZaiAccountID(httptest.NewRequest("GET", "/api/current/zai"+query, nil)))
		tokens, ok := current["tokensLimit"].(map[string]interface{})
		if !ok {
			t.Fatalf("no tokensLimit in response for %q", query)
		}
		pct, _ := tokens["percent"].(float64)
		return pct
	}

	if got := percentFor(""); got != 91 {
		t.Fatalf("no account param gave %v%%, want the default account's 91", got)
	}
	if got := percentFor("?account=" + strconv.FormatInt(spare.ID, 10)); got != 7 {
		t.Fatalf("account=spare gave %v%%, want 7 — the view ignores the requested account", got)
	}
}
