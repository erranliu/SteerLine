package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

type terminalFixture struct {
	NativeFinal                                                               string
	NativeGate                                                                bool
	Repository, Environment, Agent, Incarnation, Worktree, Branch, Base, Name string
	Sends                                                                     int
	Lines                                                                     []string
	CommandMode                                                               bool
	CollapsedInput                                                            bool
	Launches                                                                  int
	Prompts                                                                   []string
}

func fakeTerminalOrca(directory string, args []string) int {
	var state terminalFixture
	raw, err := os.ReadFile(filepath.Join(directory, "transport.json"))
	if err != nil || json.Unmarshal(raw, &state) != nil || arg(args, "--environment") != state.Environment {
		return 2
	}
	fail := func(err error) int {
		fmt.Println(string(mustJSON(map[string]any{"ok": false, "error": map[string]any{"message": err.Error()}})))
		return 1
	}
	git := func(path string, args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%v: %s", err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	worktree := func() map[string]any {
		head, _ := git(state.Worktree, "rev-parse", "HEAD")
		return map[string]any{"id": "fixture::" + state.Worktree, "instanceId": "fixture-" + state.Name, "repoId": "fixture", "path": state.Worktree, "branch": state.Branch, "head": head, "baseRef": state.Base, "displayName": state.Name}
	}
	var result any
	if len(args) > 0 && args[0] == "status" {
		result = map[string]any{"runtime": map[string]any{"state": "ready", "appVersion": "fixture", "capabilities": []string{"terminal.prompt-delivery.v1"}}}
	} else if len(args) < 2 {
		return 2
	} else {
		switch args[0] + " " + args[1] {
		case "repo list":
			result = map[string]any{"repos": []any{map[string]any{"id": "fixture", "kind": "git", "path": state.Repository}}}
		case "worktree create":
			if state.Worktree != "" {
				return fail(fmt.Errorf("duplicate create"))
			}
			state.Name, state.Base = arg(args, "--name"), arg(args, "--base-branch")
			state.Worktree = filepath.Join(directory, "worker")
			state.Branch = "worker/" + state.Name
			if _, err = git(state.Repository, "worktree", "add", "--quiet", "-b", state.Branch, state.Worktree, state.Base); err != nil {
				return fail(err)
			}
			result = map[string]any{"worktree": worktree(), "agentTerminalHandle": "original"}
		case "worktree show":
			result = map[string]any{"worktree": worktree()}
		case "terminal show":
			agent := state.Agent
			if state.CommandMode && state.Launches == 0 {
				agent = ""
			}
			result = map[string]any{"terminal": map[string]any{"handle": "original", "incarnationId": state.Incarnation, "worktreeId": "fixture::" + state.Worktree, "worktreePath": state.Worktree, "branch": state.Branch, "agentIdentity": agent, "connected": true, "status": "running"}}
		case "terminal list":
			result = map[string]any{"terminals": []any{map[string]any{"handle": "original", "worktreeId": "fixture::" + state.Worktree}}}
		case "terminal wait":
			result = map[string]any{"wait": map[string]any{"satisfied": true}}
		case "terminal read":
			if state.NativeGate && state.Sends == 3 {
				phase, _ := os.ReadFile(filepath.Join(directory, "gate-phase"))
				if strings.TrimSpace(string(phase)) == "reply" {
					state.Lines = append(state.Lines, "› "+state.Prompts[len(state.Prompts)-1], "• "+state.NativeFinal, "  Worked for 3s • 18:50", "› Ask Codex")
				}
			}
			if strings.Contains(strings.Join(args, " "), "--screen") {
				result = map[string]any{"terminal": map[string]any{"handle": "original", "source": "screen", "tail": state.Lines, "oldestCursor": "0", "nextCursor": "0", "latestCursor": "0"}}
				break
			}
			cursor := 0
			if value := arg(args, "--cursor"); value != "" {
				cursor, err = strconv.Atoi(value)
				if err != nil {
					return fail(err)
				}
			}
			if cursor > len(state.Lines) {
				return fail(fmt.Errorf("cursor reset"))
			}
			result = map[string]any{"terminal": map[string]any{"handle": "original", "source": "stream", "tail": state.Lines[cursor:], "oldestCursor": "0", "nextCursor": strconv.Itoa(len(state.Lines)), "latestCursor": strconv.Itoa(len(state.Lines)), "limited": false, "truncated": false}}
		case "terminal send":
			text := arg(args, "--text")
			if state.CommandMode && strings.HasPrefix(text, "& 'opaque-worker'") {
				if state.Launches != 0 {
					return fail(fmt.Errorf("duplicate agent startup"))
				}
				state.Launches++
				result = map[string]any{"send": map[string]any{"handle": "original", "accepted": true, "prompt": map[string]any{"requestId": arg(args, "--retry-request"), "processIncarnation": state.Incarnation}}}
				break
			}
			if strings.Contains(text, "Fetch ") || strings.Contains(text, "SL_RESULT") || strings.Contains(text, "reply_b64") {
				return fail(fmt.Errorf("unexpected transport contract"))
			}
			final, modelErr := testprotocol.StagedBusiness(state.Worktree, text, os.Getenv("STEERLINE_STAGE_PROVIDER"), os.Getenv("STEERLINE_FAKE_GIT_FETCH_REMOTE"))
			if modelErr != nil {
				return fail(modelErr)
			}
			state.NativeFinal = final
			echo := text
			if state.CollapsedInput {
				echo = strings.Join(strings.Fields(text), " ")
				if len(echo) > 40 {
					echo = echo[:40] + "…"
				}
			}
			state.Lines = append(state.Lines, "› "+echo)
			if state.CollapsedInput {
				state.Lines = append(state.Lines, "     1 -previous tool output", "    + Show details")
			}
			if state.NativeGate && state.Sends == 2 {
				// Preserve an old valid report while its completion footer changes,
				// reproducing the original screen fingerprint/idle false positive.
				state.Lines = state.Lines[:len(state.Lines)-1]
				state.Lines[len(state.Lines)-2] = "  Worked for 90s • 18:50"
			} else {
				state.Lines = append(state.Lines, "• "+final, "  Worked for "+strconv.Itoa(state.Sends+1)+"s • 18:50", "› Ask Codex to do anything")
			}
			state.Prompts = append(state.Prompts, text)
			state.Sends++
			result = map[string]any{"send": map[string]any{"handle": "original", "accepted": true, "prompt": map[string]any{"requestId": arg(args, "--retry-request"), "processIncarnation": state.Incarnation, "stages": []string{"input_accepted", "turn_started"}}}}
		case "terminal close":
			result = map[string]any{"close": map[string]any{"handle": "original", "ptyKilled": true}}
		default:
			return fail(fmt.Errorf("unsupported primitive: %v", args))
		}
	}
	if len(args) > 1 && (args[0]+" "+args[1] == "worktree create" || args[0]+" "+args[1] == "terminal send") {
		if err = AtomicWrite(filepath.Join(directory, "transport.json"), mustJSON(state)); err != nil {
			return fail(err)
		}
	}
	fmt.Println(string(mustJSON(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": "fixed-runtime"}})))
	return 0
}
