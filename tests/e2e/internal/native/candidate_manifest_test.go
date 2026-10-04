package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCandidateManifestRejectsForeignAndAmbiguousSelections(t *testing.T) {
	valid := `{"version":1,"source_commit":"` + strings.Repeat("a", 40) + `","repository":"erranliu/SteerlineTest","profiles":[{"id":"orca","chain":"orca-codex"},{"id":"local","chain":"local-codex"}]}`
	path := filepath.Join(t.TempDir(), "manifest.json")
	save := func(raw string) {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	save(valid)
	m, err := LoadCandidateManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Selection("candidates.local"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{valid + "{}", strings.Replace(valid, `"local","chain":"local-codex"`, `"orca","chain":"local-codex"`, 1), strings.Replace(valid, "local-codex", "orca-codex", 1), strings.Replace(valid, "local-codex", "orca-remote", 1), strings.Replace(valid, `"repository"`, `"remote":{},"repository"`, 1), strings.Replace(valid, `"repository"`, `"scenarios":{},"repository"`, 1)} {
		save(raw)
		if _, err := LoadCandidateManifest(path); err == nil {
			t.Fatal("foreign/duplicate/trailing input accepted:", raw)
		}
	}
}
