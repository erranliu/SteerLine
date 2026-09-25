package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeOrca is a protocol-level stand-in for the Orca CLI. `worktree create`
// really creates a git worktree so Steerline's workspace verification runs for
// real; `terminal send` writes the R-a result file. No Orca runtime is needed.
type fakeOrcaState struct {
	Handle       string   `json:"handle"`
	Incarnation  string   `json:"incarnation"`
	Worktree     string   `json:"worktree,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Prompt       string   `json:"prompt,omitempty"`
	Answered     bool     `json:"answered,omitempty"`
	Sends        int      `json:"sends,omitempty"`
	Closed       bool     `json:"closed,omitempty"`
	OutputLines  []string `json:"output_lines,omitempty"`
	OldestCursor int      `json:"oldest_cursor,omitempty"`
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
	if argv[0] == "--version" {
		fmt.Println("fake-orca/1")
		return 0
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
		case "read":
			return fakeOrcaRead(stateDir, argv[2:])
		case "close":
			if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_FAIL") == "1" {
				return fakeOrcaErr("close_failed", "injected close failure")
			}
			st, err := readFakeOrcaState(stateDir, arg(argv, "--terminal"))
			if err != nil {
				return fakeOrcaErr("terminal_handle_stale", err.Error())
			}
			if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_STAYS_LIVE") != "1" {
				st.Closed = true
				if err = writeFakeOrcaState(stateDir, st); err != nil {
					return fakeOrcaErr("state", err.Error())
				}
			}
			if path := os.Getenv("STEERLINE_FAKE_ORCA_BREAK_STOP_RECEIPT"); path != "" {
				db, err := OpenSQLiteStore(filepath.Join(filepath.Dir(path), "steerline.db"))
				if err != nil {
					return 2
				}
				_, err = db.db.Exec(`CREATE TRIGGER reject_stop_receipt BEFORE UPDATE ON execution_operations BEGIN SELECT RAISE(ABORT, 'injected receipt persistence failure'); END`)
				db.Close()
				if err != nil {
					return 2
				}
				_ = os.Mkdir(path, 0700)
			}
			return fakeOrcaOK(map[string]any{"close": map[string]any{"handle": st.Handle, "ptyKilled": st.Closed}})
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
	if os.Getenv("STEERLINE_FAKE_ORCA_CREATE_RESPONSE_LOST") == "1" {
		return 2 // External worktree/terminal exist, but the caller got no identity.
	}
	reportedBranch := branch
	if os.Getenv("STEERLINE_FAKE_ORCA_WRONG_WORKSPACE_BRANCH") == "1" {
		reportedBranch = "orca/unverified-branch"
	}
	return fakeOrcaOK(map[string]any{
		"worktree":            map[string]any{"id": "fake::" + filepath.ToSlash(wt), "path": filepath.ToSlash(wt), "branch": "refs/heads/" + reportedBranch, "head": strings.TrimSpace(string(head))},
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
	if os.Getenv("STEERLINE_FAKE_ORCA_SEND_RECEIPT") == "reject" {
		return fakeOrcaOK(map[string]any{"send": map[string]any{"handle": st.Handle, "accepted": false}})
	}
	src := fakePromptPath(arg(argv, "--text"))
	if src != "" {
		if _, e := os.Stat(src); e != nil {
			return fakeOrcaErr("prompt_missing", e.Error())
		}
		st.Prompt = src
		st.Answered = false
	} else {
		// Continuation sends reference continue.txt, not the dispatch pointer; the
		// stored dispatch prompt still carries task_id/run_id/base_commit.
		if e := fakeWriteResult(st.Worktree, st.Prompt, false); e != nil {
			return fakeOrcaErr("prompt", e.Error())
		}
		st.Answered = true
	}
	st.Sends++
	if e := writeFakeOrcaState(stateDir, st); e != nil {
		return fakeOrcaErr("state", e.Error())
	}
	if os.Getenv("STEERLINE_FAKE_ORCA_SEND_RESPONSE_LOST") == "1" {
		return 2 // The terminal accepted input, but its response was lost.
	}
	handle, incarnation := st.Handle, st.Incarnation
	switch os.Getenv("STEERLINE_FAKE_ORCA_SEND_RECEIPT") {
	case "wrong_handle":
		handle = "replacement-handle"
	case "wrong_incarnation":
		incarnation = "replacement-incarnation"
	case "missing":
		return fakeOrcaOK(map[string]any{})
	}
	code := fakeOrcaOK(map[string]any{"send": map[string]any{"handle": handle, "accepted": true, "prompt": map[string]any{"processIncarnation": incarnation, "stages": []string{"input_accepted", "turn_started"}}}})
	if os.Getenv("STEERLINE_FAKE_ORCA_SEND_RECEIPT") == "exit_error" {
		return 2
	}
	return code
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
	if st.Handle == os.Getenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_HANDLE") {
		if lockPath := os.Getenv("STEERLINE_FAKE_ORCA_SHOW_EXCLUSIVE"); lockPath != "" {
			lock, lockErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if lockErr != nil {
				return fakeOrcaErr("overlapping_observation", lockErr.Error())
			}
			lock.Close()
			defer os.Remove(lockPath)
		}
		if marker := os.Getenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_STARTED"); marker != "" {
			_ = os.WriteFile(marker, []byte("started"), 0600)
		}
		if delay, err := strconv.Atoi(os.Getenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_MS")); err == nil && delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
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
	status := "running"
	connected := true
	orphaned := false
	if os.Getenv("STEERLINE_FAKE_ORCA_DISCONNECTED") == "1" {
		connected = false
	}
	if st.Closed {
		status = "exited"
		connected = false
		if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_SHOW_UNCERTAIN") == "1" {
			status, orphaned = "running", true
		}
	}
	return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle, "incarnationId": inc, "status": status, "connected": connected, "orphaned": orphaned, "executionHostId": "local", "agentIdentity": os.Getenv("STEERLINE_FAKE_ORCA_AGENT_IDENTITY")}})
}

func TestOrcaHistoryLaunchRequiresSameLiveCodexAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "local")
	state := fakeOrcaState{Handle: "bound-agent", Incarnation: "inc-bound"}
	require(t, writeFakeOrcaState(dir, state))
	fixed := orcaTerminal{Handle: state.Handle, RuntimeID: "fake-runtime", IncarnationID: state.Incarnation, ExecutionHostID: "local"}
	t.Setenv("STEERLINE_FAKE_ORCA_AGENT_IDENTITY", "codex")
	term, err := waitOrcaCodexHistoryAgent(context.Background(), orcaClient{cfg: cfg}, fixed)
	if err != nil || term.AgentIdentity != "codex" {
		t.Fatalf("live Codex binding was rejected: term=%+v err=%v", term, err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "replacement-incarnation")
	if _, err = waitOrcaCodexHistoryAgent(context.Background(), orcaClient{cfg: cfg}, fixed); err == nil {
		t.Fatal("replaced terminal was accepted as the bound agent")
	}
}

func fakeOrcaRead(stateDir string, argv []string) int {
	st, err := readFakeOrcaState(stateDir, arg(argv, "--terminal"))
	if err != nil {
		return fakeOrcaErr("terminal_handle_stale", err.Error())
	}
	cursor := arg(argv, "--cursor")
	position := 0
	if cursor != "" {
		position, err = strconv.Atoi(cursor)
		if err != nil || position < 0 {
			return fakeOrcaErr("cursor_invalid", "invalid cursor")
		}
	}
	limit, err := strconv.Atoi(arg(argv, "--limit"))
	if err != nil || limit <= 0 {
		return fakeOrcaErr("limit_invalid", "invalid limit")
	}
	if cursor == "" {
		// The real Orca CLI treats an omitted cursor as a tail preview.
		position = len(st.OutputLines) - limit
		if position < 0 {
			position = 0
		}
	}
	if position > len(st.OutputLines) {
		position = len(st.OutputLines)
	}
	if position < st.OldestCursor {
		position = st.OldestCursor
	}
	end := position + limit
	if end > len(st.OutputLines) {
		end = len(st.OutputLines)
	}
	source := os.Getenv("STEERLINE_FAKE_ORCA_OUTPUT_SOURCE")
	if source == "" {
		source = "stream"
	}
	return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle, "source": source, "tail": st.OutputLines[position:end], "nextCursor": strconv.Itoa(end), "oldestCursor": strconv.Itoa(st.OldestCursor), "status": "running"}})
}

func TestOrcaReadOutputBindsCursorToExactSource(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	handle := "terminal-output-test"
	state := fakeOrcaState{Handle: handle, Incarnation: "inc-output", OutputLines: []string{"one", "two", "three"}}
	if err := writeFakeOrcaState(dir, state); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, External: ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-output", Executable: cfg.Orca.Executable}}
	backend := orcaTerminalBackend{config: cfg}
	first, err := backend.ReadOutput(context.Background(), ref, "", 1)
	if err != nil || string(first.Data) != "one" || first.Source != "stream" || first.NextCursor == "" {
		t.Fatalf("first output=%#v err=%v", first, err)
	}
	second, err := backend.ReadOutput(context.Background(), ref, first.NextCursor, 2)
	if err != nil || string(second.Data) != "two\nthree" || second.Gap || second.SourceChanged {
		t.Fatalf("continuation=%#v err=%v", second, err)
	}
	other := ref
	other.AttemptID = newID()
	if _, err = backend.ReadOutput(context.Background(), other, first.NextCursor, 1); err == nil {
		t.Fatal("cross-attempt cursor accepted")
	}
	state.OldestCursor = 2
	if err = writeFakeOrcaState(dir, state); err != nil {
		t.Fatal(err)
	}
	gap, err := backend.ReadOutput(context.Background(), ref, first.NextCursor, 1)
	if err != nil || !gap.Gap || string(gap.Data) != "three" {
		t.Fatalf("retention gap=%#v err=%v", gap, err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_OUTPUT_SOURCE", "screen")
	changed, err := backend.ReadOutput(context.Background(), ref, first.NextCursor, 1)
	if err != nil || !changed.SourceChanged || changed.Source != "screen" {
		t.Fatalf("source change=%#v err=%v", changed, err)
	}
}

func TestOrcaReadOutputReportsRetentionGapOnInitialRead(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	handle := "terminal-initial-gap-test"
	state := fakeOrcaState{Handle: handle, Incarnation: "inc-initial-gap", OutputLines: []string{"evicted-one", "evicted-two", "retained-three", "retained-four", "retained-five"}, OldestCursor: 2}
	if err := writeFakeOrcaState(dir, state); err != nil {
		t.Fatal(err)
	}
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: newID(), External: ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: state.Incarnation, Executable: cfg.Orca.Executable}}
	batch, err := (orcaTerminalBackend{config: cfg}).ReadOutput(context.Background(), ref, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Gap || batch.RetainedFrom != "2" || string(batch.Data) != "retained-three" {
		t.Fatalf("initial retention boundary was hidden: %+v", batch)
	}
}

// TestOrcaNativeRetentionGap is gated to an isolated live Orca terminal whose
// upstream history has actually evicted its earliest output rows.
func TestOrcaNativeRetentionGap(t *testing.T) {
	path := os.Getenv("STEERLINE_ORCA_RETENTION_PROBE")
	if path == "" {
		t.Skip("set an exact isolated live Orca terminal probe to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var probe struct {
		Workspace   string `json:"workspace"`
		Terminal    string `json:"terminal"`
		Runtime     string `json:"runtime"`
		Incarnation string `json:"incarnation"`
		Retained    string `json:"retained_from"`
	}
	require(t, json.Unmarshal(raw, &probe))
	if !filepath.IsAbs(probe.Workspace) || probe.Terminal == "" || probe.Runtime == "" || probe.Incarnation == "" || probe.Retained == "" {
		t.Fatal("Orca retention probe lacks an exact bound terminal")
	}
	exe, err := exec.LookPath("orca")
	require(t, err)
	config := Config{Repository: probe.Workspace, Host: "orca", CommandTimeout: "15s", Orca: OrcaConfig{Executable: exe}}
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(config), Agent: AgentSpec{Name: "codex"}, RunID: "retention-probe-run", AttemptID: "retention-probe-attempt", WorkspacePath: probe.Workspace, External: ProcessIdentity{Kind: "orca", Terminal: probe.Terminal, RuntimeID: probe.Runtime, Incarnation: probe.Incarnation, Executable: exe}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first, err := (orcaTerminalBackend{config: config}).ReadOutput(ctx, ref, "", 5)
	require(t, err)
	if !first.Gap || first.SourceChanged || first.Source != "stream" || first.RetainedFrom != probe.Retained || !strings.HasPrefix(string(first.Data), "PR04B-RETENTION-"+probe.Retained) || first.NextCursor == "" {
		t.Fatalf("real Orca retention loss was hidden: %+v", first)
	}
	second, err := (orcaTerminalBackend{config: config}).ReadOutput(ctx, ref, first.NextCursor, 5)
	require(t, err)
	if second.Gap || second.SourceChanged || second.Data == nil || second.NextCursor == first.NextCursor {
		t.Fatalf("bounded continuation after real retention gap failed: %+v", second)
	}
	t.Logf("real Orca retention: oldest=%s, first_rows=%q, next_rows=%q", first.RetainedFrom, first.Data, second.Data)
}

func orcaTestConfig(t *testing.T, dir, _ string) Config {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	return Config{Repository: dir, RunDir: dir, CommandTimeout: "5s", Host: "orca", Orca: OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", dir}, Shell: "cmd.exe", WaitTimeout: "5s"}}
}

func TestOrcaPreflightDoesNotRequireUnusedLocalCodex(t *testing.T) {
	f := newFixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-preflight")
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	f.c.Host = "orca"
	f.c.Remote = "web"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
	f.c.Codex.Executable = filepath.Join(stateDir, "missing-codex.exe")
	if err = f.c.Preflight(context.Background(), false); err != nil {
		t.Fatalf("Orca native agent required unused local Codex: %v", err)
	}
	f.c.Model = "requires-direct-codex"
	if err = f.c.Preflight(context.Background(), false); err == nil {
		t.Fatal("direct Codex override did not require its executable")
	}
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

func TestOrcaStopExecutionConfirmsFixedTerminalExit(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	handle := "terminal-stop-test"
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: "inc-stop"}); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, External: ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-stop", Executable: cfg.Orca.Executable}}
	backend := orcaTerminalBackend{config: cfg}
	receipt, err := backend.StopExecution(context.Background(), ref, Operation{ID: "stop-" + attemptID}, nil)
	if err != nil || receipt.Disposition != "confirmed" {
		t.Fatalf("stop receipt=%#v err=%v", receipt, err)
	}
	obs, err := backend.Observe(context.Background(), ref)
	if err != nil || obs.Liveness != "exited" {
		t.Fatalf("stop evidence=%#v err=%v", obs, err)
	}
}

func TestOrcaStopFailureDoesNotConfirmExit(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	handle := "terminal-stop-fails"
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: "inc-failure"}); err != nil {
		t.Fatal(err)
	}
	ref := ExecutionRef{Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), External: ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-failure"}}
	t.Setenv("STEERLINE_FAKE_ORCA_CLOSE_FAIL", "1")
	receipt, err := (orcaTerminalBackend{config: cfg}).StopExecution(context.Background(), ref, Operation{}, nil)
	var backendErr *BackendError
	if receipt.Disposition == "confirmed" || !errors.As(err, &backendErr) || backendErr.Code != BackendOutcomeUnknown {
		t.Fatalf("stop failure was misconfirmed: %#v, %v", receipt, err)
	}
	obs, err := (orcaTerminalBackend{config: cfg}).Observe(context.Background(), ref)
	if err != nil || obs.Liveness != "live" {
		t.Fatalf("failed stop changed original terminal: %#v, %v", obs, err)
	}
}

func TestOrcaDetachObservationIsRepeatableAndRefLocal(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "")
	backend := orcaTerminalBackend{config: cfg}
	workers := make([]*Worker, 0, 2)
	refs := make([]ExecutionRef, 0, 2)
	for _, handle := range []string{"detach-one", "detach-two"} {
		state := fakeOrcaState{Handle: handle, Incarnation: "inc-" + handle}
		if err := writeFakeOrcaState(dir, state); err != nil {
			t.Fatal(err)
		}
		proc := &orcaProcess{cfg: cfg, term: orcaTerminal{Handle: handle, RuntimeID: "fake-runtime", IncarnationID: state.Incarnation}}
		workers = append(workers, superviseOrcaResult(cfg, proc, filepath.Join(dir, handle+"-missing-result"), filepath.Join(dir, handle+"-result.json")))
		refs = append(refs, ExecutionRef{Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), External: proc.Identity()})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := backend.DetachObservation(ctx, workers[0]); err != nil {
		t.Fatal(err)
	}
	if err := backend.DetachObservation(ctx, workers[0]); err != nil {
		t.Fatalf("repeated detach blocked: %v", err)
	}
	for i, ref := range refs {
		obs, err := backend.Observe(ctx, ref)
		if err != nil || obs.Liveness != "live" {
			t.Fatalf("detach canceled terminal %d: %+v %v", i, obs, err)
		}
	}
	if err := backend.DetachObservation(ctx, workers[1]); err != nil {
		t.Fatal(err)
	}
}

func TestSlowOrcaObservationDoesNotBlockOtherRefStop(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "")
	cfg.CommandTimeout = "10s"
	backend := orcaTerminalBackend{config: cfg}
	refFor := func(handle string) ExecutionRef {
		t.Helper()
		identity := ProcessIdentity{Kind: "orca", RuntimeID: "fake-runtime", Terminal: handle, Incarnation: "inc-" + handle}
		if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: identity.Incarnation}); err != nil {
			t.Fatal(err)
		}
		return ExecutionRef{Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), External: identity}
	}
	slow, stopped := refFor("slow-observe"), refFor("quick-stop")
	marker := filepath.Join(dir, "slow-show-started")
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_HANDLE", slow.External.Terminal)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_STARTED", marker)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_MS", "5000")
	slowCtx, cancelSlow := context.WithCancel(context.Background())
	defer cancelSlow()
	slowDone := make(chan error, 1)
	go func() { _, err := backend.Observe(slowCtx, slow); slowDone <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("slow observation did not start")
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStop()
	started := time.Now()
	receipt, err := backend.StopExecution(stopCtx, stopped, Operation{}, nil)
	if err != nil || receipt.Disposition != "confirmed" || time.Since(started) >= 3*time.Second {
		t.Fatalf("other ref stop waited for slow observation: %#v %v", receipt, err)
	}
	cancelSlow()
	select {
	case <-slowDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled slow observation leaked")
	}
	state, err := readFakeOrcaState(dir, slow.External.Terminal)
	if err != nil || state.Closed {
		t.Fatalf("canceling observation closed another terminal: %+v %v", state, err)
	}
}

func TestOrcaSameRefObservationsDoNotOverlapOrLeakGate(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "")
	backend := orcaTerminalBackend{config: cfg}
	handle := "same-ref-observe"
	identity := ProcessIdentity{Kind: "orca", RuntimeID: "fake-runtime", Terminal: handle, Incarnation: "inc-same-ref"}
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: identity.Incarnation}); err != nil {
		t.Fatal(err)
	}
	ref := ExecutionRef{Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), External: identity}
	marker := filepath.Join(dir, "first-show-started")
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_HANDLE", handle)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_STARTED", marker)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_MS", "500")
	t.Setenv("STEERLINE_FAKE_ORCA_SHOW_EXCLUSIVE", filepath.Join(dir, "active-show"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { _, err := backend.Observe(ctx, ref); results <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("first observation did not enter the fake platform")
	}
	go func() { _, err := backend.Observe(ctx, ref); results <- err }()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("same-ref observation overlapped or failed: %v", err)
		}
	}
	orcaObservationGates.Lock()
	remaining := len(orcaObservationGates.byIdentity)
	orcaObservationGates.Unlock()
	if remaining != 0 {
		t.Fatalf("observation gates leaked: %d", remaining)
	}
}

func TestServiceShutdownPersistsOrcaStopEvidence(t *testing.T) {
	t.Setenv("STEERLINE_FAKE_ORCA_CLOSE_SHOW_UNCERTAIN", "1")
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	cfg.ShutdownGrace = "20ms"
	handle := "terminal-service-stop"
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: "inc-service-stop"}); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	identity := ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-service-stop", Executable: cfg.Orca.Executable}
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, WorkspacePath: dir, WorkspaceBranch: "orca/test", BaseCommit: strings.Repeat("a", 40), External: identity, Generation: identity.Incarnation}
	if err := (executionRecords{root: dir}).saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &orcaProcess{cfg: cfg, term: orcaTerminal{Handle: handle, RuntimeID: identity.RuntimeID, IncarnationID: identity.Incarnation}, cancel: cancel}
	w := &Worker{proc: proc, Events: make(chan WorkerEvent), Done: make(chan error, 1)}
	go func() { <-ctx.Done(); close(w.Events); w.Done <- ctx.Err() }()
	r := &Run{TaskID: "t-stop", RunID: runID, AttemptID: attemptID, Claimant: cfg.Claimant, Host: "orca", ExecutionStatus: "running", FinishTaskStatus: "pending", Process: &identity, LogPath: ptr(filepath.Join(dir, "events.jsonl"))}
	s := &Service{Config: cfg, State: Runtime{SchemaVersion: CurrentRuntimeSchema, Runs: []*Run{r}, SlotOccupied: true, CurrentTask: ptr(r.TaskID)}, store: &Store{Path: filepath.Join(dir, "runtime.json")}, worker: w, active: r, activities: map[string]*ActivityWriter{}}
	s.shutdown()
	op, err := (executionRecords{root: dir}).loadOperation(runID, attemptID, "stop-"+attemptID)
	if err != nil || op.Status != OperationConfirmed {
		t.Fatalf("stop operation=%#v err=%v", op, err)
	}
	if s.State.SlotOccupied || s.worker != nil {
		t.Fatalf("confirmed stop retained slot/worker: slot=%v worker=%v", s.State.SlotOccupied, s.worker)
	}
}

func TestServiceShutdownRetainsSlotWhenExactStopReceiptCannotPersist(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "")
	cfg.ShutdownGrace = "20ms"
	handle := "terminal-stop-receipt-fault"
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: "inc-receipt-fault"}); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	identity := ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-receipt-fault", Executable: cfg.Orca.Executable}
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, WorkspacePath: dir, WorkspaceBranch: "orca/test", BaseCommit: strings.Repeat("a", 40), External: identity, Generation: identity.Incarnation}
	records := executionRecords{root: dir}
	if err := records.saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &orcaProcess{cfg: cfg, term: orcaTerminal{Handle: handle, RuntimeID: identity.RuntimeID, IncarnationID: identity.Incarnation}, cancel: cancel}
	w := &Worker{proc: proc, Events: make(chan WorkerEvent), Done: make(chan error, 1)}
	go func() { <-ctx.Done(); close(w.Events); w.Done <- ctx.Err() }()
	r := &Run{TaskID: "t-stop-receipt-fault", RunID: runID, AttemptID: attemptID, Claimant: cfg.Claimant, Host: "orca", ExecutionStatus: "running", FinishTaskStatus: "pending", Process: &identity, LogPath: ptr(filepath.Join(dir, "events.jsonl"))}
	s := &Service{Config: cfg, State: Runtime{SchemaVersion: CurrentRuntimeSchema, Runs: []*Run{r}, SlotOccupied: true, CurrentTask: ptr(r.TaskID)}, store: &Store{Path: filepath.Join(dir, "runtime.json")}, worker: w, active: r, activities: map[string]*ActivityWriter{}}
	operationPath := filepath.Join(dir, "stop-receipt-fault-injected")
	t.Setenv("STEERLINE_FAKE_ORCA_BREAK_STOP_RECEIPT", operationPath)
	s.shutdown()
	if !s.State.SlotOccupied || r.ExecutionStatus != "unknown" || r.FinishTaskStatus != "blocked" {
		t.Fatalf("unpersisted close receipt released execution: slot=%v run=%#v", s.State.SlotOccupied, r)
	}
	if info, err := os.Stat(operationPath); err != nil || !info.IsDir() {
		t.Fatalf("stop receipt fault was not injected: %v", err)
	}
}

func TestServiceShutdownRetainsSlotWhenOrcaStopLacksExitEvidence(t *testing.T) {
	dir := t.TempDir()
	cfg := orcaTestConfig(t, dir, "fail")
	cfg.ShutdownGrace = "20ms"
	handle := "terminal-service-stop-unknown"
	if err := writeFakeOrcaState(dir, fakeOrcaState{Handle: handle, Incarnation: "inc-service-unknown"}); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	identity := ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-service-unknown", Executable: cfg.Orca.Executable}
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, WorkspacePath: dir, WorkspaceBranch: "orca/test", BaseCommit: strings.Repeat("a", 40), External: identity, Generation: identity.Incarnation}
	if err := (executionRecords{root: dir}).saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &orcaProcess{cfg: cfg, term: orcaTerminal{Handle: handle, RuntimeID: identity.RuntimeID, IncarnationID: identity.Incarnation}, cancel: cancel}
	w := &Worker{proc: proc, Events: make(chan WorkerEvent), Done: make(chan error, 1)}
	go func() { <-ctx.Done(); close(w.Events); w.Done <- ctx.Err() }()
	r := &Run{TaskID: "t-stop-unknown", RunID: runID, AttemptID: attemptID, Claimant: cfg.Claimant, Host: "orca", ExecutionStatus: "running", FinishTaskStatus: "pending", Process: &identity, LogPath: ptr(filepath.Join(dir, "events.jsonl"))}
	s := &Service{Config: cfg, State: Runtime{SchemaVersion: CurrentRuntimeSchema, Runs: []*Run{r}, SlotOccupied: true, CurrentTask: ptr(r.TaskID)}, store: &Store{Path: filepath.Join(dir, "runtime.json")}, worker: w, active: r, activities: map[string]*ActivityWriter{}}
	t.Setenv("STEERLINE_FAKE_ORCA_CLOSE_STAYS_LIVE", "1")
	s.shutdown()
	op, err := (executionRecords{root: dir}).loadOperation(runID, attemptID, "stop-"+attemptID)
	if err != nil || op.Status != OperationUnknown {
		t.Fatalf("stop operation=%#v err=%v", op, err)
	}
	if !s.State.SlotOccupied || r.ExecutionStatus != "unknown" || r.FinishTaskStatus != "blocked" {
		t.Fatalf("unconfirmed stop released execution: slot=%v run=%#v", s.State.SlotOccupied, r)
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
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
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

func TestOrcaLostCreateReceiptNeverStartsReplacement(t *testing.T) {
	f := newFixture(t, task("t-1"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-lost-receipt")
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	f.c.Host = "orca"
	f.c.Agent = "codex"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s"}
	t.Setenv("STEERLINE_FAKE_ORCA_CREATE_RESPONSE_LOST", "1")
	s := f.service(t)
	if err = s.dispatch(context.Background()); err == nil {
		t.Fatal("lost Orca create receipt was treated as rejected/no side effect")
	}
	if len(s.State.Runs) != 1 || s.State.Runs[0].ExecutionStatus != "unknown" || !s.State.SlotOccupied || readFake(f.db).Claims["t-1"] != 1 {
		t.Fatalf("lost create receipt released task or slot: %+v", s.State)
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Orca external resources not captured: state files=%d", count)
	}
	restarted := f.service(t)
	_ = restarted.dispatch(context.Background())
	entries, err = os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	if count != 1 || readFake(f.db).Claims["t-1"] != 1 {
		t.Fatalf("restart created a replacement Orca worktree: states=%d claims=%v", count, readFake(f.db).Claims)
	}
}
