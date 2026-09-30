package orcaworker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRelayIgnoredIsIdempotent(t *testing.T) {
	gitDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := EnsureRelayIgnored(gitDir); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "/.steerline-relay/") != 1 {
		t.Fatalf("relay exclusion was duplicated: %q", raw)
	}
}
