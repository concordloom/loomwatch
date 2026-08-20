package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

var (
	agentPackageTestHome                string
	agentPackageExternalCredentialsPath string
)

// TestMain runs before all tests in the agent package. It enables test mode
// on the api package to prevent any keychain/keyring operations during tests.
// This ensures tests never read or write real Claude Code OAuth tokens.
//
// HOME and USERPROFILE are redirected for the entire package run because
// api.SetTestMode only disables keychain/keyring access; file-backed credential
// writes remain enabled. Without package-level isolation, an Anthropic refresh
// fixture can overwrite the user's real ~/.claude/.credentials.json.
func TestMain(m *testing.M) {
	api.SetTestMode(true)

	originalUserHome, _ := os.UserHomeDir()
	originalHome, hadOriginalHome := os.LookupEnv("HOME")
	originalUserProfile, hadOriginalUserProfile := os.LookupEnv("USERPROFILE")
	var externalCredentialsBefore credentialFileMetadata
	if originalUserHome != "" {
		agentPackageExternalCredentialsPath = filepath.Join(originalUserHome, ".claude", ".credentials.json")
		var err error
		externalCredentialsBefore, err = captureCredentialFileMetadata(agentPackageExternalCredentialsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat external Anthropic credentials before agent tests: %v\n", err)
			os.Exit(1)
		}
	}

	isolationRoot, err := os.MkdirTemp("", "onwatch-agent-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create agent test HOME: %v\n", err)
		os.Exit(1)
	}
	agentPackageTestHome = isolationRoot
	if err := os.Setenv("HOME", isolationRoot); err != nil {
		_ = os.RemoveAll(isolationRoot)
		fmt.Fprintf(os.Stderr, "set agent test HOME: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("USERPROFILE", isolationRoot); err != nil {
		if hadOriginalHome {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		_ = os.RemoveAll(isolationRoot)
		fmt.Fprintf(os.Stderr, "set agent test USERPROFILE: %v\n", err)
		os.Exit(1)
	}

	// Clearing these is safe now: their fallback resolves inside the isolated
	// package HOME rather than the user's real home directory.
	os.Unsetenv("OPENCODE_HOME")
	os.Unsetenv("XDG_DATA_HOME")

	code := m.Run()

	if agentPackageExternalCredentialsPath != "" {
		externalCredentialsAfter, err := captureCredentialFileMetadata(agentPackageExternalCredentialsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat external Anthropic credentials after agent tests: %v\n", err)
			if code == 0 {
				code = 1
			}
		} else if !sameCredentialFileMetadata(externalCredentialsBefore, externalCredentialsAfter) {
			fmt.Fprintln(os.Stderr, "external Anthropic credentials metadata changed during agent tests")
			if code == 0 {
				code = 1
			}
		}
	}

	if hadOriginalHome {
		if err := os.Setenv("HOME", originalHome); err != nil {
			fmt.Fprintf(os.Stderr, "restore HOME after agent tests: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	} else if err := os.Unsetenv("HOME"); err != nil {
		fmt.Fprintf(os.Stderr, "unset HOME after agent tests: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	if hadOriginalUserProfile {
		if err := os.Setenv("USERPROFILE", originalUserProfile); err != nil {
			fmt.Fprintf(os.Stderr, "restore USERPROFILE after agent tests: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	} else if err := os.Unsetenv("USERPROFILE"); err != nil {
		fmt.Fprintf(os.Stderr, "unset USERPROFILE after agent tests: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	if err := os.RemoveAll(isolationRoot); err != nil {
		fmt.Fprintf(os.Stderr, "remove agent test HOME: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}
