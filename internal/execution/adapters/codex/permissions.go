package codexworker

import (
	"errors"
)

// LinkedWorktreeWriteArgs compiles four previously verified, fixed Git
// directories into Codex writable-path arguments. Resource discovery belongs
// to workspace; this package only owns argv generation.
func LinkedWorktreeWriteArgs(objects, refs, logs, admin string) ([]string, error) {
	if objects == "" || refs == "" || logs == "" || admin == "" {
		return nil, errors.New("linked worktree Git write paths are incomplete")
	}
	return []string{"--add-dir", objects, "--add-dir", refs, "--add-dir", logs, "--add-dir", admin}, nil
}

// LocalSandboxArgs fixes the native Windows implementation without expanding
// the turn's workspace-write roots or initiating administrator setup.
