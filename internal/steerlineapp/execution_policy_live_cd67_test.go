package steerlineapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Explicit opt-in. This exercises the new policy/compiler and original native
// sessions, without readiness receipts, Task dispatch, commits or publication.
func TestExecutionPolicyNativeToolsLive(t *testing.T) {
	host, executable, reportPath := os.Getenv("STEERLINE_POLICY_LIVE_HOST"), os.Getenv("STEERLINE_POLICY_LIVE_CODEX"), os.Getenv("STEERLINE_POLICY_LIVE_REPORT")
	if host == "" || executable == "" || reportPath == "" {
		t.Skip("isolated policy native validation was not selected")
	}
	if host != "local" && host != "orca" {
		t.Fatal("invalid live host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	root, err := os.MkdirTemp("", "steerline-policy59-"+host+"-")
	require(t, err)
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	source, err := filepath.Abs("../..")
	require(t, err)
	repository := filepath.Join(root, "repo")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	if existing := os.Getenv("STEERLINE_POLICY_LIVE_REPO"); existing != "" {
		if !strings.HasPrefix(filepath.Base(existing), "steerline-pr154-repo-") || git(existing, "status", "--porcelain") != "" {
			t.Fatal("only the clean retained isolated readiness fixture can be reused")
		}
		repository = existing
	} else {
		git(source, "clone", "--quiet", "--local", "--no-hardlinks", source, repository)
	}
	c := Config{ExecutionPolicy: workspacePolicyFixture(), WorkflowProtocol: WorkerFlowVersion, Repository: repository, RunDir: filepath.Join(root, "state"), CommandTimeout: "90s", Git: executionbackend.Command{Executable: "git"}, Host: host, Agent: "codex"}
	codex := codexworker.ProfileConfig{Executable: executable}
	orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "4m"}
	if host == "orca" {
		codex.WindowsSandbox = "unelevated"
	} else {
		orca = nil
	}
	setTestImplementationProfiles(t, &c, codex, orca)
	require(t, c.ValidateProfiles())
	require(t, c.ResolveIdentity(ctx))
	profile := unitProjectProfile(c).Freeze(c.Identity())
	c = c.ApplyFixedExecutionProfile(profile)
	e := executionmodel.ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: nativePolicyID(), WorkflowRunID: nativePolicyID(), CurrentAttemptID: nativePolicyID(), Generation: nativePolicyID(), ConfigurationDigest: profile.Digest, Target: nativePolicyTarget(c)}
	run := &steerline.Run{RunID: e.WorkflowRunID, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID}
	path := filepath.Join(root, "linked")
	var terminal orcaworker.Terminal
	var runtimeID string
	client := orcaworker.New(nativeTerminalConfig(c, nativePolicyTarget(c)))
	if host == "local" {
		git(repository, "worktree", "add", "--quiet", "-b", "policy59-local", path, "HEAD")
	} else {
		_, err := client.MetaCall(ctx, "repo", "add", "--path", repository, "--json")
		require(t, err)
		repoID, err := orcaworker.FindRepo(ctx, client.Call, repository)
		require(t, err)
		base := git(repository, "rev-parse", "HEAD")
		name := "policy59-" + e.ExecutionID[:12]
		raw, err := orcaworker.CreateFixedWorktree(ctx, client.MetaCall, repoID, name, base, "#59 isolated native tool check")
		require(t, err)
		worktree, err := orcaworker.ParseFixedWorktreeReceipt(raw, name, repoID, base)
		require(t, err)
		path = filepath.FromSlash(worktree.Path)
		e.Workspace = executionmodel.WorkspaceEvidence{Repository: c.Identity(), Worktree: path, Branch: strings.TrimPrefix(worktree.Branch, "refs/heads/")}
		writeArgs, err := linkedWorktreeWriteArgs(ctx, c, path)
		require(t, err)
		require(t, bindNativePolicyProbeResources(ctx, c, &e, path, writeArgs))
		command, err := managedTerminalCommand(c, path, e.PolicyResources.WriteRoots[1:])
		require(t, err)
		title := "policy59-" + e.ExecutionID
		raw, err = orcaworker.CreateFixedAgentTerminal(ctx, client.MetaCall, worktree.ID, title, mustTerminalSettings(c).Shell, command)
		require(t, err)
		terminal, err = orcaworker.ParseFixedAgentTerminalReceipt(raw, worktree.ID, title)
		require(t, err)
		runtimeID = raw.RuntimeID
		e.Process = &executionbackend.ProcessIdentity{Kind: "orca-native", Terminal: terminal.Handle, RuntimeID: runtimeID, Incarnation: terminal.IncarnationID, WorktreeID: worktree.ID}
	}
	t.Logf("isolated native resources: %s / %s", root, path)
	run.WorktreePath = &path
	e.Workspace.Worktree = path
	writeArgs, err := linkedWorktreeWriteArgs(ctx, c, path)
	require(t, err)
	require(t, bindNativePolicyProbeResources(ctx, c, &e, path, writeArgs))
	script := filepath.Join(path, "policy59-probe.ps1")
	resultFile := filepath.Join(path, "policy59-result.json")
	nonce := nativePolicyID()
	scriptBody := fmt.Sprintf(`$ErrorActionPreference='Stop'
$hostRead=(Get-Content -LiteralPath "$env:SystemRoot/System32/drivers/etc/hosts" -Raw).Length -ge 0
$tempFile=Join-Path (Get-Location) 'policy59-data.tmp'
[IO.File]::WriteAllText($tempFile,'policy59:%s')
$read=(Get-Content -LiteralPath $tempFile -Raw) -eq 'policy59:%s'
git add -- policy59-data.tmp
if ($LASTEXITCODE -ne 0) { throw 'Git index write failed' }
$blob=(git rev-parse :policy59-data.tmp).Trim()
$admin=(git rev-parse --absolute-git-dir).Trim()
$common=(git rev-parse --path-format=absolute --git-common-dir).Trim()
$ref='refs/steerline-policy59/%s'
git update-ref --create-reflog $ref HEAD
if ($LASTEXITCODE -ne 0) { throw 'Git ref/reflog write failed' }
git reset --quiet HEAD -- policy59-data.tmp
if ($LASTEXITCODE -ne 0) { throw 'Git index cleanup failed' }
Remove-Item -LiteralPath $tempFile
$result=@{nonce='%s';host_read=$hostRead;workspace_read=$read;workspace_write=$true;delete=$true;git_index_write=$true;git_ref_log_write=$true;git_admin=$admin;git_common=$common;blob=$blob;status=(git status --porcelain --untracked-files=no | Out-String).Trim()}
$result | ConvertTo-Json | Set-Content -Encoding UTF8 -LiteralPath 'policy59-result.json'
Write-Output ('SLPOLICY59:%s:' + ($result | ConvertTo-Json -Compress))
`, nonce, nonce, nonce, nonce, nonce)
	require(t, os.WriteFile(script, []byte(scriptBody), 0600))
	prompt := "This is an isolated native permission check, not a development task. Run exactly one native shell tool command: powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./policy59-probe.ps1 . Do not edit the script, commit, push, or ask for permission. Report its SLPOLICY59 marker and finish."
	partial, err := json.MarshalIndent(map[string]any{"scope": "in-progress native policy check", "profile": profile, "execution": e, "state_root": root}, "", "  ")
	require(t, err)
	require(t, os.WriteFile(reportPath, partial, 0600))
	var journalPath string
	var nativeTurn string
	var sendReceipt json.RawMessage
	if host == "local" {
		backend := nativePolicyBackend(t, c, run, &e, profile)
		input := executionbackend.TurnInput{CommandID: nativePolicyID(), Prompt: prompt}
		session, binding, err := backend.PrepareSession(ctx, codexworker.Binding{}, input)
		if binding.Endpoint != "" || binding.SessionID != "" {
			setNativePolicyBinding(t, &e, binding.Endpoint, binding.Generation, binding.SessionID)
			e.Process = binding.Process
		}
		require(t, err)
		defer session.Close()
		observer, turn, err := backend.StartTurn(ctx, session, input, writeArgs)
		require(t, err)
		nativeTurn, journalPath = turn, session.Paths.JournalPath
		defer func() { observer.Cancel(); <-observer.Finished; observer.Process.Kill(); observer.Process.Close() }()
		for event := range observer.Events {
			event.Persisted <- nil
		}
		require(t, <-observer.Done)
	} else {
		// The production input qualifier checks the current rendered composer and
		// original terminal/incarnation, rather than the CLI's stream idle heuristic.
		for {
			fact, canSend, err := orcaworker.InspectAgentInput(ctx, client.MetaCall, orcaworker.TerminalIdentity{Kind: e.Process.Kind, Handle: e.Process.Terminal, RuntimeID: e.Process.RuntimeID, Incarnation: e.Process.Incarnation})
			require(t, err)
			if canSend {
				break
			}
			if fact.Reason == "host_wait" {
				t.Fatalf("host input blocked: %+v", fact)
			}
			if ctx.Err() != nil {
				t.Fatalf("native input not verified: %+v", fact)
			}
			time.Sleep(time.Second)
		}
		sent, err := client.MetaCall(ctx, "terminal", "send", "--terminal", terminal.Handle, "--text", prompt, "--enter", "--wait-submit", "30", "--json")
		require(t, err)
		sendReceipt = sent.Payload
		for ctx.Err() == nil {
			if _, err := os.Stat(resultFile); err == nil {
				break
			}
			time.Sleep(time.Second)
		}
		require(t, ctx.Err())
		for {
			_, idle, err := orcaworker.InspectAgentInput(ctx, client.MetaCall, orcaworker.TerminalIdentity{Kind: e.Process.Kind, Handle: e.Process.Terminal, RuntimeID: e.Process.RuntimeID, Incarnation: e.Process.Incarnation})
			require(t, err)
			if idle {
				break
			}
			if ctx.Err() != nil {
				t.Fatal("original native tool turn did not settle")
			}
			time.Sleep(time.Second)
		}
		show, err := client.MetaCall(ctx, "terminal", "show", "--terminal", terminal.Handle, "--json")
		require(t, err)
		if show.RuntimeID != runtimeID || !strings.Contains(string(show.Payload), terminal.IncarnationID) {
			t.Fatal("native terminal identity changed")
		}
		journalPath = findPolicyNativeJournal(t, path)
	}
	raw, err := os.ReadFile(resultFile)
	require(t, err)
	var result struct {
		Nonce    string `json:"nonce"`
		HostRead bool   `json:"host_read"`
		Read     bool   `json:"workspace_read"`
		Write    bool   `json:"workspace_write"`
		Delete   bool   `json:"delete"`
		Index    bool   `json:"git_index_write"`
		Refs     bool   `json:"git_ref_log_write"`
		Admin    string `json:"git_admin"`
		Common   string `json:"git_common"`
		Blob     string `json:"blob"`
		Status   string `json:"status"`
	}
	require(t, contracts.DecodeJSON(raw, &result))
	if !nativePolicyCommit.MatchString(result.Blob) || result.Nonce != nonce || !result.HostRead || !result.Read || !result.Write || !result.Delete || !result.Index || !result.Refs || result.Status != "" || nativePolicyCanonical(result.Common) != c.Identity() {
		t.Fatalf("native tool checks failed: %s", raw)
	}
	records, err := os.ReadFile(journalPath)
	require(t, err)
	if !strings.Contains(string(records), "SLPOLICY59:"+nonce) || !strings.Contains(string(records), "policy59-probe.ps1") {
		t.Fatal("result has no original native tool evidence")
	}
	// Save only bounded native evidence for this new session, never unrelated history.
	evidence := map[string]any{"captured_at": time.Now().UTC(), "scope": "new policy mapping; isolated native tools; no readiness receipt or business dispatch", "profile": profile, "execution": e, "turn_id": nativeTurn, "send_receipt": sendReceipt, "result": json.RawMessage(nativePolicyTrimBOM(raw)), "state_root": root, "journal_path": journalPath, "native_events": policyNativeEvents(records, nonce)}
	data, err := json.MarshalIndent(evidence, "", "  ")
	require(t, err)
	require(t, os.WriteFile(reportPath, data, 0600))
	git(path, "update-ref", "-d", "refs/steerline-policy59/"+nonce)
	require(t, os.Remove(script))
	require(t, os.Remove(resultFile))
	// The blob exists only in this no-hardlinks acceptance clone; delete this exact object.
	object := filepath.Join(c.Identity(), "objects", result.Blob[:2], result.Blob[2:])
	require(t, os.Remove(object))
	if git(path, "status", "--porcelain") != "" {
		t.Fatal("acceptance worktree not clean")
	}
	if host == "orca" {
		_, err := client.MetaCall(ctx, "terminal", "close", "--terminal", terminal.Handle, "--json")
		require(t, err)
	}
	t.Logf("new native policy evidence saved: %s", reportPath)
}

func findPolicyNativeJournal(t *testing.T, workspace string) string {
	t.Helper()
	home := codexworker.EnvironmentHome()
	if supplied := os.Getenv("STEERLINE_POLICY_LIVE_NATIVE_HOME"); supplied != "" {
		home = supplied
	}
	root := filepath.Join(home, "sessions", time.Now().Format("2006"), time.Now().Format("01"), time.Now().Format("02"))
	var found string
	require(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		line, _, _ := strings.Cut(string(data), "\n")
		var meta struct {
			Payload struct {
				Cwd string `json:"cwd"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &meta) == nil && nativePolicyCanonical(meta.Payload.Cwd) == nativePolicyCanonical(workspace) {
			if found != "" {
				return fmt.Errorf("multiple native sessions for isolated workspace")
			}
			found = path
		}
		return nil
	}))
	if found == "" {
		t.Fatal("original Orca Codex journal not found")
	}
	return found
}

// Collection resumes from the exact durable acceptance snapshot after a tool
// turn has already settled. It never starts a terminal or resends an input.
func TestExecutionPolicyNativeToolsCollectLive(t *testing.T) {
	reportPath := os.Getenv("STEERLINE_POLICY_LIVE_COLLECT")
	if reportPath == "" {
		t.Skip("native evidence collection was not selected")
	}
	raw, err := os.ReadFile(reportPath)
	require(t, err)
	var saved struct {
		Execution executionmodel.ExecutionRecord       `json:"execution"`
		Profile   executionmodel.FixedExecutionProfile `json:"profile"`
		StateRoot string                               `json:"state_root"`
		Scope     string                               `json:"scope"`
	}
	require(t, contracts.DecodeJSON(raw, &saved))
	if saved.Scope != "in-progress native policy check" || !testVerifyFixedExecutionProfile(saved.Profile) {
		t.Fatal("not an original incomplete native check")
	}
	e := saved.Execution
	c := bindUnitDirectory(t, Config{Repository: filepath.Dir(e.Workspace.Repository), CommandTimeout: "90s", Git: executionbackend.Command{Executable: "git"}})
	require(t, c.ResolveIdentity(context.Background()))
	c = c.ApplyFixedExecutionProfile(saved.Profile)
	require(t, verifyNativePolicyProbeResources(c, &e, saved.Profile.Digest, e.Workspace.Worktree))
	client := orcaworker.New(nativeTerminalConfig(c, nativePolicyTarget(c)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fact, idle, err := orcaworker.InspectAgentInput(ctx, client.MetaCall, orcaworker.TerminalIdentity{Kind: e.Process.Kind, Handle: e.Process.Terminal, RuntimeID: e.Process.RuntimeID, Incarnation: e.Process.Incarnation})
	require(t, err)
	if !idle || fact.Runtime != e.Process.RuntimeID || fact.Incarnation != e.Process.Incarnation {
		t.Fatalf("original session not verified: %+v", fact)
	}
	resultPath := filepath.Join(e.Workspace.Worktree, "policy59-result.json")
	resultRaw, err := os.ReadFile(resultPath)
	require(t, err)
	var result map[string]any
	require(t, json.Unmarshal(nativePolicyTrimBOM(resultRaw), &result))
	nonce, _ := result["nonce"].(string)
	blob, _ := result["blob"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(nonce) || !nativePolicyCommit.MatchString(blob) || nativePolicyCanonical(fmt.Sprint(result["git_common"])) != c.Identity() || result["status"] != "" {
		t.Fatal("tool result differs from original workspace")
	}
	for _, key := range []string{"host_read", "workspace_read", "workspace_write", "delete", "git_index_write", "git_ref_log_write"} {
		if result[key] != true {
			t.Fatalf("native check failed: %s", key)
		}
	}
	journal := findPolicyNativeJournal(t, e.Workspace.Worktree)
	events, err := os.ReadFile(journal)
	require(t, err)
	if !policyNativeToolProof(events, nonce) {
		t.Fatal("no original native shell tool proof")
	}
	first, _, _ := strings.Cut(string(events), "\n")
	var meta struct {
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}
	require(t, json.Unmarshal([]byte(first), &meta))
	envelope, ok, err := nativePolicyBindingEnvelope(&e)
	require(t, err)
	if !ok {
		t.Fatal("saved native execution lost its app-server binding envelope")
	}
	binding, err := codexworker.DecodeAppServerBindingEnvelope(envelope, codexworker.AppServerBindingIdentity{
		ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID,
		Generation: e.Generation, ConfigDigest: e.ConfigurationDigest,
	})
	require(t, err)
	binding.SessionID = meta.Payload.ID
	setNativePolicyBinding(t, &e, binding.Endpoint, binding.Generation, binding.SessionID)
	evidence := map[string]any{"captured_at": time.Now().UTC(), "scope": "new policy mapping; original persisted snapshot; native tool collection only; no resend or readiness receipt", "profile": saved.Profile, "execution": e, "result": result, "state_root": saved.StateRoot, "journal_path": journal, "native_events": policyNativeEvents(events, nonce)}
	data, err := json.MarshalIndent(evidence, "", "  ")
	require(t, err)
	require(t, os.WriteFile(reportPath, data, 0600))
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", e.Workspace.Worktree}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("cleanup %v: %s/%v", args, out, err)
		}
	}
	git("update-ref", "-d", "refs/steerline-policy59/"+nonce)
	require(t, os.Remove(filepath.Join(e.Workspace.Worktree, "policy59-probe.ps1")))
	require(t, os.Remove(resultPath))
	require(t, os.Remove(filepath.Join(c.Identity(), "objects", blob[:2], blob[2:])))
	_, err = client.MetaCall(ctx, "terminal", "close", "--terminal", e.Process.Terminal, "--json")
	require(t, err)
	t.Logf("original new-policy tool evidence collected without resend: %s", reportPath)
}

func policyNativeToolProof(data []byte, nonce string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
				Item struct {
					Type     string   `json:"type"`
					Command  []string `json:"command"`
					Status   string   `json:"status"`
					ExitCode *int     `json:"exit_code"`
					Output   string   `json:"aggregated_output"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		item := event.Payload.Item
		if event.Type == "event_msg" && event.Payload.Type == "item_completed" && item.Type == "CommandExecution" && item.Status == "completed" && item.ExitCode != nil && *item.ExitCode == 0 && strings.Contains(strings.Join(item.Command, " "), "policy59-probe.ps1") && strings.Contains(item.Output, "SLPOLICY59:"+nonce) {
			return true
		}
	}
	return false
}

func policyNativeEvents(data []byte, nonce string) []json.RawMessage {
	var out []json.RawMessage
	for _, line := range strings.Split(string(data), "\n") {
		if !json.Valid([]byte(line)) {
			continue
		}
		if strings.Contains(line, `"session_meta"`) {
			var meta map[string]any
			_ = json.Unmarshal([]byte(line), &meta)
			if payload, ok := meta["payload"].(map[string]any); ok {
				for _, key := range []string{"base_instructions", "creator_user_id", "creator_account_id", "git"} {
					delete(payload, key)
				}
			}
			encoded, _ := json.Marshal(meta)
			out = append(out, encoded)
			continue
		}
		if strings.Contains(line, "policy59-probe.ps1") || strings.Contains(line, "SLPOLICY59:"+nonce) || strings.Contains(line, `"turn_context"`) || strings.Contains(line, `"turn/completed"`) || strings.Contains(line, `"thread/settings/updated"`) {
			out = append(out, json.RawMessage(line))
		}
	}
	return out
}

var nativePolicyCommit = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func nativePolicyID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func nativePolicyCanonical(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(absolute))
	}
	return filepath.Clean(absolute)
}

func nativePolicyTrimBOM(raw []byte) []byte { return bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}) }

func nativePolicyTarget(c Config) executionbackend.ExecutionTarget {
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		panic(err)
	}
	return entry.Target(c)
}

// These helpers construct only an acceptance report DTO. They receive neither
// a Service nor a Store, and invoke the actual catalog's resource codec.
func nativePolicyResourceDigest(resources executionmodel.PolicyResources) string {
	resources.Digest = ""
	sum := sha256.Sum256(mustJSON(resources))
	return hex.EncodeToString(sum[:])
}

func bindNativePolicyProbeResources(ctx context.Context, c Config, e *executionmodel.ExecutionRecord, path string, linked []string) error {
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return err
	}
	if len(c.ImplementationData) == 0 {
		return errors.New("accepted native profile data missing")
	}
	profile := c.ImplementationData[0]
	id := execution.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}
	value, err := entry.BuildPolicyResource(ctx, execution.PolicyResourceBuildRequest{ProfileData: c.ImplementationData, ProjectID: c.Identity(), ProfileID: profile.ProfileID, AcceptedConfigVersion: profile.AcceptedConfigVersion, Resource: id, Policy: c.ExecutionPolicy, Host: c.Host, Agent: c.Agent, Workspace: path, LinkedWorktreeArgs: linked, RepositoryPath: c.Repository, CommandTimeout: 90 * time.Second})
	if err != nil {
		return err
	}
	if err := entry.VerifyPolicyResource(value.Envelope, id); err != nil {
		return err
	}
	resources := executionmodel.PolicyResources{Version: 3, ProfileDigest: e.ConfigurationDigest, Workspace: path, WriteRoots: append([]string(nil), value.WriteRoots...), TemporaryRoots: []string{}, ImplementationData: value.Envelope.Clone()}
	resources.Digest = nativePolicyResourceDigest(resources)
	if e.PolicyResources != nil && !reflect.DeepEqual(*e.PolicyResources, resources) {
		return errors.New("original native acceptance resources changed")
	}
	e.PolicyResources = &resources
	return nil
}

func verifyNativePolicyProbeResources(c Config, e *executionmodel.ExecutionRecord, digest, path string) error {
	r := e.PolicyResources
	if r == nil || r.Version != 3 || r.Digest != nativePolicyResourceDigest(*r) || r.ProfileDigest != digest || r.Workspace != path || len(r.TemporaryRoots) != 0 || r.ImplementationData.ConfigDigest != digest {
		return errors.New("original acceptance resource snapshot differs")
	}
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil {
		return err
	}
	return entry.VerifyPolicyResource(r.ImplementationData, execution.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
}

func setNativePolicyBinding(t *testing.T, e *executionmodel.ExecutionRecord, endpoint, generation, session string) {
	t.Helper()
	envelope, err := codexworker.NewAppServerBindingEnvelope(codexworker.AppServerBindingIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, codexworker.AppServerBinding{Endpoint: endpoint, Generation: generation, SessionID: session})
	require(t, err)
	found := false
	for i := range e.ImplementationData {
		if e.ImplementationData[i].ImplementationKey == envelope.ImplementationKey {
			if found {
				t.Fatal("duplicate native binding envelope")
			}
			e.ImplementationData[i] = envelope.Clone()
			found = true
		}
	}
	if !found {
		e.ImplementationData = append(e.ImplementationData, envelope.Clone())
	}
	e.SessionID, e.SessionGeneration = session, generation
}

func nativePolicyBindingEnvelope(e *executionmodel.ExecutionRecord) (executionbackend.ImplementationDataEnvelope, bool, error) {
	var value executionbackend.ImplementationDataEnvelope
	found := false
	for _, envelope := range e.ImplementationData {
		if envelope.ImplementationKey == codexworker.ExecutionBindingCodecKey {
			if found {
				return value, false, errors.New("duplicate native binding envelope")
			}
			value, found = envelope.Clone(), true
		}
	}
	return value, found, nil
}

func nativePolicyBackend(t *testing.T, c Config, run *steerline.Run, e *executionmodel.ExecutionRecord, profile executionmodel.FixedExecutionProfile) codexworker.Backend {
	t.Helper()
	entry, err := c.ExecutionDirectory.Resolve(c)
	require(t, err)
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: entry.Key.Backend, Protocol: entry.Key.Protocol, Target: e.Target, Agent: executionbackend.AgentSpec{Name: c.Agent}, RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: *run.WorktreePath}, Session: executionbackend.SessionContext{ProjectID: c.Identity(), ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, WorkflowRunID: run.RunID, Agent: executionbackend.AgentSpec{Name: c.Agent}}, ResourceGeneration: e.Generation, ConfigurationDigest: e.ConfigurationDigest}
	port, err := entry.Lifecycle(executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileData: c.ImplementationData, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion, Policy: c.ExecutionPolicy, ProcessHost: c.ProcessHost, RunDirectory: c.RunDir, RepositoryPath: c.Repository, CommandTimeout: 90 * time.Second, BlockedEnvironment: []string{"GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN"}})
	require(t, err)
	actual, ok := port.(*codexworker.ExecutionLifecycle)
	if !ok {
		t.Fatal("native app-server lifecycle type differs")
	}
	return actual.Backend
}
