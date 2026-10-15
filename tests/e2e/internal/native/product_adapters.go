package native

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const SupportedCodexVersion = "codex-cli 0.159.2"
const OrcaWorktreeTrashDirectory = ".orca-worktree-trash"

var ErrOrcaRepoUnavailable = errors.New("fixed Orca repository unavailable")

// Tool discovery is a native test precondition, not a Runner configuration codec.
func CodexExecutable(command string) (string, error) {
	if configured := os.Getenv("STEERLINE_NATIVE_CODEX_EXE"); configured != "" {
		command = configured
	}
	if command == "" {
		command = "codex"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		return path, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	var magic [2]byte
	_, readErr := file.Read(magic[:])
	file.Close()
	if readErr == nil && magic == [2]byte{'M', 'Z'} {
		return path, nil
	}
	// npm's documented Windows distribution carries the direct native executable.
	root := filepath.Join(filepath.Dir(path), "node_modules", "@openai", "codex")
	for _, candidate := range []string{filepath.Join(root, "node_modules", "@openai", "codex-win32-x64", "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe"), filepath.Join(root, "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe")} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return filepath.EvalSymlinks(candidate)
		}
	}
	return "", errors.New("select the existing native Codex executable with STEERLINE_NATIVE_CODEX_EXE")
}

// These DTOs consume only Orca's public CLI inventory for fixture ownership.
type OrcaWorktree struct{ ID, RepoID, Path string }
type inventoryWorktree struct {
	ID          string `json:"id"`
	RepoID      string `json:"repoId"`
	Path        string `json:"path"`
	DisplayName string `json:"displayName"`
}

func OrcaPayload(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.OK || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil, errors.New("Orca CLI response unavailable or rejected")
	}
	return envelope.Result, nil
}
func FindOrcaRepo(ctx context.Context, call func(context.Context, ...string) (json.RawMessage, error), repository string) (string, error) {
	raw, err := call(ctx, "repo", "list", "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Repos     []struct{ ID, Path string }
		Truncated bool
	}
	if json.Unmarshal(raw, &listing) != nil || listing.Truncated {
		return "", ErrOrcaRepoUnavailable
	}
	key := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\\", "/"), "/") }
	id := ""
	for _, repo := range listing.Repos {
		if strings.EqualFold(key(repo.Path), key(repository)) {
			if id != "" || repo.ID == "" {
				return "", ErrOrcaRepoUnavailable
			}
			id = repo.ID
		}
	}
	if id == "" {
		return "", ErrOrcaRepoUnavailable
	}
	return id, nil
}
func OrcaCreatedWorktree(raw []byte, name string) (OrcaWorktree, error) {
	var receipt struct{ Worktree inventoryWorktree }
	if json.Unmarshal(raw, &receipt) != nil {
		return OrcaWorktree{}, errors.New("invalid native worktree receipt")
	}
	w := receipt.Worktree
	if w.ID == "" || w.RepoID == "" || w.Path == "" || w.DisplayName != name {
		return OrcaWorktree{}, errors.New("native worktree ownership differs")
	}
	return OrcaWorktree{w.ID, w.RepoID, w.Path}, nil
}
func OrcaWorktrees(raw []byte) ([]OrcaWorktree, error) {
	var listing struct {
		Worktrees []inventoryWorktree
		Truncated bool
	}
	if json.Unmarshal(raw, &listing) != nil || listing.Truncated {
		return nil, errors.New("incomplete native worktree inventory")
	}
	result := make([]OrcaWorktree, 0, len(listing.Worktrees))
	for _, w := range listing.Worktrees {
		result = append(result, OrcaWorktree{w.ID, w.RepoID, w.Path})
	}
	return result, nil
}
