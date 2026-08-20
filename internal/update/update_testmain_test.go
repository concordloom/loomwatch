package update

import (
	"os"
	"testing"
)

// TestMain neutralises the launchd hooks for the whole package. Without this a
// developer who has the onWatch auto-start agent installed would have their
// real agent kickstarted (and their running onWatch killed) by any test that
// reaches Restart(). Tests that exercise the launchd branch override the hooks
// themselves.
func TestMain(m *testing.M) {
	launchdSupported = func() bool { return false }
	launchdInstalled = func() bool { return false }
	launchdLoaded = func() bool { return false }
	launchdRestart = func() error { return nil }

	os.Exit(m.Run())
}
