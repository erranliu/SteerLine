package testprotocol

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StagedBusiness is the existing executable fixture's deterministic model
// substitute. Its artifacts are real Git facts; each native input does only
// the action for its bound step. It is never reachable from production code.
func StagedBusiness(cwd, prompt, provider, remote string) (string, error) {
	git := func(args ...string) (string, error) {
		data, err := exec.Command("git", append([]string{"-C", cwd}, args...)...).CombinedOutput()
		if err != nil {
			return "", errors.New(string(data))
		}
		return strings.TrimSpace(string(data)), nil
	}
	switch {
	case strings.Contains(prompt, "Current step: implement.execute"):
		if err := os.WriteFile(filepath.Join(cwd, "stage-result.txt"), []byte("staged fixture result\n"), 0600); err != nil {
			return "", err
		}
		if _, err := git("add", "--", "stage-result.txt"); err != nil {
			return "", err
		}
		if _, err := git("-c", "user.name=Stage Fixture", "-c", "user.email=stage@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "staged candidate"); err != nil {
			return "", err
		}
		return "Implemented, verified the fixture result and committed the local candidate; no publication performed.", nil
	case strings.Contains(prompt, "Current step: implement.report"):
		head, err := git("rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		return `{"outcome":"done","outputs":{"candidate_sha":"` + head + `"},"summary":"source implementation completed"}`, nil
	case strings.Contains(prompt, "Current step: publish"):
		head, err := git("rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		branch, err := git("symbolic-ref", "--short", "HEAD")
		if err != nil {
			return "", err
		}
		if _, err := git("push", remote, "HEAD:refs/heads/"+branch); err != nil {
			return "", err
		}
		body, _ := json.Marshal(map[string]any{"url": "https://github.com/owner/repo/pull/84", "repository": "owner/repo", "base_branch": "main", "head_repository": "owner/repo", "head_branch": branch, "head_revision": head, "state": "open", "merged": false})
		if err := os.WriteFile(provider, body, 0600); err != nil {
			return "", err
		}
		return `{"outcome":"done","outputs":{"pr_url":"https://github.com/owner/repo/pull/84"},"summary":"candidate pushed and PR created"}`, nil
	}
	return "", errors.New("fixture received an unknown stage")
}

func AppendStagedTranscript(path, cwd, task, turn, final string) (string, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	var events []any
	if stat.Size() == 0 {
		events = append(events, map[string]any{"type": "session_meta", "payload": map[string]any{"id": "00000000-0000-0000-0000-000000000901", "cwd": cwd}})
	}
	events = append(events,
		map[string]any{"type": "event_msg", "payload": map[string]any{"type": "task_started", "turn_id": turn}},
		map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": "Your task ID is: " + task}}}},
		map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "phase": "final_answer", "content": []any{map[string]string{"type": "output_text", "text": final}}}},
		map[string]any{"type": "event_msg", "payload": map[string]any{"type": "task_complete", "turn_id": turn}})
	for _, event := range events {
		data, _ := json.Marshal(event)
		if _, err := file.Write(append(data, '\n')); err != nil {
			return "", err
		}
	}
	if historyPath := os.Getenv("STEERLINE_INTEGRATION_HISTORY_FILE"); historyPath != "" {
		var history struct {
			Cwd   string           `json:"cwd"`
			Turns []map[string]any `json:"turns"`
		}
		old, _ := os.ReadFile(historyPath)
		_ = json.Unmarshal(old, &history)
		history.Cwd = cwd
		history.Turns = append(history.Turns, map[string]any{"id": turn, "status": "completed", "completedAt": 1, "items": []any{map[string]any{"id": "user-" + turn, "type": "userMessage", "content": []any{map[string]string{"type": "text", "text": "Your task ID is: " + task}}}, map[string]any{"id": "final-" + turn, "type": "agentMessage", "phase": "final_answer", "text": final}}})
		raw, _ := json.Marshal(history)
		if err := os.WriteFile(historyPath, raw, 0600); err != nil {
			return "", err
		}
	}
	return path + ":00000000000000000000", file.Sync()
}

func StagedHistoryBinding(command string) error {
	_, rest, ok := strings.Cut(command, "'-config' '")
	if !ok {
		return errors.New("staged launcher missing original config")
	}
	path := strings.SplitN(rest, "'", 2)[0]
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var launch struct {
		Binding     map[string]any `json:"binding"`
		BindingPath string         `json:"binding_path"`
	}
	if err := json.Unmarshal(raw, &launch); err != nil {
		return err
	}
	launch.Binding["session_id"] = "00000000-0000-0000-0000-000000000901"
	encoded, err := json.Marshal(launch.Binding)
	if err != nil {
		return err
	}
	return os.WriteFile(launch.BindingPath, encoded, 0600)
}
