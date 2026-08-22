package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/service"
)

// autostartHarness swaps every auto-start/daemon indirection for a recorder and
// restores the TestMain defaults afterwards.
type autostartHarness struct {
	supported   bool
	installed   bool
	loaded      bool
	installErr  error
	restartErr  error
	installCall int
	uninstall   int
	restarts    int
	spawns      []string
	spawnPID    int
	spawnErr    error
	terminal    bool
}

func newAutostartHarness(t *testing.T) *autostartHarness {
	t.Helper()
	h := &autostartHarness{}

	prevSupported, prevInstalled, prevLoaded := autostartSupported, autostartInstalled, autostartLoaded
	prevInstall, prevUninstall := autostartInstall, autostartUninstall
	prevTerm := stdinIsTerminal

	autostartSupported = func() bool { return h.supported }
	autostartInstalled = func() bool { return h.installed }
	autostartLoaded = func() bool { return h.loaded }
	autostartInstall = func(service.Options) (string, error) {
		h.installCall++
		if h.installErr != nil {
			return "", h.installErr
		}
		h.installed = true
		return "/tmp/fake/dev.onllm.onwatch.plist", nil
	}
	autostartUninstall = func() error { h.uninstall++; h.installed = false; return nil }
	stdinIsTerminal = func() bool { return h.terminal }

	t.Cleanup(func() {
		autostartSupported, autostartInstalled, autostartLoaded = prevSupported, prevInstalled, prevLoaded
		autostartInstall, autostartUninstall = prevInstall, prevUninstall
		stdinIsTerminal = prevTerm
	})
	return h
}

// isolateHome points HOME (and the PID file) at a temp dir so the decline
// marker and PID lookups never touch the real install.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	prevPID := pidFile
	pidFile = filepath.Join(home, "onwatch.pid")
	t.Cleanup(func() { pidFile = prevPID })
	return home
}

func TestParsePIDContent(t *testing.T) {
	cases := map[string]int{
		"1234":       1234,
		"1234:9211":  1234,
		" 4321:80\n": 4321,
		"":           0,
		"garbage":    0,
		":9211":      0,
	}
	for in, want := range cases {
		if got := parsePIDContent(in); got != want {
			t.Errorf("parsePIDContent(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestOfferAutostartSkipsWhenUnsupported(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = false
	h.terminal = true

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("y\n"))) })

	if out != "" {
		t.Errorf("expected no output on unsupported platforms, got: %s", out)
	}
	if h.installCall != 0 {
		t.Error("must not install on unsupported platforms")
	}
}

func TestOfferAutostartSkipsWhenAlreadyInstalled(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported, h.installed, h.terminal = true, true, true

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("y\n"))) })

	if out != "" {
		t.Errorf("expected no prompt when already installed, got: %s", out)
	}
	if h.installCall != 0 {
		t.Errorf("must not reinstall, installCall=%d", h.installCall)
	}
}

func TestOfferAutostartInstallsOnYes(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported, h.terminal = true, true

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("y\n"))) })

	if h.installCall != 1 {
		t.Fatalf("expected install, installCall=%d (output: %s)", h.installCall, out)
	}
	if !strings.Contains(out, "Auto-start enabled") {
		t.Errorf("expected confirmation, got: %s", out)
	}
}

// Declining must be remembered - setup and update should not nag every run.
func TestOfferAutostartRemembersDecline(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported, h.terminal = true, true

	captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("n\n"))) })
	if h.installCall != 0 {
		t.Fatal("declining must not install")
	}
	if !autostartDeclined() {
		t.Fatal("decline was not recorded")
	}

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("y\n"))) })
	if out != "" {
		t.Errorf("expected silence after a recorded decline, got: %s", out)
	}
}

// A piped install (curl | bash, cron) has no terminal: print guidance, never block.
func TestOfferAutostartNonInteractivePrintsHint(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = true
	h.terminal = false

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader(""))) })

	if h.installCall != 0 {
		t.Error("must not install without an explicit answer")
	}
	if !strings.Contains(out, "onwatch service install") {
		t.Errorf("expected the enable command in the hint, got: %s", out)
	}
}

func TestInstallAutostartClearsPreviousDecline(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = true
	recordAutostartDecline()

	captureStdout(t, func() {
		if err := installAutostart(); err != nil {
			t.Fatalf("installAutostart: %v", err)
		}
	})
	if autostartDeclined() {
		t.Error("explicit install must clear the decline marker")
	}
}

func TestServiceAction(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"onwatch", "service", "install"}, "install"},
		{[]string{"onwatch", "service", "STATUS"}, "status"},
		{[]string{"onwatch", "service", "--uninstall"}, "uninstall"},
		{[]string{"onwatch", "service"}, ""},
		{[]string{"onwatch", "autostart", "install"}, "install"},
		{[]string{"onwatch"}, ""},
	}
	for _, c := range cases {
		prev := os.Args
		os.Args = c.args
		got := serviceAction()
		os.Args = prev
		if got != c.want {
			t.Errorf("serviceAction(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestRunServiceInstallUninstallStatus(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = true

	prev := os.Args
	t.Cleanup(func() { os.Args = prev })

	os.Args = []string{"onwatch", "service", "install"}
	captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("install: %v", err)
		}
	})
	if h.installCall != 1 {
		t.Errorf("installCall=%d, want 1", h.installCall)
	}

	os.Args = []string{"onwatch", "service", "status"}
	out := captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "installed") {
		t.Errorf("status output = %q", out)
	}

	os.Args = []string{"onwatch", "service", "uninstall"}
	out = captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
	})
	if h.uninstall != 1 {
		t.Errorf("uninstall=%d, want 1", h.uninstall)
	}
	if !autostartDeclined() {
		t.Error("uninstall should stop the offer from coming back")
	}
	if !strings.Contains(out, "no longer start at login") {
		t.Errorf("uninstall output = %q", out)
	}
}

func TestRunServiceStatusWhenNotInstalled(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = true

	prev := os.Args
	os.Args = []string{"onwatch", "service", "status"}
	t.Cleanup(func() { os.Args = prev })

	out := captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "not installed") {
		t.Errorf("expected 'not installed', got: %s", out)
	}
}

func TestRunServiceUnsupportedPlatform(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = false

	prev := os.Args
	t.Cleanup(func() { os.Args = prev })

	os.Args = []string{"onwatch", "service", "install"}
	if err := runService(); err == nil {
		t.Error("expected an error installing a launchd agent off macOS")
	}

	os.Args = []string{"onwatch", "service", "status"}
	out := captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "macOS-only") {
		t.Errorf("expected a macOS-only note, got: %s", out)
	}
}

func TestRunServiceUnknownActionPrintsHelp(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported = true

	prev := os.Args
	os.Args = []string{"onwatch", "service", "wat"}
	t.Cleanup(func() { os.Args = prev })

	out := captureStdout(t, func() {
		if err := runService(); err != nil {
			t.Fatalf("runService: %v", err)
		}
	})
	if !strings.Contains(out, "Usage: onwatch service") {
		t.Errorf("expected usage output, got: %s", out)
	}
}

// A launchd job must stay in the foreground and write its own PID file: launchd
// tracks the PID it started, so forking would look like a crash (relaunch loop)
// and no parent exists to record the PID.
func TestShouldDaemonize(t *testing.T) {
	cases := []struct {
		name                                     string
		debug, daemonChild, underLaunchd, docker bool
		want                                     bool
	}{
		{"plain run forks", false, false, false, false, true},
		{"debug stays in foreground", true, false, false, false, false},
		{"daemon child does not re-fork", false, true, false, false, false},
		{"launchd job stays in foreground", false, false, true, false, false},
		{"docker stays in foreground", false, false, false, true, false},
		{"launchd plus debug stays in foreground", true, false, true, false, false},
	}
	for _, c := range cases {
		if got := shouldDaemonize(c.debug, c.daemonChild, c.underLaunchd, c.docker); got != c.want {
			t.Errorf("%s: shouldDaemonize = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldWriteOwnPIDFile(t *testing.T) {
	cases := []struct {
		name                string
		debug, underLaunchd bool
		want                bool
	}{
		{"daemon child relies on its parent", false, false, false},
		{"debug writes its own", true, false, true},
		{"launchd job writes its own", false, true, true},
	}
	for _, c := range cases {
		if got := shouldWriteOwnPIDFile(c.debug, c.underLaunchd); got != c.want {
			t.Errorf("%s: shouldWriteOwnPIDFile = %v, want %v", c.name, got, c.want)
		}
	}
}

// /dev/null is a character device, so a ModeCharDevice check would call it a
// terminal and `onwatch setup < /dev/null` would take the prompt's default -
// silently installing a login item nobody asked for.
func TestStdinIsTerminalRejectsDevNull(t *testing.T) {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer devnull.Close()

	prevStdin := os.Stdin
	os.Stdin = devnull
	t.Cleanup(func() { os.Stdin = prevStdin })

	prev := stdinIsTerminal
	stdinIsTerminal = realStdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = prev })

	if stdinIsTerminal() {
		t.Error("stdinIsTerminal reported /dev/null as a terminal")
	}
}

// A pipe is not a terminal either.
func TestStdinIsTerminalRejectsPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	prevStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prevStdin })

	prev := stdinIsTerminal
	stdinIsTerminal = realStdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = prev })

	if stdinIsTerminal() {
		t.Error("stdinIsTerminal reported a pipe as a terminal")
	}
}

// Even when stdin looks interactive, an unanswerable prompt (EOF) must not be
// read as consent.
func TestOfferAutostartTreatsEOFAsNoAnswer(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported, h.terminal = true, true

	out := captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader(""))) })

	if h.installCall != 0 {
		t.Errorf("EOF must not install, installCall=%d", h.installCall)
	}
	if autostartDeclined() {
		t.Error("EOF is not a decline - it should not be recorded as one")
	}
	if !strings.Contains(out, "No answer") {
		t.Errorf("expected a 'No answer' notice, got: %s", out)
	}
}

// A bare Enter accepts the default, which is yes.
func TestOfferAutostartEmptyLineAcceptsDefault(t *testing.T) {
	isolateHome(t)
	h := newAutostartHarness(t)
	h.supported, h.terminal = true, true

	captureStdout(t, func() { offerAutostart(bufio.NewReader(strings.NewReader("\n"))) })

	if h.installCall != 1 {
		t.Errorf("empty line should accept the (Y/n) default, installCall=%d", h.installCall)
	}
}

// A test binary that backgrounds itself has been observed taking over the
// user's real installation, so daemonize() must confine such a child to
// throwaway state - and must leave real onWatch builds untouched.
func TestTestDaemonIsolationEnv(t *testing.T) {
	env := testDaemonIsolationEnv("/Users/x/.onwatch/bin/onwatch")
	if env != nil {
		t.Errorf("a real build must get no overrides, got %v", env)
	}

	env = testDaemonIsolationEnv("/tmp/go-build1/b001/onwatch.test")
	if len(env) == 0 {
		t.Fatal("a test binary must get isolation overrides")
	}

	var home, db, port string
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "HOME="):
			home = strings.TrimPrefix(kv, "HOME=")
		case strings.HasPrefix(kv, "ONWATCH_DB_PATH="):
			db = strings.TrimPrefix(kv, "ONWATCH_DB_PATH=")
		case strings.HasPrefix(kv, "ONWATCH_PORT="):
			port = strings.TrimPrefix(kv, "ONWATCH_PORT=")
		}
	}
	t.Cleanup(func() {
		if home != "" {
			os.RemoveAll(home)
		}
	})

	realHome, _ := os.UserHomeDir()
	if home == "" || home == realHome {
		t.Errorf("HOME override = %q, must be a scratch directory", home)
	}
	if db == "" || strings.HasPrefix(db, realHome) {
		t.Errorf("ONWATCH_DB_PATH = %q, must not point into the real install", db)
	}
	if port == "" || port == "9211" {
		t.Errorf("ONWATCH_PORT = %q, must be an ephemeral port, never the default", port)
	}
}

func TestIsTestBinary(t *testing.T) {
	cases := map[string]bool{
		"/tmp/go-build123/b001/onwatch.test": true,
		"/tmp/b001/onwatch.test.exe":         true,
		"/Users/x/.onwatch/bin/onwatch":      false,
		"/opt/homebrew/bin/onwatch":          false,
		"/usr/local/bin/onwatch.exe":         false,
	}
	for in, want := range cases {
		if got := isTestBinary(in); got != want {
			t.Errorf("isTestBinary(%q) = %v, want %v", in, got, want)
		}
	}
}

// During a takeover the new instance writes the PID file while the old one is
// still shutting down. The old instance's cleanup must not delete the new
// instance's entry.
func TestRemovePIDFileOnlyRemovesOwnEntry(t *testing.T) {
	tmp := t.TempDir()
	prev := pidFile
	pidFile = filepath.Join(tmp, "onwatch.pid")
	t.Cleanup(func() { pidFile = prev })

	// Another instance owns the file.
	if err := os.WriteFile(pidFile, []byte("999998:9211"), 0o644); err != nil {
		t.Fatal(err)
	}
	removePIDFile()
	if _, err := os.Stat(pidFile); err != nil {
		t.Error("removePIDFile deleted another instance's PID file")
	}

	// We own the file.
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d:9211", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	removePIDFile()
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("removePIDFile left our own PID file behind")
	}

	// Missing file is a no-op, not a panic.
	removePIDFile()
}

// The default-port fallback SIGTERMs whatever onwatch listens on 9211 - which
// under `go test` is the developer's real daemon. It must be inert in a test
// binary and live in a real build.
func TestScanningDefaultPortsAllowed(t *testing.T) {
	if !isTestBinary(os.Args[0]) {
		t.Skipf("not running from a test binary: %s", os.Args[0])
	}
	if scanningDefaultPortsAllowed() {
		t.Error("a test binary must not scan and kill processes on the default ports")
	}
}

// A test that forgets to isolate the pidFile global would otherwise read the
// developer's real PID file and SIGTERM their running onWatch.
func TestProductionPIDFileOffLimitsInTests(t *testing.T) {
	if !isTestBinary(os.Args[0]) {
		t.Skipf("not running from a test binary: %s", os.Args[0])
	}

	prev := pidFile
	t.Cleanup(func() { pidFile = prev })

	pidFile = filepath.Join(defaultPIDDir(), "onwatch.pid")
	if !productionPIDFileOffLimits() {
		t.Error("a test binary must not act on the real installation's PID file")
	}

	pidFile = filepath.Join(t.TempDir(), "onwatch.pid")
	if productionPIDFileOffLimits() {
		t.Error("an isolated PID file must stay usable by tests")
	}
}

// stopPreviousInstance documents that test mode uses the PID file only. Its
// Method-1 port fallback used to ignore that, so a test whose PID file named
// port 9211 scanned it and SIGTERMed the developer's real running onWatch.
func TestStopPreviousInstanceTestModeDoesNotPortScan(t *testing.T) {
	tmp := t.TempDir()
	prev := pidFile
	pidFile = filepath.Join(tmp, "onwatch.pid")
	t.Cleanup(func() { pidFile = prev })

	// Occupy a port and record it in the PID file alongside a PID that is not
	// signalled (our own), which is what drives the fallback.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d:%d", os.Getpid(), port)), 0o644); err != nil {
		t.Fatal(err)
	}

	scanned := false
	prevScan := findOnwatchOnPortFn
	findOnwatchOnPortFn = func(p int) []int { scanned = true; return nil }
	t.Cleanup(func() { findOnwatchOnPortFn = prevScan })

	stopPreviousInstance(0, true)

	if scanned {
		t.Error("test mode must not port-scan - it can kill the real daemon")
	}
}
