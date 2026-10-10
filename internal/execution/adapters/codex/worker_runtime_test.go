package codexworker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkerRuntimeTempUsesFixedAdmin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows worker tools")
	}
	root := t.TempDir()
	worktree, admin := filepath.Join(root, "worktree"), filepath.Join(root, "repo", "worktrees", "run")
	for _, path := range []string{worktree, admin} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+filepath.ToSlash(admin)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := LinkedWorktreeWriteArgs(filepath.Join(root, "repo", "objects"), filepath.Join(root, "repo", "refs"), filepath.Join(root, "repo", "logs"), admin)
	if err != nil {
		t.Fatal(err)
	}
	r, err := prepareWorkerRuntime(worktree, args)
	if err != nil {
		t.Fatal(err)
	}
	if r.Temp != filepath.Join(admin, "steerline-temp") || !isNativeCodexExecutable(r.Shell) {
		t.Fatalf("unexpected native runtime: %+v", r)
	}
	if err := os.WriteFile(filepath.Join(r.Temp, "probe"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "steerline-temp")); !os.IsNotExist(err) {
		t.Fatal("temp polluted the working tree")
	}
	changed := append([]string(nil), args...)
	changed[len(changed)-1] = filepath.Join(root, "other")
	if _, err := prepareWorkerRuntime(worktree, changed); err == nil {
		t.Fatal("unbound admin root accepted")
	}
	if err := os.RemoveAll(r.Temp); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "sibling")
	if err := os.Mkdir(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", r.Temp, sibling).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v %s", err, out)
	}
	if _, err := prepareWorkerRuntime(worktree, args); err == nil {
		t.Fatal("redirected temp accepted")
	}
	if err := os.Remove(r.Temp); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerRuntimeEnvironmentPreservesGitConfigAndRejectsAlias(t *testing.T) {
	alias := t.TempDir()
	if err := os.WriteFile(filepath.Join(alias, "pwsh.exe"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := &workerRuntime{Shell: os.Args[0], Temp: filepath.Join(t.TempDir(), "temp")}
	base := []string{"Path=" + alias + string(os.PathListSeparator) + "tools", "TEMP=old", "tmp=old", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.example.extraheader", "GIT_CONFIG_VALUE_0=private-test-placeholder"}
	env, err := r.environment(base)
	if err != nil {
		t.Fatal(err)
	}
	if environmentValue(env, "GIT_CONFIG_COUNT") != "3" || environmentValue(env, "GIT_CONFIG_VALUE_0") != "private-test-placeholder" || environmentValue(env, "TEMP") != r.Temp || environmentValue(env, "TMP") != r.Temp {
		t.Fatal("runtime discarded existing Git configuration or temp setup")
	}
	if strings.Contains(environmentValue(env, "PATH"), alias) {
		t.Fatal("zero-length PowerShell alias remained in PATH")
	}
	if environmentValue(env, "GIT_CONFIG_KEY_1") != "maintenance.auto" || environmentValue(env, "GIT_CONFIG_VALUE_1") != "false" || environmentValue(env, "GIT_CONFIG_KEY_2") != "gc.auto" || environmentValue(env, "GIT_CONFIG_VALUE_2") != "0" {
		t.Fatal("automatic Git maintenance remained enabled")
	}
	if base[1] != "TEMP=old" {
		t.Fatal("mutated the inherited environment")
	}
	if _, err := r.environment([]string{"GIT_CONFIG_COUNT=invalid"}); err == nil {
		t.Fatal("invalid inherited Git configuration accepted")
	}
	if _, err := r.environment([]string{"GIT_CONFIG_COUNT=1"}); err == nil {
		t.Fatal("incomplete inherited Git configuration accepted")
	}
}
