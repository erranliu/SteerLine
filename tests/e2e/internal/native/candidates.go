package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// CandidateManifest contains references and frozen selections, never credentials.
type CandidateManifest struct {
	Version      int                `json:"version"`
	SourceCommit string             `json:"source_commit"`
	Repository   string             `json:"repository"`
	Profiles     []CandidateProfile `json:"profiles"`
}
type CandidateProfile struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
}

func LoadCandidateManifest(path string) (CandidateManifest, error) {
	var m CandidateManifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("candidate manifest: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, errors.New("candidate manifest has trailing JSON")
	}
	if m.Version != 1 || len(m.SourceCommit) != 40 || m.Repository != "erranliu/SteerlineTest" {
		return m, errors.New("candidate manifest requires version 1, exact source commit and authorized SteerlineTest repository")
	}
	ids := map[string]bool{}
	for _, p := range m.Profiles {
		if p.ID == "" || ids[p.ID] || (p.Chain != "local-codex" && p.Chain != "orca-codex") {
			return m, errors.New("invalid/duplicate candidate profile")
		}
		ids[p.ID] = true
	}
	if _, err := m.Selection("candidates.local"); err != nil {
		return m, err
	}
	return m, nil
}

func (m CandidateManifest) Selection(id string) ([]CandidateProfile, error) {
	profiles := m.Profiles
	if id != "candidates.local" || len(profiles) != 2 || profiles[0].ID == "" || profiles[1].ID == "" || profiles[0].ID == profiles[1].ID || profiles[0].Chain != "orca-codex" || profiles[1].Chain != "local-codex" {
		return nil, errors.New("local acceptance requires distinct local Codex and local Orca configurations")
	}
	return profiles, nil
}

// Preflight has no model calls and creates no external resources.
func (m CandidateManifest) Preflight(ctx context.Context, root, authHome, id string) error {
	profiles, err := m.Selection(id)
	if err != nil {
		return err
	}
	head, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil || string(bytes.TrimSpace(head)) != m.SourceCommit {
		return errors.New("candidate source commit differs from manifest")
	}
	if _, err := os.Stat(filepath.Join(authHome, "auth.json")); err != nil {
		return errors.New("candidate native authentication source unavailable")
	}
	if _, err := CodexExecutable("codex"); err != nil {
		return err
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return err
	}
	for _, p := range profiles {
		if p.Chain == "orca-codex" {
			if _, err := exec.LookPath("orca"); err != nil {
				return err
			}
		}
	}
	return nil
}
