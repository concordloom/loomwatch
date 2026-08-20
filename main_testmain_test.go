package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain runs before all tests in the main package. It isolates OpenCode
// credential lookup from the host so setup tests cannot read a real login and
// enter an unexpected interactive prompt.
func TestMain(m *testing.M) {
	isolationRoot, err := os.MkdirTemp("", "onwatch-test-auth-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("OPENCODE_HOME", filepath.Join(isolationRoot, "opencode")); err != nil {
		_ = os.RemoveAll(isolationRoot)
		panic(err)
	}
	if err := os.Setenv("XDG_DATA_HOME", filepath.Join(isolationRoot, "xdg")); err != nil {
		_ = os.RemoveAll(isolationRoot)
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(isolationRoot)
	os.Exit(code)
}
