package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testprocess"
)

type OrcaEnvironment struct{ Executable, Home, LogDir, PowerShellDir string }

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// OrcaMain delegates every operation to the real Orca executable. Only a new
// agent terminal's shell environment changes. Native argv/receipts stay intact.
func OrcaMain(args []string) error {
	raw, err := os.ReadFile(filepath.Join(os.Getenv("STEERLINE_HOME"), "orca-env.json"))
	if err != nil {
		return err
	}
	var c OrcaEnvironment
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	args = append([]string(nil), args...)
	if len(args) > 1 && args[0] == "terminal" && args[1] == "create" {
		// Trust only the freshly created owned worktree in its isolated home;
		// this is fixture configuration, not a UI bypass on a user's project.
		for i := 2; i+1 < len(args); i++ {
			if args[i] == "--worktree" {
				_, path, found := strings.Cut(strings.TrimPrefix(args[i+1], "id:"), "::")
				if found && filepath.IsAbs(filepath.FromSlash(path)) {
					quoted, _ := json.Marshal(filepath.ToSlash(path))
					f, err := os.OpenFile(filepath.Join(c.Home, "codex", "config.toml"), os.O_APPEND|os.O_WRONLY, 0600)
					if err != nil {
						return err
					}
					_, writeErr := fmt.Fprintf(f, "\n[projects.%s]\ntrust_level = \"trusted\"\n", quoted)
					if err := f.Close(); err != nil {
						return err
					}
					if writeErr != nil {
						return writeErr
					}
				}
			}
		}
		for i := 2; i+1 < len(args); i++ {
			if args[i] == "--command" && strings.HasPrefix(args[i+1], "& '") {
				prefix := "$env:CODEX_HOME=" + psQuote(filepath.Join(c.Home, "codex")) + "; $env:GIT_CONFIG_GLOBAL=" + psQuote(os.DevNull) + "; $env:GIT_CONFIG_NOSYSTEM='1'; $env:GIT_TERMINAL_PROMPT='0'; "
				if c.PowerShellDir != "" {
					prefix += "$env:PATH=" + psQuote(c.PowerShellDir+string(os.PathListSeparator)) + "+$env:PATH; "
				}
				if _, err := os.Stat(filepath.Join(c.Home, "github-token")); err == nil {
					prefix += "$env:GH_TOKEN=[IO.File]::ReadAllText(" + psQuote(filepath.Join(c.Home, "github-token")) + ").Trim(); "
				}
				args[i+1] = prefix + args[i+1]
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.Command(c.Executable, args...)
	var out, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = testprocess.Run(ctx, cmd)
	if err == nil && len(args) > 1 && args[0] == "terminal" && args[1] == "create" {
		var receipt struct {
			Result struct {
				Terminal struct{ Handle, Title string } `json:"terminal"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(out.String()), &receipt) == nil && strings.HasPrefix(receipt.Result.Terminal.Title, "SteerLine agent ") && receipt.Result.Terminal.Handle != "" {
			mount := exec.Command(c.Executable, "terminal", "switch", "--terminal", receipt.Result.Terminal.Handle, "--json")
			if mountErr := testprocess.Run(ctx, mount); mountErr != nil {
				return mountErr
			}
		}
	}
	// The installed CLI may leave a long injected prompt as a bracketed paste
	// in Codex's composer. Finish that same accepted input once; never resend
	// the prompt or manufacture a started/settled receipt.
	if err == nil && len(args) > 1 && args[0] == "orchestration" && args[1] == "dispatch" {
		var reply struct {
			OK     bool `json:"ok"`
			Result struct {
				Terminal struct{ Handle, IncarnationID, AgentIdentity string } `json:"terminal"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(out.String()), &reply) == nil && reply.OK {
			terminal := reply.Result.Terminal.Handle
			if terminal == "" {
				for i, arg := range args {
					if arg == "--to" && i+1 < len(args) {
						terminal = args[i+1]
					}
				}
			}
			if terminal == "" {
				return fmt.Errorf("native Dispatch terminal missing")
			}
			var screen strings.Builder
			read := exec.Command(c.Executable, "terminal", "read", "--terminal", terminal, "--limit", "50", "--json")
			read.Stdout = &screen
			// The UI input queue is asynchronous: an accepted RPC may precede
			// the composer update. Observe without injecting any new text.
			deadline := time.Now().Add(25 * time.Second)
			lastDraft := ""
			stableSince := time.Now()
			for {
				screen.Reset()
				read = exec.Command(c.Executable, "terminal", "read", "--terminal", terminal, "--limit", "50", "--json")
				read.Stdout = &screen
				if readErr := testprocess.Run(ctx, read); readErr != nil {
					return readErr
				}
				var current struct {
					Result struct {
						Terminal struct{ Handle, Source, Draft string } `json:"terminal"`
					} `json:"result"`
				}
				if json.Unmarshal([]byte(screen.String()), &current) == nil && current.Result.Terminal.Handle == terminal && current.Result.Terminal.Source == "screen" {
					if current.Result.Terminal.Draft != lastDraft {
						lastDraft, stableSince = current.Result.Terminal.Draft, time.Now()
					}
				}
				// A bracketed-paste preview may appear before the queued prompt's
				// remaining bytes arrive. Submit only after the full draft settles.
				if (strings.Contains(lastDraft, "[Pasted Content ") && time.Since(stableSince) >= 3*time.Second) || strings.Contains(screen.String(), "esc to interrupt") || time.Now().After(deadline) {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
				}
			}
			if logErr := os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("input-screen-%d.json", time.Now().UnixNano())), []byte(screen.String()), 0600); logErr != nil {
				return logErr
			}
			var view struct {
				OK     bool `json:"ok"`
				Result struct {
					Terminal struct {
						Handle, Source string
						Draft          string
						Tail           []string
					} `json:"terminal"`
				} `json:"result"`
			}
			if json.Unmarshal([]byte(screen.String()), &view) == nil && view.OK && view.Result.Terminal.Handle == terminal && view.Result.Terminal.Source == "screen" {
				pending, busy := false, false
				if strings.Contains(view.Result.Terminal.Draft, "[Pasted Content ") && strings.Contains(view.Result.Terminal.Draft, " chars]") {
					pending = true
				}
				for _, line := range view.Result.Terminal.Tail {
					if strings.Contains(line, "[Pasted Content ") && strings.Contains(line, " chars]") {
						pending = true
					}
					if strings.Contains(line, "esc to interrupt") {
						busy = true
					}
				}
				if pending && !busy {
					if time.Since(stableSince) < 3*time.Second {
						return fmt.Errorf("native pasted Dispatch draft did not settle before submission")
					}
					var submission strings.Builder
					send := exec.Command(c.Executable, "terminal", "send", "--terminal", terminal, "--enter", "--json")
					send.Stdout, send.Stderr = &submission, &submission
					sendErr := testprocess.Run(ctx, send)
					if logErr := os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("submit-%d.json", time.Now().UnixNano())), []byte(submission.String()), 0600); logErr != nil {
						return logErr
					}
					if sendErr != nil {
						return sendErr
					}
					// Observe without any second Enter or text injection. Acceptance
					// alone can leave the original draft in the asynchronous UI queue.
					observeCtx, observeCancel := context.WithTimeout(ctx, 25*time.Second)
					observed, observeErr := waitForOrcaPasteSubmission(observeCtx, terminal, func(ctx context.Context) ([]byte, error) {
						var observed strings.Builder
						confirm := exec.Command(c.Executable, "terminal", "read", "--terminal", terminal, "--screen", "--limit", "50", "--json")
						confirm.Stdout = &observed
						readErr := testprocess.Run(ctx, confirm)
						return []byte(observed.String()), readErr
					})
					observeCancel()
					if logErr := os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("after-submit-%d.json", time.Now().UnixNano())), observed, 0600); logErr != nil {
						return logErr
					}
					// Keep the original mutation receipt and diagnostic even when
					// submission remains unproven; do not fabricate a started stage.
					if observeErr != nil {
						err = observeErr
					}
				}
			}
		}
	}
	// Mutating call evidence includes exact returned IDs for failed preparation cleanup.
	if len(args) > 1 && (args[1] == "create" || args[1] == "rm" || args[1] == "close" || args[1] == "dispatch") {
		record, _ := json.Marshal(map[string]any{"command": append([]string{c.Executable}, args...), "stdout": out.String(), "stderr": stderr.String(), "error": fmt.Sprint(err)})
		if logErr := os.WriteFile(filepath.Join(c.LogDir, fmt.Sprintf("orca-%d.json", time.Now().UnixNano())), record, 0600); logErr != nil {
			return logErr
		}
	}
	fmt.Fprint(os.Stdout, out.String())
	fmt.Fprint(os.Stderr, stderr.String())
	return err
}

func waitForOrcaPasteSubmission(ctx context.Context, terminal string, read func(context.Context) ([]byte, error)) ([]byte, error) {
	var last []byte
	for {
		raw, err := read(ctx)
		if err != nil {
			return last, err
		}
		last = raw
		payload, err := OrcaPayload(raw)
		if err != nil {
			return last, err
		}
		var view struct {
			Terminal struct {
				Handle, Source, Draft string
				Tail                  []string
			} `json:"terminal"`
		}
		if err := json.Unmarshal(payload, &view); err != nil {
			return last, err
		}
		if view.Terminal.Handle != terminal || view.Terminal.Source != "screen" {
			return last, fmt.Errorf("native submission observation is not the original terminal screen")
		}
		pending, busy := view.Terminal.Draft != "", false
		for _, line := range view.Terminal.Tail {
			pending = pending || strings.Contains(line, "[Pasted Content ")
			busy = busy || strings.Contains(line, "esc to interrupt")
		}
		if !pending || busy {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, fmt.Errorf("original pasted Dispatch remains unsubmitted: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}
