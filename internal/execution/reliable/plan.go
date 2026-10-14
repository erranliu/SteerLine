package reliable

import (
	"context"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Next owns generic ordering. Adapter facts describe only original native
// resources; they cannot choose the next operation or retry an unknown effect.
// GateError distinguishes a permanent missing capability from missing evidence.
// Neither state grants permission to skip a required reliability goal.
type GateError struct {
	Gate   string
	Status b.PreparationGate
}

// Capability admission precedes resource creation. Applicable mechanisms may
// still have unknown native facts, which Next keeps behind their own gates.
func ValidatePreparationCapabilities(c b.PreparationCapabilities) error {
	for _, entry := range []struct {
		name  string
		value b.Applicability
	}{{"workspace", c.Workspace}, {"agent_resource", c.Resource}, {"session", c.Session}} {
		switch entry.value {
		case b.MechanismApplicable, b.MechanismNotApplicable:
		case b.MechanismUnsupported:
			return &GateError{Gate: entry.name, Status: b.GateUnsupported}
		default:
			return &GateError{Gate: entry.name, Status: b.GateUnverified}
		}
	}
	return nil
}

const PreparationStepLimit = 16
const ReconciliationStepLimit = 8
const CleanupStepLimit = 8

func (e *GateError) Error() string { return "preparation " + e.Gate + " is " + string(e.Status) }
func gate(name string, decl b.Applicability, fact b.PreparationGate) (bool, error) {
	switch decl {
	case b.MechanismNotApplicable:
		if fact != b.GateNotApplicable {
			return false, &GateError{Gate: name, Status: b.GateUnverified}
		}
		return false, nil
	case b.MechanismUnsupported:
		return false, &GateError{Gate: name, Status: b.GateUnsupported}
	case b.MechanismApplicable:
		switch fact {
		case b.GateReady:
			return false, nil
		case b.GateRequired:
			return true, nil
		case b.GateUnsupported:
			return false, &GateError{Gate: name, Status: b.GateUnsupported}
		}
	}
	return false, &GateError{Gate: name, Status: b.GateUnverified}
}
func Next(ctx context.Context, port b.LifecyclePreparationPort, snapshot b.LifecycleSnapshot) (b.LifecycleCandidate, error) {
	if candidate, found, err := UnresolvedCandidate(snapshot); found || err != nil {
		return candidate, err
	}
	if err := ValidatePreparationCapabilities(port.PolicyCapabilities().Preparation); err != nil {
		return b.LifecycleCandidate{}, err
	}
	facts, err := port.PreparationFacts(ctx, snapshot.Clone())
	if err != nil {
		return b.LifecycleCandidate{}, err
	}
	caps := port.PolicyCapabilities().Preparation
	workspace, err := gate("workspace", caps.Workspace, facts.Workspace)
	if err != nil {
		return b.LifecycleCandidate{}, err
	}
	if workspace {
		intent := snapshot.WorkspacePlan
		candidate := b.LifecycleCandidate{Action: b.LifecyclePrepareWorkspace, Input: b.LifecycleInput{Workspace: &intent}}
		return candidate, candidate.Validate()
	}
	resource, err := gate("agent_resource", caps.Resource, facts.Resource)
	if err != nil {
		return b.LifecycleCandidate{}, err
	}
	if resource {
		if facts.ResourceRequired == nil {
			return b.LifecycleCandidate{}, errors.New("required native resource has no fixed intent")
		}
		candidate := b.LifecycleCandidate{Action: b.LifecycleCreateResource, Input: b.LifecycleInput{Resource: facts.ResourceRequired}}
		return candidate, candidate.Validate()
	}
	if facts.Cleanup != nil {
		candidate := b.LifecycleCandidate{Action: b.LifecycleCleanup, Input: b.LifecycleInput{Cleanup: facts.Cleanup}}
		return candidate, candidate.Validate()
	}
	if snapshot.PendingTurn == nil {
		return b.LifecycleCandidate{Done: true}, nil
	}
	session, err := gate("session", caps.Session, facts.Session)
	if err != nil {
		return b.LifecycleCandidate{}, err
	}
	if snapshot.PendingTurn.CommandID == "" || snapshot.PendingTurn.Prompt == "" {
		return b.LifecycleCandidate{}, errors.New("lifecycle has no saved turn input")
	}
	action := b.LifecycleStartTurn
	if session {
		action = b.LifecyclePrepareSession
	}
	candidate := b.LifecycleCandidate{Action: action, Input: b.LifecycleInput{Turn: snapshot.Clone().PendingTurn}}
	return candidate, candidate.Validate()
}

func KnownAction(action b.LifecycleAction) bool {
	switch action {
	case b.LifecyclePrepareWorkspace, b.LifecycleCreateResource, b.LifecyclePrepareSession, b.LifecycleStartTurn, b.LifecycleSubmitInput, b.LifecycleReportCompletion, b.LifecycleDeliverFeedback, b.LifecycleAcknowledge, b.LifecycleStopObservation, b.LifecycleStopExecution, b.LifecycleCleanup, b.LifecycleHostAction:
		return true
	}
	return false
}
func UnresolvedCandidate(snapshot b.LifecycleSnapshot) (b.LifecycleCandidate, bool, error) {
	for _, operation := range snapshot.Operations {
		if operation.Status != b.OperationInFlight && operation.Status != b.OperationUnknown {
			continue
		}
		candidate, err := b.CandidateForOperation(operation)
		return candidate, err == nil, err
	}
	return b.LifecycleCandidate{}, false, nil
}
