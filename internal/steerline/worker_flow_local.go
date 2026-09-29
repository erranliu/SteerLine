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

func workerFlowPrompt(task TaskRecord, contract TaskContract) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" {
		return "", errors.New("worker instruction requires a fixed task, Spec and Plan")
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "SteerLine task %s: %s\n\n", task.TaskID, task.Title)
	fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement the task, commit the code, create a PR targeting %s, and report its URL. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
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
	if c.agentName() != "codex" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "resumable worker flow requires Codex"}
	}
	if c.hostName() != "local" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "Orca worker-flow delivery is deferred; only local Codex may claim a task in this release"}
	}
	command := c.Codex
	if err := resolveCommand(&command); err != nil {
		return err
	}
	c.Codex = command
	probe := append(append([]string(nil), command.Args...), "exec", "resume", "--help")
	out, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}).Output(ctx, Command{Executable: command.Executable, Args: probe})
	if err != nil || !strings.Contains(string(out), "SESSION_ID") {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "configured Codex executable does not expose exact-session resume", Cause: err}
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
	process := exec.CommandContext(ctx, c.Codex.Executable, probeArgs...)
	process.Dir, process.Stdout, process.Stderr = c.Repository, probeFile, probeFile
	if _, err := startDetachedProcess(process); err != nil {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "local host cannot retain a worker process when the center stops", Cause: err}
	}
	if err := process.Wait(); err != nil {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "detached Codex capability probe failed", Cause: err}
	}
	return nil
}

// startLocalWorkerTurn is called only after the caller has saved a fixed turn
// intent. A later invocation must provide the original native session ID; it
// never uses Codex's ambiguous --last selector or creates a replacement.
func startLocalWorkerTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, turnID, prompt string) (*Worker, string, error) {
	if run == nil || execution == nil || run.WorkflowVersion != WorkerFlowVersion || run.RunID != execution.WorkflowRunID ||
		run.ExecutionID != execution.ExecutionID || run.AttemptID == "" || !stableExecutionID(turnID) ||
		run.WorktreePath == nil || run.WorktreeBranch == nil || c.hostName() != "local" || c.agentName() != "codex" ||
		strings.TrimSpace(prompt) == "" || c.Codex.Executable == "" {
		return nil, "", errors.New("local worker turn lacks its persisted session, workspace or message identity")
	}
	if execution.SessionID != "" && !codexThreadID.MatchString(execution.SessionID) {
		return nil, "", errors.New("saved local worker session ID is not native Codex identity")
	}
	if canonical(execution.Workspace.Worktree) != canonical(*run.WorktreePath) || execution.Workspace.Branch != *run.WorktreeBranch {
		return nil, "", fmt.Errorf("worker turn workspace differs from the saved execution: execution=%q/%q run=%q/%q", execution.Workspace.Worktree, execution.Workspace.Branch, *run.WorktreePath, *run.WorktreeBranch)
	}
	dir := filepath.Join(c.RunDir, "runs", run.RunID, "turns", turnID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, "", err
	}
	events := filepath.Join(dir, "events.jsonl")
	stderr := filepath.Join(dir, "stderr.log")
	lastMessage := filepath.Join(dir, "last-message.txt")
	args := append([]string(nil), c.Codex.Args...)
	args = append(args, "exec", "--json", "--output-last-message", lastMessage, "--sandbox", c.Sandbox, "-C", *run.WorktreePath)
	writeArgs, err := codexLinkedWorktreeWriteArgs(ctx, c, *run.WorktreePath)
	if err != nil {
		return nil, "", err
	}
	args = append(args, writeArgs...)
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+c.ReasoningEffort)
	}
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	if execution.SessionID != "" {
		args = append(args, "resume", execution.SessionID)
	}
	args = append(args, "-")
	worker, err := startDurableLocalWorker(ctx, c, run, turnID, prompt, events, stderr, args)
	if err != nil {
		return nil, "", err
	}
	return worker, events, nil
}
