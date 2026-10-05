package codexworker

import (
	"errors"
	"fmt"
	"strings"
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
func LocalSandboxArgs(goos, sandbox, mode string, args []string) ([]string, error) {
	if mode != "" && !ValidWindowsSandbox(mode) {
		return nil, fmt.Errorf("windows_sandbox must be elevated or unelevated")
	}
	out := append([]string(nil), args...)
	if goos != "windows" || sandbox == "danger-full-access" || sandbox == "" {
		return out, nil
	}
	if mode == "" {
		mode = "elevated"
	}
	if sandbox == "workspace-write" && mode == "unelevated" {
		return nil, fmt.Errorf("local Windows workspace-write requires windows_sandbox=elevated: unelevated cannot enforce linked-worktree split writable roots")
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var override string
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("incomplete Codex config override")
			}
			i++
			override = args[i]
		case strings.HasPrefix(arg, "--config="):
			override = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-c"):
			override = strings.TrimPrefix(strings.TrimPrefix(arg, "-c"), "=")
		case arg == "--enable" || arg == "--disable":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("incomplete Codex feature override")
			}
			i++
			override = "features." + args[i]
		case strings.HasPrefix(arg, "--enable=") || strings.HasPrefix(arg, "--disable="):
			_, feature, _ := strings.Cut(arg, "=")
			override = "features." + feature
		}
		key, _, _ := strings.Cut(override, "=")
		key = strings.NewReplacer(`"`, "", "'", "", " ", "", "\t", "").Replace(key)
		if key == "windows" || key == "windows.sandbox" || key == "features" || key == "features.experimental_windows_sandbox" || key == "features.elevated_windows_sandbox" {
			return nil, fmt.Errorf("codex.args must not override the fixed Windows sandbox; use windows_sandbox")
		}
	}
	return append(out, "--config", `windows.sandbox="`+mode+`"`), nil
}
