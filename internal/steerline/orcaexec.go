package steerline

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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
	Path    string `json:"path"`
	Head    string `json:"head"`
	Branch  string `json:"branch"`
	BaseRef string `json:"baseRef"`
}

type orcaWorktreeCreated struct {
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

func (c orcaClient) terminalSend(ctx context.Context, handle, text string) error {
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "send", "--terminal", handle, "--text", text, "--enter", "--json")
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return callErr
		}
		return e
	}
	_ = env
	return nil
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
	// configured Codex model, create the worktree first, write the dispatch, then
	// start Codex in a fresh terminal with CLI overrides and the pointer as its
	// initial prompt. Other Orca agents retain the native --agent/--prompt path.
	customCodex := c.needsCodexExecutable()
	promptPath := filepath.Join(dir, "prompt.txt")
	pointer := "Read and execute " + promptPath + " now. Follow it exactly."
	bootstrap := "SteerLine is preparing the dispatch. Wait for a follow-up message in this terminal. Do not edit files or act on a task yet."
	var created orcaWorktreeCreated
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
	if branch == "" {
		branch = filepath.Base(path)
	}
	handle := ""
	if !customCodex {
		handle = created.AgentTerminalHandle
		if handle == "" {
			handle = created.StartupTerminal.Handle
		}
		if handle == "" {
			if terms, te := client.terminalListForWorktree(ctx, path); te == nil && len(terms) > 0 {
				handle = terms[0].Handle
			}
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
		if e := client.terminalSend(ctx, handle, pointer); e != nil {
			return nil, e
		}
	}
	if customCodex {
		command, ce := orcaCodexLaunchCommand(c, pointer)
		if ce != nil {
			return nil, ce
		}
		term, ce := client.terminalCreate(ctx, path, "SteerLine "+name, command)
		startupHandle := created.StartupTerminal.Handle
		if startupHandle != "" && startupHandle != term.Handle {
			client.closeTerminal(ctx, startupHandle)
		}
		if ce != nil {
			return nil, ce
		}
		handle = term.Handle
	}

	resultInWorktree := filepath.Join(path, orcaResultFile)
	term, _ := client.showTerminal(ctx, handle)
	proc := &orcaProcess{cfg: c, term: term}
	proc.term.Handle = handle
	return superviseOrcaResult(c, proc, resultInWorktree, *r.ResultPath), nil
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
	stall := positiveDuration(c.StallTimeout, 5*time.Minute)
	go func() {
		defer close(w.Events)
		defer close(w.observationDone)
		var lastOutput int64
		lastProgress := time.Now()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				w.Done <- ctx.Err()
				return
			case <-ticker.C:
			}
			if b, e := os.ReadFile(resultInWorktree); e == nil {
				if e := AtomicWrite(resultOut, b); e != nil {
					w.Done <- e
					return
				}
				w.Done <- nil
				return
			}
			term, e := showBoundOrcaTerminal(ctx, c, proc.Identity())
			if e == nil {
				if term.AgentWait != nil {
					w.Done <- errors.New("orca agent is waiting for human input; manual recovery required")
					return
				}
				if term.LastOutputAt != lastOutput {
					lastOutput = term.LastOutputAt
					lastProgress = time.Now()
				}
				if term.Status == "exited" || term.Orphaned {
					if _, se := os.Stat(resultInWorktree); se != nil {
						w.Done <- errors.New("orca terminal ended without a result file")
						return
					}
				}
			}
			if stall > 0 && time.Since(lastProgress) > stall {
				w.Done <- errors.New("orca execution stalled (no result file, no terminal output); possible interactive wait")
				return
			}
		}
	}()
	return w
}
