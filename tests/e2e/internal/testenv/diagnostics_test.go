//go:build e2e

package testenv

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDiagnosticsRedactRawAndEncodedGitHubCredentials(t *testing.T) {
	root := t.TempDir()
	p := Project{Root: root, Home: filepath.Join(root, "home"), Database: filepath.Join(root, "state", "steerline.db"), Scenario: &Scenario{Output: filepath.Join(root, "evidence")}}
	sessions := filepath.Join(p.Home, "codex", "sessions")
	if err := os.MkdirAll(sessions, 0700); err != nil {
		t.Fatal(err)
	}
	const token = "fixture-private-token"
	encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	if err := os.WriteFile(filepath.Join(p.Home, "github-token"), []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	log := "raw=" + token + "\nAUTHORIZATION: basic " + encoded + "\npublic diagnostic retained\n"
	if err := os.WriteFile(filepath.Join(sessions, "native.jsonl"), []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.captureNativeDiagnostics(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(p.Scenario.Output, "native-logs", "home", "codex", "sessions", "native.jsonl"))
	if err != nil || strings.Contains(string(raw), token) || strings.Contains(string(raw), encoded) || !strings.Contains(string(raw), "public diagnostic retained") || strings.Count(string(raw), "[REDACTED]") != 2 {
		t.Fatalf("diagnostic redaction failed: err=%v", err)
	}
}
