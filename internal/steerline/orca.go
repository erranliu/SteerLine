package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// orcaTerminal is the subset of `orca terminal show/create/list` fields the
// scheduler reads. Orca owns execution facts; Steerline only diagnoses them.
type orcaTerminal struct {
	Handle          string           `json:"handle"`
	Title           string           `json:"title"`
	WorktreeID      string           `json:"worktreeId"`
	WorktreePath    string           `json:"worktreePath"`
	Branch          string           `json:"branch"`
	Status          string           `json:"status"`
	Connected       bool             `json:"connected"`
	Orphaned        bool             `json:"orphaned"`
	IncarnationID   string           `json:"incarnationId"`
	ExecutionHostID string           `json:"executionHostId"`
	AgentIdentity   string           `json:"agentIdentity"`
	LastOutputAt    int64            `json:"lastOutputAt"`
	AgentWait       *json.RawMessage `json:"agentWait"`
	RuntimeID       string           `json:"-"`
}

type orcaObservationGate struct {
	token chan struct{}
	users int
}

var orcaObservationGates = struct {
	sync.Mutex
	byIdentity map[string]*orcaObservationGate
}{byIdentity: map[string]*orcaObservationGate{}}

// Show calls for one fixed terminal are serialized across Service, output
// readers and its observer. Other terminal identities have independent gates.
func showBoundOrcaTerminal(ctx context.Context, c Config, identity ProcessIdentity) (orcaTerminal, error) {
	key := digestValue(struct {
		Target   ExecutionTarget
		Identity ProcessIdentity
	}{targetFor(c), identity})
	orcaObservationGates.Lock()
	gate := orcaObservationGates.byIdentity[key]
	if gate == nil {
		gate = &orcaObservationGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		orcaObservationGates.byIdentity[key] = gate
	}
	gate.users++
	orcaObservationGates.Unlock()
	releaseUser := func() {
		orcaObservationGates.Lock()
		gate.users--
		if gate.users == 0 {
			delete(orcaObservationGates.byIdentity, key)
		}
		orcaObservationGates.Unlock()
	}
	select {
	case <-ctx.Done():
		releaseUser()
		return orcaTerminal{}, ctx.Err()
	case <-gate.token:
	}
	defer func() {
		gate.token <- struct{}{}
		releaseUser()
	}()
	return (orcaClient{cfg: c}).showTerminal(ctx, identity.Terminal)
}

type orcaEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Meta struct {
		RuntimeID string `json:"runtimeId"`
	} `json:"_meta"`
}

func inspectBoundOrcaTerminal(ctx context.Context, c Config, identity ProcessIdentity) (orcaTerminal, error) {
	term, err := showBoundOrcaTerminal(ctx, c, identity)
	if err != nil {
		return term, &BackendError{Code: BackendUnreachable, Diagnostic: "could not inspect fixed Orca terminal", Cause: err}
	}
	if identity.RuntimeID == "" || identity.Incarnation == "" || term.Handle != identity.Terminal ||
		term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return term, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal runtime/handle/incarnation changed or is incomplete"}
	}
	return term, nil
}

type orcaClient struct{ cfg Config }

func positiveDuration(s string, def time.Duration) time.Duration {
	if d, e := time.ParseDuration(s); e == nil && d > 0 {
		return d
	}
	return def
}

func (c orcaClient) call(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	r := Runner{Dir: c.cfg.Repository, Timeout: timeout}
	return r.Output(ctx, Command{Executable: c.cfg.Orca.Executable, Args: c.cfg.Orca.Args}, args...)
}

func parseEnvelope(b []byte) (orcaEnvelope, error) {
	b = trimBOM(b)
	var e orcaEnvelope
	if err := json.Unmarshal(b, &e); err != nil {
		return e, err
	}
	if !e.OK {
		msg := "orca command failed"
		if e.Error != nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return e, errors.New(msg)
	}
	return e, nil
}

func (c orcaClient) closeTerminal(ctx context.Context, handle string) {
	_, _ = c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "close", "--terminal", handle, "--json")
}

func (c orcaClient) closeTerminalChecked(ctx context.Context, handle, runtimeID string) (StopReceipt, error) {
	if strings.TrimSpace(handle) == "" {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "cannot close an empty terminal handle"}
	}
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "close", "--terminal", handle, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil {
		return StopReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca terminal close response is uncertain", Cause: errors.Join(callErr, parseErr)}
	}
	if parseErr != nil {
		return StopReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "Orca rejected terminal close", Cause: parseErr}
	}
	if runtimeID == "" || env.Meta.RuntimeID != runtimeID {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal close receipt came from a different runtime"}
	}
	var result struct {
		Close struct {
			Handle    string `json:"handle"`
			PtyKilled bool   `json:"ptyKilled"`
		} `json:"close"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Close.Handle != handle {
		return StopReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca terminal close receipt omitted the exact stopped handle", Cause: err}
	}
	if result.Close.PtyKilled {
		return StopReceipt{Disposition: "confirmed", Diagnostic: "Orca confirmed the fixed terminal PTY was killed"}, nil
	}
	return StopReceipt{Disposition: "accepted", Diagnostic: "Orca accepted close without original PTY exit proof"}, nil
}

func (c orcaClient) showTerminal(ctx context.Context, handle string) (orcaTerminal, error) {
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "show", "--terminal", handle, "--json")
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return orcaTerminal{}, callErr
		}
		return orcaTerminal{}, e
	}
	var res struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	if e := json.Unmarshal(env.Result, &res); e != nil {
		return orcaTerminal{}, e
	}
	res.Terminal.RuntimeID = env.Meta.RuntimeID
	return res.Terminal, nil
}

// orcaProcess is the host handle for an Orca-launched agent. Kill closes the
// terminal (which ends the agent tree); Close only releases the local context.
type orcaProcess struct {
	cfg           Config
	term          orcaTerminal
	ref           ExecutionRef
	outputSegment string
	cancel        context.CancelFunc
	once          sync.Once
}

func (p *orcaProcess) client() orcaClient { return orcaClient{cfg: p.cfg} }

func (p *orcaProcess) Identity() ProcessIdentity {
	return ProcessIdentity{Kind: "orca", Terminal: p.term.Handle, RuntimeID: p.term.RuntimeID, Incarnation: p.term.IncarnationID, Executable: p.cfg.Orca.Executable}
}

func (p *orcaProcess) Kill() {
	p.once.Do(func() {
		if p.term.Handle != "" {
			p.client().closeTerminal(context.Background(), p.term.Handle)
		}
	})
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *orcaProcess) Close() {
	if p.cancel != nil {
		p.cancel()
	}
}

// inspectProcessWithConfig dispatches recovery inspection by identity kind. It
// only ever diagnoses; it never adopts or resumes a worker.
func inspectProcessWithConfig(c Config, old *ProcessIdentity) string {
	if old != nil && old.Kind == "orca" {
		return inspectOrcaTerminal(c, old)
	}
	return inspectProcess(old)
}

func inspectOrcaTerminal(c Config, old *ProcessIdentity) string {
	if old.Terminal == "" {
		return "orca terminal identity unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), positiveDuration(c.CommandTimeout, 30*time.Second))
	defer cancel()
	term, e := orcaClient{cfg: c}.showTerminal(ctx, old.Terminal)
	if e != nil {
		return "orca terminal unverifiable (handle stale or runtime unreachable)"
	}
	if old.Incarnation != "" && term.IncarnationID != "" && old.Incarnation != term.IncarnationID {
		return "orca terminal replaced (incarnation changed)"
	}
	if term.Orphaned {
		return "orca terminal orphaned"
	}
	if term.Status == "exited" {
		return "orca terminal exited"
	}
	if !term.Connected {
		return "orca terminal not connected"
	}
	return "matching process still exists"
}
