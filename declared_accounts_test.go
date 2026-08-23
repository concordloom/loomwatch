package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// A subscription named in configuration has to exist after start, with its
// credential in place. Before this, the first account of a provider came from
// an environment variable and every further one had to be clicked into the
// panel - so the set of subscriptions lived in one SQLite file and nowhere else.
func TestApplyDeclaredAccounts_CreatesAndCredentials(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	t.Setenv("ZAI_SPARE_KEY", "zai-spare-secret")

	accounts, err := config.ParseDeclaredAccounts(`[
		{"provider":"zai","name":"spare-max","apiKeyEnv":"ZAI_SPARE_KEY"}
	]`)
	if err != nil {
		t.Fatalf("ParseDeclaredAccounts: %v", err)
	}
	if err := applyDeclaredAccounts(s, accounts, quietLogger()); err != nil {
		t.Fatalf("applyDeclaredAccounts: %v", err)
	}

	got := metadataOf(t, s, "zai", "spare-max")
	if got["api_key"] != "zai-spare-secret" {
		t.Errorf("api_key is %q, want the value of ZAI_SPARE_KEY", got["api_key"])
	}
}

// Re-declaring an account must not discard what else its metadata holds. A
// values file names the credential and the base URL; anything a later version
// keeps beside them belongs to the account, not to the declaration.
func TestApplyDeclaredAccounts_KeepsUndeclaredMetadata(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acc, err := s.GetOrCreateProviderAccount("zai", "spare-max")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(acc.ID,
		`{"api_key":"old","base_url":"https://old","poll_seconds":30}`); err != nil {
		t.Fatalf("UpdateProviderAccountMetadata: %v", err)
	}

	t.Setenv("ZAI_SPARE_KEY", "new-secret")
	accounts, err := config.ParseDeclaredAccounts(`[
		{"provider":"zai","name":"spare-max","apiKeyEnv":"ZAI_SPARE_KEY"}
	]`)
	if err != nil {
		t.Fatalf("ParseDeclaredAccounts: %v", err)
	}
	if err := applyDeclaredAccounts(s, accounts, quietLogger()); err != nil {
		t.Fatalf("applyDeclaredAccounts: %v", err)
	}

	raw := rawMetadataOf(t, s, "zai", "spare-max")
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("stored metadata is not JSON: %v", err)
	}
	if string(got["api_key"]) != `"new-secret"` {
		t.Errorf("api_key is %s, want the newly declared value", got["api_key"])
	}
	if string(got["base_url"]) != `"https://old"` {
		t.Errorf("base_url is %s; a declaration that omits it must not remove it", got["base_url"])
	}
	// A number has to come back a number. Decoding through map[string]string
	// turns 30 into "" and the account quietly loses a setting.
	if string(got["poll_seconds"]) != "30" {
		t.Errorf("poll_seconds is %s, want 30 unchanged", got["poll_seconds"])
	}
}

// An entry whose credential is missing is refused rather than applied. Creating
// the account anyway leaves it in the panel looking configured while nothing
// polls it, which is the failure this feature exists to remove.
func TestParseDeclaredAccounts_RefusesAMissingCredential(t *testing.T) {
	t.Setenv("ZAI_ABSENT_KEY", "")
	_, err := config.ParseDeclaredAccounts(`[
		{"provider":"zai","name":"spare-max","apiKeyEnv":"ZAI_ABSENT_KEY"}
	]`)
	if err == nil {
		t.Fatal("a declaration whose key is unset was accepted")
	}
}

// The list is validated whole. Half-applying it shows the operator some
// subscriptions appearing and some not, with no single place saying why.
func TestParseDeclaredAccounts_RejectsTheWholeListOnOneBadEntry(t *testing.T) {
	t.Setenv("ZAI_GOOD_KEY", "present")
	_, err := config.ParseDeclaredAccounts(`[
		{"provider":"zai","name":"good","apiKeyEnv":"ZAI_GOOD_KEY"},
		{"provider":"zai","name":""}
	]`)
	if err == nil {
		t.Fatal("a list with an unnamed entry was accepted")
	}
}

// Single-account providers take one key from the environment and have one
// account by construction. Declaring several would describe something that
// cannot exist, and the error says so rather than creating rows nothing reads.
func TestParseDeclaredAccounts_RefusesASingleAccountProvider(t *testing.T) {
	t.Setenv("GEMINI_KEY", "present")
	_, err := config.ParseDeclaredAccounts(`[
		{"provider":"gemini","name":"second","apiKeyEnv":"GEMINI_KEY"}
	]`)
	if err == nil {
		t.Fatal("a second account was accepted for a single-account provider")
	}
}

func TestApplyDeclaredAccounts_LeavesUndeclaredAccountsAlone(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	clicked, err := s.GetOrCreateProviderAccount("zai", "added-by-hand")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}
	if err := s.UpdateProviderAccountMetadata(clicked.ID, `{"api_key":"still-polling"}`); err != nil {
		t.Fatalf("UpdateProviderAccountMetadata: %v", err)
	}

	t.Setenv("ZAI_SPARE_KEY", "declared")
	accounts, _ := config.ParseDeclaredAccounts(`[
		{"provider":"zai","name":"spare-max","apiKeyEnv":"ZAI_SPARE_KEY"}
	]`)
	if err := applyDeclaredAccounts(s, accounts, quietLogger()); err != nil {
		t.Fatalf("applyDeclaredAccounts: %v", err)
	}

	// Removing a subscription because a line went missing from a values file
	// would take its history with it, and a typo must not be able to do that.
	if got := metadataOf(t, s, "zai", "added-by-hand"); got["api_key"] != "still-polling" {
		t.Errorf("an undeclared account lost its credential: %q", got["api_key"])
	}
}

func metadataOf(t *testing.T, s *store.Store, provider, name string) map[string]string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal([]byte(rawMetadataOf(t, s, provider, name)), &out); err != nil {
		t.Fatalf("metadata for %s/%s is not JSON: %v", provider, name, err)
	}
	return out
}

func rawMetadataOf(t *testing.T, s *store.Store, provider, name string) string {
	t.Helper()
	accounts, err := s.QueryActiveProviderAccounts(provider)
	if err != nil {
		t.Fatalf("QueryActiveProviderAccounts: %v", err)
	}
	for _, a := range accounts {
		if a.Name == name {
			return a.Metadata
		}
	}
	t.Fatalf("account %s/%s does not exist", provider, name)
	return ""
}
