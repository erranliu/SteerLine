package reliable

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"slices"
)

func QualifyAction(ctx context.Context, port b.LifecyclePreparationPort, request b.LifecycleRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	action := b.LifecycleAction(request.Operation.Action)
	if action != b.LifecyclePrepareWorkspace && action != b.LifecycleCreateResource && action != b.LifecyclePrepareSession && action != b.LifecycleStartTurn && action != b.LifecycleSubmitInput {
		return nil
	}
	if err := ValidatePreparationCapabilities(port.PolicyCapabilities().Preparation); err != nil {
		return err
	}
	facts, err := port.PreparationFacts(ctx, request.Snapshot.Clone())
	if err != nil {
		return err
	}
	caps := port.PolicyCapabilities().Preparation
	workspace, err := gate("workspace", caps.Workspace, facts.Workspace)
	if err != nil {
		return err
	}
	if action == b.LifecyclePrepareWorkspace {
		if !workspace || request.Input.Workspace == nil || *request.Input.Workspace != request.Snapshot.WorkspacePlan {
			return errors.New("workspace preparation differs from the required original intent")
		}
		return nil
	}
	if workspace {
		return &GateError{Gate: "workspace", Status: b.GateRequired}
	}
	resource, err := gate("agent_resource", caps.Resource, facts.Resource)
	if err != nil {
		return err
	}
	if action == b.LifecycleCreateResource {
		if !resource || facts.ResourceRequired == nil || request.Input.Resource == nil || *facts.ResourceRequired != *request.Input.Resource {
			return errors.New("agent preparation differs from the required original resource")
		}
		return nil
	}
	if resource {
		return &GateError{Gate: "agent_resource", Status: b.GateRequired}
	}
	session, err := gate("session", caps.Session, facts.Session)
	if err != nil {
		return err
	}
	if request.Input.Turn == nil || !SameInput(request.Snapshot.PendingTurn, *request.Input.Turn) {
		return errors.New("native turn action differs from the saved pending input")
	}
	if err := QualifyInput(request.Input.Turn, port.PolicyCapabilities()); err != nil {
		return err
	}
	if action == b.LifecyclePrepareSession {
		if !session {
			return errors.New("session preparation has no required original mechanism")
		}
		return nil
	}
	if session {
		return &GateError{Gate: "session", Status: b.GateRequired}
	}
	return nil
}

func QualifyInput(input *b.TurnInput, caps b.LifecyclePolicyCapabilities) error {
	if input == nil {
		return errors.New("saved turn input is missing")
	}
	if input.ReportToolPolicy != "" && caps.ReportToolPolicy != "" && input.ReportToolPolicy != caps.ReportToolPolicy {
		return errors.New("report native policy changed after its input was frozen")
	}
	return nil
}
func SameInput(candidate *b.TurnInput, input b.TurnInput) bool {
	return candidate != nil && candidate.CommandID == input.CommandID && candidate.Prompt == input.Prompt && candidate.Purpose == input.Purpose && candidate.StepID == input.StepID && candidate.InputVersion == input.InputVersion && candidate.ReportToolPolicy == input.ReportToolPolicy && slices.Equal(candidate.LinkedWorktreeArgs, input.LinkedWorktreeArgs)
}

type SavedIntent struct {
	Request       []byte
	PayloadDigest string
}

func ValidateSavedIntent(saved *SavedIntent, action b.LifecycleAction, input b.LifecycleInput) error {
	if saved == nil {
		return errors.New("lifecycle operation is missing")
	}
	value, err := InputValue(action, input)
	if err != nil {
		return err
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(saved.Request, wire) || saved.PayloadDigest != Digest(wire) {
		return errors.New("lifecycle operation input differs from its original durable intent")
	}
	return nil
}
func Digest(wire []byte) string { value := sha256.Sum256(wire); return hex.EncodeToString(value[:]) }
