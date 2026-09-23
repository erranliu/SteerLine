package steerline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeOrca is a protocol-level stand-in for the Orca CLI. `worktree create`
// really creates a git worktree so Steerline's workspace verification runs for
// real; `terminal send` writes the R-a result file. No Orca runtime is needed.
type fakeOrcaState struct {
	Handle      string `json:"handle"`
	Incarnation string `json:"incarnation"`
	Worktree    string `json:"worktree,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	Answered    bool   `json:"answered,omitempty"`
}

func fakeOrcaStatePath(dir, handle string) string { return filepath.Join(dir, handle+".json") }

func writeFakeOrcaState(dir string, st fakeOrcaState) error {
	b, _ := json.Marshal(st)
	return AtomicWrite(fakeOrcaStatePath(dir, st.Handle), b)
}

func readFakeOrcaState(dir, handle string) (fakeOrcaState, error) {
	b, e := os.ReadFile(fakeOrcaStatePath(dir, handle))
	if e != nil {
		return fakeOrcaState{}, e
	}
	var st fakeOrcaState
	e = json.Unmarshal(b, &st)
	return st, e
}

func fakeOrcaOK(result map[string]any) int {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": "fake-runtime"}})
	fmt.Println(string(b))
	return 0
}

func fakeOrcaErr(code, msg string) int {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": msg}})
	fmt.Println(string(b))
	return 1
}

func fakeOrca(stateDir string, argv []string) int {
	if len(argv) == 0 {
		return 1
	}
	switch argv[0] {
	case "status":
		return fakeOrcaOK(map[string]any{})
	case "worktree":
		if len(argv) < 2 {
			return 1
		}
		switch argv[1] {
		case "create":
			return fakeOrcaCreate(stateDir, argv[2:])
		case "rm":
			return fakeOrcaRemove(argv[2:])
		}
	case "terminal":
		if len(argv) < 2 {
			return 1
		}
		switch argv[1] {
		case "wait":
			return fakeOrcaWait()
		case "send":
			return fakeOrcaSend(stateDir, argv[2:])
		case "show":
			return fakeOrcaShow(stateDir, argv[2:])
		case "close":
			return fakeOrcaOK(map[string]any{"close": map[string]any{"ptyKilled": true}})
		}
	}
	return fakeOrcaErr("unknown", "unknown orca command")
}

func fakeOrcaCreate(stateDir string, argv []string) int {
	name := arg(argv, "--name")
	baseRef := arg(argv, "--base-branch")
	cwd, _ := os.Getwd()
	wt := filepath.Join(filepath.Dir(cwd), "orca-worktrees", name)
	if e := os.MkdirAll(filepath.Dir(wt), 0700); e != nil {
		return fakeOrcaErr("mkdir", e.Error())
	}
	git, e := exec.LookPath("git")
	if e != nil {
		return fakeOrcaErr("git", e.Error())
	}
	branch := "orca/" + name
	if out, e := exec.Command(git, "-C", cwd, "worktree", "add", "--no-track", "-b", branch, "--", wt, baseRef).CombinedOutput(); e != nil {
		return fakeOrcaErr("worktree", string(out))
	}
	head, _ := exec.Command(git, "-C", wt, "rev-parse", "HEAD").Output()
	handle := "term_fake_" + name
	if e := writeFakeOrcaState(stateDir, fakeOrcaState{Handle: handle, Incarnation: "inc_" + name, Worktree: wt, Branch: branch, Prompt: fakePromptPath(arg(argv, "--prompt"))}); e != nil {
		return fakeOrcaErr("state", e.Error())
	}
	return fakeOrcaOK(map[string]any{
		"worktree":            map[string]any{"id": "fake::" + filepath.ToSlash(wt), "path": filepath.ToSlash(wt), "branch": "refs/heads/" + branch, "head": strings.TrimSpace(string(head))},
		"agentTerminalHandle": handle,
	})
}

func fakeOrcaRemove(argv []string) int {
	selector := arg(argv, "--worktree")
	if i := strings.Index(selector, "::"); i >= 0 {
		path := filepath.FromSlash(selector[i+2:])
		if git, e := exec.LookPath("git"); e == nil {
			_ = exec.Command(git, "worktree", "remove", "--force", path).Run()
		}
	}
	return fakeOrcaOK(map[string]any{"removed": true})
}

func fakeOrcaWait() int {
	return fakeOrcaOK(map[string]any{"wait": map[string]any{"satisfied": true, "status": "idle"}})
}

func fakeOrcaSend(stateDir string, argv []string) int {
	st, e := readFakeOrcaState(stateDir, arg(argv, "--terminal"))
	if e != nil {
		return fakeOrcaErr("terminal_handle_stale", e.Error())
	}
	src := fakePromptPath(arg(argv, "--text"))
	if src == "" {
		// Continuation sends reference continue.txt, not the dispatch pointer; the
		// stored dispatch prompt still carries task_id/run_id/base_commit.
		src = st.Prompt
	}
	if e := fakeWriteResult(st.Worktree, src, false); e != nil {
		return fakeOrcaErr("prompt", e.Error())
	}
	st.Answered = true
	if e := writeFakeOrcaState(stateDir, st); e != nil {
		return fakeOrcaErr("state", e.Error())
	}
	return fakeOrcaOK(map[string]any{"send": map[string]any{"accepted": true}})
}

// fakePromptPath extracts the absolute prompt path from the pointer text
// "Read and execute <path> now. Follow it exactly.".
func fakePromptPath(text string) string {
	if i := strings.Index(text, "execute "); i >= 0 {
		rest := text[i+len("execute "):]
		if j := strings.Index(rest, " now."); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}

func fakeWriteResult(worktree, promptPath string, decision bool) error {
	raw, e := os.ReadFile(promptPath)
	if e != nil {
		return e
	}
	get := func(key string) string {
		m := regexp.MustCompile(`"` + key + `": "([^"]+)"`).FindSubmatch(raw)
		if len(m) != 2 {
			return ""
		}
		return string(m[1])
	}
	result := Result{
		TaskID:       get("task_id"),
		RunID:        get("run_id"),
		WorktreePath: ptr(worktree),
		BaseCommit:   ptr(get("base_commit")),
		Summary:      ptr("orca hosted done"),
	}
	if decision {
		result.Outcome = "needs_decision"
		result.Decision = &DecisionRequest{
			Question:    "Which option should the migration take?",
			Options:     []DecisionOption{{ID: "a", Label: "Keep a data-only compat type"}, {ID: "b", Label: "Relax the persistence layer"}},
			Recommended: "a",
		}
	} else {
		result.Outcome = "pr_submitted"
		result.PRURL = ptr(os.Getenv("STEERLINE_FAKE_ORCA_PR_BASE") + "/owner/repo/pulls/1")
	}
	b, _ := json.Marshal(result)
	return os.WriteFile(filepath.Join(worktree, orcaResultFile), b, 0600)
}

func fakeOrcaShow(stateDir string, argv []string) int {
	st, e := readFakeOrcaState(stateDir, arg(argv, "--terminal"))
	if e != nil {
		return fakeOrcaErr("terminal_handle_stale", "terminal handle stale")
	}
	// Model the agent answering once its prompt exists: Steerline calls show
	// right after writing prompt.txt, so the R-a result lands deterministically.
	// With STEERLINE_FAKE_ORCA_DECISION the first answer is a decision request; the
	// decision is only satisfied once Steerline sends the continuation.
	if st.Prompt != "" && !st.Answered {
		if _, e := os.Stat(st.Prompt); e == nil {
			_ = fakeWriteResult(st.Worktree, st.Prompt, os.Getenv("STEERLINE_FAKE_ORCA_DECISION") != "")
		}
	}
	inc := st.Incarnation
	if o := os.Getenv("STEERLINE_FAKE_ORCA_INCARNATION"); o != "" {
		inc = o
	}
	return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle, "incarnationId": inc, "status": "running", "connected": true, "orphaned": false}})
}

func orcaTestConfig(t *testing.T, dir, fallback string) Config {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	return Config{Repository: dir, RunDir: dir, CommandTimeout: "5s", Host: "orca", Orca: OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", dir}, Shell: "cmd.exe", WaitTimeout: "5s", Fallback: fallback}}
}

func TestInspectOrcaTerminal(t *testing.T) {
	dir := t.TempDir()
	c := orcaTestConfig(t, dir, "local")
	if e := writeFakeOrcaState(dir, fakeOrcaState{Handle: "h1", Incarnation: "inc1"}); e != nil {
		t.Fatal(e)
	}
	live := &ProcessIdentity{Kind: "orca", Terminal: "h1", Incarnation: "inc1"}
	if got := inspectProcessWithConfig(c, live); got != "matching process still exists" {
		t.Fatalf("live terminal: %q", got)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "inc2")
	if got := inspectProcessWithConfig(c, live); !strings.Contains(got, "replaced") {
		t.Fatalf("replaced terminal: %q", got)
	}
	missing := &ProcessIdentity{Kind: "orca", Terminal: "nope"}
	if got := inspectProcessWithConfig(c, missing); !strings.Contains(got, "unverifiable") {
		t.Fatalf("missing terminal: %q", got)
	}
}

// TestOrcaHostedRunProducesResult exercises the full v2 path: dispatch creates an
// Orca worktree and launches an agent, the agent writes the R-a result file, and
// Steerline still owns PR verification and Beads completion.
func TestOrcaHostedRunProducesResult(t *testing.T) {
	f := newFixture(t, task("t-1"))
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-state")
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		t.Fatal(e)
	}
	f.c.Host = "orca"
	f.c.Agent = "codex"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s", Fallback: "fail"}
	t.Setenv("STEERLINE_FAKE_ORCA_PR_BASE", f.server.URL)

	ctx := context.Background()
	s := f.service(t)
	require(t, s.dispatch(ctx))
	if s.worker == nil {
		t.Fatal("orca worker was not started")
	}
	finishWorker(t, s)
	run := s.State.Runs[0]
	if run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil {
		t.Fatalf("run did not bind a PR: %+v", run)
	}
	if run.WorktreePath == nil || run.Process == nil || run.Process.Kind != "orca" || run.Process.Terminal == "" {
		t.Fatalf("orca facts missing: %+v", run)
	}
	if canonical(*run.WorktreePath) != canonical(filepath.Join(filepath.Dir(f.c.Repository), "orca-worktrees", orcaSafeName("t-1", run.RunID))) {
		t.Fatalf("worktree not recorded from orca: %s", *run.WorktreePath)
	}
	b, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "result.json"))
	require(t, e)
	if !strings.Contains(string(b), "pr_submitted") {
		t.Fatalf("R-a result not persisted: %s", b)
	}
	// Completion stays with Steerline: PR merge + Beads close.
	f.pr("1", "merged")
	require(t, s.pollPRs(ctx))
	if s.State.Runs[0].FinishTaskStatus != "completed" || s.State.Runs[0].BeadsStatus != "closed" {
		t.Fatalf("run not completed after merge: %+v", s.State.Runs[0])
	}
	// The journal must keep the post-worker lifecycle, not stop at worker exit.
	act, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "activity.jsonl"))
	require(t, e)
	for _, want := range []string{"worker_start", "result", "pr", "completion"} {
		if !strings.Contains(string(act), want) {
			t.Fatalf("activity journal missing %q: %s", want, act)
		}
	}
}
