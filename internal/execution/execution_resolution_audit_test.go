package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestResolutionAuditProfilesDoNotAliasProjectValues(t *testing.T) {
	c := workerContextConfigFixture()

	before := c.PR.Query.Args[0]
	catalog := testProfileCatalogFor(c)
	catalog.Base = c
	p := catalog.AvailableExecutionProfiles()[0]

	p.Publication.Provider.Query.Args[0] = "changed"
	if c.PR.Query.Args[0] != before {
		t.Fatal("returned profile aliases project configuration")
	}
}

func TestResolutionAuditRegistrationRejectsBaseTargetDrift(t *testing.T) {
	c := withNeutralProfile(Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}, neutralSettings{Executable: "fixture-agent", Target: executionbackend.ExecutionTarget{ID: "fixture:peer", ConnectionID: "peer", ExternalRuntime: "original"}})
	dependencies, _ := neutralFixtureRuntime(c)
	target := TargetFor(c, dependencies.Directory)
	run := &reconstructiontypes.RunView{Host: c.Host, Agent: c.Agent, Backend: "fixture", Protocol: string(neutralProtocol), Target: target}
	record := &executionmodel.ExecutionRecord{Agent: c.Agent, Backend: run.Backend, Protocol: run.Protocol, Target: target}
	for _, field := range []string{"connection", "runtime"} {
		r, e := *run, *record
		if field == "connection" {
			r.Target.ConnectionID = "other"
			e.Target.ConnectionID = "other"
		} else {
			r.Target.ExternalRuntime = "other"
			e.Target.ExternalRuntime = "other"
		}
		if _, err := workerFlowRegistrationFor(c, dependencies.Directory, &r, &e); err == nil {
			t.Fatalf("unchanged target ID hid %s drift", field)
		}
	}
}

func TestResolutionAuditRestoresOriginalWorkspaceRoot(t *testing.T) {
	s, run := neutralAuditService(t)
	want := filepath.Dir(run.WorkspaceIntent.Path)
	s.baseConfig.WorktreeRoot = filepath.Join(t.TempDir(), "new-root")
	c, err := s.executionConfigForRun(run)
	require(t, err)
	if canonical(c.WorktreeRoot) != canonical(want) {
		t.Fatalf("restored execution used new worktree root: %s", c.WorktreeRoot)
	}
	require(t, prepareWorktreeOnly(context.Background(), c, s.runtime.Directory, s.runtime.ProcessHost, run))
	if _, err = os.Stat(run.WorkspaceIntent.Path); err != nil {
		t.Fatal("original workspace was not prepared", err)
	}
}

func TestResolutionAuditWorkerCommitCannotReplaceSnapshot(t *testing.T) {
	s, run := neutralAuditService(t)
	before, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.Identity())
	require(t, err)
	workflow := s.workflowRecord(run)
	workflow.ExecutionProfile.ImplementationData[0].Payload[0] = '['
	if err = s.commitWorker(run); err == nil {
		t.Fatal("worker commit replaced original snapshot")
	}
	after, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.Identity())
	require(t, err)
	if digestValue(before.WorkflowRuns) != digestValue(after.WorkflowRuns) {
		t.Fatal("rejected snapshot change mutated database")
	}
}
