package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type neutralOutputResolver struct{ changed bool }

type neutralOutputPort struct {
	executionbackend.LifecyclePort
	changed bool
}

func (r *neutralOutputResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return neutralOutputPort{LifecyclePort: &neutralLifecycle{setup: setup.Clone()}, changed: r.changed}, nil
}

func (p neutralOutputPort) PreparationBudget() time.Duration { return time.Minute }

func (p neutralOutputPort) PlatformVersion(context.Context) (string, error) {
	return "fixture-version-1", nil
}

func (p neutralOutputPort) ObserveHost(context.Context, executionbackend.ProcessIdentity) (executionbackend.HostFact, error) {
	return executionbackend.HostFact{}, nil
}

func (p neutralOutputPort) InspectAgentInput(context.Context, executionbackend.ProcessIdentity) (executionbackend.HostInputObservation, error) {
	return executionbackend.HostInputObservation{}, errors.New("source fixture accepts no input")
}

func (p neutralOutputPort) CurrentTurn(s executionbackend.LifecycleSnapshot) (*executionbackend.HostTurnFacts, error) {
	return &executionbackend.HostTurnFacts{TurnID: s.Session.CurrentTurnID}, nil
}

func (p neutralOutputPort) ResetTurnOutput(executionbackend.LifecycleSnapshot, string) (executionbackend.ImplementationDataEnvelope, error) {
	return executionbackend.ImplementationDataEnvelope{}, errors.New("source fixture cannot reset output")
}

func (p neutralOutputPort) PrepareProbe(executionbackend.ProbeIdentity) (executionbackend.ProbePlan, error) {
	return executionbackend.ProbePlan{}, errors.New("source fixture has no probe")
}

func (p neutralOutputPort) ProbePrompt(executionbackend.ProbePlan, string, string) string { return "" }

func (p neutralOutputPort) ReadProbe(executionbackend.ProbeIdentity, string) (*executionbackend.ProbeReceipt, error) {
	return nil, errors.New("source fixture has no probe")
}

func (p neutralOutputPort) ReadWorkerOutput(_ context.Context, s executionbackend.LifecycleSnapshot, _ executionbackend.HostReadInput) (executionbackend.HostOutput, executionbackend.ImplementationDataEnvelope, error) {
	source, cursor := "buffer", "original:4"
	if p.changed {
		source, cursor = "transcript", "replacement:7"
	}
	output := executionbackend.HostOutput{Source: source, Cursor: cursor, Content: "original complete content", ContentComplete: true, ContentChanged: true, SourceChanged: p.changed, UpdateAttemptCursor: true}
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: "fixture.output", CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: s.Session.ProjectID, ExecutionID: s.Session.ExecutionID, AttemptID: s.Session.AttemptID, Generation: s.ResourceGeneration, ConfigDigest: s.ConfigurationDigest}, mustJSON(struct {
		Source, Cursor string
		Complete       bool
	}{source, cursor, true}))
	return output, envelope, err
}

func TestCD64SourceReplacementRetainsEvidence(t *testing.T) {
	s, run := neutralAuditService(t)
	resolver := &neutralOutputResolver{}
	s.lifecycleResolver = resolver
	originalAttempt := run.AttemptID
	decisions, operations := len(s.domain.Decisions), len(s.domain.Operations)
	require(t, s.readHostOutput(context.Background(), run))
	require(t, s.commitWorker(run))
	before, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.Identity())
	require(t, err)
	first := snapshotAttempt(&before, originalAttempt)
	if first == nil || first.OutputSource != "buffer" || first.OutputCursor != "original:4" {
		t.Fatal("original source cursor was not committed")
	}
	resolver.changed = true
	require(t, s.readHostOutput(context.Background(), run))
	require(t, s.commitWorker(run))
	snapshot, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.Identity())
	require(t, err)
	attempt := snapshotAttempt(&snapshot, originalAttempt)
	if run.AttemptID != originalAttempt || attempt == nil || attempt.OutputSource != "transcript" || attempt.OutputCursor != "replacement:7" || len(snapshot.Decisions) != decisions || len(snapshot.Operations) != operations {
		t.Fatal("source replacement lost committed original Attempt evidence or created business control")
	}
}
