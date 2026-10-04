package backend

import (
	"context"
	"time"
)

type ProbeReceipt struct {
	ProbeIdentity
	InputSHA256 string `json:"input_sha256"`
	GitHead     string `json:"git_head"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Delete      bool   `json:"delete"`
}

type ProbeToolEvidence struct {
	SourceIdentity string `json:"source_identity"`
	CallID         string `json:"call_id"`
	MessageID      string `json:"message_id"`
	Command        string `json:"command"`
	Marker         string `json:"marker"`
	Verified       bool   `json:"verified"`
	FailureCode    string `json:"failure_code,omitempty"`
}

type ToolContext struct {
	SourceIdentity string `json:"source_identity,omitempty"`
	TaskBound      bool   `json:"task_bound,omitempty"`
	CallID         string `json:"call_id,omitempty"`
	Started        bool   `json:"started,omitempty"`
}

type ProbePlan struct {
	Command      string `json:"command"`
	InputDigest  string `json:"input_digest"`
	ScriptDigest string `json:"script_digest"`
}

type HostInputObservation struct {
	Fact  HostFact `json:"fact"`
	Ready bool     `json:"ready"`
}

type HostTurnFacts struct {
	TurnID, TaskID, Purpose, Outcome     string
	IntentAt, Deadline                   time.Time
	Settled, HasDispatch, HasDoneMessage bool
}

type HostOutput struct {
	Source, Cursor, Content, FallbackReason                             string
	ContentComplete, SourceChanged, UpdateAttemptCursor, ContentChanged bool
	Clipping                                                            []string
	Fact                                                                *HostFact
	ProbeEvidence                                                       *ProbeToolEvidence
}

// HostReadInput contains only immutable evidence needed to inspect one output.
// The implementation cannot mutate the center's preparation or blocker state.
type HostReadInput struct {
	Probe *HostProbeObservation
}

type HostProbeObservation struct {
	Command, Nonce, ScriptDigest string
	Evidence                     ProbeToolEvidence
}

type HostEnvironmentPort interface {
	PreparationBudget() time.Duration
	PlatformVersion(context.Context) (string, error)
	ObserveHost(context.Context, ProcessIdentity) (HostFact, error)
	InspectAgentInput(context.Context, ProcessIdentity) (HostInputObservation, error)
	CurrentTurn(LifecycleSnapshot) (*HostTurnFacts, error)
	ResetTurnOutput(LifecycleSnapshot, string) (ImplementationDataEnvelope, error)
	PrepareProbe(ProbeIdentity) (ProbePlan, error)
	ProbePrompt(ProbePlan, string, string) string
	ReadProbe(ProbeIdentity, string) (*ProbeReceipt, error)
	ReadWorkerOutput(context.Context, LifecycleSnapshot, HostReadInput) (HostOutput, ImplementationDataEnvelope, error)
}

type HostFact struct {
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
	Source      string `json:"source"`
	Detail      string `json:"detail,omitempty"`
	Runtime     string `json:"runtime"`
	Terminal    string `json:"terminal"`
	Incarnation string `json:"incarnation"`
	Turn        string `json:"turn,omitempty"`
}
