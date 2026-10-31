package testenv

import (
	"errors"
	"strings"
)

// Both Orca targets use the same public repo and terminal operations. Only the
// destination selector differs; handles and runtime identity stay bound there.
type orcaTarget struct {
	Profile                                            map[string]any
	Executable, Environment, Runtime, Repository, Root string
	Args                                               []string
	sequence                                           int
	ownedLocalSetup                                    bool
}

func (r *orcaTarget) matchesRepository(path, remote, localRoot, expectedRemote string) bool {
	if !strings.EqualFold(strings.TrimSuffix(remote, ".git"), strings.TrimSuffix(expectedRemote, ".git")) {
		return false
	}
	return r.Environment != "" || sameOrcaPath(path, localRoot)
}

func sameOrcaPath(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.ReplaceAll(a, "\\", "/"), "/"), strings.TrimRight(strings.ReplaceAll(b, "\\", "/"), "/"))
}

type orcaCleanupWorkspace struct {
	ID, RepoID, Path, HostID, Branch string
	IsMainWorktree                   bool
}

// Validate the complete list before removing any workspace. Ownership comes
// from the original environment/candidate Runs, never from a name prefix.
func ownedOrcaWorktrees(repo, source string, trees []orcaCleanupWorkspace, owned map[string]string) ([]string, error) {
	main := 0
	var remove []string
	seen := map[string]bool{}
	for _, tree := range trees {
		if tree.ID == "" || seen[tree.ID] || tree.RepoID != repo || tree.HostID != "local" {
			return nil, errors.New("local workspace identity changed")
		}
		seen[tree.ID] = true
		if tree.IsMainWorktree {
			if !sameOrcaPath(tree.Path, source) {
				return nil, errors.New("local source changed")
			}
			main++
			continue
		}
		branch, found := owned[tree.Path]
		if !found || branch == "" || sameOrcaPath(tree.Path, source) || strings.TrimPrefix(tree.Branch, "refs/heads/") != branch {
			return nil, errors.New("local setup contains an unowned workspace")
		}
		remove = append(remove, tree.ID)
	}
	if main != 1 {
		return nil, errors.New("local source is missing or ambiguous")
	}
	return remove, nil
}

func (r *orcaTarget) commandArgs(args ...string) []string {
	result := append(append([]string{}, r.Args...), args...)
	if r.Environment != "" {
		result = append(result, "--environment", r.Environment)
	}
	return append(result, "--json")
}
