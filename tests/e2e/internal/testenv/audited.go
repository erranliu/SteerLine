package testenv

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// The live gate consumes exactly the public product endpoint used by clients.
// It does not replace the product query by reading a Runner-owned database.
func (p *Project) observeAuditedProduct(expected runnerstate.Candidate, final bool) {
	s := p.Scenario
	client, err := applicationclient.OpenProject(s.Context, p.Home, "")
	s.Must(err)
	defer client.Close()
	snapshot := client.ReadProject(s.Context, p.Tasks["a"].ID, api.ProjectSnapshot{})
	page := snapshot.Connection.Snapshot.Page
	if page == nil || len(page.Runs) == 0 {
		if final {
			s.t.Fatal("public product candidate missing", snapshot)
		}
		return
	}
	var run *api.CandidateSummary
	for _, candidate := range page.Runs {
		if candidate.TaskID == p.Tasks["a"].ID {
			run = candidate
			break
		}
	}
	if run == nil {
		if final {
			s.t.Fatal("product candidate identity missing")
		}
		return
	}
	if run.RunnerWorkflow == nil {
		if final {
			s.t.Fatal("product Runner workflow missing")
		}
		return
	}
	if run.RunnerWorkflow.ID != "development-audited" || run.RunnerWorkflow.Version != 2 || run.WorkflowVersion != "2" || run.ExecutionProfile == nil || run.ExecutionProfile.Name != "Real E2E audited preset" {
		s.t.Fatal("product preset/workflow differs", run)
	}
	if run.ExecutionProfile.ExecutionModel != s.Run.ExecutionModel || run.ExecutionProfile.ExecutionEffort != "medium" || run.ExecutionProfile.AuditModel != s.Run.AuditModel || run.ExecutionProfile.AuditEffort != "high" {
		s.t.Fatal("frozen request models differ", run.ExecutionProfile)
	}
	if run.ExecutionID == "" || run.RunnerRevision == 0 || run.InputDigest == "" {
		s.t.Fatal("public Runner binding missing", run)
	}
	if run.RunID != expected.ID || run.ExecutionID != expected.RunID || expected.InputDigest != "" && run.InputDigest != expected.InputDigest || expected.Projection != nil && run.RunnerRevision < expected.Projection.Revision {
		s.t.Fatal("public detail differs from original candidate/Run/revision", run, expected.ID, expected.RunID)
	}
	for _, step := range []string{"implement.execute", "audit.code.execute", "publish.execute"} {
		if run.WorkflowStage != step || run.RequestedModel == "" {
			continue
		}
		model, effort := s.Run.ExecutionModel, "medium"
		if strings.HasPrefix(step, "audit.") {
			model, effort = s.Run.AuditModel, "high"
		}
		if run.RequestedModel != model || run.RequestedReasoningEffort != effort {
			s.t.Fatal("public current-turn model differs", step, run.RequestedModel, run.RequestedReasoningEffort)
		}
		p.AuditObservations[step] = model
	}
	contract := p.Tasks["a"].Contract
	matched := 0
	for _, ref := range []struct{ path, commit, digest string }{{contract.SpecRef.Path, contract.SpecRef.Commit, contract.SpecRef.SHA256}, {contract.PlanRef.Path, contract.PlanRef.Commit, contract.PlanRef.SHA256}} {
		for _, material := range run.Materials {
			if material.Kind == "worktree" && material.Path == ref.path && material.Commit == ref.commit && material.Digest == "" {
				// v2 exposes worktree paths, not copied Git documents. Verify
				// the frozen caller hash against the actual original Git bytes.
				if final {
					body, err := s.RunCommand(p.Command("git", "-C", p.Repo, "show", ref.commit+":"+ref.path), "audited-material-"+filepath.Base(ref.path))
					s.Must(err)
					if testmanagement.ContentDigest(body) != ref.digest {
						s.t.Fatal("original committed material hash differs", ref.path)
					}
				}
				matched++
				break
			}
		}
	}
	if matched == 2 {
		p.AuditObservations["materials"] = "verified"
	} else if final {
		s.t.Fatal("product materials did not preserve source/hash", run.Materials)
	}
	if final {
		if run.Audit == nil || run.Audit.Model != s.Run.AuditModel || run.Audit.ReasoningEffort != "high" {
			s.t.Fatal("product audit model missing", run.Audit)
		}
		if len(run.Audit.Results) != 1 {
			s.t.Fatal("one v2 code audit required", run.Audit)
		}
		r := run.Audit.Results[0]
		src := r.ImplementationSource
		if r.Step != "audit.code.execute" || r.Failure != "" || r.CommandID == "" || r.TurnID == "" || r.SourceResultID == "" || r.ResultID == "" || src.CommandID == "" || src.TurnID == "" || src.AttemptID == "" || src.InputVersion == 0 || src.FinalDigest == "" || r.PreHead == "" || r.PostHead == "" || r.FixedCount == nil || *r.FixedCount != len(r.FixedIssues) || len(r.RemainingIssues) != 0 {
			s.t.Fatal("v2 code audit result/source incomplete", r)
		}
		p.AuditObservations["audit-results"] = "verified"
		cli := runnerclient.Client{Executable: s.Run.Runner, SHA256: expected.RunnerBuild.SHA256, StateDir: expected.StateDir}
		comparison, err := cli.Inspect(s.Context, "run-id", expected.RunID, false)
		s.Must(err)
		if comparison.Run == nil || comparison.Run.InputDigest != run.InputDigest || comparison.Run.Workflow.ID != run.RunnerWorkflow.ID || comparison.Run.Audit == nil || comparison.Run.Audit.Model != run.Audit.Model {
			s.t.Fatal("Runner comparison differs from public product", comparison)
		}
		originalAudit, err := json.Marshal(comparison.Run.Audit)
		s.Must(err)
		publicAudit, err := json.Marshal(run.Audit)
		s.Must(err)
		if string(originalAudit) != string(publicAudit) {
			s.t.Fatal("public audit differs from original Runner observation", string(originalAudit), string(publicAudit))
		}
		s.Must(controlled.Write(filepath.Join(s.Output, "audited-runner-comparison.json"), comparison))
		task, err := client.ShowTask(s.Context, p.Tasks["a"].ID)
		s.Must(err)
		if len(task.Tasks) != 1 {
			s.t.Fatal("public task detail missing")
		}
		for _, history := range task.Tasks[0].WorkflowRuns {
			if history.WorkflowRunID == run.RunID && history.ExecutionID == run.ExecutionID && history.FlowRef == "development-audited" && history.WorkflowVersion == 2 && history.ExecutionProfile != nil && history.ExecutionProfile.Name == run.ExecutionProfile.Name {
				p.AuditObservations["history"] = "verified"
			}
		}
		s.Must(controlled.Write(filepath.Join(s.Output, "audited-product-task-history.json"), task))
	}
	// Keep a response for each step rather than an unbounded poll log.
	if run.WorkflowStage != "" {
		s.Must(controlled.Write(filepath.Join(s.Output, fmt.Sprintf("audited-product-%s.json", run.WorkflowStage)), snapshot))
	}
}
