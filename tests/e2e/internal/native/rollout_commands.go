package native

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ReadBusinessRollout reads the native tool's own external evidence. It never
// knows Runner's storage layout or reads native authentication/configuration.
func ReadBusinessRollout(home, session, cwd string) ([]byte, error) {
	if home == "" || session == "" || cwd == "" {
		return nil, errors.New("native rollout binding is incomplete")
	}
	var result []byte
	err := filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "-"+session+".jsonl") {
			return nil
		}
		if result != nil {
			return errors.New("ambiguous original native rollout")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 32<<20 {
			return errors.New("native rollout exceeds bounded read")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		matched, err := BusinessRolloutMatches(raw, cwd)
		if err != nil {
			return err
		}
		var header struct {
			Payload struct {
				ID string `json:"id"`
			} `json:"payload"`
		}
		line, _, _ := bytes.Cut(raw, []byte("\n"))
		if err := json.Unmarshal(line, &header); err != nil {
			return err
		}
		if !matched || header.Payload.ID != session {
			return errors.New("native rollout belongs to another root session/worktree")
		}
		result = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("original native rollout not available")
	}
	return result, nil
}

type RolloutCommand struct {
	Command  []string `json:"command"`
	Output   string   `json:"aggregated_output"`
	ExitCode *int     `json:"exit_code"`
	Status   string   `json:"status"`
}

// RolloutTurnCommands selects native completed command items from the exact
// public session/turn, including native exit/output rather than model prose.
func RolloutTurnCommands(raw []byte, session, turn, cwd string) ([]RolloutCommand, error) {
	matched, err := BusinessRolloutMatches(raw, cwd)
	if err != nil || !matched {
		return nil, errors.New("native command rollout root differs")
	}
	var commands []RolloutCommand
	completed := false
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e struct {
			Type    string `json:"type"`
			Payload struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				Thread string `json:"thread_id"`
				Turn   string `json:"turn_id"`
				Item   struct {
					RolloutCommand
					Type string `json:"type"`
					Cwd  string `json:"cwd"`
				} `json:"item"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("invalid native rollout: %w", err)
		}
		if e.Type == "session_meta" && e.Payload.ID != session {
			return nil, errors.New("native command session differs")
		}
		if e.Type != "event_msg" || e.Payload.Turn != turn {
			continue
		}
		if e.Payload.Type == "task_complete" {
			completed = true
		}
		if e.Payload.Type != "item_completed" || e.Payload.Item.Type != "CommandExecution" {
			continue
		}
		if e.Payload.Thread != session {
			return nil, errors.New("native command thread differs")
		}
		commandCwd := e.Payload.Item.Cwd
		if strings.HasPrefix(commandCwd, "file:///") {
			u, err := url.Parse(commandCwd)
			if err != nil || u.Host != "" {
				return nil, errors.New("invalid native command cwd")
			}
			commandCwd = u.Path
			if len(commandCwd) > 2 && commandCwd[0] == '/' && commandCwd[2] == ':' {
				commandCwd = commandCwd[1:]
			}
		}
		if !samePath(commandCwd, cwd) {
			return nil, errors.New("native command cwd differs")
		}
		commands = append(commands, e.Payload.Item.RolloutCommand)
	}
	if !completed {
		return nil, ErrNativeTurnPending
	}
	return commands, nil
}
