package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// Orca terminal facts are owned by the platform adapter.
type orcaTerminal = orcaworker.Terminal
type orcaTerminalExitCause = orcaworker.TerminalExitCause

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
func showBoundOrcaTerminal(ctx context.Context, c Config, identity executionapi.ProcessIdentity) (orcaTerminal, error) {
	key := digestValue(struct {
		Target   executionapi.ExecutionTarget
		Identity executionapi.ProcessIdentity
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
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error"`
	Meta struct {
		RuntimeID string `json:"runtimeId"`
	} `json:"_meta"`
}

func inspectBoundOrcaTerminal(ctx context.Context, c Config, identity executionapi.ProcessIdentity) (orcaTerminal, error) {
	term, err := showBoundOrcaTerminal(ctx, c, identity)
	if err != nil {
		return term, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "could not inspect fixed Orca terminal", Cause: err}
	}
	if identity.RuntimeID == "" || identity.Incarnation == "" || term.Handle != identity.Terminal ||
		term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return term, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal runtime/handle/incarnation changed or is incomplete"}
	}
	return term, nil
}

type orcaClient struct{ cfg Config }

func validateOrcaRepositoryRegistration(ctx context.Context, config Config) error {
	out, err := (orcaClient{cfg: config}).call(ctx, positiveDuration(config.CommandTimeout, 30*time.Second), "repo", "list", "--json")
	if err != nil {
		return fmt.Errorf("Orca repository registration could not be verified: %w", err)
	}
	envelope, err := parseEnvelope(out)
	if err != nil {
		return fmt.Errorf("Orca repository list response is invalid: %w", err)
	}
	var response struct {
		Repos []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"repos"`
	}
	if err = json.Unmarshal(envelope.Result, &response); err != nil {
		return fmt.Errorf("Orca repository list payload is invalid: %w", err)
	}
	for _, repo := range response.Repos {
		if repo.ID != "" && repo.Kind == "git" && canonical(filepath.FromSlash(repo.Path)) == canonical(config.Repository) {
			return nil
		}
	}
	return executionapi.UnsupportedAdapterOperation("Preflight", "native Orca project repository is not registered on the local Orca host; register the fixed repository before claim")
}

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

func (c orcaClient) closeTerminalChecked(ctx context.Context, handle, runtimeID string) (executionapi.StopReceipt, error) {
	if strings.TrimSpace(handle) == "" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "cannot close an empty terminal handle"}
	}
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "close", "--terminal", handle, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca terminal close response is uncertain", Cause: errors.Join(callErr, parseErr)}
	}
	if parseErr != nil {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendRejected, Diagnostic: "Orca rejected terminal close", Cause: parseErr}
	}
	if runtimeID == "" || env.Meta.RuntimeID != runtimeID {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal close receipt came from a different runtime"}
	}
	var result struct {
		Close struct {
			Handle    string `json:"handle"`
			PtyKilled bool   `json:"ptyKilled"`
		} `json:"close"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Close.Handle != handle {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca terminal close receipt omitted the exact stopped handle", Cause: err}
	}
	if result.Close.PtyKilled {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: "Orca confirmed the fixed terminal PTY was killed"}, nil
	}
	return executionapi.StopReceipt{Disposition: "accepted", Diagnostic: "Orca accepted close without original PTY exit proof"}, nil
}

func (c orcaClient) showTerminal(ctx context.Context, handle string) (orcaTerminal, error) {
	out, callErr := c.call(ctx, positiveDuration(c.cfg.CommandTimeout, 30*time.Second), "terminal", "show", "--terminal", handle, "--json")
	env, e := parseEnvelope(out)
	if e != nil {
		if callErr != nil {
			return orcaTerminal{}, errors.Join(callErr, e)
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
	ref           executionapi.ExecutionRef
	outputSegment string
	cancel        context.CancelFunc
	once          sync.Once
}

func (p *orcaProcess) client() orcaClient { return orcaClient{cfg: p.cfg} }

func (p *orcaProcess) Identity() executionapi.ProcessIdentity {
	return executionapi.ProcessIdentity{Kind: "orca", Terminal: p.term.Handle, RuntimeID: p.term.RuntimeID, Incarnation: p.term.IncarnationID, Executable: p.cfg.Orca.Executable}
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
func inspectProcessWithConfig(c Config, old *executionapi.ProcessIdentity) string {
	if old != nil && old.Kind == "orca" {
		return inspectOrcaTerminal(c, old)
	}
	return inspectProcess(old)
}

func inspectOrcaTerminal(c Config, old *executionapi.ProcessIdentity) string {
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
	if term.Exited() {
		return "orca terminal exited"
	}
	if !term.Connected {
		return "orca terminal not connected"
	}
	return "matching process still exists"
}
