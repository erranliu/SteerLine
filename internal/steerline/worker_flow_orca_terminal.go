package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const workerOrcaPromptChunkBytes = 512

func workerOrcaPromptRoot(executionID, turnID string) string {
	return "SteerLineWorkerFlow\\" + executionID + "\\" + turnID
}

func workerOrcaPromptChunks(prompt string) [][]byte {
	data := []byte(prompt)
	chunks := make([][]byte, 0, (len(data)+workerOrcaPromptChunkBytes-1)/workerOrcaPromptChunkBytes)
	for len(data) > 0 {
		count := min(len(data), workerOrcaPromptChunkBytes)
		chunks = append(chunks, data[:count])
		data = data[count:]
	}
	return chunks
}

func workerOrcaChunkCommand(executionID, turnID string, index int, chunk []byte) (string, string, error) {
	if !stableExecutionID(executionID) || !stableExecutionID(turnID) || index < 0 || len(chunk) == 0 || len(chunk) > workerOrcaPromptChunkBytes {
		return "", "", errors.New("Orca prompt chunk has invalid identity or size")
	}
	digest := sha256.Sum256(chunk)
	sha := hex.EncodeToString(digest[:])
	marker := fmt.Sprintf("SLCHUNK:%s:%d:%s", turnID, index, sha)
	var script strings.Builder
	script.WriteString("$root = Join-Path ([IO.Path]::GetTempPath()) ")
	script.WriteString(powershellLiteral(workerOrcaPromptRoot(executionID, turnID)))
	script.WriteString("\n[IO.Directory]::CreateDirectory($root) | Out-Null\n")
	script.WriteString("$path = Join-Path $root ")
	script.WriteString(powershellLiteral(fmt.Sprintf("prompt-%04d.bin", index)))
	script.WriteString("\n$data = [Convert]::FromBase64String('")
	script.WriteString(base64.StdEncoding.EncodeToString(chunk))
	script.WriteString("')\nif (-not [IO.File]::Exists($path)) { $file = [IO.File]::Open($path, [IO.FileMode]::CreateNew); try { $file.Write($data, 0, $data.Length) } finally { $file.Close() } }\n")
	script.WriteString("if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne ")
	script.WriteString(powershellLiteral(sha))
	script.WriteString(") { throw 'fixed prompt chunk differs from the original request' }\nWrite-Output ")
	script.WriteString(powershellLiteral(marker))
	line := "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encodePowerShell(script.String())
	if len(line) > 30000 {
		return "", "", errors.New("Orca prompt chunk exceeds the Windows command limit")
	}
	return line, marker, nil
}

func waitWorkerOrcaMarker(ctx context.Context, c Config, identity ProcessIdentity, cursor int64, marker string) error {
	deadline, cancel := context.WithTimeout(ctx, positiveDuration(c.CommandTimeout, 30*time.Second))
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		batch, err := readWorkerOrcaOutput(deadline, c, identity, cursor)
		if err != nil {
			return err
		}
		for _, raw := range batch.Rows {
			if strings.TrimSpace(raw) == marker {
				return nil
			}
		}
		cursor = batch.NextCursor
		select {
		case <-deadline.Done():
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not confirm the fixed prompt chunk", Cause: deadline.Err()}
		case <-ticker.C:
		}
	}
}

func stageWorkerOrcaPrompt(ctx context.Context, c Config, identity ProcessIdentity, executionID, turnID, prompt string) error {
	for index, chunk := range workerOrcaPromptChunks(prompt) {
		command, marker, err := workerOrcaChunkCommand(executionID, turnID, index, chunk)
		if err != nil {
			return err
		}
		cursor, err := currentWorkerOrcaCursor(ctx, c, identity)
		if err != nil {
			return err
		}
		_, sendErr := sendWorkerOrcaCommand(ctx, c, identity, command)
		// A transport error is not evidence that input was rejected. The exact
		// chunk marker is the only success proof; never send this chunk twice.
		if err := waitWorkerOrcaMarker(ctx, c, identity, cursor, marker); err != nil {
			return errors.Join(sendErr, err)
		}
	}
	return nil
}

func showWorkerOrcaTerminal(ctx context.Context, c Config, handle string) (orcaTerminal, error) {
	if handle == "" {
		return orcaTerminal{}, errors.New("worker terminal handle is empty")
	}
	env, err := workerOrcaCall(ctx, c, "terminal", "show", "--terminal", handle, "--json")
	if err != nil {
		return orcaTerminal{}, err
	}
	var result struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return orcaTerminal{}, err
	}
	result.Terminal.RuntimeID = env.Meta.RuntimeID
	if result.Terminal.Handle != handle || result.Terminal.RuntimeID == "" || result.Terminal.IncarnationID == "" || result.Terminal.Orphaned || !result.Terminal.Connected || result.Terminal.exited() {
		return orcaTerminal{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original worker terminal is missing, disconnected or replaced"}
	}
	return result.Terminal, nil
}

func verifyWorkerOrcaTerminal(term orcaTerminal, run *Run, expected *ProcessIdentity) error {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" ||
		canonical(filepath.FromSlash(term.WorktreePath)) != canonical(*run.WorktreePath) ||
		strings.TrimPrefix(term.Branch, "refs/heads/") != *run.WorktreeBranch {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal changed the fixed worker workspace or branch"}
	}
	if expected != nil && (expected.Kind != "orca" || expected.Terminal != term.Handle || expected.RuntimeID != term.RuntimeID || expected.Incarnation != term.IncarnationID) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal runtime, handle or incarnation changed"}
	}
	return nil
}

func createWorkerOrcaTerminal(ctx context.Context, c Config, run *Run) (orcaTerminal, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || !stableExecutionID(run.RunID) {
		return orcaTerminal{}, errors.New("worker terminal lacks a fixed run and workspace")
	}
	title := "SteerLine-" + run.RunID
	env, err := workerOrcaCall(ctx, c, "terminal", "create", "--worktree", "path:"+filepath.ToSlash(*run.WorktreePath), "--title", title, "--shell", "cmd.exe", "--command", "echo SLREADY:"+run.RunID, "--json")
	if err != nil {
		return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca may have created the fixed worker terminal", Cause: err}
	}
	var result struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Terminal.Handle == "" {
		return orcaTerminal{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca terminal creation omitted its handle", Cause: err}
	}
	term, err := showWorkerOrcaTerminal(ctx, c, result.Terminal.Handle)
	if err != nil {
		return orcaTerminal{}, err
	}
	if err := verifyWorkerOrcaTerminal(term, run, nil); err != nil {
		return orcaTerminal{}, err
	}
	return term, nil
}

func (s *Service) prepareWorkerOrcaTerminal(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion || run.WorktreePath == nil || run.WorktreeBranch == nil {
		return errors.New("Orca terminal preparation lacks the fixed worker workspace")
	}
	if execution.Process != nil {
		term, err := showWorkerOrcaTerminal(ctx, fixedConfig, execution.Process.Terminal)
		if err != nil {
			return err
		}
		if err := verifyWorkerOrcaTerminal(term, run, execution.Process); err != nil {
			return err
		}
		s.refreshRun(run)
		return nil
	}
	opID, err := s.startOperation("prepare_worker_terminal", "worker-terminal:"+execution.ExecutionID, struct {
		ExecutionID string `json:"execution_id"`
		Workspace   string `json:"workspace"`
		Branch      string `json:"branch"`
	}{execution.ExecutionID, *run.WorktreePath, *run.WorktreeBranch})
	if err != nil {
		return err
	}
	_, execution = s.workflowRecords(run)
	if execution == nil {
		return errors.New("worker execution disappeared after Orca terminal intent was saved")
	}
	term, err := createWorkerOrcaTerminal(ctx, fixedConfig, run)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	identity := workerOrcaIdentity(term, fixedConfig)
	execution.Process = &identity
	s.refreshRun(run)
	if err := s.changeOperation(opID, "confirmed", "", mustJSON(identity), ""); err != nil {
		return fmt.Errorf("Orca terminal created but its identity could not be persisted: %w", err)
	}
	return nil
}

func workerOrcaIdentity(term orcaTerminal, c Config) ProcessIdentity {
	return ProcessIdentity{Kind: "orca", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable}
}

type workerOrcaOutput struct {
	Rows       []string
	NextCursor int64
	Oldest     int64
}

func readWorkerOrcaOutput(ctx context.Context, c Config, identity ProcessIdentity, cursor int64) (workerOrcaOutput, error) {
	if cursor < 0 || identity.Kind != "orca" || identity.Terminal == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return workerOrcaOutput{}, errors.New("Orca output has no fixed terminal or cursor identity")
	}
	term, err := showWorkerOrcaTerminal(ctx, c, identity.Terminal)
	if err != nil {
		return workerOrcaOutput{}, err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return workerOrcaOutput{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca output terminal was replaced"}
	}
	env, err := workerOrcaCall(ctx, c, "terminal", "read", "--terminal", identity.Terminal, "--cursor", strconv.FormatInt(cursor, 10), "--limit", "100", "--json")
	if err != nil {
		return workerOrcaOutput{}, err
	}
	if env.Meta.RuntimeID != identity.RuntimeID {
		return workerOrcaOutput{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca output came from another runtime"}
	}
	var result struct {
		Terminal struct {
			Handle       string   `json:"handle"`
			Tail         []string `json:"tail"`
			OldestCursor string   `json:"oldestCursor"`
			NextCursor   string   `json:"nextCursor"`
			Source       string   `json:"source"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return workerOrcaOutput{}, err
	}
	oldest, oldErr := strconv.ParseInt(result.Terminal.OldestCursor, 10, 64)
	next, nextErr := strconv.ParseInt(result.Terminal.NextCursor, 10, 64)
	if oldErr != nil || nextErr != nil || result.Terminal.Handle != identity.Terminal || result.Terminal.Source != "stream" || oldest > cursor || next < cursor || next-cursor != int64(len(result.Terminal.Tail)) {
		return workerOrcaOutput{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca worker output cursor has a gap or malformed response"}
	}
	return workerOrcaOutput{Rows: result.Terminal.Tail, NextCursor: next, Oldest: oldest}, nil
}

func sendWorkerOrcaCommand(ctx context.Context, c Config, identity ProcessIdentity, command string) (string, error) {
	if identity.Kind != "orca" || identity.Terminal == "" || command == "" {
		return "", errors.New("Orca send lacks fixed terminal or command")
	}
	term, err := showWorkerOrcaTerminal(ctx, c, identity.Terminal)
	if err != nil {
		return "", err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker send targets a replaced Orca terminal"}
	}
	env, err := workerOrcaCall(ctx, c, "terminal", "send", "--terminal", identity.Terminal, "--text", command, "--enter", "--json")
	if err != nil {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca worker command send has an uncertain receipt", Cause: err}
	}
	if env.Meta.RuntimeID != identity.RuntimeID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker send receipt came from another runtime"}
	}
	var result struct {
		Send struct {
			Handle   string `json:"handle"`
			Accepted bool   `json:"accepted"`
			Prompt   struct {
				RequestID          string `json:"requestId"`
				ProcessIncarnation string `json:"processIncarnation"`
			} `json:"prompt"`
		} `json:"send"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Send.Handle != identity.Terminal || !result.Send.Accepted || result.Send.Prompt.ProcessIncarnation != identity.Incarnation {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not confirm input acceptance on the original terminal", Cause: err}
	}
	return result.Send.Prompt.RequestID, nil
}

func workerOrcaTurnCommand(c Config, executionID, worktree, sessionID, turnID, prompt string, addDirs []string) (string, error) {
	if !stableExecutionID(executionID) || !stableExecutionID(turnID) || strings.TrimSpace(prompt) == "" || !filepath.IsAbs(worktree) || sessionID != "" && !codexThreadID.MatchString(sessionID) {
		return "", errors.New("Orca worker turn lacks fixed prompt, workspace or session")
	}
	command := c.Codex.Executable
	if command == "" {
		return "", errors.New("Orca worker Codex executable is not configured")
	}
	args := append([]string{}, c.Codex.Args...)
	args = append(args, "exec", "--json", "--sandbox", c.Sandbox, "-C", worktree)
	for _, dir := range addDirs {
		args = append(args, "--add-dir", dir)
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+c.ReasoningEffort)
	}
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	if sessionID != "" {
		args = append(args, "resume", sessionID)
	}
	args = append(args, "-")
	var script strings.Builder
	script.WriteString("$OutputEncoding = [Text.UTF8Encoding]::new($false)\n")
	script.WriteString("$root = Join-Path ([IO.Path]::GetTempPath()) ")
	script.WriteString(powershellLiteral(workerOrcaPromptRoot(executionID, turnID)))
	script.WriteString("\n$stream = [IO.MemoryStream]::new()\n")
	script.WriteString("for ($index = 0; $index -lt ")
	script.WriteString(strconv.Itoa(len(workerOrcaPromptChunks(prompt))))
	script.WriteString("; $index++) { $path = Join-Path $root ('prompt-{0:D4}.bin' -f $index); $part = [IO.File]::ReadAllBytes($path); $stream.Write($part, 0, $part.Length) }\n")
	script.WriteString("$bytes = $stream.ToArray(); $sha = [Security.Cryptography.SHA256]::Create(); try { $digest = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }\n")
	fullDigest := sha256.Sum256([]byte(prompt))
	script.WriteString("if ($digest -cne ")
	script.WriteString(powershellLiteral(hex.EncodeToString(fullDigest[:])))
	script.WriteString(") { throw 'fixed worker prompt digest differs from the saved message' }\n")
	script.WriteString("$lock = Join-Path $root 'launch.lock'; try { $file = [IO.File]::Open($lock, [IO.FileMode]::CreateNew); try { $saved = [Text.Encoding]::UTF8.GetBytes($digest); $file.Write($saved, 0, $saved.Length) } finally { $file.Close() } } catch [IO.IOException] { if ([IO.File]::Exists($lock) -and [IO.File]::ReadAllText($lock) -ceq $digest) { Write-Output 'SLALREADY:")
	script.WriteString(turnID)
	script.WriteString("'; exit 0 }; throw }\n")
	script.WriteString("$prompt = [Text.Encoding]::UTF8.GetString($bytes)\n")
	script.WriteString("$prompt | & ")
	script.WriteString(powershellLiteral(command))
	for _, arg := range args {
		script.WriteByte(' ')
		script.WriteString(powershellLiteral(arg))
	}
	script.WriteString(" | ForEach-Object { Write-Output ('SLJSON:")
	script.WriteString(turnID)
	script.WriteString(":' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_))) }\n")
	script.WriteString("Write-Output ('SLEND:")
	script.WriteString(turnID)
	script.WriteString(":' + $LASTEXITCODE)\n")
	encoded := encodePowerShell(script.String())
	line := "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encoded
	if len(line) > 30000 {
		return "", fmt.Errorf("worker launch command exceeds Windows command limit")
	}
	return line, nil
}

type workerOrcaProcess struct {
	config   Config
	identity ProcessIdentity
}

func (p *workerOrcaProcess) Identity() ProcessIdentity { return p.identity }
func (p *workerOrcaProcess) Close()                    {} // Only the center observer stops.
func (p *workerOrcaProcess) Kill() {
	if p.config.hostName() != "orca" {
		return // Remote cancellation is not a certified capability.
	}
	ctx, cancel := context.WithTimeout(context.Background(), positiveDuration(p.config.CommandTimeout, 30*time.Second))
	defer cancel()
	_, _ = workerOrcaCall(ctx, p.config, "terminal", "close", "--terminal", p.identity.Terminal, "--json")
}

func workerOrcaRemoteRepoPath(ctx context.Context, c Config, worktreeID string) (string, error) {
	repoID, _, found := strings.Cut(worktreeID, "::")
	if !found || repoID == "" {
		return "", errors.New("remote Orca worktree has no registered repository identity")
	}
	env, err := workerOrcaCall(ctx, c, "repo", "show", "--repo", "id:"+repoID, "--json")
	if err != nil {
		return "", err
	}
	var result struct {
		Repo struct {
			ID   string `json:"id"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"repo"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", err
	}
	if result.Repo.ID != repoID || result.Repo.Kind != "git" || !filepath.IsAbs(filepath.FromSlash(result.Repo.Path)) {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote worktree repository path is not verified"}
	}
	return filepath.FromSlash(result.Repo.Path), nil
}

func workerOrcaWriteDirs(ctx context.Context, c Config, run *Run) ([]string, error) {
	if c.Sandbox != "workspace-write" {
		return nil, nil
	}
	if c.hostName() == "orca_supervised" {
		path, err := workerOrcaRemoteRepoPath(ctx, c, run.Target.WorktreeID)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	args, err := codexLinkedWorktreeWriteArgs(ctx, c, *run.WorktreePath)
	if err != nil {
		return nil, err
	}
	dirs := []string{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || args[i] != "--add-dir" {
			return nil, errors.New("local Codex worktree write arguments are malformed")
		}
		dirs = append(dirs, args[i+1])
	}
	return dirs, nil
}

func startWorkerOrcaTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, turnID, prompt string) (*Worker, string, string, error) {
	if run == nil || execution == nil || run.Process == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.RunID != execution.WorkflowRunID || run.ExecutionID != execution.ExecutionID || !stableExecutionID(turnID) {
		return nil, "", "", errors.New("Orca turn has no fixed worker, workspace or process")
	}
	term, err := showWorkerOrcaTerminal(ctx, c, run.Process.Terminal)
	if err != nil {
		return nil, "", "", err
	}
	if err := verifyWorkerOrcaTerminal(term, run, run.Process); err != nil {
		return nil, "", "", err
	}
	cursor, err := currentWorkerOrcaCursor(ctx, c, *run.Process)
	if err != nil {
		return nil, "", "", err
	}
	events, sourceIdentity, err := prepareOrcaTurnJournal(c, run, turnID, cursor)
	if err != nil {
		return nil, "", "", err
	}
	if err := AtomicWrite(filepath.Join(filepath.Dir(events), "prompt.txt"), []byte(prompt)); err != nil {
		return nil, "", "", err
	}
	if err := stageWorkerOrcaPrompt(ctx, c, *run.Process, execution.ExecutionID, turnID, prompt); err != nil {
		return nil, events, "", err
	}
	dirs, err := workerOrcaWriteDirs(ctx, c, run)
	if err != nil {
		return nil, "", "", err
	}
	command, err := workerOrcaTurnCommand(c, execution.ExecutionID, *run.WorktreePath, execution.SessionID, turnID, prompt, dirs)
	if err != nil {
		return nil, "", "", err
	}
	requestID, err := sendWorkerOrcaCommand(ctx, c, *run.Process, command)
	if err != nil {
		return nil, events, requestID, err
	}
	return observeWorkerOrcaTurn(c, run, turnID, *run.Process, events, sourceIdentity), events, requestID, nil
}

func resumeWorkerOrcaTurn(c Config, run *Run, turnID string) (*Worker, error) {
	if run == nil || run.Process == nil || !stableExecutionID(turnID) {
		return nil, errors.New("saved Orca worker turn has no terminal or turn identity")
	}
	events := filepath.Join(orcaTurnPath(c, run.RunID, turnID), "events.jsonl")
	f, sourceIdentity, err := openJournal(events)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return observeWorkerOrcaTurn(c, run, turnID, *run.Process, events, sourceIdentity), nil
}
