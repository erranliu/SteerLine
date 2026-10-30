//go:build e2e

package testenv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/google/uuid"
)

// Explicit live acceptance, invoked only by scripts/test-project-progress-live.ps1.
func RunProjectProgress(t *testing.T) {
	var frozen Run
	_ = json.Unmarshal([]byte(os.Getenv("STEERLINE_E2E_RUN")), &frozen)
	source := os.Getenv("STEERLINE_PROGRESS_PROJECT")
	if frozen.Remote != nil {
		source = frozen.Remote.Project
	}
	if source == "" {
		t.Skip("requires explicit isolated live progress invocation")
	}
	if !filepath.IsAbs(source) {
		t.Fatal("project source must be absolute")
	}
	scenarioID := "testing.project-progress-local-codex"
	if frozen.Remote != nil {
		scenarioID = "testing.project-progress-orca-browser"
	}
	s := NewScenario(t, scenarioID)
	p := s.NewLiveProject("steerlinetest", "local-codex")
	// Writable native sessions authenticate the real repository identity. Local
	// transport uses loopback; remote execution gets an owned GitHub base ref.
	p.RemoteURL = githubURL
	remote := loadOrcaTarget(s, p)
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	provider, err := controlled.Read(p.Provider)
	s.Must(err)
	provider.Repository = githubRepository
	s.Must(controlled.Write(p.Provider, provider))
	requestedCommit := os.Getenv("STEERLINE_PROGRESS_COMMIT")
	if s.Run.Remote != nil {
		requestedCommit = s.Run.Remote.Commit
	}
	commit := p.git(source, "rev-parse", "--verify", requestedCommit+"^{commit}")
	p.git(p.Repo, "fetch", "--quiet", source, commit)
	p.BaseBranch = "progress-live"
	if remote != nil {
		p.BaseBranch = "codex/e2e/" + strings.ReplaceAll(uuid.NewString(), "-", "") + "/" + s.ID + "/base"
	}
	p.git(p.Repo, "checkout", "--quiet", "-b", p.BaseBranch, "FETCH_HEAD")
	// Keep the actual project's versioned programs unchanged in this independent checkout.
	programs := []string{"tools/test-runtime-progress.py", "tools/runtime_progress.py", "tools/steerline_reports.py"}
	programDigests := map[string]string{}
	for _, path := range programs {
		body, err := os.ReadFile(filepath.Join(p.Repo, filepath.FromSlash(path)))
		s.Must(err)
		original := p.git(source, "rev-parse", commit+":"+path)
		if got := p.git(p.Repo, "hash-object", "--", path); got != original {
			t.Fatal("project program differs from original Git blob", path)
		}
		programDigests[path] = testmanagement.ContentDigest(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")))
	}
	keys := []string{"demo.progress.roundtrip", "demo.progress.retry", "demo.progress.cleanup"}
	spec := "# Live project progress acceptance\n\nThis is a test-only task, not a sample implementation. Do not edit tracked files, commit, push, create a PR, or publish. Read the original Plan and run its exact existing Python test entry once. Do not set STEERLINE_PROGRESS_FILE: Runner already injects it. After the program exits, summarize actual evidence and finish BLOCKED because publication is intentionally not authorized. No questions or additional work are needed.\n"
	plan := "# Live project progress plan\n\nExactly these three mandatory automatic L4 tests must be selected: demo.progress.roundtrip, demo.progress.retry, demo.progress.cleanup. Run `python tools/test-runtime-progress.py --out .steerrunner/artifacts/live-progress` exactly once in implement.execute. Keep the default six-second observation windows. Its own real subprocess negative assertion must stay incomplete until the corrected retry passes. Do not manufacture or edit progress or reports. No changes, implementation commit or publication are requested. The external acceptance reads public Runner snapshots and SteerLine API while this program executes, then stops the original Run.\n"
	if remote != nil {
		plan = strings.ReplaceAll(plan, "--out .steerrunner/artifacts/live-progress", "--pause-seconds 15 --out .steerrunner/artifacts/live-progress")
		plan = strings.ReplaceAll(plan, "Keep the default six-second observation windows.", "Keep the fifteen-second observation windows so the same real browser can observe each transition. Wait for this foreground command to exit; if the shell tool returns a running session, poll that exact session without restarting the command. Do not detach it.")
	}
	for path, body := range map[string]string{"SteerlineFlow/spec/task.spec.md": spec, "SteerlineFlow/plans/task.plan.md": plan} {
		s.Must(os.WriteFile(filepath.Join(p.Repo, filepath.FromSlash(path)), []byte(body), 0600))
	}
	s.Must(os.WriteFile(filepath.Join(p.Repo, "SteerlineFlow/plans/task.tests.json"), []byte(`{"tests":["demo.progress.roundtrip","demo.progress.retry","demo.progress.cleanup"]}`), 0600))
	agentPath := filepath.Join(p.Repo, "AGENTS.md")
	agents, err := os.ReadFile(agentPath)
	s.Must(err)
	s.Must(os.WriteFile(agentPath, append(agents, []byte("\nFor Runner readiness-only input, execute only its exact probe command and preserve the complete native command result including exit_code/output. Use login=false for shell tools. For this test-only acceptance follow its bound Spec and Plan; do not implement sample 101–105 or publish.\n")...), 0600))
	validationBase := ""
	if remote != nil {
		validationBase = p.bindValidationBase()
	}
	p.git(p.Repo, "add", "--", "AGENTS.md", "SteerlineFlow/spec/task.spec.md", "SteerlineFlow/plans/task.plan.md", "SteerlineFlow/plans/task.tests.json")
	if remote != nil {
		p.git(p.Repo, "add", "--", env.DocumentPath)
	}
	p.git(p.Repo, "commit", "--quiet", "-m", "Bind isolated live progress acceptance")
	head := p.git(p.Repo, "rev-parse", "HEAD")
	var config map[string]any
	body, err := os.ReadFile(p.Config)
	s.Must(err)
	s.Must(json.Unmarshal(body, &config))
	config["branch"] = p.BaseBranch
	config["pr"].(map[string]any)["repository"] = githubRepository
	if remote != nil {
		gh, err := exec.LookPath("gh")
		s.Must(err)
		config["git"] = map[string]any{"executable": "git"}
		query := map[string]any{"executable": gh, "args": []string{"pr", "view", "{pr_url}", "--repo", "{repository}", "--json", githubFields, "--jq", githubInfoQuery}}
		config["pr"] = map[string]any{"kind": "command", "base_url": "https://github.com", "repository": githubRepository, "query": query}
	}
	s.Must(controlled.Write(p.Config, config))
	var catalog map[string]any
	body, err = os.ReadFile(filepath.Join(p.Home, "execution.json"))
	s.Must(err)
	s.Must(json.Unmarshal(body, &catalog))
	profile := catalog["execution_profiles"].([]any)[0].(map[string]any)
	profile["workflow"] = map[string]any{"id": "development", "version": 2, "prompt_overrides": map[string]string{
		"publish": `This is a progress-only acceptance. Publication is not authorized. Do not execute any commands or query any provider. Return exactly {"outcome":"blocked","summary":"Progress-only acceptance; publication intentionally not authorized","blocker":{"category":"prerequisite","action":"publish","error":"Publication intentionally not authorized"}}.`,
	}}
	if remote != nil {
		remote.Profile["id"] = "orca-progress"
		remote.Profile["workflow"] = profile["workflow"]
		catalog["execution_profiles"] = append(catalog["execution_profiles"].([]any), remote.Profile)
		p.Driver.Profile = "orca-progress"
		provider.GitURL = githubURL
		provider.BaseBranch = p.BaseBranch
		s.Must(controlled.Write(p.Provider, provider))
		p.git(p.Repo, "config", "credential.helper", "!gh auth git-credential")
		p.RealPR, p.DefaultBranch = true, "main"
		s.register("github-base-branch", p.BaseBranch, 0)
		profile = remote.Profile
	}
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), catalog))
	if remote != nil {
		t.Cleanup(func() { p.cleanupRemoteValidation(validationBase) })
		s.Must(controlled.Write(filepath.Join(s.Output, "remote-ref-intent.json"), map[string]any{"source_branch": p.BaseBranch, "validation_base": validationBase, "commit": head}))
	}
	p.git(p.Repo, "push", "--quiet", provider.GitURL, p.BaseBranch)
	if remote != nil {
		p.git(p.Repo, "push", "--quiet", provider.GitURL, "HEAD:refs/heads/"+validationBase)
	}
	if remote != nil {
		remote.Fetch(s, p, head)
		p.Chain = "orca-remote"
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "source.json"), map[string]any{"repository": source, "source_commit": commit, "fixture_commit": head, "program_sha256": programDigests, "program_digest_normalization": "CRLF to LF only; original Git blobs checked before dispatch", "runner": catalog["runner"], "profile": profile}))
	if remote != nil {
		p.ensureRemoteReady(s.Run.Remote.RoundSeconds)
	}
	p.CreateTasks()
	public, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer public.Close()
	id := public.Identity().ProjectID
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	selection, err := runnerclientSelection(p.Home)
	s.Must(err)
	runner, _, err := selection.Open(s.Context, runnerstatepath.Directory(p.Home), 10*time.Second)
	s.Must(err)
	service, err := runner.ServiceStatus(s.Context)
	s.Must(err)
	runner.InstanceID = service.InstanceID
	browser := startProgressBrowser(s, p)
	p.Claim("a")
	var candidateID, runID, digest string
	s.Wait("original real candidate binding", func() (bool, error) {
		state, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		for _, c := range state.Runner.Candidates {
			if c.RunID != "" && c.InputDigest != "" && c.Projection != nil {
				candidateID, runID, digest = c.ID, c.RunID, c.InputDigest
				if remote != nil {
					p.verifyRemoteExecution(c.Submit.Input)
				}
				s.Must(controlled.Write(filepath.Join(s.Output, "initial-binding.json"), c))
				return true, nil
			}
		}
		return false, nil
	})
	if browser != nil {
		browser.Bind(s, id, p.Tasks["a"].ID, candidateID)
	}
	// Preserve original public facts and reports before scenario cleanup even if
	// a later assertion fails. Never turn a diagnostic-copy failure into a pass.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		caller, callerErr := driver.ReadWorkflow(ctx, p.Database, p.ProjectID)
		if err := controlled.Write(filepath.Join(s.Output, "caller-final-workflow.json"), map[string]any{"snapshot": caller, "error": fmt.Sprint(callerErr)}); err != nil {
			t.Error(err)
		}
		observed, err := runner.Inspect(ctx, "run-id", runID, false)
		if err != nil || observed.Run == nil {
			return // A successful body already retained evidence before stopping.
		}
		if err := controlled.Write(filepath.Join(s.Output, "cleanup-run.json"), observed.Run); err != nil {
			t.Error(err)
		}
		workspace := observed.Run.Execution.Workspace
		if workspace == "" {
			return // Execution was rejected before an original workspace existed.
		}
		if remote != nil {
			if _, err := os.Stat(filepath.Join(s.Output, "original-reports")); err == nil {
				return
			}
			workspace = remote.Capture(s, p, *observed.Run, false)
			if workspace == "" {
				return
			}
		}
		reports := filepath.Join(workspace, ".steerrunner", "artifacts", "live-progress")
		if _, err := os.Stat(reports); err == nil {
			destination := filepath.Join(s.Output, "original-reports")
			if _, err := os.Stat(destination); os.IsNotExist(err) {
				if err := os.CopyFS(destination, os.DirFS(reports)); err != nil {
					t.Error(err)
				}
			}
		}
	})
	expected := [][3]bool{{false, false, false}, {true, false, false}, {true, true, false}, {true, false, false}, {true, true, false}, {true, true, true}}
	matched := 0
	observations := []map[string]any{}
	rawByVersion := map[uint64]map[string]bool{}
	lastVersion := uint64(0)
	lastView := uint64(0)
	var terminalSince time.Time
	var view testmanagement.View
	s.Wait("real Runner snapshots and SteerLine progress sequence", func() (bool, error) {
		ctx, cancel := context.WithTimeout(s.Context, 10*time.Second)
		defer cancel()
		observed, err := runner.Inspect(ctx, "run-id", runID, false)
		if err != nil {
			return false, err
		}
		if observed.Run != nil {
			if observed.Run.Status == "blocked" || observed.Run.Status == "failed" || observed.Run.Status == "stopped" {
				s.Must(controlled.Write(filepath.Join(s.Output, "original-run.json"), observed.Run))
				if terminalSince.IsZero() {
					terminalSince = time.Now()
				}
				if time.Since(terminalSince) > 15*time.Second {
					t.Fatalf("native Run ended before required progress: status=%s phase=%s diagnostic=%v", observed.Run.Status, observed.Run.Phase, observed.Run.Diagnostic)
				}
			}
			reports := filepath.Join(observed.Run.Execution.Workspace, ".steerrunner", "artifacts", "live-progress")
			if raw, err := readLocalProgressSummary(remote, reports); err == nil {
				var summary struct {
					Passed bool `json:"passed"`
				}
				s.Must(json.Unmarshal(raw, &summary))
				if !summary.Passed {
					s.Must(os.CopyFS(filepath.Join(s.Output, "original-reports"), os.DirFS(reports)))
					s.Must(controlled.Write(filepath.Join(s.Output, "original-run.json"), observed.Run))
					t.Fatal("original project test program failed; preserved original reports")
				}
			}
		}
		snapshot, err := runner.ProgressFile(ctx, runID)
		if err != nil {
			return false, err
		}
		if snapshot.Version < lastVersion {
			t.Fatal("public progress version regressed")
		}
		lastVersion = snapshot.Version
		if snapshot.Version > 0 {
			raw, err := runnerclient.ValidateProgressFile(snapshot, runID)
			if err != nil {
				return false, err
			}
			fields := map[string]bool{}
			if err = json.Unmarshal(raw, &fields); err != nil {
				return false, err
			}
			rawByVersion[snapshot.Version] = fields
			if err = controlled.Write(filepath.Join(s.Output, fmt.Sprintf("runner-%03d.json", snapshot.Version)), snapshot); err != nil {
				return false, err
			}
		}
		view = testmanagement.View{}
		if err := client.Get(ctx, "/projects/"+id+"/runs/"+candidateID+"/tests", &view); err != nil {
			return false, err
		}
		if view.Total != 3 || view.ProgressVersion == 0 {
			return false, nil
		}
		if view.ProgressVersion < lastView {
			t.Fatal("API progress version regressed")
		}
		lastView = view.ProgressVersion
		fields, known := rawByVersion[view.ProgressVersion]
		if !known {
			return false, nil
		}
		var state [3]bool
		for i, key := range keys {
			found := false
			for _, row := range view.Rows {
				if row.ID == key {
					state[i], found = row.Status == "passed", true
					if state[i] != fields["test."+key] {
						t.Fatal("API and original Runner snapshot differ", key)
					}
				}
			}
			if !found {
				t.Fatal("frozen project test omitted", key)
			}
		}
		if len(observations) == 0 || !reflect.DeepEqual(observations[len(observations)-1]["state"], state) {
			observations = append(observations, map[string]any{"at": time.Now().UTC(), "state": state, "api": view})
			s.Must(controlled.Write(filepath.Join(s.Output, "api-observations.json"), observations))
		}
		if matched < len(expected) && state == expected[matched] {
			matched++
		}
		return matched == len(expected), nil
	})
	if browser != nil {
		browser.Verify(s, rawByVersion)
	}
	var original runnercontract.Run
	s.Wait("real native final and original project reports", func() (bool, error) {
		observed, err := runner.Inspect(s.Context, "run-id", runID, false)
		if err != nil || observed.Run == nil {
			return false, err
		}
		original = *observed.Run
		s.Must(controlled.Write(filepath.Join(s.Output, "original-run.json"), original))
		if original.PR != nil || original.InputDigest != digest {
			t.Fatalf("original Run differs: PR=%v input=%q expected=%q", original.PR, original.InputDigest, digest)
		}
		return original.Final != nil && original.Final.Verified && original.Final.Source != "", nil
	})
	if remote == nil && (original.Execution.Protocol != "local_codex_app_server_v1" || original.Execution.Backend != "local") {
		t.Fatal("native execution binding differs", original.Execution)
	}
	if remote != nil && (original.Execution.Backend != "orca" || original.Execution.Protocol != "orca_terminal_v3") {
		t.Fatal("remote native execution binding differs", original.Execution)
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "original-run.json"), original))
	evidenceWorkspace := original.Execution.Workspace
	if remote != nil {
		evidenceWorkspace = remote.Capture(s, p, original, true)
	}
	reports := filepath.Join(evidenceWorkspace, ".steerrunner", "artifacts", "live-progress")
	body, err = os.ReadFile(filepath.Join(reports, "summary.json"))
	s.Must(err)
	var summary struct {
		Passed      bool                       `json:"passed"`
		Complete    bool                       `json:"complete"`
		Before      string                     `json:"before_commit"`
		After       string                     `json:"after_commit"`
		BeforeClean bool                       `json:"before_clean"`
		AfterClean  bool                       `json:"after_clean"`
		Details     map[string]json.RawMessage `json:"details"`
	}
	s.Must(json.Unmarshal(body, &summary))
	if !summary.Passed || !summary.Complete || summary.Before != head || summary.After != head || !summary.BeforeClean || !summary.AfterClean {
		t.Fatal("actual project program source or result differs")
	}
	workerRawDigests := map[string]string{}
	for _, path := range programs {
		raw, err := os.ReadFile(filepath.Join(evidenceWorkspace, filepath.FromSlash(path)))
		s.Must(err)
		workerRawDigests[path] = testmanagement.ContentDigest(raw)
		canonical := testmanagement.ContentDigest(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")))
		if canonical != programDigests[path] {
			t.Fatalf("actual project program differs after Git line-ending normalization: %s got=%s expected=%s", path, canonical, programDigests[path])
		}
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "worker-program-sha256.json"), workerRawDigests))
	s.Must(os.CopyFS(filepath.Join(s.Output, "original-reports"), os.DirFS(reports)))
	// This acceptance owns a still-active workflow, rather than a delivered PR.
	// Settle its original Run explicitly before shutting down the caller service.
	s.stopOriginalRun(runner, runID, digest, "")
	// Let the live caller consume the independently requested native stop before
	// closing its polling loop. Preserve the same original candidate binding.
	callerStopCtx, callerStopCancel := context.WithTimeout(s.Context, 30*time.Second)
	defer callerStopCancel()
	s.Wait("caller observes original native stop", func() (bool, error) {
		if err := callerStopCtx.Err(); err != nil {
			t.Fatal("caller stop projection did not settle:", err)
		}
		state, err := p.SnapshotContext(callerStopCtx)
		if err != nil {
			return false, err
		}
		for _, candidate := range state.Runner.Candidates {
			if candidate.ID == candidateID {
				s.Must(controlled.Write(filepath.Join(s.Output, "caller-stop-candidate.json"), candidate))
				if candidate.RunID != runID || candidate.InputDigest != digest {
					t.Fatal("caller original binding changed during native stop")
				}
				return candidate.Projection != nil && candidate.Projection.StopStatus == "stopped", nil
			}
		}
		return false, nil
	})
	p.StopService()
	p.WaitStoppedCandidates()
	scope := "real local Runner and Codex execute unchanged SteerlineTest programs; live public Runner and SteerLine API; loopback Git, no publication"
	if remote != nil {
		scope = "real remote Orca/Codex executes unchanged SteerlineTest programs; Runner single-file snapshots, SteerLine API and one continuous Edge page; owned GitHub base only, no publication"
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "proof.json"), map[string]any{"passed": true, "source_commit": commit, "fixture_commit": head, "candidate_id": candidateID, "run_id": runID, "input_digest": digest, "native_source": original.Final.Source, "observed_states": expected, "observed_count": len(observations), "final_api": view, "scope": scope}))
	s.Must(s.FinishBody())
}

func runnerclientSelection(home string) (runnerclient.Selection, error) {
	var catalog struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	raw, err := os.ReadFile(filepath.Join(home, "execution.json"))
	if err == nil {
		err = json.Unmarshal(raw, &catalog)
	}
	return catalog.Runner, err
}
