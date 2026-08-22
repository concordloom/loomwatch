package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/onllm-dev/onwatch/v2/internal/service"
)

// Auto-start (launchd) plumbing, indirected so tests never shell out to
// launchctl and never touch the user's real LaunchAgents directory.
var (
	autostartInstall   = service.Install
	autostartUninstall = service.Uninstall
	autostartInstalled = service.IsInstalled
	autostartLoaded    = service.Loaded
	autostartOptions   = service.DefaultOptions
	autostartSupported = service.Supported
)

// autostartDeclinedFile marks that the user said no to auto-start, so setup and
// update stop asking. Deleting it (or running `onwatch service install`)
// re-enables the offer.
func autostartDeclinedFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".onwatch", ".autostart-declined")
}

func autostartDeclined() bool {
	path := autostartDeclinedFile()
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func recordAutostartDecline() {
	path := autostartDeclinedFile()
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("onwatch service install\n"), 0o644)
}

func clearAutostartDecline() {
	if path := autostartDeclinedFile(); path != "" {
		_ = os.Remove(path)
	}
}

// stdinIsTerminal reports whether prompts can be answered. Piped installs and
// cron runs must never block - or, worse, take a default - on input nobody is
// there to give. A ModeCharDevice check is not enough: /dev/null is a character
// device too, so `onwatch update < /dev/null` would look interactive.
var stdinIsTerminal = realStdinIsTerminal

func realStdinIsTerminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// offerAutostart asks - once - whether onWatch should start itself at login.
// It is a no-op unless the platform supports it, the agent is missing, the user
// has not already declined, and there is a terminal to answer the question.
func offerAutostart(reader *bufio.Reader) {
	if !autostartSupported() || autostartInstalled() || autostartDeclined() {
		return
	}
	if !stdinIsTerminal() {
		fmt.Println()
		fmt.Println("  onWatch will not start automatically after a reboot.")
		fmt.Println("  Enable it with: onwatch service install")
		return
	}

	fmt.Println()
	fmt.Println("  onWatch does not start automatically after a reboot or logout.")
	fmt.Print("  Start onWatch automatically at login? (Y/n): ")
	answer, err := reader.ReadString('\n')
	if err != nil && answer == "" {
		// Stdin closed before answering - treat silence as "not now" rather
		// than installing a login item nobody asked for.
		fmt.Println()
		fmt.Println("  No answer - skipped. Enable it later with: onwatch service install")
		return
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		// Accept - a bare Enter takes the (Y/n) default.
	case "n", "no":
		recordAutostartDecline()
		fmt.Println("  Skipped. Enable it later with: onwatch service install")
		return
	default:
		// Unrecognised input is not consent for a login item.
		fmt.Println("  Not recognised - skipped. Enable it later with: onwatch service install")
		return
	}
	if err := installAutostart(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not enable auto-start: %v\n", err)
	}
}

// installAutostart writes and loads the launchd agent, printing what it did.
func installAutostart() error {
	opts, err := autostartOptions()
	if err != nil {
		return err
	}
	path, err := autostartInstall(opts)
	if err != nil {
		return err
	}
	clearAutostartDecline()
	fmt.Printf("  Auto-start enabled: %s\n", path)
	fmt.Printf("  Runs: %s\n", opts.BinPath)
	fmt.Println("  Disable with: onwatch service uninstall")
	return nil
}

func runService() error {
	action := serviceAction()

	if !autostartSupported() {
		switch action {
		case "status":
			fmt.Println("Auto-start management is macOS-only.")
			fmt.Println("On Linux the daemon runs under whatever unit you created: systemctl --user status onwatch")
			return nil
		default:
			return fmt.Errorf("onwatch service: auto-start management is macOS-only (Linux uses systemd)")
		}
	}

	switch action {
	case "install", "enable":
		return installAutostart()
	case "uninstall", "disable", "remove":
		// launchctl bootout stops the job as well as unloading it, so a
		// launchd-owned daemon goes down with the agent. Say so, and offer the
		// one command that brings it back.
		wasRunning := autostartLoaded()
		if err := autostartUninstall(); err != nil {
			return err
		}
		recordAutostartDecline()
		fmt.Println("Auto-start disabled. onWatch will no longer start at login.")
		if wasRunning {
			fmt.Println("The launchd-managed instance was stopped with it - restart onWatch with: onwatch")
		}
		return nil
	case "status":
		printServiceStatus()
		return nil
	default:
		printServiceHelp()
		return nil
	}
}

// serviceAction returns the word following the `service` subcommand.
func serviceAction() string {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "service" || arg == "--service" || arg == "autostart" {
			if i+1 < len(args) {
				return strings.ToLower(strings.TrimPrefix(args[i+1], "--"))
			}
			return ""
		}
	}
	return ""
}

func printServiceStatus() {
	if !autostartInstalled() {
		fmt.Println("Auto-start: not installed")
		fmt.Println("  onWatch will NOT come back after a reboot or logout.")
		fmt.Println("  Enable with: onwatch service install")
		return
	}
	path, _ := service.PlistPath()
	fmt.Println("Auto-start: installed")
	fmt.Printf("  Agent:  %s\n", service.Label)
	fmt.Printf("  Plist:  %s\n", path)
	if autostartLoaded() {
		fmt.Println("  Loaded: yes (launchd will restart onWatch at login and after a crash)")
	} else {
		fmt.Println("  Loaded: no (run `onwatch service install` to reload it)")
	}
}

func printServiceHelp() {
	fmt.Println("Usage: onwatch service <install|uninstall|status>")
	fmt.Println()
	fmt.Println("  install     Start onWatch automatically at login (launchd agent)")
	fmt.Println("  uninstall   Remove the launchd agent")
	fmt.Println("  status      Show whether the launchd agent is installed and loaded")
}

// --- post-update restart -------------------------------------------------

// parsePIDContent handles both the "PID:PORT" and legacy "PID" file formats.
func parsePIDContent(content string) int {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, ":"); idx >= 0 {
		content = content[:idx]
	}
	pid, _ := strconv.Atoi(content)
	return pid
}

func daemonPIDFromFile() int {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	return parsePIDContent(string(data))
}
