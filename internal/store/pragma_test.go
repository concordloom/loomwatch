package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNew_EveryConnectionGetsThePragmas
//
// database/sql hands out connections from a pool, so a PRAGMA executed through
// db.Exec after opening configures whichever connection happened to serve it.
// The pool holds two for a file database, which left the second one without
// busy_timeout - it failed a contended write immediately rather than waiting -
// and without foreign_keys, so whether a constraint was enforced depended on
// which connection a statement landed on.
//
// It showed up in production as SQLITE_BUSY the moment five provider agents
// polled at once on startup, losing a snapshot on every restart. The test holds
// one connection open so the query is forced onto the other.
func TestNew_EveryConnectionGetsThePragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pragma.db")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Pin the first connection by holding a transaction on it.
	held, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer held.Close()

	// And take the second one explicitly, so the checks below cannot be served
	// by the connection that was configured first.
	other, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer other.Close()

	for _, c := range []struct {
		name string
		want map[string]int
	}{
		{"held", map[string]int{"busy_timeout": 5000, "foreign_keys": 1}},
		{"other", map[string]int{"busy_timeout": 5000, "foreign_keys": 1}},
	} {
		conn := held
		if c.name == "other" {
			conn = other
		}
		for pragma, want := range c.want {
			var got int
			if err := conn.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatalf("%s: PRAGMA %s: %v", c.name, pragma, err)
			}
			if got != want {
				t.Errorf("%s connection has %s=%d, want %d: a pragma that reaches "+
					"only one pooled connection makes behaviour depend on which one "+
					"a statement lands on", c.name, pragma, got, want)
			}
		}
	}
}

// TestNew_AcceptsAPathThatAlreadyHasAQuery guards the separator.
//
// A caller may pass a full URI - "file:/path?cache=shared" is a documented
// SQLite form and an existing test uses it. Appending the pragmas with a second
// "?" produces a DSN the driver reads as nonsense and reports as "out of
// memory", which sends whoever hits it looking for a memory problem.
func TestNew_AcceptsAPathThatAlreadyHasAQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uri.db")
	s, err := New("file:" + path + "?cache=shared")
	if err != nil {
		t.Fatalf("New with an existing query string: %v", err)
	}
	defer s.Close()

	var timeout int
	if err := s.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout is %d, want 5000: the pragmas have to survive a "+
			"path that already carries a query", timeout)
	}
}
