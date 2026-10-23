package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	pmm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmanagementmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

// testManagement drives only public application/Runner operations and the fixture's
// actual assertion program. No result JSON is synthesized by this driver or an agent.
func (d *debugRelease) testManagement(source, runnerCandidate string) (result error) {
	if !filepath.IsAbs(source) {
		return errors.New("test management source must be absolute")
	}
	webSource := filepath.Join(source, "web")
	if _, err := os.Stat(filepath.Join(webSource, "tests", "test-management.spec.ts")); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out := filepath.Join(d.root, "evidence", "test-management")
	if err := os.MkdirAll(out, 0700); err != nil {
		return err
	}
	proof := feedback.TestManagementProof{Package: d.root}
	// Reject a public protocol fixture before pushing or starting a live package.
	versionRaw, e := d.command(filepath.Join(d.root, "bin", "steerrunner.exe"), "version", "--json").Output()
	if e != nil {
		return e
	}
	var version struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if e = json.Unmarshal(versionRaw, &version); e != nil {
		return e
	}
	if version.Data.Version == "" || strings.Contains(strings.ToLower(version.Data.Version), "fixture") {
		return errors.New("live test management requires a verified real Runner binary")
	}
	var pushed string
	var ownedHead string
	var stopOriginal func() error
	started := false
	defer func() {
		if started {
			if stopOriginal != nil {
				result = errors.Join(result, stopOriginal())
			}
			_, stopErr := d.tmCommand(out, "stop", d.binary, "stop")
			_, verifyErr := d.tmCommand(out, "verify-stop", d.binary, "verify-stop", "--runner-exe", filepath.Join(d.root, "bin", "steerrunner.exe"))
			proof.Stopped = stopErr == nil && verifyErr == nil
			if !proof.Stopped {
				result = errors.Join(result, fmt.Errorf("stop/settlement: %v; %v", stopErr, verifyErr))
			}
		}
		if pushed != "" && proof.Stopped {
			e := d.tmCleanupOwnedRefs(out, pushed, ownedHead, proof.CandidateRun)
			proof.Cleanup = e == nil
			if !proof.Cleanup {
				result = errors.Join(result, fmt.Errorf("owned base cleanup unverified: %v", e))
			}
		} else {
			proof.Cleanup = pushed == "" && proof.Stopped
		}
		if e := controlled.Write(filepath.Join(out, "proof.json"), proof); e != nil {
			result = errors.Join(result, e)
		}
		if result == nil {
			result = proof.Validate()
		}
	}()
	// This dedicated task intentionally ends before publication. The driver owns
	// the deterministic test inputs, while Codex supplies a real original Run.
	spec := "# Test result consumption fixture\n\nNo implementation or publication is requested. Do not edit, commit or push files, do not run publish.ps1, and do not report implementation success. Inspect the original Plan, then finish with a clear BLOCKED diagnosis: deterministic tests are owned by the external acceptance driver. Do not fabricate test results.\n\n## Acceptance\n\nTracked files remain unchanged; the original Codex turn honestly reports the external-driver boundary. No publication.\n"
	plan := "# Test result consumption plan\n\nThis Plan requires exactly one automatic test: fixture.marker from the catalog. The arrangement MUST be {\"tests\":[\"fixture.marker\"]}; an empty selection would omit the mandatory acceptance test even though product code is unchanged. Its actual program checks the run-owned ignored input. The external driver executes it once with passing input and once with failing input, in fresh .steerrunner/artifacts directories. No code change or publication is required.\n\n## 测试计划\n\n系统自动生成测试安排：fixture.marker, L1, isolated. Command: powershell -NoProfile -ExecutionPolicy Bypass -File ./test-marker.ps1 -Out .steerrunner/artifacts/<unique-execution>. Pass requires exit 0, a real assertion, original summary and managed report on clean unchanged commit. The second actual assertion must fail with exit 1. Evidence is retained in the original Run; external acceptance verifies API and browser counts.\n"
	for path, body := range map[string]string{"SteerlineFlow/spec/task.spec.md": spec, "SteerlineFlow/plans/task.plan.md": plan} {
		if err := os.WriteFile(filepath.Join(d.repo, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			return err
		}
	}
	if _, err := d.git("add", "--", "SteerlineFlow/spec/task.spec.md", "SteerlineFlow/plans/task.plan.md", "SteerlineFlow/plans/task.tests.json"); err != nil {
		return err
	}
	if _, err := d.git("commit", "--quiet", "-m", "Pin test management acceptance materials"); err != nil {
		return err
	}
	head, err := d.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	proof.Commit = head
	pushed = head
	if _, err = d.git("push", "origin", d.base+":refs/heads/"+d.base); err != nil {
		return err
	}
	if _, err := d.tmCommand(out, "start", d.binary, "start", "--no-open"); err != nil {
		return err
	}
	started = true
	var p *applicationclient.Project
	err = tmPoll(ctx, func() (bool, error) {
		var e error
		p, e = applicationclient.OpenProject(ctx, d.home, "")
		return e == nil, nil
	})
	if err != nil {
		return err
	}
	defer p.Close()
	id := p.Identity()
	reference := func(path string) (taskmodel.FileReference, error) {
		b, e := d.command("git", "-C", d.repo, "show", head+":"+path).Output()
		sum := sha256.Sum256(b)
		return taskmodel.FileReference{Path: path, Commit: head, SHA256: hex.EncodeToString(sum[:])}, e
	}
	specRef, err := reference("SteerlineFlow/spec/task.spec.md")
	if err != nil {
		return err
	}
	planRef, err := reference("SteerlineFlow/plans/task.plan.md")
	if err != nil {
		return err
	}
	contract := taskmodel.TaskContract{ContractID: "test-management", Repository: debugRemote, Remote: "origin", Branch: d.base, SpecRef: specRef, PlanRef: planRef, CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{specRef}}
	contract.Digest, err = taskmodel.ContractDigest(contract)
	if err != nil {
		return err
	}
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: id.RepositoryIdentity, Tasks: []taskmodel.TaskBatchTask{{LocalKey: "testing", TaskID: 90001, Title: "Test management evidence", Type: "task", Contract: contract}}, Dependencies: []taskmodel.TaskBatchEdge{}}
	raw, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	valid, err := p.ValidateTaskBatch(ctx, raw)
	if err != nil {
		return err
	}
	if !valid.Valid {
		return fmt.Errorf("invalid testing task: %+v", valid)
	}
	receipt, err := p.CreateTaskBatch(ctx, api.CreateInput{Payload: raw, RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), ExpectedAuthorityVersion: valid.AuthorityVersion, RequestedBy: "testing-driver"})
	if err != nil {
		return err
	}
	if receipt.Status != "accepted" {
		return fmt.Errorf("task rejected: %+v", receipt)
	}
	page, err := p.ListTasks(ctx, "", 100)
	if err != nil {
		return err
	}
	taskID := ""
	for _, t := range page.Tasks {
		if t.Title == "Test management evidence" {
			taskID = t.TaskID
		}
	}
	if taskID == "" {
		return errors.New("original testing task absent")
	}
	material, err := WaitFixtureMaterials(ctx, d.home, p.Identity(), "SteerlineFlow/plans/task.plan.md", out)
	if err != nil {
		return err
	}
	request := material.GenerationID
	var generation api.TestGeneration
	err = tmPoll(ctx, func() (bool, error) {
		var e error
		generation, e = p.TestGeneration(ctx, request)
		if e != nil {
			return false, e
		}
		if generation.State == "failed" || generation.State == "invalid" || generation.State == "rejected" {
			return false, fmt.Errorf("automatic generation: %+v", generation)
		}
		return generation.State == "generated" && generation.StopStatus == "stopped", nil
	})
	if err != nil {
		return err
	}
	if err = controlled.Write(filepath.Join(out, "generation.json"), generation); err != nil {
		return err
	}
	var catalog struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	raw, err = os.ReadFile(filepath.Join(d.home, "execution.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		return err
	}
	runner, _, err := catalog.Runner.Open(ctx, runnerstatepath.Directory(d.home), 30*time.Second)
	if err != nil {
		return err
	}
	status, err := runner.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	runner.InstanceID = status.InstanceID
	proof.RunnerSHA256 = catalog.Runner.SHA256
	gen, err := runner.Inspect(ctx, "run-id", generation.RunID, false)
	if err != nil {
		return err
	}
	if gen.Run == nil || gen.Run.Workflow.ID != "generate" || gen.Run.Final == nil || !gen.Run.Final.Verified || gen.Run.ContentResult == nil {
		return errors.New("real generate source unverified")
	}
	proof.GenerationRun = gen.Run.RunID
	proof.GenerationSource = gen.Run.Final.Source
	if err = controlled.Write(filepath.Join(out, "generation-run.json"), gen); err != nil {
		return err
	}
	if runnerCandidate != "" {
		if err = p.Close(); err != nil {
			return err
		}
		runner, err = d.tmGenerationTakeover(ctx, out, runnerCandidate, generation, *gen.Run, runner)
		if err != nil {
			return err
		}
		proof.RunnerSHA256 = runner.SHA256
		err = tmPoll(ctx, func() (bool, error) {
			p, err = applicationclient.OpenProject(ctx, d.home, id.ProjectID)
			return err == nil, nil
		})
		if err != nil {
			return err
		}
		defer p.Close()
		current, err := p.TestGeneration(ctx, request)
		if err != nil || current.ID != generation.ID || current.RunID != generation.RunID || current.State != "generated" || string(current.Result) != string(generation.Result) {
			return fmt.Errorf("generation changed across compatible takeover: %+v: %w", current, err)
		}
		if err = controlled.Write(filepath.Join(out, "generation-after-takeover.json"), current); err != nil {
			return err
		}
	}
	resumed, err := tmControl(ctx, p, out, "resume", api.ControlInput{Action: api.ResumeDispatch, RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), RequestedBy: "testing-driver"})
	if err != nil {
		return err
	}
	if resumed.Status != "applied" {
		return fmt.Errorf("resume rejected: %+v", resumed)
	}
	payload, err := p.StartTaskPayload(ctx, taskID, "live")
	if err != nil {
		return err
	}
	startedReceipt, err := tmControl(ctx, p, out, "start-control", api.ControlInput{Action: api.StartTask, Payload: payload, RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), RequestedBy: "testing-driver"})
	if err != nil {
		return err
	}
	if startedReceipt.Status != "applied" {
		return fmt.Errorf("start rejected: %+v", startedReceipt)
	}
	var candidate api.CandidateSummary
	err = tmPoll(ctx, func() (bool, error) {
		snap := p.ReadProject(ctx, taskID, api.ProjectSnapshot{})
		if snap.Connection.Snapshot.Page == nil {
			return false, nil
		}
		for _, c := range snap.Connection.Snapshot.Page.Runs {
			if c.RunID == startedReceipt.RunID && (c.DomainTaskID == taskID || c.TaskID == taskID) {
				if c.SubmissionStatus == "rejected" {
					_ = controlled.Write(filepath.Join(out, "rejected-candidate.json"), c)
					return false, fmt.Errorf("original candidate rejected; inspect rejected-candidate.json")
				}
				candidate = *c
				return c.ExecutionID != "", nil
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	var original runnercontract.Run
	stopOriginal = func() error {
		if original.RunID == "" {
			return nil
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer stopCancel()
		return d.tmStopOriginalRun(stopCtx, out, runner, original)
	}
	err = tmPoll(ctx, func() (bool, error) {
		v, e := runner.Inspect(ctx, "run-id", candidate.ExecutionID, false)
		if e != nil {
			return false, e
		}
		if v.Run == nil {
			return false, nil
		}
		original = *v.Run
		ownedHead = original.Execution.Branch
		proof.CandidateRun = original.RunID
		// The fixture can report unknown_external while waiting for this driver.
		// A committed implementation report with no reply/resume qualification
		// is also a collection boundary; it does not mean Task success.
		committedWait := original.Status == "reconciling" && original.Phase == "result_committed" && original.Step == "implement.report" && !original.Controls.CanReply && !original.Controls.CanResume
		// Current Runner commits an honest blocked report as a replyable input
		// boundary. There is no active execution at this boundary.
		inputWait := original.Status == "waiting_input" && original.Phase == "result_committed" && original.Controls.CanReply && !original.Controls.CanInterrupt
		finished := original.Status == "blocked" || original.Status == "failed" || original.Status == "stopped" || committedWait || inputWait
		return finished && original.Execution.Workspace != "" && original.Final != nil && original.Final.Verified, nil
	})
	if err != nil {
		return err
	}
	ownedHead = original.Execution.Branch
	proof.CandidateRun = original.RunID
	// Verify latest public worktree paths and the exact frozen prompt context.
	catalogueText, e := d.git("show", head+":SteerlineFlow/testing/test-list.json")
	if e != nil {
		return e
	}
	catalogue, e := testmanagement.ParseCatalogue([]byte(catalogueText))
	if e != nil {
		return e
	}
	definitions, e := testmanagement.Select(generation.Result, catalogue)
	if e != nil {
		return e
	}
	expectedSnapshot, e := json.Marshal(testmanagement.Snapshot{GenerationID: generation.ID, Inputs: material.Inputs, Status: "ready", PlanPath: material.Contract.PlanRef.Path, Commit: material.Commit, Definitions: definitions, Digest: testmanagement.Digest(definitions)})
	if e != nil {
		return e
	}
	sum := sha256.Sum256(expectedSnapshot)
	snapshotDigest := hex.EncodeToString(sum[:])
	testContext := testmanagementmodel.TestingContext(expectedSnapshot)
	if !strings.Contains(original.Workflow.PromptOverrides["implement.execute"], testContext) {
		return errors.New("Runner did not freeze exact system test prompt")
	}
	expectedPaths := map[string]bool{material.Contract.SpecRef.Path: false, material.Contract.PlanRef.Path: false, testmanagement.RulesPath: false, testmanagement.CataloguePath: false}
	for _, value := range original.Materials {
		if _, ok := expectedPaths[value.Path]; !ok {
			return errors.New("nonexistent system document reference")
		}
		if value.Kind != "worktree" || value.Commit != head || value.ExecutionLocation != value.Path || value.Digest != "" || value.SnapshotPath != "" {
			return errors.New("Runner worktree document semantics differ")
		}
		expectedPaths[value.Path] = true
	}
	for _, seen := range expectedPaths {
		if !seen {
			return errors.New("worktree path omitted")
		}
	}
	if _, e = os.Stat(filepath.Join(original.Execution.Workspace, "steerline-test-plan.json")); !os.IsNotExist(e) {
		return errors.New("system snapshot written to worktree")
	}

	if e = controlled.Write(filepath.Join(out, "system-snapshot-binding.json"), map[string]any{"passed": true, "generation_id": generation.ID, "inputs": material.Inputs, "snapshot_digest": snapshotDigest, "materials": original.Materials}); e != nil {
		return e
	}
	if original.PR != nil {
		return errors.New("fixture must not publish a PR")
	}
	if original.Final.Source == "" || original.Execution.Backend != "local" || original.Execution.Protocol != "local_codex_app_server_v1" {
		return errors.New("real development source unverified")
	}
	proof.CandidateRun = original.RunID
	proof.CandidateSource = original.Final.Source
	if err = controlled.Write(filepath.Join(out, "candidate-run.json"), original); err != nil {
		return err
	}
	if !pathWithin(runner.StateDir, original.Execution.Workspace) && !pathWithin(d.root, original.Execution.Workspace) {
		return errors.New("original Run workspace escapes this package and its isolated Runner namespace")
	}
	client, err := applicationclient.Open(ctx, d.home)
	if err != nil {
		return err
	}
	defer client.Close()
	for _, phase := range []string{"pass", "fail"} {
		input := "fixture-ok"
		if phase == "fail" {
			input = "fixture-fail"
		}
		artifacts := filepath.Join(original.Execution.Workspace, ".steerrunner", "artifacts")
		if err = os.MkdirAll(artifacts, 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(artifacts, "fixture-input.txt"), []byte(input), 0600); err != nil {
			return err
		}
		cmd := d.command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "./test-marker.ps1", "-Out", ".steerrunner/artifacts/execution-"+phase)
		cmd.Dir = original.Execution.Workspace
		output, runErr := cmd.CombinedOutput()
		if e := os.WriteFile(filepath.Join(out, phase+"-program.log"), output, 0600); e != nil {
			return e
		}
		if (phase == "pass" && runErr != nil) || (phase == "fail" && (cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1)) {
			return fmt.Errorf("actual %s assertion exit differs: %v: %s", phase, runErr, output)
		}
		reportRaw, e := os.ReadFile(filepath.Join(artifacts, "execution-"+phase, "managed-fixture.marker.json"))
		if e != nil {
			return e
		}
		report, e := testmanagement.ParseReport(reportRaw)
		if e != nil {
			return e
		}
		if report.BeforeCommit != head || report.AfterCommit != head || !report.BeforeClean || !report.AfterClean {
			return fmt.Errorf("%s report source not clean original commit", phase)
		}
		if phase == "pass" {
			proof.PassReport = report.ID
		} else {
			proof.FailReport = report.ID
		}
		var view testmanagement.View
		err = tmPoll(ctx, func() (bool, error) {
			view = testmanagement.View{}
			e := client.Get(ctx, "/projects/"+id.ProjectID+"/runs/"+candidate.RunID+"/tests", &view)
			if e != nil {
				return false, e
			}
			if phase == "pass" {
				return view.Passed == 1 && view.Failed == 0, nil
			}
			return view.Failed == 1 && view.Passed == 0 && len(view.Rows) == 1 && len(view.Rows[0].History) == 2, nil
		})
		if err != nil {
			_ = controlled.Write(filepath.Join(out, phase+"-api.json"), view)
			return fmt.Errorf("%s API: %w", phase, err)
		}
		if err = controlled.Write(filepath.Join(out, phase+"-api.json"), view); err != nil {
			return err
		}
		browser := d.command("npm.cmd", "exec", "--", "playwright", "test", "tests/test-management.spec.ts")
		browser.Dir = webSource
		browser.Env = append(browser.Env, "STEERLINE_WEB_CONFIG="+d.config, "STEERLINE_WEB_EVIDENCE="+filepath.Join(out, "browser-"+phase), "STEERLINE_TM_PHASE="+phase, "STEERLINE_TM_PROJECT="+id.ProjectID, "STEERLINE_TM_TASK="+taskID, "STEERLINE_TM_CANDIDATE="+candidate.RunID)
		b, e := browser.CombinedOutput()
		_ = os.WriteFile(filepath.Join(out, phase+"-browser.log"), b, 0600)
		if e != nil {
			return fmt.Errorf("browser %s: %w: %s", phase, e, b)
		}
		if phase == "pass" {
			proof.BrowserPassed = true
		} else {
			proof.BrowserFailed = true
		}
	}
	return nil
}

func (d *debugRelease) tmStopOriginalRun(ctx context.Context, out string, runner runnerclient.Client, original runnercontract.Run) error {
	// service stop closes the caller, not the Run's durable business authority.
	// Explicitly stop only this fixture's original Run before requesting settlement.
	latest, err := runner.Inspect(ctx, "run-id", original.RunID, false)
	if err != nil {
		return err
	}
	if latest.Run == nil || latest.Run.InputDigest != original.InputDigest {
		return errors.New("cleanup original Run/input differs")
	}
	body, _ := json.Marshal(map[string]uint64{"expected_revision": latest.Run.Revision})
	stop := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "tm-fixture-stop-" + original.RunID, Input: body}
	if err = controlled.Write(filepath.Join(out, "native-stop.json"), stop); err != nil {
		return err
	}
	_, stopErr := runner.Call(ctx, "stop", []string{"--run-id", original.RunID}, &stop)
	var failure *runnerclient.Failure
	if stopErr != nil && (!errors.As(stopErr, &failure) || !failure.Unknown) {
		return stopErr
	}
	stopCtx, stopCancel := context.WithTimeout(ctx, 45*time.Second)
	defer stopCancel()
	err = tmPoll(stopCtx, func() (bool, error) {
		v, e := runner.Inspect(stopCtx, "request-id", stop.RequestID, false)
		if e != nil {
			return false, e
		}
		if v.Command == nil || v.Command.RequestID != stop.RequestID || v.Command.Command != "stop" || v.Run == nil || v.Run.RunID != original.RunID || v.Run.InputDigest != original.InputDigest {
			return false, errors.New("original cleanup stop binding differs")
		}
		if e = controlled.Write(filepath.Join(out, "native-stopped.json"), v); e != nil {
			return false, e
		}
		return v.Run.StopStatus == "stopped", nil
	})
	if err != nil {
		return err
	}
	return nil
}
func (d *debugRelease) tmCommand(out, name, exe string, args ...string) ([]byte, error) {
	b, err := d.command(exe, args...).CombinedOutput()
	if e := os.WriteFile(filepath.Join(out, name+".log"), b, 0600); e != nil {
		return b, e
	}
	if err != nil {
		return b, fmt.Errorf("%s: %w: %s", name, err, b)
	}
	return b, nil
}
func tmPoll(ctx context.Context, check func() (bool, error)) error {
	for {
		done, err := check()
		if err != nil || done {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// WaitFixtureMaterials observes the product's automatic generator. It never
// supplies a result, writes an arrangement, or approves a material version.
func WaitFixtureMaterials(ctx context.Context, home string, identity httpcontract.Identity, plan, out string) (pmm.Material, error) {
	var material pmm.Material
	if err := os.MkdirAll(out, 0700); err != nil {
		return material, err
	}
	client, e := applicationclient.Open(ctx, home)
	if e != nil {
		return material, e
	}
	defer client.Close()
	body := json.RawMessage(`{}`)
	in := httpcontract.CommandEnvelope{RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "maintenance_check", TargetID: identity.ProjectID, ExpectedVersion: 1, Payload: body}
	var response httpcontract.CommandResponse
	if e = client.Post(ctx, "/projects/"+identity.ProjectID+"/commands", in, &response); e != nil {
		return material, e
	}
	if response.Status != "applied" {
		return material, fmt.Errorf("automatic material check: %+v", response)
	}
	e = tmPoll(ctx, func() (bool, error) {
		var view struct {
			Tasks []pmm.Material `json:"tasks"`
		}
		if e := client.Get(ctx, "/projects/"+identity.ProjectID+"/maintenance", &view); e != nil {
			return false, e
		}
		for _, m := range view.Tasks {
			if m.Contract.PlanRef.Path != plan {
				continue
			}
			material = m
			if m.Status == "failed" || m.Status == "invalid" || m.Status == "rejected" || m.Status == "configuration_required" {
				return false, fmt.Errorf("automatic materials: %+v", m)
			}
			return m.Status == "ready", nil
		}
		return false, nil
	})
	if e == nil {
		e = controlled.Write(filepath.Join(out, "automatic-material.json"), material)
	}
	return material, e
}
