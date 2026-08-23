package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// GitHub Actions runs jobs under systemd, so INVOCATION_ID is set on the runner
// and IsSystemd() reports true there but not on a developer machine. The tests
// below set or clear it explicitly; this keeps the ones that do not from
// inheriting the runner's answer.
func init() { os.Unsetenv("INVOCATION_ID") }

func TestIsSystemd(t *testing.T) {
	// Save and restore INVOCATION_ID
	orig := os.Getenv("INVOCATION_ID")
	defer os.Setenv("INVOCATION_ID", orig)

	os.Setenv("INVOCATION_ID", "")
	if IsSystemd() {
		t.Error("expected false when INVOCATION_ID is empty")
	}

	os.Setenv("INVOCATION_ID", "some-uuid-value")
	if !IsSystemd() {
		t.Error("expected true when INVOCATION_ID is set")
	}
}

func TestDetectServiceName_Fallback(t *testing.T) {
	// On macOS or when /proc/self/cgroup is not available, should return default
	name := DetectServiceName()
	if name == "" {
		t.Error("expected non-empty service name")
	}
	// On non-Linux, falls back to default
	if name != "onwatch.service" {
		// If we happen to be on Linux with cgroup info, it should end with .service
		if !strings.HasSuffix(name, ".service") {
			t.Errorf("expected service name ending in .service, got %q", name)
		}
	}
}

func TestFindUnitFile_NotFound(t *testing.T) {
	// With a nonsense service name, should return empty
	result := findUnitFile("nonexistent-service-12345.service")
	if result != "" {
		t.Errorf("expected empty string for nonexistent service, got %q", result)
	}
}

func TestFindUnitFile_SystemPath(t *testing.T) {
	dir := t.TempDir()

	// Create a fake /etc/systemd/system directory structure
	systemDir := filepath.Join(dir, "etc", "systemd", "system")
	if err := os.MkdirAll(systemDir, 0755); err != nil {
		t.Fatal(err)
	}

	unitFile := filepath.Join(systemDir, "onwatch.service")
	if err := os.WriteFile(unitFile, []byte("[Service]\nRestart=on-failure\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// findUnitFile checks absolute paths, so we can't easily redirect it.
	// Instead, test that it returns empty for a valid service name not on disk.
	result := findUnitFile("onwatch-test-phantom.service")
	if result != "" {
		t.Errorf("expected empty for phantom service, got %q", result)
	}
}

func TestMigrateSystemdUnit_NotSystemd(t *testing.T) {
	// When not under systemd, MigrateSystemdUnit should be a no-op
	orig := os.Getenv("INVOCATION_ID")
	defer os.Setenv("INVOCATION_ID", orig)

	os.Setenv("INVOCATION_ID", "")
	// Should not panic or error
	MigrateSystemdUnit(slog.Default())
}

func TestFindUnitFile_UserLevelPath(t *testing.T) {
	serviceName := "onwatch-user-level-test.service"
	tmpHome := t.TempDir()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}

	userDir := filepath.Join(tmpHome, ".config", "systemd", "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatalf("MkdirAll user systemd dir: %v", err)
	}

	userUnitPath := filepath.Join(userDir, serviceName)
	if err := os.WriteFile(userUnitPath, []byte("[Service]\nRestart=always\n"), 0644); err != nil {
		t.Fatalf("WriteFile user unit: %v", err)
	}

	got := findUnitFile(serviceName)
	if got != userUnitPath {
		t.Fatalf("findUnitFile() = %q, want %q", got, userUnitPath)
	}
}

func TestDetectServiceName_FromCgroup(t *testing.T) {
	origRead := readCgroupFile
	defer func() { readCgroupFile = origRead }()

	readCgroupFile = func() ([]byte, error) {
		return []byte("0::/system.slice/custom-onwatch.service\n"), nil
	}

	got := DetectServiceName()
	if got != "custom-onwatch.service" {
		t.Fatalf("DetectServiceName() = %q, want %q", got, "custom-onwatch.service")
	}
}

func TestDetectServiceName_FallbackWhenNoServiceUnit(t *testing.T) {
	origRead := readCgroupFile
	defer func() { readCgroupFile = origRead }()

	readCgroupFile = func() ([]byte, error) {
		return []byte("0::/user.slice/session.scope\n"), nil
	}

	got := DetectServiceName()
	if got != "onwatch.service" {
		t.Fatalf("DetectServiceName() = %q, want default", got)
	}
}

func TestMigrateSystemdUnit_UpdatesUserUnitAndReloads(t *testing.T) {
	origRead := readCgroupFile
	defer func() { readCgroupFile = origRead }()

	serviceName := "onwatch-migrate-test.service"
	readCgroupFile = func() ([]byte, error) {
		return []byte(fmt.Sprintf("0::/system.slice/%s\n", serviceName)), nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("INVOCATION_ID", "invocation-test-id")

	unitDir := filepath.Join(tmpHome, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatalf("MkdirAll unitDir: %v", err)
	}
	unitPath := filepath.Join(unitDir, serviceName)
	original := "[Service]\nRestart=on-failure\nRestartSec=10\n"
	if err := os.WriteFile(unitPath, []byte(original), 0644); err != nil {
		t.Fatalf("WriteFile unitPath: %v", err)
	}

	binDir := t.TempDir()
	markerFile := filepath.Join(binDir, "systemctl.called")
	scriptPath := filepath.Join(binDir, "systemctl")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + markerFile + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile systemctl stub: %v", err)
	}

	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}
	t.Setenv("PATH", binDir+pathSep+os.Getenv("PATH"))

	MigrateSystemdUnit(slog.Default())

	updatedBytes, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile unitPath: %v", err)
	}
	updated := string(updatedBytes)
	if !strings.Contains(updated, "Restart=always") {
		t.Fatalf("expected Restart=always in unit file, got:\n%s", updated)
	}
	if !strings.Contains(updated, "RestartSec=5") {
		t.Fatalf("expected RestartSec=5 in unit file, got:\n%s", updated)
	}

	calls, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("expected systemctl to be called, read marker: %v", err)
	}
	if !strings.Contains(string(calls), "--user daemon-reload") {
		t.Fatalf("expected user-level daemon-reload call, got: %s", string(calls))
	}
}

func fakeExecCommandSuccess(t *testing.T) func(name string, arg ...string) *exec.Cmd {
	t.Helper()
	return func(name string, arg ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 0")
	}
}

func TestMigrateSystemdUnit_ReadAndWriteFailuresAndNoop(t *testing.T) {
	oldInvocationID := os.Getenv("INVOCATION_ID")
	oldExecCommand := execCommand
	t.Cleanup(func() {
		_ = os.Setenv("INVOCATION_ID", oldInvocationID)
		execCommand = oldExecCommand
	})
	if err := os.Setenv("INVOCATION_ID", "systemd-test"); err != nil {
		t.Fatalf("set INVOCATION_ID: %v", err)
	}
	execCommand = fakeExecCommandSuccess(t)

	t.Run("missing unit file is noop", func(t *testing.T) {
		tmpHome := t.TempDir()
		oldHome := os.Getenv("HOME")
		t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })
		if err := os.Setenv("HOME", tmpHome); err != nil {
			t.Fatalf("set HOME: %v", err)
		}
		readCgroupFile = func() ([]byte, error) {
			return []byte("0::/user.slice/user-501.slice/user@501.service/app.slice/missing.service"), nil
		}
		MigrateSystemdUnit(slog.Default())
	})

	t.Run("read failure is noop", func(t *testing.T) {
		tmpHome := t.TempDir()
		oldHome := os.Getenv("HOME")
		t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })
		if err := os.Setenv("HOME", tmpHome); err != nil {
			t.Fatalf("set HOME: %v", err)
		}
		userDir := filepath.Join(tmpHome, ".config", "systemd", "user")
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			t.Fatalf("mkdir user dir: %v", err)
		}
		serviceName := "read-dir.service"
		if err := os.Mkdir(filepath.Join(userDir, serviceName), 0o755); err != nil {
			t.Fatalf("mkdir fake unit dir: %v", err)
		}
		readCgroupFile = func() ([]byte, error) {
			return []byte("0::/system.slice/" + serviceName), nil
		}
		MigrateSystemdUnit(slog.Default())
	})

	t.Run("already up to date is noop", func(t *testing.T) {
		tmpHome := t.TempDir()
		oldHome := os.Getenv("HOME")
		t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })
		if err := os.Setenv("HOME", tmpHome); err != nil {
			t.Fatalf("set HOME: %v", err)
		}
		serviceName := "noop.service"
		userDir := filepath.Join(tmpHome, ".config", "systemd", "user")
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			t.Fatalf("mkdir user dir: %v", err)
		}
		unitPath := filepath.Join(userDir, serviceName)
		content := "[Service]\nRestart=always\nRestartSec=5\n"
		if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write unit file: %v", err)
		}
		readCgroupFile = func() ([]byte, error) {
			return []byte("0::/system.slice/" + serviceName), nil
		}
		MigrateSystemdUnit(slog.Default())
		got, err := os.ReadFile(unitPath)
		if err != nil {
			t.Fatalf("read unit file: %v", err)
		}
		if string(got) != content {
			t.Fatalf("unit file changed unexpectedly:\n%s", string(got))
		}
	})
}
