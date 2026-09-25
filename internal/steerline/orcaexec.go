package steerline

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// Orca execution model v2: Orca creates the worktree and launches the agent;
// Steerline only prepares the prompt/result paths, waits for the R-a result
// file, and keeps task/completion authority. See docs/hosting.md.

const orcaResultFile = ".steerline-result.json"

type orcaWorktree struct {
	ID      string `json:"id"`
	HostID  string `json:"hostId"`
	Path    string `json:"path"`
	Head    string `json:"head"`
	Branch  string `json:"branch"`
	BaseRef string `json:"baseRef"`
}

type orcaWorktreeCreated struct {
	RuntimeID           string       `json:"-"`
	Worktree            orcaWorktree `json:"worktree"`
	AgentTerminalHandle string       `json:"agentTerminalHandle"`
	StartupTerminal     struct {
		Handle string `json:"handle"`
	} `json:"startupTerminal"`
}

func orcaSafeName(taskID, runID string) string {
	name := regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(taskID, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "task"
	}
	short := runID
	if len(short) > 8 {
		short = short[:8]
	}
	return name + "-" + short
}

func (c orcaClient) worktreeCreate(ctx context.Context, name, agent, baseRef, prompt string, activate bool) (orcaWorktreeCreated, error) {
	args := []string{"worktree", "create", "--name", name, "--no-parent", "--json"}
	if agent != "" {
		args = append(args, "--agent", agent)
	}
	if baseRef != "" {
		args = append(args, "--base-branch", baseRef)
	}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	if activate {
		args = append(args, "--activate")
	}
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), args...)
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return orcaWorktreeCreated{}, fmt.Errorf("%v (%w)", e, callErr)
		}
		return orcaWorktreeCreated{}, e
	}
	var res orcaWorktreeCreated
	if e := json.Unmarshal(env.Result, &res); e != nil {
		return orcaWorktreeCreated{}, e
	}
	res.RuntimeID = env.Meta.RuntimeID
	if strings.TrimSpace(res.Worktree.Path) == "" {
		return orcaWorktreeCreated{}, errors.New("orca worktree create returned no path")
	}
	return res, nil
}

func (c orcaClient) worktreeRemove(ctx context.Context, id string) {
	if id == "" {
		return
	}
	_, _ = c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "worktree", "rm", "--worktree", "id:"+id, "--force", "--json")
}

func (c orcaClient) terminalSend(ctx context.Context, identity ProcessIdentity, text string) (ReplyReceipt, error) {
	if identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return ReplyReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "send requires a fixed terminal identity"}
	}
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "send", "--terminal", identity.Terminal, "--text", text, "--enter", "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return ReplyReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca input response is uncertain", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Send struct {
			Handle   string `json:"handle"`
			Accepted *bool  `json:"accepted"`
			Prompt   struct {
				ProcessIncarnation string   `json:"processIncarnation"`
				Stages             []string `json:"stages"`
			} `json:"prompt"`
		} `json:"send"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return ReplyReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca input receipt is invalid", Cause: err}
	}
	if env.Meta.RuntimeID != identity.RuntimeID || result.Send.Handle != identity.Terminal || result.Send.Accepted == nil ||
		(result.Send.Prompt.ProcessIncarnation != "" && result.Send.Prompt.ProcessIncarnation != identity.Incarnation) {
		return ReplyReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca input receipt does not confirm the fixed terminal identity"}
	}
	if !*result.Send.Accepted {
		return ReplyReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "Orca did not accept the input"}
	}
	if result.Send.Prompt.ProcessIncarnation != identity.Incarnation {
		return ReplyReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca accepted input without the fixed process incarnation"}
	}
	receipt := ReplyReceipt{InputAccepted: true}
	for _, stage := range result.Send.Prompt.Stages {
		if stage == "turn_started" {
			receipt.TurnStarted = true
		}
	}
	return receipt, nil
}

func (c orcaClient) terminalCreate(ctx context.Context, worktreePath, title, command string) (orcaTerminal, error) {
	args := []string{"terminal", "create", "--worktree", "path:" + filepath.ToSlash(worktreePath), "--title", title, "--shell", c.cfg.Orca.Shell, "--command", command, "--json"}
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), args...)
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return orcaTerminal{}, fmt.Errorf("%v (%w)", e, callErr)
		}
		return orcaTerminal{}, e
	}
	var res struct {
		Terminal            orcaTerminal `json:"terminal"`
		Handle              string       `json:"handle"`
		AgentTerminalHandle string       `json:"agentTerminalHandle"`
	}
	if e = json.Unmarshal(env.Result, &res); e != nil {
		return orcaTerminal{}, e
	}
	if res.Terminal.Handle == "" {
		res.Terminal.Handle = res.Handle
	}
	if res.Terminal.Handle == "" {
		res.Terminal.Handle = res.AgentTerminalHandle
	}
	if res.Terminal.Handle == "" {
		terms, listErr := c.terminalListForWorktree(ctx, worktreePath)
		if listErr != nil {
			return orcaTerminal{}, fmt.Errorf("orca terminal create returned no handle and list failed: %w", listErr)
		}
		for _, term := range terms {
			if term.Title == title && term.Handle != "" {
				return term, nil
			}
		}
		return orcaTerminal{}, errors.New("orca terminal create returned no matching terminal handle")
	}
	return res.Terminal, nil
}

func (c orcaClient) terminalListForWorktree(ctx context.Context, worktreePath string) ([]orcaTerminal, error) {
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "list", "--worktree", "path:"+filepath.ToSlash(worktreePath), "--json")
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return nil, callErr
		}
		return nil, e
	}
	var res struct {
		Terminals []orcaTerminal `json:"terminals"`
	}
	if e := json.Unmarshal(env.Result, &res); e != nil {
		return nil, e
	}
	return res.Terminals, nil
}

// startOrcaRun creates the Orca worktree, hands the agent its dispatch pointer as
// initial work via `--prompt`, and returns a Worker that settles when the R-a result file
// appears (or the stall timeout fires). It mutates r and intent with the facts
// Orca returned so the caller persists them.
func startOrcaRun(ctx context.Context, c Config, t Task, r *Run, source BeadsContext, intent *WorkspaceIntent, dir string) (*Worker, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	client := orcaClient{cfg: c}
	name := orcaSafeName(t.ID, r.RunID)
	baseRef := "refs/steerline/bases/" + r.RunID
	// Orca's built-in agent selector cannot pass model/reasoning overrides. For a
	// configured Codex model or verified public history, create the worktree first
	// and prepare the exact session launch. Other agents retain --agent/--prompt.
	nativeHistory := c.agentName() == "codex" && nativeCodexHistoryAvailable(ctx, c)
	customCodex := c.needsCodexExecutable() || nativeHistory
	promptPath := filepath.Join(dir, "prompt.txt")
	pointer := "Read and execute " + promptPath + " now. Follow it exactly."
	bootstrap := "SteerLine is preparing the dispatch. Wait for a follow-up message in this terminal. Do not edit files or act on a task yet."
	var created orcaWorktreeCreated
	var dispatchedIdentity *ProcessIdentity
	var e error
	if customCodex {
		created, e = client.worktreeCreate(ctx, name, "", baseRef, "", c.Orca.Activate)
	} else {
		created, e = client.worktreeCreate(ctx, name, c.agentName(), baseRef, bootstrap, c.Orca.Activate)
	}
	if e != nil {
		return nil, e
	}
	path := filepath.FromSlash(created.Worktree.Path)
	branch := strings.TrimPrefix(created.Worktree.Branch, "refs/heads/")
	if branch == "" || created.RuntimeID == "" {
		return nil, errors.New("Orca create omitted the actual branch or runtime identity")
	}
	handle := ""
	if !customCodex {
		handle = created.AgentTerminalHandle
		if handle == "" {
			handle = created.StartupTerminal.Handle
		}
		if handle == "" {
			return nil, errors.New("orca worktree create returned no agent terminal handle")
		}
	}
	r.WorktreePath = ptr(path)
	r.WorktreeBranch = ptr(branch)
	r.BaseCommit = ptr(intent.BaseCommit)
	intent.Path = path
	intent.Branch = branch
	if nativeHistory && created.Worktree.HostID != "local" {
		return nil, unsupported("Codex history launch requires a verified local Orca execution host")
	}
	// The create receipt is not proof that Git actually used the pinned base.
	// Inspect before handing the native agent any task instructions.
	if e := verifyWorkspace(ctx, c, intent, true); e != nil {
		return nil, e
	}

	oc := c
	oc.workerEnv = map[string]string{"BEADS_DIR": source.Directory, "BEADS_ACTOR": r.Claimant}
	if e := AtomicWrite(filepath.Join(dir, "result.schema.json"), ResultSchema); e != nil {
		return nil, e
	}
	prompt, e := Prompt(oc, t, r)
	if e != nil {
		return nil, e
	}
	prompt += orcaPromptAddendum(promptPath, source.Directory, r.Claimant)
	if e := AtomicWrite(promptPath, []byte(prompt)); e != nil {
		return nil, e
	}
	if !customCodex {
		term, e := client.showTerminal(ctx, handle)
		if e != nil {
			return nil, e
		}
		if term.Handle != handle || term.RuntimeID != created.RuntimeID || term.IncarnationID == "" || !term.Connected || term.Orphaned || term.Status == "exited" {
			return nil, errors.New("Orca created terminal identity or liveness is unverified")
		}
		identity := ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable}
		dispatchedIdentity = &identity
		if _, e := client.terminalSend(ctx, identity, pointer); e != nil {
			return nil, e
		}
	}
	if customCodex {
		command, ce := orcaCodexLaunchCommand(c, pointer)
		if nativeHistory {
			command, ce = orcaCodexHistoryLaunchCommand(c, r, dir, path, pointer)
		}
		if ce != nil {
			return nil, ce
		}
		term, ce := client.terminalCreate(ctx, path, "SteerLine "+name, command)
		if ce != nil {
			return nil, ce
		}
		startupHandle := created.StartupTerminal.Handle
		if startupHandle != "" && startupHandle != term.Handle {
			receipt, closeErr := client.closeTerminalChecked(ctx, startupHandle, created.RuntimeID)
			if closeErr != nil {
				return nil, closeErr
			}
			if receipt.Disposition != "confirmed" {
				return nil, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca startup shell close was not confirmed"}
			}
		}
		handle = term.Handle
	}

	resultInWorktree := filepath.Join(path, orcaResultFile)
	term, e := client.showTerminal(ctx, handle)
	if e != nil {
		return nil, e
	}
	if term.Handle != handle || term.RuntimeID != created.RuntimeID || term.IncarnationID == "" {
		return nil, errors.New("Orca started terminal identity is unverified")
	}
	if dispatchedIdentity != nil && term.IncarnationID != dispatchedIdentity.Incarnation {
		return nil, errors.New("Orca terminal incarnation changed after dispatch")
	}
	r.Process = ptr(ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable})
	if nativeHistory {
		if term.ExecutionHostID != "local" {
			return nil, unsupported("Codex history cannot read an unverified or remote Orca execution host")
		}
		binding, bindErr := waitOrcaCodexHistoryBinding(ctx, filepath.Join(dir, codexHistoryBindingFile), c, r, path)
		if bindErr != nil {
			return nil, bindErr
		}
		term, bindErr = waitOrcaCodexHistoryAgent(ctx, client, term)
		if bindErr != nil {
			return nil, bindErr
		}
		binding.ExternalRef = conversationExternalRef(&ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID})
		data, marshalErr := json.MarshalIndent(binding, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		if e = AtomicWrite(filepath.Join(dir, codexHistoryBindingFile), append(data, '\n')); e != nil {
			return nil, e
		}
		r.SessionID = ptr(binding.SessionID)
	}
	proc := &orcaProcess{cfg: c, term: term}
	proc.term.Handle = handle
	attemptID := r.AttemptID
	if attemptID == "" {
		attemptID = r.RunID
	}
	proc.ref = ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(c), Agent: agentFor(c), RunID: r.RunID, AttemptID: attemptID, WorkspacePath: path, WorkspaceBranch: branch, BaseCommit: deref(r.BaseCommit), External: proc.Identity(), Generation: proc.Identity().Incarnation}
	proc.outputSegment = "start:" + r.RunID + ":" + attemptID
	return superviseOrcaResult(c, proc, resultInWorktree, *r.ResultPath), nil
}

func waitOrcaCodexHistoryAgent(ctx context.Context, client orcaClient, fixed orcaTerminal) (orcaTerminal, error) {
	ctx, cancel := context.WithTimeout(ctx, positiveDuration(client.cfg.CommandTimeout, 30*time.Second))
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		term, err := client.showTerminal(ctx, fixed.Handle)
		if err != nil {
			return orcaTerminal{}, err
		}
		if term.Handle != fixed.Handle || term.RuntimeID != fixed.RuntimeID || term.IncarnationID != fixed.IncarnationID || term.ExecutionHostID != "local" || term.Orphaned || !term.Connected || term.Status == "exited" {
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca terminal changed or exited during Codex session launch"}
		}
		if term.AgentIdentity == "codex" {
			return term, nil
		}
		select {
		case <-ctx.Done():
			return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not prove that the bound Codex agent started", Cause: ctx.Err()}
		case <-ticker.C:
		}
	}
}

func orcaCodexLaunchCommand(c Config, prompt string) (string, error) {
	if strings.TrimSpace(c.Codex.Executable) == "" {
		return "", errors.New("orca Codex launch requires codex.executable")
	}
	args := append([]string{}, c.Codex.Args...)
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+c.ReasoningEffort)
	}
	if c.Sandbox != "" {
		args = append(args, "--sandbox", c.Sandbox)
	}
	args = append(args, prompt)
	if runtime.GOOS == "windows" {
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = powershellLiteral(arg)
		}
		script := "$codex = " + powershellLiteral(c.Codex.Executable) + "\n$codexArgs = @(" + strings.Join(quoted, ", ") + ")\n& $codex @codexArgs"
		return "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encodePowerShell(script), nil
	}
	command := []string{shellQuote(c.Codex.Executable)}
	for _, arg := range args {
		command = append(command, shellQuote(arg))
	}
	return strings.Join(command, " "), nil
}

func orcaCodexHistoryLaunchCommand(c Config, run *Run, dir, workspace, prompt string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	b := codexHistoryBinding{Version: 1, ProjectID: c.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: nonEmpty(run.ExecutionID, legacyExecutionID(run.RunID)), AttemptID: nonEmpty(run.AttemptID, run.RunID), Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, TargetID: run.Target.ID, Workspace: workspace, Command: codexReaderCommand(c)}
	args := []string{}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+c.ReasoningEffort)
	}
	if c.Sandbox != "" {
		args = append(args, "--sandbox", c.Sandbox)
	}
	request := codexSessionLaunch{Binding: b, BindingPath: filepath.Join(dir, codexHistoryBindingFile), Args: args, Prompt: prompt, Sandbox: c.Sandbox}
	path := filepath.Join(dir, "codex-session-launch.json")
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return "", err
	}
	if err = AtomicWrite(path, append(data, '\n')); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encodePowerShell("& "+powershellLiteral(self)+" 'codex-session-launch' '-config' "+powershellLiteral(path)), nil
	}
	return shellQuote(self) + " codex-session-launch -config " + shellQuote(path), nil
}

func waitOrcaCodexHistoryBinding(ctx context.Context, path string, c Config, run *Run, workspace string) (codexHistoryBinding, error) {
	ctx, cancel := context.WithTimeout(ctx, positiveDuration(c.CommandTimeout, 30*time.Second))
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			var b codexHistoryBinding
			if err = json.Unmarshal(raw, &b); err != nil {
				return b, err
			}
			if err = b.validate(); err != nil {
				return b, err
			}
			if b.ProjectID != c.identity || b.WorkflowID != run.RunID || b.ExecutionID != nonEmpty(run.ExecutionID, legacyExecutionID(run.RunID)) || b.AttemptID != nonEmpty(run.AttemptID, run.RunID) || b.TargetID != run.Target.ID || canonical(b.Workspace) != canonical(workspace) {
				return b, errors.New("Orca Codex session binding does not match the prepared attempt")
			}
			return b, nil
		}
		if !os.IsNotExist(err) {
			return codexHistoryBinding{}, err
		}
		select {
		case <-ctx.Done():
			return codexHistoryBinding{}, fmt.Errorf("Orca Codex session binding unavailable: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	words := utf16.Encode([]rune(script))
	data := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(data[i*2:], word)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func orcaPromptAddendum(promptPath, beadsDir, claimant string) string {
	return "\n\nOrca-hosted execution: there is no structured output channel. Your working directory is the worktree root. After completing all work above, write the required final JSON object (a single JSON value, no Markdown fence, no extra text) to `.steerline-result.json` in that directory; do not print it only to the terminal.\n" +
		"For Beads commands use BEADS_DIR=" + beadsDir + " and BEADS_ACTOR=" + claimant + ".\n" +
		"Read the full dispatch prompt at " + promptPath + ".\n"
}

// superviseOrcaResult watches for the R-a result file and terminal state. It
// never treats a result file as task completion: the service still verifies the
// PR merge and Beads close.
func superviseOrcaResult(c Config, proc *orcaProcess, resultInWorktree, resultOut string) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	proc.cancel = cancel
	w := &Worker{proc: proc, Events: make(chan WorkerEvent), Done: make(chan error, 1), observationDone: make(chan struct{})}
	outputDone := make(chan struct{})
	if proc.ref.Backend == BackendOrca && proc.ref.External.Terminal != "" {
		go func() {
			defer close(outputDone)
			collectOrcaTerminalOutput(ctx, c, proc, w.Events)
		}()
	} else {
		close(outputDone)
	}
	stall := positiveDuration(c.StallTimeout, 5*time.Minute)
	go func() {
		defer func() {
			cancel()
			<-outputDone
			close(w.Events)
			close(w.observationDone)
		}()
		var lastOutput int64
		lastProgress := time.Now()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca observation detached without proving execution exit", Cause: ctx.Err()}
				return
			case <-ticker.C:
			}
			term, e := inspectBoundOrcaTerminal(ctx, c, proc.Identity())
			if e != nil {
				w.Done <- e
				return
			}
			if term.Status != "exited" && (term.Orphaned || !term.Connected) {
				w.Done <- &BackendError{Code: BackendUnreachable, Diagnostic: "original Orca terminal liveness is unverifiable"}
				return
			}
			if f, openErr := os.Open(resultInWorktree); openErr == nil {
				b, readErr := io.ReadAll(io.LimitReader(f, (2<<20)+1))
				closeErr := f.Close()
				if readErr != nil || closeErr != nil || len(b) > 2<<20 {
					w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca result cannot be read within the 2 MiB bound", Cause: errors.Join(readErr, closeErr)}
					return
				}
				// Do not publish output after the external identity was replaced.
				if _, e := inspectBoundOrcaTerminal(ctx, c, proc.Identity()); e != nil {
					w.Done <- e
					return
				}
				if e := AtomicWrite(resultOut, b); e != nil {
					w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca result persistence failed", Cause: e}
					return
				}
				w.Done <- nil
				return
			} else if !os.IsNotExist(openErr) {
				w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca result file is inaccessible", Cause: openErr}
				return
			}
			if term.AgentWait != nil {
				if proc.ref.Backend == BackendOrca && proc.ref.RunID != "" {
					waitSegment := "orca-host-wait-v1:" + proc.outputSegment
					waitEvent := WorkerEvent{RunID: proc.ref.RunID, AttemptID: proc.ref.AttemptID, Source: "orca-terminal-v2", SourceEventID: "host-wait:" + proc.outputSegment, SourceIdentity: "terminal:" + proc.ref.External.RuntimeID + ":" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation, SourceSegment: waitSegment, SourceSeq: 1, Session: "terminal:" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation, InputWait: true, Diagnostic: "Orca reports a host-level agent wait; no SteerLine business Decision was created", Activity: ActivityEvent{Source: "orca", Kind: "note", Title: "host input wait observed"}}
					if !sendConversationWorkerEvent(ctx, w.Events, waitEvent) {
						w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca observation detached without proving execution exit", Cause: ctx.Err()}
						return
					}
				}
				w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca agent is waiting for human input"}
				return
			}
			if term.LastOutputAt != lastOutput {
				lastOutput = term.LastOutputAt
				lastProgress = time.Now()
			}
			if term.Status == "exited" {
				w.Done <- errors.New("orca terminal ended without a result file")
				return
			}
			if stall > 0 && time.Since(lastProgress) > stall {
				w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca execution stalled; the terminal may still be running"}
				return
			}
		}
	}()
	return w
}

func collectOrcaTerminalOutput(ctx context.Context, c Config, proc *orcaProcess, events chan<- WorkerEvent) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	cursor := ""
	var sequence uint64
	unavailable := false
	continuationSegment := strings.HasPrefix(proc.outputSegment, "reply:")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		batch, err := (orcaTerminalBackend{config: c}).ReadOutput(ctx, proc.ref, cursor, 1000)
		if err != nil {
			if !unavailable {
				sequence++
				segment := "terminal-observation-v1:" + proc.outputSegment + ":unavailable"
				identity := "terminal:" + proc.ref.External.RuntimeID + ":" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation
				event := WorkerEvent{RunID: proc.ref.RunID, AttemptID: proc.ref.AttemptID, Source: "orca-terminal-v2", SourceEventID: fmt.Sprintf("gap:%s:%d", segment, sequence), SourceIdentity: identity, SourceSegment: segment, SourceSeq: sequence, SourceCursor: cursor, SourceGap: true, Session: "terminal:" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation, Diagnostic: err.Error(), Activity: ActivityEvent{Source: "orca", Kind: "unknown", Title: "terminal output unavailable"}}
				if !sendConversationWorkerEvent(ctx, events, event) {
					return
				}
				unavailable = true
			}
			continue
		}
		unavailable = false
		source := nonEmpty(batch.Source, "unknown")
		segment := "terminal-observation-v1:" + proc.outputSegment + ":" + source
		identity := "terminal:" + proc.ref.External.RuntimeID + ":" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation + ":" + source
		session := "terminal:" + proc.ref.External.Terminal + ":" + proc.ref.External.Incarnation
		gap := continuationSegment || batch.SourceChanged || batch.Gap
		diagnostic := "terminal output is unstructured text; turn and message phase are unknown"
		if continuationSegment {
			diagnostic = "continuation output observer opened a new source segment without the previous persisted cursor"
		} else if batch.SourceChanged {
			diagnostic = "Orca terminal output source changed"
		} else if batch.Gap {
			diagnostic = "Orca terminal output retention gap"
		}
		if len(batch.Data) > 0 || gap {
			sequence++
			cursorDigest := digestValue(struct {
				Source        string
				Cursor        string
				SourceChanged bool
				Gap           bool
			}{source, batch.NextCursor, batch.SourceChanged, batch.Gap})
			eventID := "cursor:" + cursorDigest
			if len(batch.Data) == 0 {
				eventID = "gap:" + cursorDigest
			}
			title := "terminal output observed"
			kind := "note"
			if gap {
				title, kind = "terminal output gap", "unknown"
			}
			event := WorkerEvent{RunID: proc.ref.RunID, AttemptID: proc.ref.AttemptID, Source: "orca-terminal-v2", SourceEventID: eventID, SourceIdentity: identity, SourceCursor: batch.NextCursor, SourceSeq: sequence, SourceSegment: segment, Session: session, Raw: append([]byte(nil), batch.Data...), SourceGap: gap, SourceChanged: batch.SourceChanged, Diagnostic: diagnostic, Activity: ActivityEvent{Source: "orca", Kind: kind, Title: title}}
			if !sendConversationWorkerEvent(ctx, events, event) {
				return
			}
			cursor = batch.NextCursor
		}
		if continuationSegment {
			continuationSegment = false
		}
	}
}

func sendConversationWorkerEvent(ctx context.Context, events chan<- WorkerEvent, event WorkerEvent) bool {
	if event.Persisted == nil {
		event.Persisted = make(chan error, 1)
	}
	for {
		select {
		case events <- event:
		case <-ctx.Done():
			return false
		}
		select {
		case err := <-event.Persisted:
			if err == nil {
				return true
			}
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return false
			case <-timer.C:
			}
		case <-ctx.Done():
			return false
		}
	}
}
