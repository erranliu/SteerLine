package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"testing"
	"time"
)

type neutralOutputResolver struct{ changed bool }
type neutralOutputPort struct {
	executionapi.LifecyclePort
	changed bool
}

func (r *neutralOutputResolver) ResolveLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	return neutralOutputPort{LifecyclePort: &neutralLifecycle{setup: setup.Clone()}, changed: r.changed}, nil
}
func (p neutralOutputPort) PreparationBudget() time.Duration { return time.Minute }
func (p neutralOutputPort) PlatformVersion(context.Context) (string, error) {
	return "fixture-version-1", nil
}
func (p neutralOutputPort) ObserveHost(context.Context, executionapi.ProcessIdentity) (contracts.HostFact, error) {
	return contracts.HostFact{}, nil
}
func (p neutralOutputPort) InspectAgentInput(context.Context, executionapi.ProcessIdentity) (contracts.HostInputObservation, error) {
	return contracts.HostInputObservation{}, errors.New("source fixture accepts no input")
}
func (p neutralOutputPort) CurrentTurn(s executionapi.LifecycleSnapshot) (*contracts.HostTurnFacts, error) {
	return &contracts.HostTurnFacts{TurnID: s.Session.CurrentTurnID}, nil
}
func (p neutralOutputPort) ResetTurnOutput(executionapi.LifecycleSnapshot, string) (executionapi.ImplementationDataEnvelope, error) {
	return executionapi.ImplementationDataEnvelope{}, errors.New("source fixture cannot reset output")
}
func (p neutralOutputPort) PrepareProbe(contracts.ProbeIdentity) (contracts.ProbePlan, error) {
	return contracts.ProbePlan{}, errors.New("source fixture has no probe")
}
func (p neutralOutputPort) ProbePrompt(contracts.ProbePlan, string, string) string { return "" }
func (p neutralOutputPort) ReadProbe(contracts.ProbeIdentity, string) (*contracts.ProbeReceipt, error) {
	return nil, errors.New("source fixture has no probe")
}
func (p neutralOutputPort) ReadWorkerOutput(_ context.Context, s executionapi.LifecycleSnapshot, _ contracts.HostReadInput) (contracts.HostOutput, executionapi.ImplementationDataEnvelope, error) {
	source, cursor := "buffer", "original:4"
	if p.changed {
		source, cursor = "transcript", "replacement:7"
	}
	output := contracts.HostOutput{Source: source, Cursor: cursor, Content: "original complete content", ContentComplete: true, ContentChanged: true, SourceChanged: p.changed, UpdateAttemptCursor: true}
	envelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: "fixture.output", CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose, ProjectID: s.Session.ProjectID, ExecutionID: s.Session.ExecutionID, AttemptID: s.Session.AttemptID, Generation: s.ResourceGeneration, ConfigDigest: s.ConfigurationDigest}, mustJSON(struct {
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
	before, err := s.domainStore.Query(context.Background(), s.Config.Identity())
	require(t, err)
	first := snapshotAttempt(&before, originalAttempt)
	if first == nil || first.OutputSource != "buffer" || first.OutputCursor != "original:4" {
		t.Fatal("original source cursor was not committed")
	}
	resolver.changed = true
	require(t, s.readHostOutput(context.Background(), run))
	require(t, s.commitWorker(run))
	snapshot, err := s.domainStore.Query(context.Background(), s.Config.Identity())
	require(t, err)
	attempt := snapshotAttempt(&snapshot, originalAttempt)
	if run.AttemptID != originalAttempt || attempt == nil || attempt.OutputSource != "transcript" || attempt.OutputCursor != "replacement:7" || len(snapshot.Decisions) != decisions || len(snapshot.Operations) != operations {
		t.Fatal("source replacement lost committed original Attempt evidence or created business control")
	}
}
