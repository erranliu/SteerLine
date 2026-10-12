package codexworker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type workerRuntime struct {
	Shell string
	Temp  string
}

// The temp directory is a child of the already frozen linked-worktree admin
// root. It needs no additional writable root and cannot enter the candidate.
func prepareWorkerRuntime(worktree string, linkedArgs []string) (*workerRuntime, error) {
	roots, err := WritableRoots(linkedArgs)
	if err != nil || len(roots) != 4 {
		return nil, errors.Join(err, errors.New("worker runtime needs four fixed linked Git roots"))
	}
	gitfile, err := os.ReadFile(filepath.Join(worktree, ".git"))
	if err != nil {
		return nil, err
	}
	admin, ok := strings.CutPrefix(strings.TrimSpace(string(gitfile)), "gitdir: ")
	if !ok {
		return nil, errors.New("worker runtime requires a linked worktree")
	}
	if !filepath.IsAbs(admin) {
		admin = filepath.Join(worktree, admin)
	}
	admin = filepath.Clean(admin)
	if !filepath.IsAbs(roots[3]) || !strings.EqualFold(admin, filepath.Clean(roots[3])) {
		return nil, errors.New("worker temp admin differs from the frozen writable root")
	}
	adminInfo, err := os.Lstat(admin)
	if err != nil || !adminInfo.IsDir() || adminInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(err, errors.New("worker admin is not a plain directory"))
	}
	real, err := filepath.EvalSymlinks(admin)
	if err != nil || !strings.EqualFold(filepath.Clean(real), admin) {
		return nil, errors.Join(err, errors.New("worker admin was redirected"))
	}
	temp := filepath.Join(admin, "steerline-temp")
	if err = os.MkdirAll(temp, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(temp)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(err, errors.New("worker temp is not a plain directory"))
	}
	real, err = filepath.EvalSymlinks(temp)
	if err != nil || !strings.EqualFold(filepath.Clean(real), temp) {
		return nil, errors.Join(err, errors.New("worker temp directory was redirected"))
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if !filepath.IsAbs(shell) || !isNativeCodexExecutable(shell) {
		return nil, errors.New("native Windows PowerShell executable is unavailable")
	}
	return &workerRuntime{Shell: shell, Temp: temp}, nil
}

func environmentValue(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		key, value, _ := strings.Cut(env[i], "=")
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func setEnvironmentValues(env []string, values map[string]string) []string {
	result := make([]string, 0, len(env)+len(values))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		replaced := false
		for name := range values {
			if strings.EqualFold(key, name) {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

func (r *workerRuntime) environment(env []string) ([]string, error) {
	paths := []string{filepath.Dir(r.Shell)}
	for _, path := range filepath.SplitList(environmentValue(env, "PATH")) {
		// WindowsApps execution aliases are zero-length reparse files. Codex's
		// shell discovery must not select one as the worker's PowerShell.
		alias := filepath.Join(path, "pwsh.exe")
		if info, err := os.Stat(alias); err == nil && (info.Size() == 0 || !isNativeCodexExecutable(alias)) {
			continue
		}
		paths = append(paths, path)
	}
	count := 0
	if value := environmentValue(env, "GIT_CONFIG_COUNT"); value != "" {
		var err error
		count, err = strconv.Atoi(value)
		if err != nil || count < 0 || count > 64 {
			return nil, errors.New("invalid inherited Git process configuration count")
		}
		for i := 0; i < count; i++ {
			if environmentValue(env, fmt.Sprintf("GIT_CONFIG_KEY_%d", i)) == "" {
				return nil, errors.New("inherited Git process configuration is incomplete")
			}
		}
	}
	values := map[string]string{
		"PATH": strings.Join(paths, string(os.PathListSeparator)), "TEMP": r.Temp, "TMP": r.Temp,
		"GIT_CONFIG_COUNT":                          strconv.Itoa(count + 2),
		fmt.Sprintf("GIT_CONFIG_KEY_%d", count):     "maintenance.auto",
		fmt.Sprintf("GIT_CONFIG_VALUE_%d", count):   "false",
		fmt.Sprintf("GIT_CONFIG_KEY_%d", count+1):   "gc.auto",
		fmt.Sprintf("GIT_CONFIG_VALUE_%d", count+1): "0",
	}
	return setEnvironmentValues(env, values), nil
}

func (r *workerRuntime) instructions() string {
	return fmt.Sprintf("SteerLine Windows tool setup: use shell %q and login=false for shell tools. TEMP and TMP point to this execution's writable temporary directory %q, outside tracked files. Git automatic maintenance is disabled in this worker; do not re-enable it or widen Git writable roots. Do not dump authentication or the complete environment. If an authorized command fails because the Windows sandbox cannot start a process, credential helper or system pipe, request native automatic review with sandbox_permissions=require_escalated for the exact command in the fixed worktree. Keep the assigned remote and branch. Before retrying publication, query its actual branch/PR outcome. A denied or unavailable review must be reported; do not work around a denial, change permission settings, or use a GitHub ref-creation API instead of uploading Git objects. Opening a native viewer or granting folder trust is not a prerequisite for task execution.", r.Shell, r.Temp)
}
