// Package reliable owns execution policy independently of business workflows.
// Values are transient projections; the actor owns the only durable write.
package reliable

import (
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

var ErrStale = b.ErrTurnStale

// AcceptedResult is a first-wins comparison of the already settled original
// result. Native provenance is never reinterpreted by business routing.
func AcceptedResult(previous *b.TurnEvidence, incoming b.TurnEvidence) (bool, error) {
	if previous == nil {
		return false, nil
	}
	p := *previous
	if p.TurnID != incoming.TurnID || p.SessionID != incoming.SessionID || p.LastMessage != incoming.LastMessage || p.Succeeded != incoming.Succeeded || p.FinalSource != incoming.FinalSource || p.FinalItemID != incoming.FinalItemID || p.FinalVerified != incoming.FinalVerified || p.FinalNativeSessionID != incoming.FinalNativeSessionID || p.FinalNativeTurnID != incoming.FinalNativeTurnID || p.Failure != incoming.Failure || p.FinalDiagnostic != incoming.FinalDiagnostic || p.NativeOutcome != incoming.NativeOutcome {
		return false, errors.New("conflicting duplicate stage result")
	}
	return true, nil
}

func VerifiedContent(verified bool, source, item string) bool {
	return verified && source != "" && item != ""
}

// Result separates native termination, available content and session eligibility.
// Workflow consumes these facts without receiving adapter envelopes or receipts.
type Result struct {
	CommandID, TurnID, SessionID                string
	Succeeded                                   bool
	NativeOutcome, Content, Failure, Diagnostic string
	ContentVerified                             bool
}

func NormalizeResult(session b.SessionContext, evidence b.TurnEvidence) (Result, error) {
	if session.CurrentCommandID != evidence.CommandID || session.CurrentTurnID != evidence.TurnID || session.SessionID != evidence.SessionID || !evidence.Started || !evidence.Completed {
		return Result{}, ErrStale
	}
	outcome := evidence.NativeOutcome
	if outcome == "" {
		outcome = "failed"
		if evidence.Succeeded {
			outcome = "succeeded"
		}
	}
	switch outcome {
	case "succeeded", "failed", "cancelled", "unknown":
	default:
		return Result{}, errors.New("invalid normalized native outcome")
	}
	return Result{CommandID: evidence.CommandID, TurnID: evidence.TurnID, SessionID: evidence.SessionID, Succeeded: evidence.Succeeded, NativeOutcome: outcome, Content: evidence.LastMessage, Failure: evidence.Failure, Diagnostic: evidence.FinalDiagnostic, ContentVerified: evidence.FinalVerified && evidence.FinalSource != "" && evidence.FinalItemID != ""}, nil
}

type ContinuationConstraints struct {
	RecoveryRequired, PriorOwner, QuestionPending, OutcomeUnknown, EnvironmentBlocked, UnconfirmedAnswer bool
}

func (c ContinuationConstraints) RequiresAuthorization() bool {
	return c.RecoveryRequired || c.PriorOwner || c.QuestionPending || c.OutcomeUnknown || c.EnvironmentBlocked || c.UnconfirmedAnswer
}
