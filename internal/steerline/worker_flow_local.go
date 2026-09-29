package steerline

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type workerTurnOutcome struct {
	SessionID    string
	NativeStatus string
	Started      bool
	Completed    bool
	LastMessage  string
	Failure      string
	PRCandidates []string
}

var workerPRURL = regexp.MustCompile(`https?://[^\s<>"']+/(?:pulls|pull)/[0-9]+`)

func workerPRClues(text string) []string {
	seen := map[string]bool{}
	var urls []string
	for _, raw := range workerPRURL.FindAllString(text, -1) {
		url := strings.TrimRight(raw, ".,;:!?)）")
		if !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

func readWorkerTurnOutcome(eventsPath string) (workerTurnOutcome, error) {
	var outcome workerTurnOutcome
	seenPR := map[string]bool{}
	addPRClues := func(message string) {
		for _, url := range workerPRClues(message) {
			if !seenPR[url] {
				seenPR[url] = true
				outcome.PRCandidates = append(outcome.PRCandidates, url)
			}
		}
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return outcome, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), JournalMaxRecord)
	for lines := 0; scanner.Scan(); lines++ {
		if lines >= 100000 {
			return outcome, errors.New("worker turn journal exceeds its observation limit")
		}
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return outcome, fmt.Errorf("worker turn journal has unreadable output: %w", err)
		}
		switch event.Type {
		case "thread.started":
			if outcome.SessionID != "" && outcome.SessionID != event.ThreadID {
				return outcome, errors.New("worker turn changed native session identity")
			}
			outcome.SessionID = event.ThreadID
		case "turn.started":
			outcome.Started = true
		case "turn.completed":
			outcome.Completed = true
			outcome.NativeStatus = "completed"
		case "turn.failed", "error":
			outcome.Failure = event.Error.Message
		case "item.completed":
			if event.Item.Type == "agent_message" && event.Item.Text != "" {
				outcome.LastMessage = event.Item.Text
				addPRClues(event.Item.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return outcome, err
	}
	lastPath := filepath.Join(filepath.Dir(eventsPath), "last-message.txt")
	if info, err := os.Stat(lastPath); err == nil && info.Size() <= 1<<20 {
		if data, err := os.ReadFile(lastPath); err == nil && strings.TrimSpace(string(data)) != "" {
			outcome.LastMessage = string(data)
			addPRClues(outcome.LastMessage)
		}
	}
	return outcome, nil
}

func workerFlowPrompt(task TaskRecord, contract TaskContract, localPublish bool) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" {
		return "", errors.New("worker instruction requires a fixed task, Spec and Plan")
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "SteerLine task %s: %s\n\n", task.TaskID, task.Title)
	if localPublish {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement and verify the task, then stage only the intended changes. SteerLine will commit, push, and create a draft PR targeting %s from the host after your explicit publication request. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	} else {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement the task, commit the code, create a PR targeting %s, and report its URL. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	}
	fmt.Fprintf(&prompt, "Spec: %s at %s (SHA-256 %s)\n", contract.SpecRef.Path, contract.SpecRef.Commit, contract.SpecRef.SHA256)
	fmt.Fprintf(&prompt, "Plan: %s at %s (SHA-256 %s)\n", contract.PlanRef.Path, contract.PlanRef.Commit, contract.PlanRef.SHA256)
	for _, ref := range contract.CommonRefs {
		fmt.Fprintf(&prompt, "Common instruction: %s at %s (SHA-256 %s)\n", ref.Path, ref.Commit, ref.SHA256)
	}
	for _, ref := range contract.AcceptanceRefs {
		fmt.Fprintf(&prompt, "Acceptance reference: %s at %s (SHA-256 %s)\n", ref.Path, ref.Commit, ref.SHA256)
	}
	return prompt.String(), nil
}

func requireWorkerFlowCapabilities(ctx context.Context, c Config) error {
	if c.hostName() == "orca" {
		if c.Model != "" || c.ReasoningEffort != "" || c.Profile != "" || c.Sandbox != "workspace-write" {
			return &BackendError{Code: BackendUnsupported, Diagnostic: "Orca starts the selected agent with its own settings; model, effort, Codex profile and sandbox overrides are unavailable"}
		}
		if _, err := workerOrcaCall(ctx, c, "status", "--json"); err != nil {
			return err
		}
		_, err := nativeOrcaRepo(ctx, c)
		return err
	}
	if c.agentName() != "codex" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "resumable worker flow requires Codex"}
	}
	if c.hostName() != "local" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "this worker-flow host is not supported"}
	}
	command := c.Codex
	if err := resolveCommand(&command); err != nil {
		return err
	}
	c.Codex = command
	nativeExecutable, err := codexAppServerExecutable(command)
	if err != nil {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "local Codex app-server needs a directly bound native process", Cause: err}
	}
	runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	configuredVersion, configuredErr := runner.Output(ctx, Command{Executable: command.Executable, Args: append(append([]string(nil), command.Args...), "--version")})
	nativeVersion, nativeErr := runner.Output(ctx, Command{Executable: nativeExecutable, Args: append(append([]string(nil), command.Args...), "--version")})
	if configuredErr != nil || nativeErr != nil || strings.TrimSpace(string(configuredVersion)) == "" || strings.TrimSpace(string(configuredVersion)) != strings.TrimSpace(string(nativeVersion)) {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "configured Codex and native app-server versions differ or cannot be verified", Cause: errors.Join(configuredErr, nativeErr)}
	}
	probe := append(append([]string(nil), command.Args...), "app-server", "--help")
	out, err := runner.Output(ctx, Command{Executable: nativeExecutable, Args: probe})
	if err != nil || !strings.Contains(string(out), "--listen") {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "configured Codex executable does not expose app-server WebSocket transport", Cause: err}
	}
	if !nativeCodexHistoryAvailable(ctx, c) {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "configured Codex session history cannot be read for exact turn reconciliation"}
	}
	probeFile, err := os.CreateTemp(c.RunDir, "worker-detach-probe-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(probeFile.Name())
	defer probeFile.Close()
	probeArgs := append(append([]string(nil), c.Codex.Args...), "--version")
	process := exec.CommandContext(ctx, nativeExecutable, probeArgs...)
	process.Dir, process.Stdout, process.Stderr = c.Repository, probeFile, probeFile
	if _, err := startDetachedProcess(process); err != nil {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "local host cannot retain a worker process when the center stops", Cause: err}
	}
	if err := process.Wait(); err != nil {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "detached Codex capability probe failed", Cause: err}
	}
	return nil
}
