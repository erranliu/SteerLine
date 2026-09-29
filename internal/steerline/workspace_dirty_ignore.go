package steerline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"runtime"
	"strings"
)

// WorkspaceDirtyIgnore changes only the Git cleanliness gate. Ignored files
// remain uncommitted and never become part of a candidate commit or PR.
type WorkspaceDirtyIgnore struct {
	Paths      []string `json:"paths,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

func (r WorkspaceDirtyIgnore) empty() bool { return len(r.Paths) == 0 && len(r.Extensions) == 0 }

func (r WorkspaceDirtyIgnore) validate() error {
	seen := make(map[string]bool)
	for _, name := range r.Paths {
		value := strings.ReplaceAll(name, `\`, "/")
		trimmed := strings.TrimSuffix(value, "/")
		if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, ":") || trimmed == "" || trimmed == "." || path.Clean(trimmed) != trimmed || strings.HasPrefix(trimmed, ".git/") || trimmed == ".git" {
			return fmt.Errorf("invalid workspace_dirty_ignore path %q", name)
		}
		for _, segment := range strings.Split(trimmed, "/") {
			if segment == "." || segment == ".." {
				return fmt.Errorf("invalid workspace_dirty_ignore path %q", name)
			}
		}
		key := strings.ToLower(value)
		if seen[key] {
			return fmt.Errorf("duplicate workspace_dirty_ignore path %q", name)
		}
		seen[key] = true
	}
	for _, ext := range r.Extensions {
		if len(ext) < 2 || ext[0] != '.' || strings.ContainsAny(ext, `/\:*?[]`) || strings.TrimSpace(ext) != ext || strings.Contains(ext[1:], ".") {
			return fmt.Errorf("invalid workspace_dirty_ignore extension %q", ext)
		}
		key := strings.ToLower(ext)
		if seen[key] {
			return fmt.Errorf("duplicate workspace_dirty_ignore extension %q", ext)
		}
		seen[key] = true
	}
	return nil
}

func (r WorkspaceDirtyIgnore) matches(name string) bool {
	name = strings.ReplaceAll(name, `\`, "/")
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	for _, configured := range r.Paths {
		match := strings.ReplaceAll(configured, `\`, "/")
		if runtime.GOOS == "windows" {
			match = strings.ToLower(match)
		}
		if strings.HasSuffix(match, "/") && strings.HasPrefix(name, match) || name == match {
			return true
		}
	}
	for _, ext := range r.Extensions {
		match := ext
		if runtime.GOOS == "windows" {
			match = strings.ToLower(match)
		}
		if strings.HasSuffix(name, match) {
			return true
		}
	}
	return false
}

// dirtyFromPorcelain reads every tracked and untracked path, including both
// sides of a rename. A malformed status is never interpreted as clean.
func (r WorkspaceDirtyIgnore) dirtyFromPorcelain(output []byte) (bool, error) {
	if len(output) == 0 {
		return false, nil
	}
	if output[len(output)-1] != 0 {
		return true, errors.New("Git porcelain status is not NUL terminated")
	}
	entries := bytes.Split(output[:len(output)-1], []byte{0})
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 || entry[2] != ' ' {
			return true, errors.New("Git porcelain status has an invalid entry")
		}
		ignored := r.matches(string(entry[3:]))
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
			i++
			if i >= len(entries) {
				return true, errors.New("Git porcelain rename lacks its original path")
			}
			ignored = ignored && r.matches(string(entries[i]))
		}
		if !ignored {
			return true, nil
		}
	}
	return false, nil
}

func inspectGitDirty(ctx context.Context, c Config, worktree string) (bool, error) {
	output, err := gitRunner(c, worktree).Output(ctx, c.Git, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return true, err
	}
	return c.WorkspaceDirtyIgnore.dirtyFromPorcelain(output)
}
