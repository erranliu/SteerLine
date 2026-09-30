package orcaworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProbeIdentity binds an exact command to one preparation intent, never to a
// model's assertion or a reusable session health cache.
type ProbeIdentity struct {
	Execution     string `json:"execution"`
	Turn          string `json:"turn"`
	Nonce         string `json:"nonce"`
	ProfileDigest string `json:"profile_digest"`
	Runtime       string `json:"runtime"`
	Terminal      string `json:"terminal"`
	Incarnation   string `json:"incarnation"`
	Worktree      string `json:"worktree"`
}

type ProbeReceipt struct {
	ProbeIdentity
	InputSHA256 string `json:"input_sha256"`
	GitHead     string `json:"git_head"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Delete      bool   `json:"delete"`
}

func probeDirectory(p ProbeIdentity) (string, error) {
	if !stableID(p.Execution) || !stableID(p.Turn) || !stableID(p.Nonce) || !filepath.IsAbs(p.Worktree) || p.ProfileDigest == "" || p.Runtime == "" || p.Terminal == "" || p.Incarnation == "" {
		return "", errors.New("readiness probe lacks frozen identity")
	}
	return filepath.Join(".steerline-readiness", p.Execution, p.Turn), nil
}

// noLinks rejects links/reparse-point directories before opening an os.Root;
// os.Root also confines all subsequent accesses against path substitution.
func noLinks(root, relative string) error {
	path := filepath.Clean(root)
	for _, part := range append([]string{""}, strings.Split(relative, string(filepath.Separator))...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !samePath(resolved, path) {
			return errors.New("readiness path contains a link or reparse point")
		}
	}
	return nil
}

func probeScript() string {
	return `$ErrorActionPreference = 'Stop'
$probeRoot = $PSScriptRoot
$inputPath = Join-Path $probeRoot 'input.json'
$p = Get-Content -LiteralPath $inputPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ([IO.Path]::GetFullPath((Get-Location).Path) -ne [IO.Path]::GetFullPath($p.worktree)) { throw 'readiness worktree differs' }
$head = git rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'readiness Git metadata unavailable' }
$temp = Join-Path $probeRoot ($p.nonce + '.tmp')
if (Test-Path -LiteralPath $temp) { throw 'readiness file already exists' }
[IO.File]::WriteAllText($temp, $p.nonce)
if ([IO.File]::ReadAllText($temp) -cne $p.nonce) { throw 'readiness readback differs' }
Remove-Item -LiteralPath $temp -ErrorAction Stop
if (Test-Path -LiteralPath $temp) { throw 'readiness delete failed' }
$r = @{}
foreach ($prop in $p.PSObject.Properties) { $r[$prop.Name] = $prop.Value }
$r.input_sha256 = (Get-FileHash -LiteralPath $inputPath -Algorithm SHA256).Hash.ToLowerInvariant()
$r.git_head = $head
$r.read = $true; $r.write = $true; $r.delete = $true
$pending = Join-Path $probeRoot 'receipt.pending'
$receipt = Join-Path $probeRoot 'receipt.json'
if ((Test-Path -LiteralPath $pending) -or (Test-Path -LiteralPath $receipt)) { throw 'readiness receipt already exists' }
$r | ConvertTo-Json -Compress | Set-Content -LiteralPath $pending -Encoding utf8
Move-Item -LiteralPath $pending -Destination $receipt
Write-Output ('SLREADINESS:' + $p.nonce + ':' + (Get-FileHash -LiteralPath (Join-Path $probeRoot 'probe.ps1') -Algorithm SHA256).Hash.ToLowerInvariant())
`
}

func sha(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func PrepareProbe(p ProbeIdentity) (command, inputDigest, scriptDigest string, err error) {
	dir, err := probeDirectory(p)
	if err != nil {
		return "", "", "", err
	}
	if err = noLinks(p.Worktree, dir); err != nil {
		return "", "", "", err
	}
	root, err := os.OpenRoot(p.Worktree)
	if err != nil {
		return "", "", "", err
	}
	defer root.Close()
	if err = root.MkdirAll(dir, 0700); err != nil {
		return "", "", "", err
	}
	input, err := json.Marshal(p)
	if err != nil {
		return "", "", "", err
	}
	script := []byte(probeScript())
	for _, f := range []struct {
		name string
		data []byte
	}{{"input.json", input}, {"probe.ps1", script}} {
		path := filepath.Join(dir, f.name)
		if err = noLinks(p.Worktree, path); err != nil {
			return "", "", "", err
		}
		old, readErr := root.ReadFile(path)
		if readErr == nil {
			if string(old) != string(f.data) {
				return "", "", "", errors.New("original readiness input changed")
			}
			continue
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", "", "", readErr
		}
		out, e := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return "", "", "", e
		}
		_, e = out.Write(f.data)
		if e = errors.Join(e, out.Sync(), out.Close()); e != nil {
			return "", "", "", e
		}
	}
	return "powershell.exe -NoProfile -ExecutionPolicy Bypass -File " + powershellLiteral(filepath.ToSlash(filepath.Join(dir, "probe.ps1"))), sha(input), sha(script), nil
}

func ReadProbe(p ProbeIdentity, digest string) (*ProbeReceipt, error) {
	dir, err := probeDirectory(p)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "receipt.json")
	if err = noLinks(p.Worktree, path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.Worktree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return nil, errors.New("readiness receipt is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(data) > 8192 {
		return nil, errors.New("readiness receipt exceeded limit")
	}
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
	var receipt ProbeReceipt
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err = d.Decode(&receipt); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF || receipt.ProbeIdentity != p || receipt.InputSHA256 != digest || !receipt.Read || !receipt.Write || !receipt.Delete || len(receipt.GitHead) != 40 && len(receipt.GitHead) != 64 {
		return nil, errors.New("readiness receipt does not match original probe identity or operations")
	}
	return &receipt, nil
}

func CleanupProbe(p ProbeIdentity) error {
	dir, err := probeDirectory(p)
	if err != nil {
		return err
	}
	if err = noLinks(p.Worktree, dir); err != nil {
		return err
	}
	root, err := os.OpenRoot(p.Worktree)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range []string{"input.json", "probe.ps1", "receipt.json"} {
		if err = noLinks(p.Worktree, filepath.Join(dir, name)); err != nil {
			return err
		}
		if err = root.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

type ProbeToolEvidence struct {
	SourceIdentity string `json:"source_identity"`
	CallID         string `json:"call_id"`
	MessageID      string `json:"message_id"`
	Command        string `json:"command"`
	Marker         string `json:"marker"`
	Verified       bool   `json:"verified"`
	FailureCode    string `json:"failure_code,omitempty"`
}

var singleCommandWrapper = regexp.MustCompile(`(?s)^\s*text\(\s*await\s+tools\.exec_command\(\s*(\{.*\})\s*\)\s*\)\s*;?\s*$`)
var commandObjectKeys = regexp.MustCompile(`([,{]\s*)(cmd|max_output_tokens|yield_time_ms|workdir|shell|login|tty)\s*:`)

func commandInvocation(name, input string) (string, bool) {
	if name == "exec" {
		match := singleCommandWrapper.FindStringSubmatch(input)
		if len(match) != 2 {
			return "", false
		}
		input = commandObjectKeys.ReplaceAllString(match[1], `$1"$2":`)
	} else if name != "exec_command" {
		return "", false
	}
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(input), &args) != nil {
		return "", false
	}
	for key := range args {
		switch key {
		case "cmd", "max_output_tokens", "yield_time_ms", "workdir", "shell", "login", "tty":
		default:
			return "", false
		}
	}
	var cmd string
	if json.Unmarshal(args["cmd"], &cmd) != nil {
		return "", false
	}
	return cmd, true
}

type transcriptMessage struct {
	Timestamp int64  `json:"timestamp"`
	ID        string `json:"id"`
	Role      string `json:"role"`
	Source    string `json:"source"`
	Blocks    []struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Input  string `json:"input"`
		Text   string `json:"text"`
		Output string `json:"output"`
		CallID string `json:"callId"`
	} `json:"blocks"`
}

type ToolContext struct {
	SourceIdentity string `json:"source_identity,omitempty"`
	TaskBound      bool   `json:"task_bound,omitempty"`
	CallID         string `json:"call_id,omitempty"`
	Started        bool   `json:"started,omitempty"`
}

// Business diagnostics require the current native Task's injected user header
// and a provider command event after its saved intent. Old tails and assistant
// quotations, as well as ordinary command exit codes, cannot report a fault.
func ObserveToolInitialization(output WorkerOutput, taskID string, intentMillis int64, prior ToolContext) (ToolContext, bool, error) {
	if output.Source != "transcript" || !output.SourceExact || output.Provider != "codex" || output.SourceIdentity == "" {
		return prior, false, nil
	}
	if prior.SourceIdentity != output.SourceIdentity {
		prior = ToolContext{SourceIdentity: output.SourceIdentity}
	}
	var transcript struct {
		Messages []transcriptMessage `json:"messages"`
	}
	if err := json.Unmarshal(output.Transcript, &transcript); err != nil {
		return prior, false, err
	}
	for _, message := range transcript.Messages {
		if message.Source != "transcript" {
			prior.TaskBound = false
			prior.CallID = ""
			continue
		}
		if message.Role == "user" {
			prior.TaskBound = false
			prior.CallID = ""
			for _, block := range message.Blocks {
				if block.Type == "text" && strings.Contains(block.Input+block.Output+block.Text, "Your task ID is: "+taskID) && message.Timestamp >= intentMillis {
					prior.TaskBound = true
				}
			}
		}
		for _, block := range message.Blocks {
			if block.Type == "tool-call" && message.Role == "assistant" {
				prior.CallID = ""
				if _, invoked := commandInvocation(block.Name, block.Input); invoked && prior.TaskBound && message.Timestamp >= intentMillis && block.CallID != "" {
					prior.CallID = block.CallID
					prior.Started = true
				}
			}
			if block.Type == "tool-result" && message.Role == "tool" && prior.CallID != "" && message.Timestamp >= intentMillis && (block.CallID == "" || block.CallID == prior.CallID) {
				result := block.Output
				if index := strings.Index(result, "Output:\n"); index >= 0 {
					result = result[index+len("Output:\n"):]
				}
				var ordinary struct {
					ExitCode *int   `json:"exit_code"`
					Output   string `json:"output"`
				}
				if json.Unmarshal([]byte(result), &ordinary) != nil && strings.Contains(result, "helper_unknown_error") && strings.Contains(result, "setup refresh had errors") {
					return prior, true, nil
				}
				prior.CallID = ""
			}
		}
	}
	return prior, false, nil
}

// ObserveProbe consumes exact provider transcript blocks. The Codex transcript
// adapter omits result call IDs; accept only the immediate result of one pending
// command call, with an independently matching nonce + script hash and exit 0.
// Persist this small pending association across pages and discard on source gaps.
func ObserveProbe(output WorkerOutput, command, nonce, scriptDigest string, prior ProbeToolEvidence) (ProbeToolEvidence, error) {
	if output.Source != "transcript" || !output.SourceExact || output.Provider != "codex" || output.SourceIdentity == "" {
		return prior, nil
	}
	if prior.SourceIdentity != "" && prior.SourceIdentity != output.SourceIdentity {
		prior = ProbeToolEvidence{}
	}
	var transcript struct {
		Messages []transcriptMessage `json:"messages"`
	}
	if err := json.Unmarshal(output.Transcript, &transcript); err != nil {
		return prior, err
	}
	marker := "SLREADINESS:" + nonce + ":" + scriptDigest
	for _, message := range transcript.Messages {
		if message.Source != "transcript" {
			prior = ProbeToolEvidence{}
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == "tool-call" && message.Role == "assistant" {
				if prior.Verified {
					continue
				}
				prior = ProbeToolEvidence{}
				// Support actual exec_command and the installed Codex exec wrapper.
				if invoked, ok := commandInvocation(block.Name, block.Input); ok && invoked == command && block.CallID != "" && !strings.Contains(block.Input, "… (truncated)") {
					prior = ProbeToolEvidence{SourceIdentity: output.SourceIdentity, CallID: block.CallID, Command: command, Marker: marker}
				}
			} else if block.Type == "tool-result" && message.Role == "tool" && !prior.Verified && prior.CallID != "" {
				resultText := block.Output
				if index := strings.Index(resultText, "Output:\n"); index >= 0 {
					resultText = resultText[index+len("Output:\n"):]
				}
				var result struct {
					ExitCode *int   `json:"exit_code"`
					Output   string `json:"output"`
				}
				if json.Unmarshal([]byte(resultText), &result) != nil && strings.Contains(resultText, "helper_unknown_error") && strings.Contains(resultText, "setup refresh had errors") {
					prior.FailureCode = "tool_initialization_failed"
					prior.MessageID = message.ID
					continue
				}
				if json.Unmarshal([]byte(resultText), &result) == nil && result.ExitCode != nil && *result.ExitCode == 0 && strings.TrimSpace(result.Output) == marker && (block.CallID == "" || block.CallID == prior.CallID) {
					prior.Verified, prior.MessageID = true, message.ID
				} else {
					prior = ProbeToolEvidence{}
				}
			}
		}
	}
	return prior, nil
}

func ProbePrompt(command, nonce, digest string) string {
	return fmt.Sprintf("Readiness preparation only. Use your actual command execution tool to run exactly this command once in the original worktree: %s\nDo not modify the probe input or script, implement business work, use apply_patch to create a receipt, or relax sandbox permissions. Success requires exit code 0 and the exact output SLREADINESS:%s:%s. Then report native worker_done succeeded using the result relay instruction below; report failed on failure. Do not rerun an accepted or unknown probe command.", command, nonce, digest)
}
