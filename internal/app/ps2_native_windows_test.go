//go:build windows && ps2native

package app

import (
	"context"
	"encoding/json"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestPS2NativeDeveloperAndAutomaticReview(t *testing.T) {
	host, executable, output := os.Getenv("STEERLINE_PS2_HOST"), os.Getenv("STEERLINE_PS2_CODEX"), os.Getenv("STEERLINE_PS2_OUTPUT")
	if host != "local" || executable == "" || output == "" {
		t.Fatal("explicit local PS2 native executable/output are required; Orca uses TestOrcaNativeWorkerSmoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	root, err := os.MkdirTemp("", "steerline-ps2-"+host+"-")
	require(t, err)
	require(t, os.MkdirAll(output, 0700))
	git := func(dir string, args ...string) string {
		t.Helper()
		raw, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	// Authentication stays in the isolated home; only non-secret receipts are exported.
	authSource := os.Getenv("CODEX_HOME")
	if authSource == "" {
		userHome, err := os.UserHomeDir()
		require(t, err)
		authSource = filepath.Join(userHome, ".codex")
	}
	auth, err := os.ReadFile(filepath.Join(authSource, "auth.json"))
	require(t, err)
	codexHome := filepath.Join(root, "codex")
	require(t, os.MkdirAll(codexHome, 0700))
	require(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), auth, 0600))
	defer os.Remove(filepath.Join(codexHome, "auth.json"))
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "home"))
	repo := filepath.Join(root, "repo")
	require(t, os.MkdirAll(repo, 0700))
	git(repo, "init", "--quiet", "--initial-branch=main")
	git(repo, "config", "user.name", "PS2 validation")
	git(repo, "config", "user.email", "ps2@example.invalid")
	git(repo, "config", "commit.gpgsign", "false")
	require(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Isolated PS2 developer fixture\n"), 0600))
	git(repo, "add", "README.md")
	git(repo, "commit", "--quiet", "-m", "PS2 fixture")
	c := Config{Repository: repo, Branch: "main", RunDir: filepath.Join(root, "state"), WorkflowProtocol: WorkerFlowVersion, Host: host, Agent: "codex", Git: executionbackend.Command{Executable: "git"}, CommandTimeout: "45s"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: executable, Model: "gpt-6-luna", ReasoningEffort: "medium"}, nil)
	require(t, c.ResolveIdentity(ctx))
	profile := unitProjectProfile(c).Freeze(c.Identity())
	c = c.ApplyFixedExecutionProfile(profile)
	config := c.executionConfig()
	e := executionmodel.ExecutionRecord{ProjectIdentity: c.Identity(), WorkflowRunID: nativePolicyID(), ExecutionID: nativePolicyID(), CurrentAttemptID: nativePolicyID(), Generation: nativePolicyID(), ConfigurationDigest: profile.Digest, Target: nativePolicyTarget(config, c.ExecutionDirectory)}
	path := filepath.Join(root, "candidate")
	git(repo, "worktree", "add", "--quiet", "-b", "ps2-candidate", path, "HEAD")
	e.Workspace = executionmodel.WorkspaceEvidence{Repository: c.Identity(), Worktree: path, Branch: git(path, "branch", "--show-current")}
	linked, err := linkedWorktreeWriteArgs(ctx, config, path, c.ProcessHost)
	require(t, err)
	require(t, bindNativePolicyProbeResources(ctx, config, c.ExecutionDirectory, c.ProcessHost, &e, path, linked))
	trust := "[projects.'" + repo + "']\ntrust_level = \"trusted\"\n[projects.'" + path + "']\ntrust_level = \"trusted\"\n"
	require(t, os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(trust), 0600))
	remote := filepath.Join(root, "receive.git")
	git(path, "init", "--bare", "--quiet", remote)
	git(remote, "config", "http.receivepack", "true")
	gitExecutable, err := exec.LookPath("git")
	require(t, err)
	receiver := httptest.NewServer(&cgi.Handler{Path: gitExecutable, Args: []string{"http-backend"}, Dir: root, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}})
	defer receiver.Close()
	remoteURL := receiver.URL + "/receive.git"
	sibling := filepath.Join(root, "sibling-candidate")
	require(t, os.MkdirAll(sibling, 0700))
	reviewPath := filepath.Join(root, "approved-review-marker.txt")
	nonce := nativePolicyID()
	probe := `$ErrorActionPreference='Stop'
[IO.File]::WriteAllText((Join-Path (Get-Location) 'result.txt'),'PS2:` + nonce + `')
if ((Get-Content -LiteralPath ./result.txt -Raw) -cne 'PS2:` + nonce + `') { throw 'project test failed' }
Write-Output 'PS2 PROJECT TEST PASSED'
$siblingDenied=$false
try { [IO.File]::WriteAllText('` + strings.ReplaceAll(filepath.Join(sibling, "must-not-write.txt"), "'", "''") + `','forbidden') } catch { $siblingDenied=$true }
if (-not $siblingDenied) { throw 'sibling boundary failed' }
git add -- result.txt
if ($LASTEXITCODE -ne 0) { throw 'git add failed' }
git -c commit.gpgsign=false commit -m 'PS2 native development'
if ($LASTEXITCODE -ne 0) { throw 'git commit failed' }
git -c gc.auto=0 -c maintenance.auto=false push ` + remoteURL + ` HEAD:refs/heads/validated
if ($LASTEXITCODE -ne 0) { throw 'git push failed' }
@{nonce='` + nonce + `';sibling_denied=$siblingDenied;project_test=$true;commit=(git rev-parse HEAD).Trim();pushed=(git ls-remote ` + remoteURL + ` refs/heads/validated).Split()[0]} | ConvertTo-Json | Set-Content -Encoding UTF8 ./ps2-result.json
`
	require(t, os.WriteFile(filepath.Join(path, "ps2-probe.ps1"), []byte(probe), 0600))
	require(t, os.WriteFile(filepath.Join(path, "ps2-review.ps1"), []byte("[IO.File]::WriteAllText('"+strings.ReplaceAll(reviewPath, "'", "''")+"','PS2:"+nonce+"')\n"), 0600))
	prompt := "This isolated fixture authorizes development only in this candidate. First run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./ps2-probe.ps1 using the normal sandbox, WITHOUT an escalation request. It performs a file change, project test, commit and push only to the local bare fixture. The sibling write is an expected denial caught by the script; never request approval to write that sibling. Do not modify either probe script. Then make exactly one bounded native approval request using sandbox_permissions=require_escalated to run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./ps2-review.ps1. That script writes only the explicitly authorized temporary marker at " + reviewPath + ". Route this request to native automatic review, not human confirmation. A reviewer denial/unavailability must be reported as-is; do not retry or change settings. Do not publish any PR, use real remotes, call another model, or perform other work. Finish with the development result and the native review decision."
	receipt := map[string]any{"host": host, "root": root, "candidate": path, "profile": profile, "execution": e, "review_path": reviewPath, "uac_observed": false, "uac_observation": "no administrator initialization or launch requested; capture records native output"}
	writeEvidence := func() {
		t.Helper()
		raw, err := json.MarshalIndent(receipt, "", "  ")
		require(t, err)
		require(t, os.WriteFile(filepath.Join(output, "receipt.json"), raw, 0600))
	}
	writeEvidence()
	t.Log("native fixture", root, path)
	var journal string
	defer func() {
		if journal != "" {
			if records, err := os.ReadFile(journal); err == nil {
				if err := os.WriteFile(filepath.Join(output, "native-events.jsonl"), records, 0600); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	run := &reconstructiontypes.RunView{RunID: e.WorkflowRunID, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, WorktreePath: &path}
	backend := nativePolicyBackend(t, c, run, &e, profile)
	input := executionbackend.TurnInput{CommandID: nativePolicyID(), Prompt: prompt}
	session, binding, err := backend.PrepareSession(ctx, codexworker.Binding{}, input)
	require(t, err)
	defer session.Close()
	e.Process = binding.Process
	observer, turn, err := backend.StartTurn(ctx, session, input, linked)
	require(t, err)
	defer func() { observer.Cancel(); <-observer.Finished; observer.Process.Kill(); observer.Process.Close() }()
	journal = session.Paths.JournalPath
	receipt["turn_id"], receipt["execution"] = turn, e
	writeEvidence()
	for event := range observer.Events {
		event.Persisted <- nil
	}
	require(t, <-observer.Done)
	raw, err := os.ReadFile(filepath.Join(path, "ps2-result.json"))
	require(t, err)
	var result struct {
		Nonce         string `json:"nonce"`
		SiblingDenied bool   `json:"sibling_denied"`
		ProjectTest   bool   `json:"project_test"`
		Commit        string `json:"commit"`
		Pushed        string `json:"pushed"`
	}
	require(t, json.Unmarshal(nativePolicyTrimBOM(raw), &result))
	if result.Nonce != nonce || !result.SiblingDenied || !result.ProjectTest || result.Commit == "" || result.Commit != result.Pushed {
		t.Fatalf("development evidence incomplete: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(sibling, "must-not-write.txt")); !os.IsNotExist(err) {
		t.Fatal("sibling candidate was written")
	}
	records, err := os.ReadFile(journal)
	require(t, err)
	require(t, os.WriteFile(filepath.Join(output, "native-events.jsonl"), records, 0600))
	if !strings.Contains(strings.ToLower(string(records)), "autoreview") && !strings.Contains(strings.ToLower(string(records)), "auto_review") {
		t.Fatal("native automatic reviewer evidence missing")
	}
	receipt["development"], receipt["native_journal"], receipt["complete"] = result, journal, true
	if marker, err := os.ReadFile(reviewPath); err == nil {
		receipt["review_marker_written"] = string(marker) == "PS2:"+nonce
	} else {
		receipt["review_marker_written"] = false
	}
	writeEvidence()
}
