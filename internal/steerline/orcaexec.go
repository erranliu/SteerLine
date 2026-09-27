package steerline

import (
	"bytes"
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
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

// Local and Orca execute the same fixed native step protocol. Orca supplies a
// terminal on the already-verified center worktree; it does not create a
// whole-run worktree or return a whole-run result artifact.

func (c orcaClient) terminalCreate(ctx context.Context, worktreePath, title, command string) (orcaTerminal, error) {
	// Native steps should be visible in Orca while they run. Focus the
	// terminal so the user can follow the exact task worktree and step.
	args := []string{"terminal", "create", "--worktree", "path:" + filepath.ToSlash(worktreePath), "--title", title, "--shell", c.cfg.Orca.Shell, "--command", command, "--focus", "--json"}
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
		if term.Handle != fixed.Handle || term.RuntimeID != fixed.RuntimeID || term.IncarnationID != fixed.IncarnationID || term.ExecutionHostID != "local" || term.Orphaned || !term.Connected || term.exited() {
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

func (s *Service) startOrcaMinimalStepWorker(ctx context.Context, c Config, task TaskRecord, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord, workerInput any, contract StepContract, dir, result string, input MinimalStepInput, env map[string]string) (*Worker, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || c.hostName() != "orca" || run.Backend != string(BackendOrca) || run.Protocol != string(ProtocolOrcaTerminalV2) {
		return nil, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "native Orca step lacks its fixed terminal and workspace identity"}
	}
	if contract.ExecutionSite != "target" || contract.Environment != nil && len(contract.Environment) != 0 {
		return nil, &BackendError{Code: BackendRejected, Diagnostic: "Orca native step contract cannot be launched with its configured target environment"}
	}
	nativeInput, ok := workerInput.(NativeTaskWorkerContext)
	if !ok {
		return nil, nativeErr("contract_mismatch", "Orca worker input is missing native-task/v1 context")
	}
	if err := validateNativeTaskWorkerContext(nativeInput, s.Config.identity, task.TaskID, run, workflow, execution, attempt); err != nil {
		return nil, err
	}
	schema := filepath.Join(dir, "step-result.schema.json")
	if err := AtomicWrite(schema, MinimalStepResultSchema); err != nil {
		return nil, err
	}
	inputPath := filepath.Join(dir, "native-task-input.json")
	promptPath := filepath.Join(dir, "prompt.txt")
	if err := writeNativeMinimalPrompt(promptPath, nativeInput, contract); err != nil {
		return nil, err
	}
	inputBytes, err := json.MarshalIndent(workerInput, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = AtomicWrite(inputPath, append(inputBytes, '\n')); err != nil {
		return nil, err
	}
	stepExecutable, stepArgs, stdinPath, outputPath, captureStdout, err := s.orcaNativeStepCommand(c, run, attempt, input, contract, schema, inputPath, promptPath, result)
	if err != nil {
		return nil, err
	}
	launcher, err := writeNativeOrcaBatchRunner(ctx, c, run, attempt, stepExecutable, stepArgs, stdinPath, outputPath, captureStdout, env)
	if err != nil {
		return nil, err
	}
	attempt.NativeLauncherPath = launcher.path
	attempt.NativeLauncherDigest = operationDigest(launcher.bytes)
	attempt.UpdatedAt = now()
	if err = s.save(); err != nil {
		_ = launcher.cleanup()
		return nil, err
	}
	command := "call " + filepath.Base(launcher.path) + " & exit"
	client := orcaClient{cfg: c}
	title := "SteerLine " + run.RunID[:8] + " " + attempt.StepID + " " + attempt.AttemptID[:8]
	term, err := client.terminalCreate(ctx, *run.WorktreePath, title, command)
	if err != nil {
		return nil, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca may have created the fixed native step terminal", Cause: err}
	}
	if term.Handle == "" {
		return nil, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca created a native step terminal without a usable handle"}
	}
	term, err = client.showTerminal(ctx, term.Handle)
	if err != nil {
		return nil, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca created a native step terminal whose identity cannot be confirmed", Cause: err}
	}
	if term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" || term.Orphaned || term.ExecutionHostID != "local" || term.WorktreePath != "" && canonical(filepath.FromSlash(term.WorktreePath)) != canonical(*run.WorktreePath) || term.Branch != "" && strings.TrimPrefix(term.Branch, "refs/heads/") != *run.WorktreeBranch || !term.exited() && !term.Connected {
		return nil, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca native step terminal does not match the fixed local workspace or live identity"}
	}
	proc := &orcaProcess{cfg: c, term: term, launcher: launcher}
	proc.ref = ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(c), Agent: agentFor(c), RunID: run.RunID, AttemptID: attempt.AttemptID, WorkspacePath: *run.WorktreePath, WorkspaceBranch: *run.WorktreeBranch, BaseCommit: *run.BaseCommit, External: proc.Identity(), Generation: proc.Identity().Incarnation}
	proc.outputSegment = "native-step:" + attempt.AttemptID
	worker := superviseOrcaStep(c, proc, result)
	return worker, nil
}

func validateNativeTaskWorkerContext(value any, project, taskID string, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord) error {
	input, ok := value.(NativeTaskWorkerContext)
	if !ok || run == nil || workflow == nil || execution == nil || attempt == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || input.SchemaVersion != NativeTaskInputSchema || input.ProjectID != project || input.TaskID != taskID || input.WorkflowRunID != workflow.WorkflowRunID || input.ExecutionID != execution.ExecutionID || input.AttemptID != attempt.AttemptID || input.CurrentStep != attempt.StepID || input.Contract.Digest != input.ContractDigest || input.Contract.Repository != input.Repository || input.Branch != *run.WorktreeBranch || input.BaseCommit != *run.BaseCommit || input.StepInput.TaskID != taskID || input.StepInput.AttemptID != attempt.AttemptID {
		return nativeErr("contract_mismatch", "Orca worker input does not match the fixed native task execution")
	}
	return nil
}

func (s *Service) orcaNativeStepCommand(c Config, run *Run, attempt *StepAttemptRecord, input MinimalStepInput, contract StepContract, schema, inputPath, promptPath, result string) (string, []string, string, string, bool, error) {
	workConfig := c
	workConfig.Repository = deref(run.WorktreePath)
	if contract.Provider == "command" {
		executable, args, err := commandStepArgs(workConfig, run, attempt, input, contract, result)
		if err != nil {
			return "", nil, "", "", false, err
		}
		return executable, args, inputPath, result, true, nil
	}
	if contract.Provider != "agent_skill" {
		return "", nil, "", "", false, fmt.Errorf("unsupported Orca native step provider %q", contract.Provider)
	}
	if workConfig.agentName() != "codex" || workConfig.Codex.Executable == "" {
		return "", nil, "", "", false, UnsupportedAdapterOperation("native-task/v1", "Orca target agent_skill steps currently require Codex")
	}
	args := append([]string(nil), workConfig.Codex.Args...)
	args = append(args, "exec", "--json", "--output-schema", schema, "--output-last-message", result, "--sandbox", workConfig.Sandbox)
	if workConfig.Model != "" {
		args = append(args, "--model", workConfig.Model)
	}
	if workConfig.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+workConfig.ReasoningEffort)
	}
	if workConfig.Profile != "" {
		args = append(args, "--profile", workConfig.Profile)
	}
	args = append(args, "-")
	return workConfig.Codex.Executable, args, promptPath, "", false, nil
}

type orcaStepLauncher struct {
	path           string
	bytes          []byte
	excludePath    string
	excludeBefore  []byte
	excludeAfter   []byte
	excludeExisted bool
}

func (l *orcaStepLauncher) cleanup() error {
	if l == nil {
		return nil
	}
	if data, err := os.ReadFile(l.path); err == nil {
		if !bytes.Equal(data, l.bytes) {
			return errors.New("Orca native launcher changed before cleanup")
		}
		if err = os.Remove(l.path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	current, err := os.ReadFile(l.excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(current, l.excludeAfter) {
		return errors.New("Git exclude file changed while the Orca native step ran")
	}
	return restoreOrcaExclude(l.excludePath, l.excludeBefore, l.excludeExisted)
}

func restoreOrcaExclude(path string, before []byte, existed bool) error {
	if existed {
		return AtomicWrite(path, before)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeNativeOrcaBatchRunner(ctx context.Context, c Config, run *Run, attempt *StepAttemptRecord, executable string, args []string, stdinPath, outputPath string, captureStdout bool, env map[string]string) (*orcaStepLauncher, error) {
	if run == nil || run.WorktreePath == nil || attempt == nil || !stableExecutionID(attempt.AttemptID) {
		return nil, errors.New("Orca native launcher requires the fixed worktree and attempt identity")
	}
	launcherPath := filepath.Join(*run.WorktreePath, ".steerline-launch-"+attempt.AttemptID+".cmd")
	values := append([]string{launcherPath, executable, stdinPath, outputPath}, args...)
	for key, value := range env {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) {
			return nil, fmt.Errorf("invalid Orca native worker environment name %q", key)
		}
		values = append(values, value)
	}
	for _, value := range values {
		if strings.ContainsAny(value, "%\r\n\"") || !isASCII(value) {
			return nil, errors.New("Orca native command, paths and worker identity must use ASCII text without batch expansion markers or embedded quotes")
		}
	}
	gitDir, err := gitRunner(c, *run.WorktreePath).Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return nil, fmt.Errorf("resolve the exact worktree Git exclude file: %w", err)
	}
	excludePath := strings.TrimSpace(string(gitDir))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(*run.WorktreePath, excludePath)
	}
	excludeBefore, err := os.ReadFile(excludePath)
	excludeExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	pattern := "/" + filepath.Base(launcherPath)
	for _, line := range strings.Split(string(excludeBefore), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil, errors.New("Orca native launcher ignore identity is already present")
		}
	}
	excludeAfter := append([]byte(nil), excludeBefore...)
	if len(excludeAfter) > 0 && excludeAfter[len(excludeAfter)-1] != '\n' {
		excludeAfter = append(excludeAfter, '\n')
	}
	excludeAfter = append(excludeAfter, []byte(pattern+"\n")...)
	if err = AtomicWrite(excludePath, excludeAfter); err != nil {
		return nil, err
	}
	scriptBytes := renderNativeOrcaBatchRunner(executable, args, stdinPath, outputPath, captureStdout, env)
	file, createErr := os.OpenFile(launcherPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if createErr != nil {
		_ = restoreOrcaExclude(excludePath, excludeBefore, excludeExisted)
		return nil, createErr
	}
	if _, err = file.Write(scriptBytes); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(launcherPath)
		_ = restoreOrcaExclude(excludePath, excludeBefore, excludeExisted)
		return nil, err
	}
	return &orcaStepLauncher{path: launcherPath, bytes: scriptBytes, excludePath: excludePath, excludeBefore: excludeBefore, excludeAfter: excludeAfter, excludeExisted: excludeExisted}, nil
}

func renderNativeOrcaBatchRunner(executable string, args []string, stdinPath, outputPath string, captureStdout bool, env map[string]string) []byte {
	var script strings.Builder
	script.WriteString("@echo off\r\n@setlocal DisableDelayedExpansion\r\n")
	script.WriteString("for /f \"tokens=1 delims==\" %%V in ('set BEADS_ 2^>nul') do set \"%%V=\"\r\n")
	script.WriteString("for /f \"tokens=1 delims==\" %%V in ('set BD_ 2^>nul') do set \"%%V=\"\r\n")
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		script.WriteString("set \"" + key + "=" + env[key] + "\"\r\n")
	}
	command := windowsCommandLineArg(executable)
	for _, arg := range args {
		command += " " + windowsCommandLineArg(arg)
	}
	script.WriteString(command + " < " + windowsCommandLineArg(stdinPath))
	if captureStdout {
		script.WriteString(" > " + windowsCommandLineArg(outputPath))
	}
	script.WriteString("\r\nset \"steerline_exit=%ERRORLEVEL%\"\r\nendlocal & exit /b %steerline_exit%\r\n")
	return []byte(script.String())
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 0x7f {
			return false
		}
	}
	return true
}

func windowsCommandLineArg(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			out.WriteString(strings.Repeat(`\`, backslashes*2+1))
			out.WriteRune(r)
			backslashes = 0
			continue
		}
		out.WriteString(strings.Repeat(`\`, backslashes))
		backslashes = 0
		out.WriteRune(r)
	}
	out.WriteString(strings.Repeat(`\`, backslashes*2))
	out.WriteByte('"')
	return out.String()
}

func orcaCodexHistoryLaunchCommand(c Config, run *Run, dir, workspace, prompt string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	b := codexHistoryBinding{Version: 1, ProjectID: c.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, TargetID: run.Target.ID, Workspace: workspace, Command: codexReaderCommand(c)}
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
			if b.ProjectID != c.identity || b.WorkflowID != run.RunID || b.ExecutionID != run.ExecutionID || b.AttemptID != run.AttemptID || b.TargetID != run.Target.ID || canonical(b.Workspace) != canonical(workspace) {
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

// superviseOrcaStep waits for the bound native step terminal and its fixed
// per-attempt result file. It never copies or imports a whole-run result.
func superviseOrcaStep(c Config, proc *orcaProcess, resultPath string) *Worker {
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
			// Orca can briefly report a detached tab while the same terminal
			// keeps running. Preserve the fixed identity and wait for exit or
			// the normal stall bound before declaring its outcome unknown.
			if f, openErr := os.Open(resultPath); openErr == nil {
				b, readErr := io.ReadAll(io.LimitReader(f, (2<<20)+1))
				closeErr := f.Close()
				if readErr != nil || closeErr != nil || len(b) > 2<<20 {
					w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca native step result cannot be read within the 2 MiB bound", Cause: errors.Join(readErr, closeErr)}
					return
				}
				// Do not publish output after the external identity was replaced.
				settled, e := inspectBoundOrcaTerminal(ctx, c, proc.Identity())
				if e != nil {
					w.Done <- e
					return
				}
				if settled.exited() {
					if e := ctx.Err(); e != nil {
						w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca observation ended before step settlement", Cause: e}
						return
					}
					if e := os.Chmod(resultPath, 0600); e != nil {
						w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca native step result could not be protected", Cause: e}
						return
					}
					if e := proc.launcher.cleanup(); e != nil {
						w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca native step settled but its exact launcher cleanup could not be confirmed", Cause: e}
						return
					}
				w.Done <- nil
				return
				}
				term = settled
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
			if term.exited() {
				if e := proc.launcher.cleanup(); e != nil {
					w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca native step exited but its exact launcher cleanup could not be confirmed", Cause: e}
					return
				}
				w.Done <- errors.New("Orca terminal ended without its fixed native step result")
				return
			}
			if stall > 0 && time.Since(lastProgress) > stall {
				diagnostic := "Orca execution stalled; the terminal may still be running"
				if term.Orphaned || !term.Connected {
					diagnostic = "original Orca terminal liveness is unverifiable"
				}
				w.Done <- &BackendError{Code: BackendOutcomeUnknown, Diagnostic: diagnostic}
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
		gap := batch.SourceChanged || batch.Gap
		diagnostic := "terminal output is unstructured text; turn and message phase are unknown"
		if batch.SourceChanged {
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
