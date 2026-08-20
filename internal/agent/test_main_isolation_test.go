package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

type credentialFileMetadata struct {
	exists bool
	info   os.FileInfo
}

func captureCredentialFileMetadata(path string) (credentialFileMetadata, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return credentialFileMetadata{}, nil
	}
	if err != nil {
		return credentialFileMetadata{}, err
	}
	return credentialFileMetadata{exists: true, info: info}, nil
}

func sameCredentialFileMetadata(before, after credentialFileMetadata) bool {
	if before.exists != after.exists {
		return false
	}
	if !before.exists {
		return true
	}
	return os.SameFile(before.info, after.info) &&
		before.info.Size() == after.info.Size() &&
		before.info.Mode() == after.info.Mode() &&
		before.info.ModTime().Equal(after.info.ModTime())
}

// TestAgentPackageHomeIsolationProtectsExternalAnthropicCredentials proves that
// a production credential write during agent tests is confined to TestMain's
// temporary HOME. The external credentials file is checked by metadata only;
// its contents are intentionally never read, copied, or restored.
func TestAgentPackageHomeIsolationProtectsExternalAnthropicCredentials(t *testing.T) {
	if agentPackageTestHome == "" {
		t.Fatal("agent package TestMain did not configure an isolated HOME")
	}
	if got := os.Getenv("HOME"); got != agentPackageTestHome {
		t.Fatalf("agent package HOME = %q, want isolated %q", got, agentPackageTestHome)
	}
	resolvedHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve agent package user home: %v", err)
	}
	if resolvedHome != agentPackageTestHome {
		t.Fatalf("agent package user home = %q, want isolated %q", resolvedHome, agentPackageTestHome)
	}

	isolatedCredentialsPath := filepath.Join(agentPackageTestHome, ".claude", ".credentials.json")
	if agentPackageExternalCredentialsPath != "" && agentPackageExternalCredentialsPath == isolatedCredentialsPath {
		t.Fatal("external and isolated Anthropic credentials paths are identical")
	}
	if err := os.MkdirAll(filepath.Dir(isolatedCredentialsPath), 0o700); err != nil {
		t.Fatalf("create isolated Anthropic credentials directory: %v", err)
	}
	if err := os.WriteFile(isolatedCredentialsPath, []byte(`{"claudeAiOauth":{"accessToken":"old-fixture","refreshToken":"old-refresh"}}`), 0o600); err != nil {
		t.Fatalf("create isolated Anthropic credentials fixture: %v", err)
	}

	var before credentialFileMetadata
	if agentPackageExternalCredentialsPath != "" {
		var err error
		before, err = captureCredentialFileMetadata(agentPackageExternalCredentialsPath)
		if err != nil {
			t.Fatalf("stat external Anthropic credentials before write: %v", err)
		}
	}

	if err := api.WriteAnthropicCredentials("fixture-access-token", "fixture-refresh-token", 3600); err != nil {
		t.Fatalf("WriteAnthropicCredentials() in isolated HOME: %v", err)
	}
	isolatedData, err := os.ReadFile(isolatedCredentialsPath)
	if err != nil {
		t.Fatalf("read isolated Anthropic credentials after write: %v", err)
	}
	if !bytes.Contains(isolatedData, []byte("fixture-access-token")) || !bytes.Contains(isolatedData, []byte("fixture-refresh-token")) {
		t.Fatal("isolated Anthropic credentials did not receive fixture tokens")
	}

	if agentPackageExternalCredentialsPath == "" {
		return
	}
	after, err := captureCredentialFileMetadata(agentPackageExternalCredentialsPath)
	if err != nil {
		t.Fatalf("stat external Anthropic credentials after write: %v", err)
	}
	if !sameCredentialFileMetadata(before, after) {
		t.Fatal("external Anthropic credentials metadata changed")
	}
}
