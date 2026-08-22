package service

// systemd support for the Linux side of service management.
//
// These helpers lived in internal/update until self-update was removed. They
// were written to make a self-update restart survive systemd, but only the
// migration was ever about that: IsSystemd and DetectServiceName are what
// `onwatch service` uses to restart the daemon, and they outlive the feature
// they shipped beside.

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// execCommand is the seam these helpers stub in tests. It is deliberately not
// runCommand, the seam launchd.go uses: MigrateSystemdUnit runs the command
// itself so it can log the failure, and so it needs the *exec.Cmd rather than
// an error.
var execCommand = exec.Command

// IsSystemd returns true if the process is managed by systemd.
// Detected via INVOCATION_ID environment variable which systemd sets for all services.
func IsSystemd() bool {
	return os.Getenv("INVOCATION_ID") != ""
}

var readCgroupFile = func() ([]byte, error) {
	return os.ReadFile("/proc/self/cgroup")
}

// DetectServiceName reads /proc/self/cgroup to find the systemd service name.
// Falls back to "onwatch.service" if detection fails.
func DetectServiceName() string {
	data, err := readCgroupFile()
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			// cgroup v2 line: "0::/system.slice/onwatch.service"
			if idx := strings.LastIndex(line, "/"); idx >= 0 {
				unit := strings.TrimSpace(line[idx+1:])
				if strings.HasSuffix(unit, ".service") {
					return unit
				}
			}
		}
	}
	return "onwatch.service"
}

// findUnitFile locates the systemd unit file on disk.
// Checks system-level first (/etc/systemd/system/), then user-level (~/.config/systemd/user/).
func findUnitFile(serviceName string) string {
	systemPath := filepath.Join("/etc/systemd/system", serviceName)
	if _, err := os.Stat(systemPath); err == nil {
		return systemPath
	}

	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".config", "systemd", "user", serviceName)
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
	}

	return ""
}

// MigrateSystemdUnit brings an existing unit file up to the restart policy the
// daemon expects: Restart=always rather than on-failure, and a shorter
// RestartSec.
//
// The units it edits were written by an installer this fork no longer ships,
// so it only ever finds something to do on a host installed before that. In a
// container there is no systemd and this is a no-op, which is the deployment
// this fork supports.
//
// Safe to call on every startup - no-op if already up to date or not under systemd.
func MigrateSystemdUnit(logger *slog.Logger) {
	if !IsSystemd() {
		return
	}

	serviceName := DetectServiceName()
	unitPath := findUnitFile(serviceName)
	if unitPath == "" {
		return
	}

	content, err := os.ReadFile(unitPath)
	if err != nil {
		logger.Warn("Could not read systemd unit file", "path", unitPath, "error", err)
		return
	}

	original := string(content)
	updated := original

	updated = strings.Replace(updated, "Restart=on-failure", "Restart=always", 1)
	updated = strings.Replace(updated, "RestartSec=10", "RestartSec=5", 1)

	if updated == original {
		return // already up to date
	}

	if err := os.WriteFile(unitPath, []byte(updated), 0644); err != nil {
		logger.Warn("Could not update systemd unit file", "path", unitPath, "error", err)
		return
	}

	var cmd *exec.Cmd
	if strings.HasPrefix(unitPath, "/etc/systemd/system") {
		cmd = execCommand("systemctl", "daemon-reload")
	} else {
		cmd = execCommand("systemctl", "--user", "daemon-reload")
	}
	if err := cmd.Run(); err != nil {
		logger.Warn("systemctl daemon-reload failed", "error", err)
		return
	}

	logger.Info("Migrated systemd unit file",
		"path", unitPath,
		"changes", "Restart=always, RestartSec=5")
}
