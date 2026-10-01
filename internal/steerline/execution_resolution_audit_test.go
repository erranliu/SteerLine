package steerline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestResolutionAuditProfilesDoNotAliasProjectValues(t *testing.T) {
	c := workerContextConfigFixture()
	c.requirementsEvaluation = &executionapi.RequirementsEvaluation{Tools: []executionapi.RequirementEvidence{{Source: "original"}}}
	before := c.PR.Query.Args[0]
	p := c.AvailableExecutionProfiles()[0]
	p.Publication.Provider.Query.Args[0] = "changed"
	p.RequirementsEvaluation.Tools[0].Source = "changed"
	if c.PR.Query.Args[0] != before || c.requirementsEvaluation.Tools[0].Source != "original" {
		t.Fatal("returned profile aliases project configuration")
	}
}

func TestResolutionAuditRegistrationRejectsBaseTargetDrift(t *testing.T) {
	c := Config{Host: "orca_supervised", Agent: "codex"}
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "model", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}, &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "original", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1})
	target := targetFor(c)
	run := &Run{Host: c.Host, Agent: c.Agent, Backend: "orca", Protocol: string(executionapi.ProtocolOrcaSupervised), Target: target}
	record := &ExecutionRecord{Agent: c.Agent, Backend: run.Backend, Protocol: run.Protocol, Target: target}
	for _, field := range []string{"connection", "runtime"} {
		r, e := *run, *record
		if field == "connection" {
			r.Target.ConnectionID = "other"
			e.Target.ConnectionID = "other"
		} else {
			r.Target.ExternalRuntime = "other"
			e.Target.ExternalRuntime = "other"
		}
		if _, err := workerFlowRegistrationFor(c, &r, &e); err == nil {
			t.Fatalf("unchanged target ID hid %s drift", field)
		}
	}
}

func TestResolutionAuditRestoresOriginalWorkspaceRoot(t *testing.T) {
	s, run := appServerAuditService(t)
	want := filepath.Dir(run.WorkspaceIntent.Path)
	s.baseConfig.WorktreeRoot = filepath.Join(t.TempDir(), "new-root")
	c, err := s.executionConfigForRun(run)
	require(t, err)
	if canonical(c.WorktreeRoot) != canonical(want) {
		t.Fatalf("restored execution used new worktree root: %s", c.WorktreeRoot)
	}
	require(t, prepareWorktreeOnly(context.Background(), c, run))
	if _, err = os.Stat(run.WorkspaceIntent.Path); err != nil {
		t.Fatal("original workspace was not prepared", err)
	}
}

func TestResolutionAuditRequirementsDoNotBlockOriginalObservation(t *testing.T) {
	tokenEnv := "STEERLINE_AUDIT_TOKEN"
	t.Setenv(tokenEnv, "reference-present")
	r := executionapi.ExecutionRequirements{Credentials: []executionapi.CredentialRequirement{{ID: "pr.token_env", Site: "service"}}}
	s, run := appServerAuditServiceWithRequirements(t, workspacePolicyFixture(), r, tokenEnv)
	t.Setenv(tokenEnv, "")
	entry, err := s.workerFlowRegistration(run)
	require(t, err)
	copy := *entry
	observed := false
	copy.Resume = func(_ *Service, _ context.Context, _ *TaskRecord, _ *Run) (bool, error) {
		observed = true
		return true, nil
	}
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	_, err = s.resumeRegisteredWorkerFlow(context.Background(), task, run, &copy)
	require(t, err)
	if !observed {
		t.Fatal("missing reference suppressed original observer")
	}
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	fixed.execution.Status = "active"
	if err = s.checkCurrentExecutionRequirements(context.Background(), fixed); err == nil {
		t.Fatal("missing reference did not block new input")
	}
	if fixed.execution.Status != "active" {
		t.Fatal("requirement check overwrote original native liveness")
	}
	before := len(s.domain.Operations)
	if _, err = s.startWorkerOperation(run, "orca_native_task", "audit-new-intent", struct{}{}); err == nil {
		t.Fatal("missing reference allowed new native intent")
	}
	if len(s.domain.Operations) != before {
		t.Fatal("rejected requirement created intent")
	}
}

func TestResolutionAuditEvidenceCannotChangeUnderOriginalProfileDigest(t *testing.T) {
	c := workerContextConfigFixture()
	c.Repository = t.TempDir()
	c.ExecutionRequirements.Tools = []executionapi.ToolRequirement{{ID: "git"}}
	e, err := evaluateExecutionRequirements(context.Background(), c)
	require(t, err)
	c.requirementsEvaluation = &e
	fixed := c.projectExecutionProfile().Freeze()
	changed := fixed.RequirementsEvaluation.Clone()
	changed.CheckedAt = changed.CheckedAt.Add(time.Second)
	fixed.RequirementsEvaluation = &changed
	fixed.RequirementsEvidenceDigest = requirementsEvidenceDigest(&changed)
	if verifyFixedExecutionProfile(fixed) {
		t.Fatal("evidence replacement retained original full profile digest")
	}
}

func TestResolutionAuditWorkerCommitCannotReplaceSnapshot(t *testing.T) {
	s, run := appServerAuditService(t)
	before, err := s.domainStore.Query(context.Background(), s.Config.Identity())
	require(t, err)
	workflow := s.workflowRecord(run)
	workflow.ExecutionProfile.ImplementationData[0].Payload[0] = '['
	if err = s.commitWorker(run); err == nil {
		t.Fatal("worker commit replaced original snapshot")
	}
	after, err := s.domainStore.Query(context.Background(), s.Config.Identity())
	require(t, err)
	if digestValue(before.WorkflowRuns) != digestValue(after.WorkflowRuns) {
		t.Fatal("rejected snapshot change mutated database")
	}
}
