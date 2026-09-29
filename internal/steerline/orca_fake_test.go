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
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeOrca is a protocol-level stand-in for the supported terminal identity,
// observation and close operations. No Orca runtime is needed.
type fakeOrcaState struct {
	Handle       string   `json:"handle"`
	Incarnation  string   `json:"incarnation"`
	Worktree     string   `json:"worktree,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Exited       bool     `json:"exited,omitempty"`
	Closed       bool     `json:"closed,omitempty"`
	Switched     bool     `json:"switched,omitempty"`
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
	case "repo":
		if len(argv) > 1 && argv[1] == "list" {
			cwd, _ := os.Getwd()
			return fakeOrcaOK(map[string]any{"repos": []any{map[string]any{"id": "fake-repo", "path": filepath.ToSlash(cwd), "kind": "git"}}})
		}
	case "worktree":
		if len(argv) < 2 {
			return 1
		}
		switch argv[1] {
		case "rm":
			return fakeOrcaRemove(argv[2:])
		}
	case "terminal":
		if len(argv) < 2 {
			return 1
		}
		switch argv[1] {
		case "create":
			return fakeOrcaTerminalCreate(stateDir, argv[2:])
		case "show":
			return fakeOrcaShow(stateDir, argv[2:])
		case "switch":
			if os.Getenv("STEERLINE_FAKE_ORCA_SWITCH_FAIL") == "1" {
				return fakeOrcaErr("switch_failed", "injected UI switch failure")
			}
			st, err := readFakeOrcaState(stateDir, arg(argv, "--terminal"))
			if err != nil {
				return fakeOrcaErr("terminal_handle_stale", err.Error())
			}
			st.Switched = true
			if err = writeFakeOrcaState(stateDir, st); err != nil {
				return fakeOrcaErr("state", err.Error())
			}
			return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle}})
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

func fakeOrcaTerminalCreate(stateDir string, argv []string) int {
	for _, value := range argv {
		if value == "--focus" {
			return fakeOrcaErr("terminal_create", "focus must be requested after the terminal is bound")
		}
	}
	selector := strings.TrimPrefix(arg(argv, "--worktree"), "path:")
	title := arg(argv, "--title")
	command := arg(argv, "--command")
	if selector == "" || title == "" || command == "" {
		return fakeOrcaErr("terminal_create", "native step terminal requires exact worktree, title and command")
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fakeOrcaErr("terminal_create", err.Error())
	}
	branchBytes, err := exec.Command("git", "-C", selector, "branch", "--show-current").Output()
	if err != nil {
		return fakeOrcaErr("terminal_create", err.Error())
	}
	handle := "term_fake_" + regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(title, "_")
	state := fakeOrcaState{Handle: handle, Incarnation: "inc_" + handle, Worktree: selector, Branch: strings.TrimSpace(string(branchBytes))}
	if err = writeFakeOrcaState(stateDir, state); err != nil {
		return fakeOrcaErr("terminal_create", err.Error())
	}
	if runtime.GOOS != "windows" {
		return fakeOrcaErr("terminal_create", "fake Orca native step execution requires Windows PowerShell")
	}
	proc := exec.Command("cmd.exe", "/c", command)
	proc.Dir = selector
	output, runErr := proc.CombinedOutput()
	state.OutputLines = strings.Split(strings.TrimRight(string(output), "\r\n"), "\n")
	state.Exited = true
	if err = writeFakeOrcaState(stateDir, state); err != nil {
		return fakeOrcaErr("terminal_create", err.Error())
	}
	if os.Getenv("STEERLINE_FAKE_ORCA_TERMINAL_CREATE_RESPONSE_LOST") == "1" {
		return 2 // The worker and terminal settled, but the create receipt was lost.
	}
	if runErr != nil {
		return fakeOrcaErr("terminal_create", fmt.Sprintf("command failed: %v: %s", runErr, output))
	}
	return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": handle, "worktreePath": filepath.ToSlash(selector), "branch": "refs/heads/" + state.Branch}})
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

func fakeOrcaShow(stateDir string, argv []string) int {
	if os.Getenv("STEERLINE_FAKE_ORCA_SHOW_UNREACHABLE") == "1" {
		return 2
	}
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
	if os.Getenv("STEERLINE_FAKE_ORCA_ORPHANED") == "1" {
		orphaned = true
	}
	if st.Closed || st.Exited {
		status = "exited"
		connected = false
		if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_SHOW_UNCERTAIN") == "1" {
			status, orphaned = "running", true
		}
	}
	return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle, "incarnationId": inc, "status": status, "connected": connected, "orphaned": orphaned, "worktreePath": filepath.ToSlash(st.Worktree), "branch": "refs/heads/" + st.Branch, "executionHostId": "local", "agentIdentity": os.Getenv("STEERLINE_FAKE_ORCA_AGENT_IDENTITY")}})
}

func TestOrcaTerminalExitCauseConfirmsCommandExit(t *testing.T) {
	var term orcaTerminal
	if err := json.Unmarshal([]byte(`{"handle":"term-exit","connected":false,"exitCause":{"kind":"exited","exitCode":0}}`), &term); err != nil {
		t.Fatal(err)
	}
	if !term.exited() {
		t.Fatalf("Orca exitCause was not treated as terminal exit evidence: %+v", term)
	}
	term = orcaTerminal{Status: "running", Connected: true}
	if term.exited() {
		t.Fatal("live Orca terminal was misclassified as exited")
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

// TestOrcaNativeHostWait requires an exact isolated terminal that is actively
// waiting for a human choice through Orca's public terminal/show response.
func TestOrcaNativeHostWait(t *testing.T) {
	path := os.Getenv("STEERLINE_ORCA_HOST_WAIT_PROBE")
	if path == "" {
		t.Skip("set an exact isolated waiting Orca terminal to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var probe struct {
		Workspace   string `json:"workspace"`
		Terminal    string `json:"terminal"`
		Runtime     string `json:"runtime"`
		Incarnation string `json:"incarnation"`
	}
	require(t, json.Unmarshal(raw, &probe))
	if !filepath.IsAbs(probe.Workspace) || probe.Terminal == "" || probe.Runtime == "" || probe.Incarnation == "" {
		t.Fatal("Orca host-wait probe lacks an exact bound terminal")
	}
	exe, err := exec.LookPath("orca")
	require(t, err)
	config := Config{Repository: probe.Workspace, Host: "orca", CommandTimeout: "15s", Orca: OrcaConfig{Executable: exe}}
	identity := ProcessIdentity{Kind: "orca", Terminal: probe.Terminal, RuntimeID: probe.Runtime, Incarnation: probe.Incarnation, Executable: exe}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	term, err := inspectBoundOrcaTerminal(ctx, config, identity)
	require(t, err)
	if term.AgentWait == nil || term.AgentIdentity != "codex" {
		t.Fatalf("real Orca host wait was not reported by the bound Codex terminal: %+v", term)
	}
	var wait struct {
		Source string `json:"source"`
		Since  int64  `json:"since"`
	}
	require(t, json.Unmarshal(*term.AgentWait, &wait))
	if wait.Source == "" || wait.Since == 0 {
		t.Fatalf("real Orca wait lacks source/time: %+v", wait)
	}
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "wait.db"))
	require(t, err)
	defer store.Close()
	runID, attemptID := newID(), newID()
	run := &Run{RunID: runID, AttemptID: attemptID, ExecutionID: "host-wait-execution", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), Agent: "codex", Target: targetFor(config), Process: &identity}
	service := &Service{Config: Config{identity: "project"}, domainStore: store}
	source := WorkerEvent{RunID: runID, AttemptID: attemptID, Source: "orca-terminal-v2", SourceSegment: "orca-host-wait-v1:probe", SourceEventID: "host-wait:probe", SourceIdentity: "terminal:" + probe.Runtime + ":" + probe.Terminal + ":" + probe.Incarnation, SourceSeq: 1, Session: "terminal:" + probe.Terminal + ":" + probe.Incarnation, InputWait: true, Diagnostic: "Orca reports a host-level agent wait; no SteerLine business Decision was created"}
	require(t, service.recordConversationEvent(run, source))
	page, err := (ConversationQueryService{Store: store, Project: "project"}).ReadConversation(ctx, ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: attemptID}, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != ConversationInputWait || page.Events[0].Status != "waiting" || page.Events[0].Attributes["wait_origin"] != "host" || page.Events[0].Attributes["business_decision"] != false || page.Events[0].Evidence["wait_source"] != "orca-terminal-show.agentWait" {
		t.Fatalf("real host wait was confused with a business decision: %+v", page.Events)
	}
	t.Logf("real Orca host wait: source=%s, reported_since=%d, conversation_event=%s", wait.Source, wait.Since, page.Events[0].Type)
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
	fixtureGit(t, f.c.Repository, "remote", "add", "web", f.server.URL+"/owner/repo.git")
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
