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

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

// CandidateManifest contains references and frozen selections, never credentials.
// Remote permission evidence is verified by the product codec, not a manifest flag.
type CandidateManifest struct {
	Version      int                 `json:"version"`
	SourceCommit string              `json:"source_commit"`
	Repository   string              `json:"repository"`
	Profiles     []CandidateProfile  `json:"profiles"`
	Scenarios    map[string][]string `json:"scenarios"`
}
type CandidateProfile struct {
	ID     string                            `json:"id"`
	Chain  string                            `json:"chain"`
	Remote *orca_remote_worker.ProfileConfig `json:"remote,omitempty"`
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
		if p.ID == "" || ids[p.ID] || (p.Chain != "local-codex" && p.Chain != "orca-codex" && p.Chain != "orca-remote") {
			return m, errors.New("invalid/duplicate candidate profile")
		}
		if (p.Chain == "orca-remote") != (p.Remote != nil) {
			return m, errors.New("remote profile requires exact remote target configuration")
		}
		ids[p.ID] = true
	}
	for id, selection := range m.Scenarios {
		if id != "candidates.local" && id != "candidates.remote" && id != "candidates.mixed" {
			return m, errors.New("unknown candidate scenario")
		}
		if len(selection) != 2 || selection[0] == selection[1] || !ids[selection[0]] || !ids[selection[1]] {
			return m, errors.New("candidate scenario requires two distinct registered profiles")
		}
	}
	return m, nil
}

func (m CandidateManifest) Selection(id string) ([]CandidateProfile, error) {
	selected := m.Scenarios[id]
	if len(selected) != 2 {
		return nil, errors.New("candidate scenario has no exact two-profile selection")
	}
	var profiles []CandidateProfile
	remotes := 0
	for _, name := range selected {
		for _, p := range m.Profiles {
			if p.ID == name {
				profiles = append(profiles, p)
				if p.Chain == "orca-remote" {
					remotes++
				}
			}
		}
	}
	if len(profiles) != 2 || (id == "candidates.local" && remotes != 0) || (id == "candidates.remote" && remotes != 2) || (id == "candidates.mixed" && remotes != 1) {
		return nil, errors.New("candidate target composition differs from selected scenario")
	}
	if id == "candidates.local" && profiles[0].Chain == profiles[1].Chain {
		return nil, errors.New("local acceptance requires distinct local Codex and local Orca configurations")
	}
	return profiles, nil
}

// Preflight has no model calls and creates no external resources. Check remote
// authority first so local siblings cannot hide a mixed-path admission failure.
func (m CandidateManifest) Preflight(ctx context.Context, root, authHome, id string) error {
	profiles, err := m.Selection(id)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		if p.Remote != nil {
			policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "host"}, Network: "allow", Escalation: "deny"}
			err := orca_remote_worker.CheckPolicy(policy, orca_remote_worker.Settings{ConnectionID: p.Remote.ConnectionID, EnvironmentID: p.Remote.EnvironmentID, RepositorySelector: p.Remote.RepositorySelector, ExpectedCodexVersion: p.Remote.ExpectedCodexVersion})
			if err != nil {
				return fmt.Errorf("profile %s: %w", p.ID, err)
			}
		}
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
