package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func retryTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// One transient network failure used to cost a whole polling interval, and the
// collector's health flag is derived from how long ago a poll last succeeded -
// so a blip did not just leave a gap in a graph, it made the account look stale.
//
// Observed on a live deployment as three "TLS handshake timeout" errors inside
// the same second: three accounts ticking together, each opening its own TLS
// connection to the same host.
func TestZaiClient_RetriesATransientNetworkFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			// Hang up without answering: the client sees a network error, the
			// same class as a handshake that timed out.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support hijacking")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"success":true,"data":{"limits":[]}}`))
	}))
	defer srv.Close()

	c := NewZaiClient("test-key", retryTestLogger(), WithZaiBaseURL(srv.URL))
	// Keep the test quick: the production delay is seconds.
	old := zaiRetryDelay
	zaiRetryDelay = 10 * time.Millisecond
	defer func() { zaiRetryDelay = old }()

	if _, err := c.FetchQuotas(context.Background()); err != nil {
		t.Fatalf("a fetch that failed once and then succeeded returned %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server saw %d calls, want 2: the first failure has to be retried", got)
	}
}

// A refusal is an answer. Repeating it turns one wrong result into two and
// doubles the load on a provider that is already saying no.
func TestZaiClient_DoesNotRetryARefusal(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"bad key"}`))
	}))
	defer srv.Close()

	c := NewZaiClient("test-key", retryTestLogger(), WithZaiBaseURL(srv.URL))

	if _, err := c.FetchQuotas(context.Background()); err == nil {
		t.Fatal("an unauthorised response was reported as success")
	} else if errors.Is(err, ErrZaiNetworkError) {
		t.Errorf("an HTTP refusal was classified as a network error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server saw %d calls, want 1: a refusal must not be repeated", got)
	}
}
