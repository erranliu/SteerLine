// Package orcatransport implements terminal communication, not worker scheduling.
package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Turn struct {
	Native      NativeProof `json:"native"`
	Command     string      `json:"command"`
	Request     string      `json:"request,omitempty"`
	InputDigest string      `json:"input_digest"`
	Attempted   bool        `json:"attempted"`
	Rejected    bool        `json:"rejected,omitempty"`
	Accepted    bool        `json:"accepted"`
	Started     bool        `json:"started"`
	Settled     bool        `json:"settled"`
	Prompt      string      `json:"prompt"`
	Reply       string      `json:"reply,omitempty"`
	ReplyDigest string      `json:"reply_digest,omitempty"`
	Candidate   string      `json:"candidate,omitempty"`
	CandidateAt time.Time   `json:"candidate_at,omitempty"`
}
type State struct {
	Process         b.ProcessIdentity `json:"process"`
	Turns           []Turn            `json:"turns"`
	LaunchDigest    string            `json:"launch_digest,omitempty"`
	LaunchRequest   string            `json:"launch_request,omitempty"`
	LaunchAttempted bool              `json:"launch_attempted,omitempty"`
	LaunchAccepted  bool              `json:"launch_accepted,omitempty"`
	LaunchConfirmed bool              `json:"launch_confirmed,omitempty"`
}
type Lifecycle struct {
	setup        b.LifecycleSetup
	profile      Profile
	client       native.Client
	stateFactory native.StateFeedFactory
	feed         native.StateFeed
	feedProcess  b.ProcessIdentity
	feedMu       sync.Mutex
	clock        func() time.Time
	nativeAccess bool
}

func NewLifecycle(setup b.LifecycleSetup) (b.LifecyclePort, error) {
	port, err := NewLifecycleWithStateFactory(setup, native.OpenStateFeed)
	if err == nil {
		port.(*Lifecycle).nativeAccess = true
	}
	return port, err
}

// The explicit port supplies isolated native-response fixtures without adding
// simulation flags to profiles or the shared business workflow.
func NewLifecycleWithStateFactory(setup b.LifecycleSetup, factory native.StateFeedFactory) (b.LifecyclePort, error) {
	if factory == nil {
		return nil, errors.New("missing Orca native state source")
	}
	setup = setup.Clone()
	if setup.ProcessHost == nil || setup.Snapshot.Ref.Backend != b.BackendOrca || setup.Snapshot.Ref.Protocol != b.ProtocolOrcaTerminalV3 {
		return nil, errors.New("unsupported Orca terminal lifecycle")
	}
	var p Profile
	found := false
	for _, data := range setup.ProfileData {
		if data.ImplementationKey != ProfileKey {
			continue
		}
		if found || data.CodecVersion != ProfileVersion {
			return nil, errors.New("invalid terminal profile codec")
		}
		found = true
		if err := data.ValidateProfileIdentity(setup.Snapshot.Session.ProjectID, setup.ProfileID, setup.AcceptedConfigVersion); err != nil {
			return nil, err
		}
		var err error
		p, err = DecodeProfile(data.Payload)
		if err != nil {
			return nil, err
		}
	}
	if !found || setup.Snapshot.Ref.Target.ID != p.Target().ID {
		return nil, errors.New("terminal profile differs from frozen target")
	}
	l := &Lifecycle{setup: setup, profile: p, stateFactory: factory, clock: func() time.Time { return time.Now().UTC() }}
	var pairedOnce sync.Once
	var pairedErr error
	call := func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		all := append(append([]string(nil), p.Args...), args...)
		blocked := []string{"ORCA_ENVIRONMENT", "ORCA_PAIRING_CODE", "ORCA_REMOTE_PAIRING"}
		if p.Environment != "" {
			all = append(all, "--environment", p.Environment)
		} else if l.nativeAccess {
			pairedOnce.Do(func() {
				runtime, err := native.LocalRuntimeID()
				if err == nil {
					_, err = native.QueryWorktreeAgents(ctx, "", runtime)
				}
				pairedErr = err
			})
			if pairedErr != nil {
				return nil, pairedErr
			}
			blocked = []string{"ORCA_ENVIRONMENT"}
		}
		return setup.ProcessHost.OutputCommand(ctx, b.LaunchCommand{Executable: p.Executable, Args: all, WorkingDirectory: setup.RepositoryPath, Timeout: timeout, BlockedEnvironment: blocked})
	}
	l.client = native.New(native.Config{CommandTimeout: "40s", Target: p.Target(), Output: call})
	return l, nil
}

func (l *Lifecycle) PolicyCapabilities() b.LifecyclePolicyCapabilities {
	return b.LifecyclePolicyCapabilities{ServiceStopBehavior: ServiceStopBehavior, StructuredFinalEvidence: true, ReportToolPolicy: "host_managed_permissions; business_read_only_is_prompt_policy", ResumeActiveObservation: true, LinkedWorkspaceWriteRoots: false, SeparateTurnStart: true}
}

func (l *Lifecycle) validate(s b.LifecycleSnapshot) error {
	w := l.setup.Snapshot
	if s.Ref.Backend != w.Ref.Backend || s.Ref.Protocol != w.Ref.Protocol || s.Ref.Target.ID != w.Ref.Target.ID || s.Ref.Agent != w.Ref.Agent || s.Ref.RunID != w.Ref.RunID || s.Ref.AttemptID != w.Ref.AttemptID || s.Session.ProjectID != w.Session.ProjectID || s.Session.WorkflowRunID != w.Session.WorkflowRunID || s.Ref.Target.ExternalRuntime != l.profile.Target().ExternalRuntime || s.Ref.Target.ExecutionHost != l.profile.Target().ExecutionHost || s.Session.ExecutionID != w.Session.ExecutionID || s.Session.AttemptID != w.Session.AttemptID || s.ConfigurationDigest != w.ConfigurationDigest || s.ResourceGeneration != w.ResourceGeneration {
		return errors.New("Orca terminal execution binding changed")
	}
	if s.Session.ProjectID == "" || s.ConfigurationDigest == "" || s.ResourceGeneration == "" {
		return errors.New("incomplete Orca terminal execution")
	}
	return nil
}

func (l *Lifecycle) state(s b.LifecycleSnapshot) (State, error) {
	if err := l.validate(s); err != nil {
		return State{}, err
	}
	state := State{}
	found := false
	for _, e := range s.Implementation {
		if e.ImplementationKey == StateKey {
			if found || e.CodecVersion != 3 {
				return state, errors.New("unsupported terminal state")
			}
			found = true
			if err := e.ValidateResourceIdentity(s.Session.ProjectID, s.Session.ExecutionID, s.Session.AttemptID, s.ResourceGeneration, s.ConfigurationDigest); err != nil {
				return state, err
			}
			if err := b.DecodeStrictJSON(e.Payload, &state, "trailing terminal state"); err != nil {
				return state, err
			}
		}
	}
	if state.Process.Terminal != "" && state.Process != s.Ref.External {
		return state, errors.New("saved original terminal differs from current resource")
	}
	if state.LaunchAttempted && (state.LaunchDigest != digest([]byte(l.setup.AgentLaunchCommand)) || state.LaunchRequest == "") || state.LaunchAccepted && !state.LaunchAttempted || state.LaunchConfirmed && !state.LaunchAccepted {
		return state, errors.New("original agent launch binding changed")
	}
	seen := map[string]bool{}
	for _, turn := range state.Turns {
		if !b.StableExecutionID(turn.Command) || seen[turn.Command] || len(turn.InputDigest) != 64 || turn.Request != "" && !b.StableExecutionID(strings.ReplaceAll(turn.Request, "-", "")) || turn.Started && !turn.Accepted || turn.Accepted && !turn.Attempted || turn.Settled && (turn.Reply == "" || turn.ReplyDigest != digest([]byte(turn.Reply))) {
			return state, errors.New("invalid original terminal input binding")
		}
		seen[turn.Command] = true
		if s.PendingTurn != nil && s.PendingTurn.CommandID == turn.Command && turn.InputDigest != digest([]byte(s.PendingTurn.Prompt)) {
			return state, errors.New("saved terminal input differs from original command")
		}
		if turn.InputDigest != digest([]byte(turn.Prompt)) || len(turn.Reply) > maxReplyBytes {
			return state, errors.New("invalid saved terminal reply binding")
		}
		if turn.Native.Pane == "" || turn.Native.WorkingAt > 0 && (turn.Native.Provider == "" || turn.Native.Authority == "" || turn.Native.Revision <= 0) || turn.Native.EndedAt > 0 && turn.Native.EndedAt <= turn.Native.WorkingAt || turn.Settled && (turn.Native.EndedAt == 0 || turn.Native.ExpectedReply != replyAssociation(turn.Reply)) {
			return state, errors.New("invalid original native turn evidence")
		}
	}
	return state, nil
}

func (l *Lifecycle) envelope(s b.LifecycleSnapshot, state State) ([]b.ImplementationDataEnvelope, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	e, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: 3, Purpose: b.ImplementationResourcePurpose, ImplementationKey: StateKey, ProjectID: s.Session.ProjectID, ExecutionID: s.Session.ExecutionID, AttemptID: s.Session.AttemptID, Generation: s.ResourceGeneration, ConfigDigest: s.ConfigurationDigest}, raw)
	if err != nil {
		return nil, err
	}
	return []b.ImplementationDataEnvelope{e}, nil
}

func SessionID(p b.ProcessIdentity) string {
	return "orca-terminal:" + p.RuntimeID + ":" + p.Terminal + ":" + p.Incarnation
}
func turnFor(state *State, command string) *Turn {
	for i := range state.Turns {
		if state.Turns[i].Command == command {
			return &state.Turns[i]
		}
	}
	return nil
}
func command(s b.LifecycleSnapshot) string {
	if s.PendingTurn != nil {
		return s.PendingTurn.CommandID
	}
	if s.Session.PendingCommandID != "" {
		return s.Session.PendingCommandID
	}
	return s.Session.CurrentCommandID
}

func (l *Lifecycle) original(ctx context.Context, s b.LifecycleSnapshot) (native.Terminal, error) {
	state, stateErr := l.state(s)
	if stateErr != nil {
		return native.Terminal{}, stateErr
	}
	initializing := l.setup.AgentLaunchCommand != "" && !state.LaunchConfirmed
	t, err := l.client.InspectBoundTerminal(ctx, l.profile.Target(), s.Ref.External)
	if err != nil {
		return t, err
	}
	if t.WorktreePath != s.Workspace.Path || strings.TrimPrefix(t.Branch, "refs/heads/") != s.Workspace.Branch && !(initializing && t.Branch == "") {
		return t, errors.New("original Orca workspace or branch changed")
	}
	// A connected native background terminal can be unadopted by the UI
	// (orphaned=true). Its original runtime/incarnation/worktree and agent must
	// still match; adoption into a visible tab is not execution identity.
	if t.WorktreeID != s.Ref.External.WorktreeID || t.AgentIdentity != s.Ref.Agent.Name && !(initializing && t.AgentIdentity == "") || !t.Connected || t.Exited() {
		return t, fmt.Errorf("original Orca agent/worktree changed (worktree=%q expected=%q agent=%q orphaned=%t)", t.WorktreeID, s.Ref.External.WorktreeID, t.AgentIdentity, t.Orphaned)
	}
	return t, nil
}

func (l *Lifecycle) PreflightLifecycle(ctx context.Context, setup b.LifecycleSetup) (b.CapabilityReport, error) {
	if err := l.validate(setup.Snapshot); err != nil {
		return b.CapabilityReport{}, err
	}
	if setup.Snapshot.Ref.Agent.Name != "codex" {
		return b.CapabilityReport{}, errors.New("Orca rendered reply collection is currently verified only for the Codex terminal renderer")
	}
	r, err := l.client.MetaCall(ctx, "status", "--json")
	if err != nil {
		return b.CapabilityReport{}, fmt.Errorf("Orca terminal preflight: %w", err)
	}
	var status struct {
		Runtime struct {
			State        string   `json:"state"`
			AppVersion   string   `json:"appVersion"`
			Capabilities []string `json:"capabilities"`
		} `json:"runtime"`
	}
	if json.Unmarshal(r.Payload, &status) != nil || r.RuntimeID == "" || status.Runtime.State != "ready" {
		return b.CapabilityReport{}, errors.New("Orca runtime is not ready")
	}
	supported := false
	for _, cap := range status.Runtime.Capabilities {
		if cap == "terminal.prompt-delivery.v1" {
			supported = true
		}
	}
	if !supported {
		return b.CapabilityReport{}, errors.New("Orca prompt identity/replay capability is unavailable")
	}
	if l.nativeAccess {
		if _, err := native.QueryWorktreeAgents(ctx, l.profile.Environment, r.RuntimeID); err != nil {
			return b.CapabilityReport{}, fmt.Errorf("Orca native observation authentication/access: %w", err)
		}
	}
	return b.CapabilityReport{Version: 1, Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, Target: setup.Snapshot.Ref.Target, Agent: setup.Snapshot.Ref.Agent, PlatformVersion: status.Runtime.AppVersion, CheckedAt: time.Now().UTC(), Capabilities: []b.Capability{{Name: "start", Status: b.CapabilityVerified}, {Name: "observe", Status: b.CapabilityVerified}, {Name: "output_final_answer", Status: b.CapabilityVerified, Evidence: "original request, native main-agent end and complete ordinary reply association"}}}, nil
}

func (l *Lifecycle) Next(_ context.Context, s b.LifecycleSnapshot) (b.LifecycleCandidate, error) {
	if err := l.validate(s); err != nil {
		return b.LifecycleCandidate{}, err
	}
	if s.Workspace.Path == "" {
		intent := s.WorkspacePlan
		return b.LifecycleCandidate{Action: b.LifecyclePrepareWorkspace, Input: b.LifecycleInput{Workspace: &intent}}, nil
	}
	if l.setup.AgentLaunchCommand != "" {
		state, err := l.state(s)
		if err != nil {
			return b.LifecycleCandidate{}, err
		}
		if !state.LaunchConfirmed {
			return b.LifecycleCandidate{Action: b.LifecycleCreateResource, Input: b.LifecycleInput{Resource: &b.ResourceIntent{Purpose: "agent-start", Name: "original-terminal", Objective: l.setup.AgentLaunchCommand}}}, nil
		}
	}
	if s.PendingTurn == nil {
		return b.LifecycleCandidate{Done: true}, nil
	}
	if s.Ref.External.Terminal == "" {
		return b.LifecycleCandidate{}, errors.New("workspace has no original agent terminal; do not create a replacement")
	}
	state, err := l.state(s)
	if err != nil {
		return b.LifecycleCandidate{}, err
	}
	input := *s.PendingTurn
	if turnFor(&state, input.CommandID) == nil {
		return b.LifecycleCandidate{Action: b.LifecyclePrepareSession, Input: b.LifecycleInput{Turn: &input}}, nil
	}
	return b.LifecycleCandidate{Action: b.LifecycleStartTurn, Input: b.LifecycleInput{Turn: &input}}, nil
}

func (l *Lifecycle) Execute(ctx context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	if err := r.Validate(); err != nil {
		return receipt, nil, err
	}
	if err := l.validate(r.Snapshot); err != nil {
		return receipt, nil, err
	}
	var err error
	switch b.LifecycleAction(r.Operation.Action) {
	case b.LifecyclePrepareWorkspace:
		receipt, err = l.prepareWorkspace(ctx, r, false)
	case b.LifecycleCreateResource:
		receipt, err = l.launchAgent(ctx, r, false)
	case b.LifecyclePrepareSession:
		receipt, err = l.prepareInput(ctx, r)
	case b.LifecycleStartTurn, b.LifecycleSubmitInput:
		receipt, err = l.submit(ctx, r)
	default:
		err = errors.New("unsupported terminal action")
	}
	if err == nil && (b.LifecycleAction(r.Operation.Action) == b.LifecycleStartTurn || b.LifecycleAction(r.Operation.Action) == b.LifecycleSubmitInput) && receipt.State.InputAccepted {
		snapshot := r.Snapshot.Clone()
		snapshot.Implementation = receipt.Implementation
		snapshot.PendingTurn = r.Input.Turn
		return receipt, l.observe(snapshot), nil
	}
	return receipt, nil, err
}

func (l *Lifecycle) ready(ctx context.Context, s b.LifecycleSnapshot) error {
	wait, _ := time.ParseDuration(l.profile.WaitTimeout)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := l.original(ctx, s); err != nil {
			return err
		}
		r, err := l.client.MetaCallWithTimeout(ctx, wait+10*time.Second, "terminal", "wait", "--terminal", s.Ref.External.Terminal, "--for", "tui-idle", "--timeout-ms", strconv.FormatInt(wait.Milliseconds(), 10), "--json")
		var value struct {
			Wait struct {
				Satisfied     bool   `json:"satisfied"`
				BlockedReason string `json:"blockedReason"`
			} `json:"wait"`
		}
		valid := json.Unmarshal(r.Payload, &value) == nil && r.RuntimeID == s.Ref.External.RuntimeID && !r.CallUncertain && r.ErrorCode == ""
		if valid && value.Wait.Satisfied && value.Wait.BlockedReason == "" && err == nil {
			break
		}
		timedOut := r.ErrorCode == "timeout" && r.RuntimeID == s.Ref.External.RuntimeID && !r.CallUncertain || valid && !value.Wait.Satisfied && value.Wait.BlockedReason == ""
		if attempt == 0 && timedOut && ctx.Err() == nil {
			wait = min(2*wait, time.Minute)
			continue
		}
		cause := errors.Join(err, fmt.Errorf("original Orca readiness is unverified (blocked=%q); no input was sent", value.Wait.BlockedReason))
		return l.readinessDiagnostic(ctx, s, cause)
	}
	_, err := l.original(ctx, s)
	if err != nil {
		return err
	}
	page, err := l.client.ReadScreen(ctx, s.Ref.External)
	if err != nil {
		return err
	}
	if strings.TrimSpace(page.Draft) != "" {
		return errors.New("original Orca composer is not empty; finish native session preparation before starting; no input was sent")
	}
	_, err = l.original(ctx, s)
	return err
}

func (l *Lifecycle) readinessDiagnostic(ctx context.Context, s b.LifecycleSnapshot, cause error) error {
	term, err := l.original(ctx, s)
	if err != nil {
		return errors.Join(cause, err)
	}
	page, err := l.client.ReadScreen(ctx, s.Ref.External)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("readiness screen unavailable: %w", err))
	}
	lines := page.Tail
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	preview := []rune(strings.Join(lines, "\n"))
	if len(preview) > 1200 {
		preview = preview[len(preview)-1200:]
	}
	return fmt.Errorf("%w; terminal=%s status=%s agent=%s screen=%q", cause, term.Handle, term.Status, term.AgentIdentity, string(preview))
}

func nativeRequest(operation string) string {
	value := digest([]byte(operation))[:32]
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}

// Persist the original input and native state baseline before submission.
// A visible reply cannot substitute for this turn's native ending evidence.
func (l *Lifecycle) prepareInput(ctx context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, error) {
	s := r.Snapshot
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationRejected}
	state, err := l.state(s)
	if err != nil {
		return receipt, err
	}
	if l.setup.AgentLaunchCommand != "" && !state.LaunchConfirmed {
		return receipt, errors.New("original agent launch is not confirmed; no task input was sent")
	}
	input := r.Input.Turn
	if input == nil || !b.StableExecutionID(input.CommandID) {
		return receipt, errors.New("missing original worker input")
	}
	inputDigest := digest([]byte(input.Prompt))
	if turn := turnFor(&state, input.CommandID); turn != nil {
		if turn.InputDigest != inputDigest {
			return receipt, errors.New("prepared worker input changed")
		}
	} else {
		if err = l.ready(ctx, s); err != nil {
			return receipt, err
		}
		feed, feedErr := l.stateFeed(ctx, state.Process)
		if feedErr != nil {
			return receipt, feedErr
		}
		defer l.closeStateFeed()
		baseline := nativeBaseline(feed.Initial(), l.clock())
		_, readErr := l.client.ReadScreen(ctx, s.Ref.External)
		if readErr != nil {
			return receipt, readErr
		}
		if _, err = l.original(ctx, s); err != nil {
			return receipt, err
		}
		state.Turns = append(state.Turns, Turn{Command: input.CommandID, Prompt: input.Prompt, InputDigest: inputDigest, Native: baseline})
	}
	receipt.Process = &state.Process
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(state.Process), SessionGeneration: state.Process.Incarnation, SessionAvailable: true, WorkerLive: true}
	receipt.Implementation, err = l.envelope(s, state)
	if err == nil {
		receipt.Disposition = b.OperationConfirmed
	}
	return receipt, err
}
func (l *Lifecycle) submit(ctx context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, error) {
	s := r.Snapshot
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationRejected}
	state, err := l.state(s)
	if err != nil {
		return receipt, err
	}
	if l.setup.AgentLaunchCommand != "" && !state.LaunchConfirmed {
		return receipt, errors.New("original agent launch is not confirmed; no task input was sent")
	}
	input := r.Input.Turn
	if input == nil || !b.StableExecutionID(input.CommandID) {
		return receipt, errors.New("missing original worker input")
	}
	turn := turnFor(&state, input.CommandID)
	if turn == nil || turn.InputDigest != digest([]byte(input.Prompt)) {
		return receipt, errors.New("original input has no committed output baseline")
	}
	if turn.Attempted {
		return receipt, errors.New("original terminal input was already attempted; reconcile only")
	}
	if err = l.ready(ctx, s); err != nil {
		return receipt, err
	}
	feed, feedErr := l.stateFeed(ctx, state.Process)
	if feedErr != nil {
		return receipt, feedErr
	}
	initial := feed.Initial()
	if initial.PaneKey != turn.Native.Pane || turn.Native.Provider != "" && initial.ProviderSession.ID != turn.Native.Provider {
		l.closeStateFeed()
		return receipt, errors.New("native original session changed before submission")
	}
	turn.Native = nativeBaseline(initial, l.clock())
	turn.Request = nativeRequest(r.Operation.ID)
	turn.Attempted = true
	receipt.Disposition = b.OperationUnknown
	receipt.Process = &state.Process
	receipt.RequestID = turn.Request
	receipt.Implementation, err = l.envelope(s, state)
	if err != nil {
		return receipt, err
	}
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(state.Process), SessionGeneration: state.Process.Incarnation, TurnID: input.CommandID, SessionAvailable: true, WorkerLive: true}
	result, callErr := l.client.MetaCallWithTimeout(ctx, 50*time.Second, "terminal", "send", "--terminal", state.Process.Terminal, "--text", input.Prompt, "--enter", "--wait-submit", "30", "--retry-request", turn.Request, "--json")
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	var sent struct {
		Send struct {
			Handle   string `json:"handle"`
			Accepted *bool  `json:"accepted"`
			Prompt   struct {
				RequestID          string   `json:"requestId"`
				ProcessIncarnation string   `json:"processIncarnation"`
				Stages             []string `json:"stages"`
			} `json:"prompt"`
		} `json:"send"`
	}
	bound := json.Unmarshal(result.Payload, &sent) == nil && result.RuntimeID == state.Process.RuntimeID && sent.Send.Handle == state.Process.Terminal && sent.Send.Prompt.RequestID == turn.Request && sent.Send.Prompt.ProcessIncarnation == state.Process.Incarnation && !result.CallUncertain && result.ErrorCode == ""
	if bound && sent.Send.Accepted != nil && !*sent.Send.Accepted && len(sent.Send.Prompt.Stages) == 0 {
		turn.Rejected = true
		receipt.Disposition = b.OperationRejected
		receipt.Implementation, err = l.envelope(s, state)
		l.closeStateFeed()
		return receipt, errors.Join(err, errors.New("original native input explicitly rejected; no input was accepted"))
	}
	if !bound || sent.Send.Accepted == nil || !*sent.Send.Accepted {
		l.closeStateFeed()
		return receipt, errors.Join(callErr, errors.New("original terminal input outcome is unknown; do not resend"))
	}
	turn.Accepted = true
	for _, stage := range sent.Send.Prompt.Stages {
		if stage == "turn_started" {
			turn.Started = true
		}
	}
	receipt.Implementation, err = l.envelope(s, state)
	receipt.State.InputAccepted, receipt.State.TurnStarted = true, turn.Started
	receipt.Disposition = b.OperationAccepted
	if turn.Started {
		receipt.Facts = []b.EventFact{{Kind: b.EventStarted, Source: "orca.terminal.send", SourceEventID: turn.Request, SessionID: SessionID(state.Process), StartedTurnID: turn.Command, InputAccepted: true, Matched: true}}
	}
	return receipt, err
}
func (l *Lifecycle) observeOnce(ctx context.Context, s b.LifecycleSnapshot) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	receipt := b.LifecycleReceipt{Disposition: b.OperationUnknown}
	state, err := l.state(s)
	if err != nil {
		return receipt, nil, err
	}
	turn := turnFor(&state, command(s))
	if turn == nil || !turn.Attempted {
		return receipt, nil, nil
	}
	receipt.Process = &state.Process
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(state.Process), SessionGeneration: state.Process.Incarnation, TurnID: turn.Command, InputAccepted: turn.Accepted, TurnStarted: turn.Started}
	if !turn.Settled {
		if _, err = l.original(ctx, s); err != nil {
			return receipt, nil, err
		}
		receipt.Diagnostic = l.observeNative(ctx, s, &state, turn)
		// Bind every streamed observation to the original live PTY, including
		// ended-without-body facts. Never infer identity from an omitted field.
		if _, err = l.original(ctx, s); err != nil {
			return receipt, nil, err
		}
		if turn.Native.EndedAt == 0 || turn.Native.ExpectedReply == "" || receipt.Diagnostic != "" {
			goto observed
		}
		page, readErr := l.client.ReadScreen(ctx, state.Process)
		if readErr != nil {
			return receipt, nil, readErr
		}
		if _, err = l.original(ctx, s); err != nil {
			return receipt, nil, err
		}
		text, fingerprint, problem := renderedReply(s.Ref.Agent.Name, page.Tail)
		if turn.Accepted && turn.Started && strings.TrimSpace(page.Draft) == "" && problem == "" && turn.Native.matchesVisible(text) {
			if turn.Candidate != fingerprint {
				turn.Candidate, turn.CandidateAt = fingerprint, time.Now().UTC()
			} else if time.Since(turn.CandidateAt) >= time.Second {
				turn.Reply, turn.ReplyDigest, turn.Settled = turn.Native.ReplyBody, digest([]byte(turn.Native.ReplyBody)), true
				turn.Native.ReplyBody = ""
			}
		} else {
			turn.Candidate, turn.CandidateAt = "", time.Time{}
			if turn.Native.EndedAt > 0 {
				if problem == "" {
					problem = "complete visible reply does not match this native ended turn, or composer is occupied"
				}
				receipt.Diagnostic = "Original terminal reply is unverified: " + problem + "; original submission/start and complete visible reply are required; no input was repeated"
			}
		}
	}

observed:
	if turn.Native.EndedAt > 0 && !turn.Settled && turn.Candidate == "" && receipt.Diagnostic == "" {
		receipt.Diagnostic = "Native turn ended; awaiting the complete original ordinary reply"
	}
	receipt.Implementation, err = l.envelope(s, state)
	if err != nil {
		return receipt, nil, err
	}
	receipt.State.InputAccepted, receipt.State.TurnStarted = turn.Accepted, turn.Started
	receipt.Disposition = b.OperationAccepted
	receipt.State.WorkerLive = turn.Native.EndedAt == 0
	receipt.State.TurnCompleted = turn.Native.EndedAt > 0
	if !turn.Settled {
		return receipt, nil, nil
	}
	receipt.Disposition = b.OperationConfirmed
	l.closeStateFeed()
	receipt.State.TurnCompleted = true
	nativeOutcome := turn.Native.Outcome
	if nativeOutcome == "success" {
		nativeOutcome = "succeeded"
	}
	receipt.TurnEvidence = &b.TurnEvidence{NativeOutcome: nativeOutcome, FinalOrigin: "terminal_observed", FinalSource: replySource, FinalItemID: turn.ReplyDigest, FinalVerified: true, TurnID: turn.Command, CommandID: turn.Command, SessionID: SessionID(state.Process), Started: true, Completed: true, Succeeded: true, LastMessage: turn.Reply}
	return receipt, nil, nil
}

// Recovery resumes observation of the same resource and saved input/baseline only.
func (l *Lifecycle) ObserveLifecycle(ctx context.Context, s b.LifecycleSnapshot) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	receipt, _, err := l.observeOnce(ctx, s)
	if err != nil || receipt.TurnEvidence != nil || receipt.Process == nil {
		return receipt, nil, err
	}
	snapshot := s.Clone()
	snapshot.Implementation = receipt.Implementation
	return receipt, l.observe(snapshot), nil
}

func (l *Lifecycle) Reconcile(ctx context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	defer l.closeStateFeed()
	if err := r.Validate(); err != nil {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, err
	}
	if err := l.validate(r.Snapshot); err != nil {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, err
	}
	switch b.LifecycleAction(r.Operation.Action) {
	case b.LifecyclePrepareWorkspace:
		receipt, err := l.prepareWorkspace(ctx, r, true)
		return receipt, nil, err
	case b.LifecycleCreateResource:
		receipt, err := l.launchAgent(ctx, r, true)
		return receipt, nil, err
	case b.LifecyclePrepareSession:
		receipt, err := l.prepareInput(ctx, r)
		return receipt, nil, err
	case b.LifecycleStartTurn, b.LifecycleSubmitInput:
		state, stateErr := l.state(r.Snapshot)
		if stateErr == nil {
			if turn := turnFor(&state, command(r.Snapshot)); turn != nil && turn.Rejected {
				return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationRejected, Implementation: r.Snapshot.Implementation, Diagnostic: "Original native input was rejected; no input was resent"}, nil, nil
			}
		}
		receipt, _, err := l.observeOnce(ctx, r.Snapshot)
		receipt.Operation = r.Operation
		if receipt.TurnEvidence == nil {
			receipt.Disposition = b.OperationUnknown
			if receipt.Diagnostic == "" {
				receipt.Diagnostic = "Original terminal input is unresolved; no input or resource was recreated"
			}
		}
		return receipt, nil, err
	default:
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}, nil, errors.New("unsupported terminal reconciliation")
	}
}

func (l *Lifecycle) StopLifecycle(ctx context.Context, r b.LifecycleRequest, _ b.StopHandle) (b.LifecycleReceipt, error) {
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	if err := r.Validate(); err != nil {
		return receipt, err
	}
	if err := l.validate(r.Snapshot); err != nil {
		return receipt, err
	}
	if b.LifecycleAction(r.Operation.Action) == b.LifecycleStopObservation {
		receipt.Disposition = b.OperationConfirmed
		return receipt, nil
	}
	if _, err := l.original(ctx, r.Snapshot); err != nil {
		return receipt, err
	}
	if b.LifecycleAction(r.Operation.Action) != b.LifecycleStopExecution {
		return receipt, errors.New("unsupported terminal stop")
	}
	proof, err := l.client.CloseTerminalChecked(ctx, r.Snapshot.Ref.External.Terminal, r.Snapshot.Ref.External.RuntimeID)
	if err != nil {
		return receipt, err
	}
	if proof.Disposition == "confirmed" {
		receipt.Disposition = b.OperationConfirmed
		receipt.State.Stopped = true
		receipt.State.WorkerAbsent = true
	} else {
		receipt.Disposition = b.OperationAccepted
	}
	return receipt, nil
}

func (l *Lifecycle) CleanupLifecycle(ctx context.Context, r b.LifecycleRequest) (b.LifecycleReceipt, error) {
	if r.Input.Cleanup == nil || len(r.Input.Cleanup.Resources) != 1 || r.Input.Cleanup.Resources[0] != r.Snapshot.Ref.External {
		return b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationRejected}, errors.New("cleanup differs from original terminal")
	}
	copy := r
	copy.Operation.Action = string(b.LifecycleStopExecution)
	copy.Input = b.LifecycleInput{Stop: &b.StopInput{}}
	// Keep the original operation binding; cleanup and stop have the same exact resource effect.
	receipt, err := l.StopLifecycle(ctx, copy, nil)
	receipt.Operation = r.Operation
	return receipt, err
}

func (l *Lifecycle) ShutdownLifecycle(_ context.Context, s b.LifecycleSnapshot, _ b.StopHandle) (b.LifecycleStateFacts, error) {
	return b.LifecycleStateFacts{}, l.validate(s)
}
func (l *Lifecycle) InspectLifecycleWorkspace(ctx context.Context, s b.LifecycleSnapshot) (b.WorkspaceInspection, error) {
	return InspectWorkspace(ctx, l.client, s.Ref, s.Session.ProjectID)
}
func (l *Lifecycle) VerifyManualRecovery(ctx context.Context, s b.LifecycleSnapshot) (b.LifecycleRecoveryEvidence, error) {
	_, err := l.original(ctx, s)
	return b.LifecycleRecoveryEvidence{SessionAvailable: err == nil}, err
}
func (l *Lifecycle) BindRecovery(_ context.Context, original, replacement b.LifecycleSnapshot) (b.ImplementationDataEnvelope, error) {
	return b.ImplementationDataEnvelope{}, errors.New("replacing an Orca session requires a new execution")
}
