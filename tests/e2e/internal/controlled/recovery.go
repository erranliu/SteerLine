package controlled

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RecoveryFixture controls only the simulated native host, never center state.
type RecoveryFixture struct {
	FailStep, NoFinalStep, DropStep, HoldStep string
	MissingThread                             bool
	Orca                                      *TerminalFixture
}
type TerminalFixture struct {
	Root, Runtime, Handle, Incarnation, Environment, Worktree, Branch, Base, Name string
	Agent, Draft, HeldCommand, HeldStep, Prompt, NativeReply                      string
	Lines                                                                         []string
	Creates, Sends, Closes                                                        int
	Busy, LostSend, Gap, Lost, WrongTarget                                        bool
}

var fixtureSteps = []string{"Implement the following Spec and Plan.", "Format the preceding implementation result as JSON.", "Push and create or reuse a PR"}
var fixtureStepIDs = []string{"implement.execute", "implement.report", "publish"}

func recoveryStep(prompt string) (int, error) {
	for i, prefix := range fixtureSteps {
		if strings.HasPrefix(prompt, prefix) {
			return i, nil
		}
	}
	return 0, errors.New("unbound recovery fixture input")
}

func recoveryStart(path string, s State, id json.RawMessage, thread, prompt string, send func(any) error) error {
	index, err := recoveryStep(prompt)
	if err != nil {
		return err
	}
	if thread != s.Session {
		return errors.New("recovery changed original thread")
	}
	turn := map[string]any{"id": uuid.NewString(), "status": "inProgress", "items": []any{}, "fixture_step": fixtureStepIDs[index], "fixture_index": index}
	s.Dispatches++
	s.Turns = append(s.Turns, turn)
	if s.Recovery.HoldStep != fixtureStepIDs[index] {
		if err := completeRecoveryTurn(path, &s, index); err != nil {
			return err
		}
	}
	if err := Write(path, s); err != nil {
		return err
	}
	for _, v := range []any{map[string]any{"id": id, "result": map[string]any{"turn": map[string]any{"id": turn["id"], "status": "inProgress"}}}, map[string]any{"method": "turn/started", "params": map[string]any{"threadId": thread, "turn": map[string]any{"id": turn["id"]}}}} {
		if err := send(v); err != nil {
			return err
		}
	}
	if s.Recovery.DropStep == fixtureStepIDs[index] || s.Recovery.HoldStep == fixtureStepIDs[index] {
		return io.EOF
	}
	if items, ok := turn["items"].([]any); ok {
		for _, item := range items {
			if err := send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": thread, "turnId": turn["id"], "item": item}}); err != nil {
				return err
			}
		}
	}
	return send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": thread, "turn": turn}})
}

func completeRecoveryTurn(path string, s *State, index int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	turn := s.Turns[len(s.Turns)-1]
	status, body := "completed", ""
	if s.Recovery.FailStep == fixtureStepIDs[index] {
		status = "failed"
		s.Recovery.FailStep = ""
		turn["error"] = map[string]string{"message": "injected native stream failure"}
	} else {
		var err error
		body, err = executeControlledStage(ctx, path, s, index)
		if err != nil {
			return err
		}
	}
	turn["status"], turn["completedAt"] = status, time.Now().Unix()
	if s.Recovery.NoFinalStep != fixtureStepIDs[index] && status == "completed" {
		turn["items"] = []any{map[string]any{"type": "agentMessage", "id": "message-" + fmt.Sprint(turn["id"]), "phase": "final_answer", "text": body}}
	}
	if s.Recovery.NoFinalStep == fixtureStepIDs[index] {
		s.Recovery.NoFinalStep = ""
	}
	return nil
}

func releaseRecovery(path string) error {
	s, err := Read(path)
	if err != nil {
		return err
	}
	if s.Recovery == nil {
		return errors.New("missing native recovery fixture")
	}
	if s.Recovery.Orca != nil {
		o := s.Recovery.Orca
		if o.HeldCommand == "" {
			return errors.New("no original terminal input to release")
		}
		if err := terminalReply(path, &s, o.HeldCommand, o.HeldStep); err != nil {
			return err
		}
		o.HeldCommand, o.HeldStep, o.Busy = "", "", false
	} else {
		if len(s.Turns) == 0 || s.Turns[len(s.Turns)-1]["status"] != "inProgress" {
			return errors.New("no original turn to release")
		}
		index, ok := s.Turns[len(s.Turns)-1]["fixture_index"].(float64)
		if !ok {
			return errors.New("original step missing")
		}
		if err := completeRecoveryTurn(path, &s, int(index)); err != nil {
			return err
		}
	}
	s.Recovery.HoldStep = ""
	return Write(path, s)
}

func terminalReply(path string, s *State, command, prompt string) error {
	index, err := recoveryStep(prompt)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, err := executeControlledStage(ctx, path, s, index)
	if err != nil {
		return err
	}
	s.Recovery.Orca.NativeReply = text
	s.Recovery.Orca.Lines = append(s.Recovery.Orca.Lines, "• "+text, "Worked for 1s • 12:00")
	return nil
}

func orcaFixture(path string, args []string) error {
	s, err := Read(path)
	if err != nil {
		return err
	}
	if s.Recovery == nil || s.Recovery.Orca == nil {
		return errors.New("missing single-terminal fixture")
	}
	o := s.Recovery.Orca
	if o.Environment != argument(args, "--environment") {
		return errors.New("wrong native environment routing")
	}
	if len(args) == 0 {
		return errors.New("missing terminal primitive")
	}
	key := args[0]
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		key += " " + args[1]
	}
	runtime := o.Runtime
	if o.WrongTarget {
		runtime = "other-runtime"
	}
	term := func() any {
		return map[string]any{"handle": o.Handle, "incarnationId": o.Incarnation, "worktreeId": "fixture::" + o.Worktree, "worktreePath": o.Worktree, "branch": o.Branch, "agentIdentity": o.Agent, "connected": !o.Lost, "status": map[bool]string{true: "exited", false: "running"}[o.Lost]}
	}
	worktree := func() any {
		head, _ := Git(context.Background(), o.Worktree, "rev-parse", "HEAD")
		return map[string]any{"id": "fixture::" + o.Worktree, "instanceId": "fixture-original-worktree", "repoId": "fixture", "path": o.Worktree, "branch": o.Branch, "baseRef": o.Base, "head": head, "displayName": o.Name}
	}
	var result any
	switch key {
	case "repo list":
		result = map[string]any{"repos": []any{map[string]any{"id": "fixture", "kind": "git", "path": s.Cwd}}}
	case "status":
		result = map[string]any{"runtime": map[string]any{"state": "ready", "appVersion": "e2e-terminal", "capabilities": []string{"terminal.prompt-delivery.v1"}}}
	case "worktree create":
		if o.Creates != 0 {
			return errors.New("duplicate original resource creation")
		}
		o.Name, o.Base, o.Agent = argument(args, "--name"), argument(args, "--base-branch"), argument(args, "--agent")
		o.Worktree, o.Branch = filepath.Join(o.Root, o.Name), "worker/"+o.Name
		if _, err := Git(context.Background(), s.Cwd, "worktree", "add", "-b", o.Branch, o.Worktree, o.Base); err != nil {
			return err
		}
		o.Creates++
		result = map[string]any{"worktree": worktree(), "agentTerminalHandle": o.Handle}
	case "worktree show":
		result = map[string]any{"worktree": worktree()}
	case "terminal show":
		if o.Lost {
			return errors.New("original terminal handle lost")
		}
		result = map[string]any{"terminal": term()}
	case "terminal wait":
		result = map[string]any{"wait": map[string]any{"satisfied": !o.Busy && o.Draft == ""}}
	case "terminal read":
		source := "stream"
		lines := o.Lines
		cursor := 0
		if argument(args, "--source") == "screen" || containsArgument(args, "--screen") {
			source = "screen"
		} else if token := argument(args, "--cursor"); token != "" {
			cursor, err = strconv.Atoi(token)
			if err != nil || cursor > len(lines) {
				return errors.New("original cursor unavailable")
			}
			lines = lines[cursor:]
		}
		if o.Gap && source == "stream" {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": map[string]string{"code": "terminal_cursor_invalid", "message": "original stream gap"}, "_meta": map[string]string{"runtimeId": runtime}})
		}
		result = map[string]any{"terminal": map[string]any{"handle": o.Handle, "source": source, "tail": lines, "oldestCursor": "0", "nextCursor": strconv.Itoa(len(o.Lines)), "latestCursor": strconv.Itoa(len(o.Lines)), "draft": o.Draft}}
	case "terminal send":
		if o.Busy || o.Draft != "" {
			return errors.New("input sent to busy original terminal")
		}
		prompt := argument(args, "--text")
		command := argument(args, "--retry-request")
		o.Prompt, o.NativeReply = prompt, ""
		s.Cwd = o.Worktree
		o.Sends++
		s.Dispatches++
		o.Lines = append(o.Lines, "› "+prompt)
		index, err := recoveryStep(prompt)
		if err != nil {
			return err
		}
		if s.Recovery.HoldStep == fixtureStepIDs[index] {
			o.Busy, o.HeldCommand, o.HeldStep = true, command, prompt
		} else {
			if err := terminalReply(path, &s, command, prompt); err != nil {
				return err
			}
		}
		result = map[string]any{"send": map[string]any{"handle": o.Handle, "accepted": true, "prompt": map[string]any{"requestId": argument(args, "--retry-request"), "processIncarnation": o.Incarnation, "stages": []string{"input_accepted", "turn_started"}}}}
		if o.LostSend {
			o.LostSend = false
			if err := Write(path, s); err != nil {
				return err
			}
			return errors.New("accepted original send receipt lost")
		}
	case "terminal close":
		o.Closes++
		o.Lost = true
		result = map[string]any{"close": map[string]any{"handle": o.Handle, "ptyKilled": true}}
	default:
		return fmt.Errorf("unsupported current terminal primitive %q", key)
	}
	if key == "worktree create" || key == "terminal send" || key == "terminal close" {
		if err := Write(path, s); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtime}})
}

func containsArgument(args []string, key string) bool {
	for _, arg := range args {
		if arg == key {
			return true
		}
	}
	return false
}
