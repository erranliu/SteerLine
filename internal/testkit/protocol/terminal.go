package testprotocol

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// fakeOrca is a protocol-level stand-in for the supported terminal identity,
// observation and close operations. No Orca runtime is needed.
type TerminalFixtureState struct {
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

func TerminalFixtureStatePath(dir, handle string) string { return filepath.Join(dir, handle+".json") }

func WriteTerminalFixtureState(dir string, st TerminalFixtureState) error {
	b, _ := json.Marshal(st)
	return writeTerminalFile(TerminalFixtureStatePath(dir, st.Handle), b)
}

func ReadTerminalFixtureState(dir, handle string) (TerminalFixtureState, error) {
	b, e := os.ReadFile(TerminalFixtureStatePath(dir, handle))
	if e != nil {
		return TerminalFixtureState{}, e
	}
	var st TerminalFixtureState
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

func FakeTerminal(stateDir string, argv []string) int {
	if len(argv) == 0 {
		return 1
	}
	if argv[0] == "--version" {
		fmt.Println("1.4.223")
		return 0
	}
	switch argv[0] {
	case "status":
		return fakeOrcaOK(map[string]any{"target": map[string]string{"kind": "local"}, "runtime": map[string]any{"state": "ready", "reachable": true, "runtimeId": "fake-runtime", "appVersion": "1.4.223"}})
	case "repo":
		if len(argv) > 1 && argv[1] == "list" {
			cwd, _ := os.Getwd()
			return fakeOrcaOK(map[string]any{"repos": []any{map[string]any{"id": "fake-repo", "path": filepath.ToSlash(cwd), "kind": "git"}}})
		}
		if len(argv) > 1 && argv[1] == "show" {
			url, err := exec.Command("git", "remote", "get-url", "origin").Output()
			if err != nil {
				return fakeOrcaErr("repo_origin_missing", err.Error())
			}
			return fakeOrcaOK(map[string]any{"repo": map[string]any{"id": "fake-repo", "kind": "git", "gitRemoteIdentity": map[string]string{"remoteUrl": strings.TrimSpace(string(url))}}})
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
			st, err := ReadTerminalFixtureState(stateDir, nativeArg(argv, "--terminal"))
			if err != nil {
				return fakeOrcaErr("terminal_handle_stale", err.Error())
			}
			st.Switched = true
			if err = WriteTerminalFixtureState(stateDir, st); err != nil {
				return fakeOrcaErr("state", err.Error())
			}
			return fakeOrcaOK(map[string]any{"terminal": map[string]any{"handle": st.Handle}})
		case "read":
			return fakeOrcaRead(stateDir, argv[2:])
		case "close":
			if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_FAIL") == "1" {
				return fakeOrcaErr("close_failed", "injected close failure")
			}
			st, err := ReadTerminalFixtureState(stateDir, nativeArg(argv, "--terminal"))
			if err != nil {
				return fakeOrcaErr("terminal_handle_stale", err.Error())
			}
			if os.Getenv("STEERLINE_FAKE_ORCA_CLOSE_STAYS_LIVE") != "1" {
				st.Closed = true
				if err = WriteTerminalFixtureState(stateDir, st); err != nil {
					return fakeOrcaErr("state", err.Error())
				}
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
	selector := strings.TrimPrefix(nativeArg(argv, "--worktree"), "path:")
	title := nativeArg(argv, "--title")
	command := nativeArg(argv, "--command")
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
	state := TerminalFixtureState{Handle: handle, Incarnation: "inc_" + handle, Worktree: selector, Branch: strings.TrimSpace(string(branchBytes))}
	if err = WriteTerminalFixtureState(stateDir, state); err != nil {
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
	if err = WriteTerminalFixtureState(stateDir, state); err != nil {
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
	selector := nativeArg(argv, "--worktree")
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
	st, e := ReadTerminalFixtureState(stateDir, nativeArg(argv, "--terminal"))
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

func fakeOrcaRead(stateDir string, argv []string) int {
	st, err := ReadTerminalFixtureState(stateDir, nativeArg(argv, "--terminal"))
	if err != nil {
		return fakeOrcaErr("terminal_handle_stale", err.Error())
	}
	cursor := nativeArg(argv, "--cursor")
	position := 0
	if cursor != "" {
		position, err = strconv.Atoi(cursor)
		if err != nil || position < 0 {
			return fakeOrcaErr("cursor_invalid", "invalid cursor")
		}
	}
	limit, err := strconv.Atoi(nativeArg(argv, "--limit"))
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
func writeTerminalFile(path string, value []byte) error {
	temporary := path + ".terminal.tmp"
	if err := os.WriteFile(temporary, value, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func nativeArg(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
