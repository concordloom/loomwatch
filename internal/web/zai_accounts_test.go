package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// Fork change / adversarial finding: buildZaiInsights took an accountID but
// still queried history for the default account, so a quiet subscription showed
// the busy one's rate, projection and trend under its own name. That is exactly
// the substitution multi-account support exists to prevent, so it gets a test.
func TestZaiInsightsUseTheRequestedAccount(t *testing.T) {
	h, s := newZaiTestHandler(t)

	// The load is placed in the default account specifically: the substitution
	// happened onto it, so a test that puts the spend in some other account
	// does not catch the defect - the history simply comes back empty.
	busyID, err := s.DefaultZaiAccountID()
	if err != nil || busyID == 0 {
		t.Fatalf("default account: %v", err)
	}
	busy := struct{ ID int64 }{ID: busyID}
	idle, err := s.CreateOrRestoreProviderAccount("zai", "idle")
	if err != nil {
		t.Fatalf("create idle: %v", err)
	}

	// Only the busy account consumes anything; the idle one never moves.
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 10; i++ {
		at := now.Add(time.Duration(-9+i) * time.Hour)
		snap := zaiSnapshot(at, 10*i)
		snap.TokensCurrentValue = float64(100000 * i)
		snap.TokensUsage = 1000000
		if _, err := s.InsertZaiSnapshot(snap, busy.ID); err != nil {
			t.Fatalf("insert busy: %v", err)
		}
		idleSnap := zaiSnapshot(at, 0)
		idleSnap.TokensCurrentValue = 0
		idleSnap.TokensUsage = 1000000
		if _, err := s.InsertZaiSnapshot(idleSnap, idle.ID); err != nil {
			t.Fatalf("insert idle: %v", err)
		}
	}

	resp := h.buildZaiInsights(map[string]bool{}, idle.ID)
	for _, item := range resp.Insights {
		switch item.Key {
		case "token_rate", "projected_usage", "trend_24h", "usage_7d":
			t.Fatalf("idle account reported %q (%s: %s) — history came from another account",
				item.Key, item.Metric, item.Desc)
		}
	}
}

// The same guard for the logging history table: the consumption column has to
// carry consumption, not the budget. Reporting the budget inverted the signal —
// the exhausted subscription read as idle and the untouched one as the heavy
// consumer.
func TestZaiLoggingHistoryReportsConsumptionNotBudget(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "sub")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	snap := zaiSnapshot(time.Now().UTC().Add(-time.Hour).Truncate(time.Second), 91)
	snap.TokensUsage = 140000       // window budget
	snap.TokensCurrentValue = 12345 // actual spend
	if _, err := s.InsertZaiSnapshot(snap, acc.ID); err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest("GET",
		"/api/logging-history?provider=zai&limit=10&range=1&account="+strconv.FormatInt(acc.ID, 10), nil)
	w := httptest.NewRecorder()
	h.LoggingHistory(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "12345") {
		t.Fatalf("consumption 12345 missing from the table payload: %s", body[:min(len(body), 400)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Issue #21: account metadata is the credential store, and it is updated
// read-modify-write. The merge used to start from an empty map whenever the
// stored blob failed to parse, so an update carrying only base_url wrote back
// metadata with no api_key and destroyed the last copy of the key. These three
// tests pin the whole boundary: a blob that cannot be parsed must abort the
// write, a blob that parses must still merge, and a blob holding a value this
// handler does not understand must survive the merge unchanged.

func TestZaiAccountUpdateRefusesToOverwriteUnparsableMetadata(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "broken-meta")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A truncated blob - a half-written metadata column. It still carries the
	// key as text, and once the JSON stops parsing that text is the only copy
	// of the credential left anywhere.
	const corrupt = `{"api_key":"zai-secret-key","base_url":"https://old"`
	if err := s.UpdateProviderAccountMetadata(acc.ID, corrupt); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	// The harm first, the mechanism second: what matters is that the key is
	// still there, not which status code said so.
	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	if !strings.Contains(after.Metadata, "zai-secret-key") {
		t.Fatalf("api_key gone after an update that never carried one: metadata is now %q", after.Metadata)
	}
	if w.Code < 400 {
		t.Fatalf("status %d, want a refusal: the handler accepted an update it could not merge", w.Code)
	}
}

// Checking the parse error is not on its own enough, because one damaged value
// does not produce an error at all: JSON null decodes into a nil map with err
// == nil, and the next assignment into it panics. Nothing in internal/web
// recovers from a panic, so that would take the connection down rather than
// return a status. The metadata carries no key, so the update is allowed - the
// point of the test is that it returns instead of crashing.
func TestZaiAccountUpdateSurvivesNullMetadata(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "null-meta")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID, `null`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 - null metadata holds no credential, so there is nothing to refuse", w.Code)
	}

	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(after.Metadata), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after the update: %q", after.Metadata)
	}
	if meta["base_url"] != "https://new" {
		t.Fatalf("base_url is %v, want the updated value", meta["base_url"])
	}
}

// The metadata is parsed before the rename and the restore run, so a request
// that cannot be merged applies nothing at all. Without this test that ordering
// is just an assertion in a comment: the store has no transaction - every write
// is its own db.Exec - so a refusal raised after the rename would leave the
// account renamed and the caller told the update failed.
func TestZaiAccountUpdateAppliesNothingWhenMetadataCannotBeMerged(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "untouched")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID,
		`{"api_key":"zai-secret-key","base_url":"https://old"`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	if err := s.MarkProviderAccountDeletedByID(acc.ID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"name":"renamed","restore":true,"base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", w.Code)
	}

	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	if after.Name != "untouched" {
		t.Fatalf("account was renamed to %q by a request that was refused", after.Name)
	}
	if after.DeletedAt == nil {
		t.Fatalf("account was restored by a request that was refused")
	}
	if !strings.Contains(after.Metadata, "zai-secret-key") {
		t.Fatalf("api_key gone from a refused request: metadata is now %q", after.Metadata)
	}
}

// The way out of the state the test above pins. Refusing every update to a
// damaged row would leave the account unrepairable through the API, so a
// request that carries a key of its own is allowed to replace the blob: there
// is no credential left to lose at that point.
func TestZaiAccountUpdateRepairsUnparsableMetadataWhenGivenAKey(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "repairable")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID,
		`{"api_key":"zai-secret-key","base_url":"https://old"`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"api_key":"zai-replacement-key","base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 - an update carrying its own key must be able to repair the row", w.Code)
	}

	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(after.Metadata), &meta); err != nil {
		t.Fatalf("metadata is still not valid JSON after the repair: %q", after.Metadata)
	}
	if meta["api_key"] != "zai-replacement-key" {
		t.Fatalf("api_key is %v, want the key the request supplied", meta["api_key"])
	}
	if meta["base_url"] != "https://new" {
		t.Fatalf("base_url is %v, want the updated value", meta["base_url"])
	}
}

// The positive control for the test above. Without it a refusal for some
// unrelated reason - a missing store, a rejected id, an account lookup that
// fails - would turn that test green while proving nothing.
func TestZaiAccountUpdateMergesIntoWellFormedMetadata(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "good-meta")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID,
		`{"api_key":"zai-secret-key","base_url":"https://old"}`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 - the identical request on readable metadata must succeed", w.Code)
	}

	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(after.Metadata), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after the update: %q", after.Metadata)
	}
	if meta["api_key"] != "zai-secret-key" {
		t.Fatalf("api_key is %v, want it preserved by the merge", meta["api_key"])
	}
	if meta["base_url"] != "https://new" {
		t.Fatalf("base_url is %v, want the updated value", meta["base_url"])
	}
}

// The subcase that decides how a parse failure has to be detected. Decoding
// into map[string]string reports an error for metadata that is perfectly valid
// JSON but holds a non-string value, while the agent manager - which decodes
// the same blob into a struct - takes the key from it happily and polls the
// account. So this account is live: its update must not be refused, and the
// field the handler does not understand must come back out as it went in
// rather than flattened to an empty string.
func TestZaiAccountUpdatePreservesUnknownMetadataValues(t *testing.T) {
	h, s := newZaiTestHandler(t)

	acc, err := s.CreateOrRestoreProviderAccount("zai", "typed-meta")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID,
		`{"api_key":"zai-secret-key","poll_seconds":30}`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	req := httptest.NewRequest("PUT",
		"/api/zai/accounts?id="+strconv.FormatInt(acc.ID, 10),
		strings.NewReader(`{"base_url":"https://new"}`))
	w := httptest.NewRecorder()
	h.ZaiAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 - this account parses for the agent manager and is being polled", w.Code)
	}

	after, err := s.GetProviderAccountByID(acc.ID)
	if err != nil || after == nil {
		t.Fatalf("reload account: %v", err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(after.Metadata), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON after the update: %q", after.Metadata)
	}
	if meta["api_key"] != "zai-secret-key" {
		t.Fatalf("api_key is %v, want it preserved: metadata is now %q", meta["api_key"], after.Metadata)
	}
	if meta["poll_seconds"] != float64(30) {
		t.Fatalf("poll_seconds is %#v, want the stored number 30: metadata is now %q",
			meta["poll_seconds"], after.Metadata)
	}
}
