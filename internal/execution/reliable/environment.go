package reliable

import (
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"time"
)

type PreparationControl struct {
	Paused, EffectPending, Blocked, RecoveryRequired, Verified, Invalidated bool
}
type PreparationProgress string

const (
	PreparationWait    PreparationProgress = "wait"
	PreparationRecover PreparationProgress = "recover"
	PreparationReady   PreparationProgress = "ready"
	PreparationVerify  PreparationProgress = "verify"
)

func PreparationState(c PreparationControl) PreparationProgress {
	if c.Paused || c.EffectPending || c.Blocked {
		return PreparationWait
	}
	if c.RecoveryRequired {
		return PreparationRecover
	}
	if c.Verified && !c.Invalidated {
		return PreparationReady
	}
	return PreparationVerify
}
func EnvironmentDeadline(verified bool, deadline time.Time, preparationTurn string, turn *b.HostTurnFacts, currentTurn string, at time.Time) *b.HostFact {
	if !verified && !at.Before(deadline) {
		return &b.HostFact{Reason: "preparation_unverified", Confidence: "unverified", Source: "preparation.deadline", Turn: preparationTurn}
	}
	if turn != nil && turn.Purpose == "business" && !turn.Settled && !turn.HasDoneMessage && currentTurn != turn.TurnID && !at.Before(turn.Deadline) {
		return &b.HostFact{Reason: "business_start_unverified", Confidence: "unverified", Source: "dispatch.intent.deadline", Turn: turn.TurnID}
	}
	return nil
}
func AcceptHostFact(fact b.HostFact, turnID string, process *b.ProcessIdentity) bool {
	if fact.Turn != "" && fact.Turn != turnID {
		return false
	}
	return fact.Runtime == "" || process == nil || fact.Runtime == process.RuntimeID && fact.Terminal == process.Terminal && fact.Incarnation == process.Incarnation || fact.Reason == "identity_mismatch"
}

type BlockerRevision struct {
	TurnID  string
	Fact    b.HostFact
	Version uint64
}

func BlockerChange(prior *BlockerRevision, turn string, fact b.HostFact) (BlockerRevision, bool) {
	if prior == nil || prior.TurnID != turn || prior.Fact.Reason != fact.Reason || prior.Fact.Source != fact.Source {
		return BlockerRevision{TurnID: turn, Fact: fact, Version: 1}, true
	}
	next := *prior
	if next.Fact != fact {
		next.Fact = fact
		next.Version++
	}
	return next, false
}
func InvalidatesPreparation(fact b.HostFact) bool {
	return fact.Reason == "tool_initialization_failed" || fact.Reason == "preparation_failed"
}
func ProbeSettled(receipt *b.ProbeReceipt, evidence b.ProbeToolEvidence, invalidated bool) bool {
	return receipt != nil && evidence.Verified && !invalidated
}
