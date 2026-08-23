package agent

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Issue #21, the consumer half. parseZaiAccountMeta used to drop its decode
// error, so a damaged metadata blob produced exactly the same empty struct as
// an account that had never been configured. The manager then skipped the
// account at Debug level, which is how a subscription stops being polled with
// nothing said about it anywhere.

func TestParseZaiAccountMetaSeparatesDamageFromAbsence(t *testing.T) {
	if _, err := parseZaiAccountMeta(`{"api_key":"zai-k"`); err == nil {
		t.Fatal("a truncated blob decoded without error: damage is indistinguishable from an account that was never configured")
	}

	meta, err := parseZaiAccountMeta("")
	if err != nil {
		t.Fatalf("empty metadata must stay the quiet never-configured case, got error: %v", err)
	}
	if meta.APIKey != "" {
		t.Fatalf("empty metadata yielded api_key %q", meta.APIKey)
	}

	meta, err = parseZaiAccountMeta(`{"api_key":"zai-k","base_url":"https://x"}`)
	if err != nil {
		t.Fatalf("well-formed metadata failed to decode: %v", err)
	}
	if meta.APIKey != "zai-k" || meta.BaseURL != "https://x" {
		t.Fatalf("decoded %+v, want the stored key and base_url", meta)
	}

	// The contract the web handler's merge depends on: it now writes values it
	// does not understand back untouched, so a blob carrying one must still
	// hand this reader the key rather than becoming an error.
	meta, err = parseZaiAccountMeta(`{"api_key":"zai-k","poll_seconds":30}`)
	if err != nil {
		t.Fatalf("a non-string field made the whole blob unreadable: %v", err)
	}
	if meta.APIKey != "zai-k" {
		t.Fatalf("decoded api_key %q, want the key alongside the unknown field", meta.APIKey)
	}
}

// An account that has dropped out of the polling rotation has to be audible
// without turning debug logging on, and an account that was simply never given
// a key must not warn on every reload.
func TestLoadAndStartAccountsWarnsAboutAccountsItWillNotPoll(t *testing.T) {
	logsFor := func(t *testing.T, metadata string) string {
		t.Helper()
		s, err := store.New(":memory:")
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		t.Cleanup(func() { s.Close() })

		acc, err := s.CreateOrRestoreProviderAccount("zai", "subject")
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if metadata != "" {
			if err := s.UpdateProviderAccountMetadata(acc.ID, metadata); err != nil {
				t.Fatalf("seed metadata: %v", err)
			}
		}

		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		m := NewZaiAgentManager(s, nil, time.Minute, logger)
		if err := m.loadAndStartAccounts(); err != nil {
			t.Fatalf("loadAndStartAccounts: %v", err)
		}
		return buf.String()
	}

	// Damaged blob: nothing can read a key out of it, so the account is not
	// polled and that has to show at Info level or above.
	if got := logsFor(t, `{"api_key":"zai-k"`); !strings.Contains(got, "not readable") {
		t.Fatalf("an unreadable blob produced no visible log line; got: %q", got)
	}

	// Configured at some point, no key now - the state a metadata overwrite
	// leaves behind.
	if got := logsFor(t, `{"base_url":"https://x"}`); !strings.Contains(got, "no API key") {
		t.Fatalf("an account with metadata but no key produced no visible log line; got: %q", got)
	}

	// Never configured. The migration creates one of these on every fresh
	// install, so it must not warn. "{}" counts as never configured too - it is
	// what clearing base_url on a keyless account writes, and it is the pair
	// main.go already treats as "not seeded yet".
	for _, quiet := range []string{"", "{}"} {
		got := logsFor(t, quiet)
		if strings.Contains(got, "no API key") || strings.Contains(got, "not readable") {
			t.Fatalf("a never-configured account (metadata %q) warned on load; got: %q", quiet, got)
		}
	}
}
