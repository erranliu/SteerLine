package orcaworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func positiveDuration(s string, def time.Duration) time.Duration {
	if d, e := time.ParseDuration(s); e == nil && d > 0 {
		return d
	}
	return def
}

type Client struct{ config Config }

func New(c Config) Client { return Client{config: c} }
func (c Client) Raw(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	return c.config.Output(ctx, timeout, args...)
}
func (c Client) MetaCall(ctx context.Context, args ...string) (Result, error) {
	out, callErr := c.Raw(ctx, positiveDuration(c.config.CommandTimeout, 30*time.Second), args...)
	env, parseErr := ParseEnvelope(out)
	result := Result{Payload: env.Result, RuntimeID: env.Meta.RuntimeID}
	// A completed nonzero CLI exit may carry a valid platform rejection. A
	// timeout/cancellation or interrupted transport cannot prove non-delivery.
	var exit *exec.ExitError
	result.CallUncertain = callErr != nil && (!errors.As(callErr, &exit) || !exit.Exited())
	if env.Error != nil {
		result.ErrorCode, result.ErrorData = env.Error.Code, env.Error.Data
	}
	return result, errors.Join(callErr, parseErr)
}
func (c Client) Call(ctx context.Context, args ...string) (json.RawMessage, error) {
	v, e := c.MetaCall(ctx, args...)
	return v.Payload, e
}

type Envelope struct {
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

func ParseEnvelope(b []byte) (Envelope, error) {
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	var e Envelope
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
func (c Client) ShowBoundTerminal(ctx context.Context, target executionapi.ExecutionTarget, identity executionapi.ProcessIdentity) (Terminal, error) {
	key := digestValue(struct {
		Target   executionapi.ExecutionTarget
		Identity executionapi.ProcessIdentity
	}{target, identity})
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
		return Terminal{}, ctx.Err()
	case <-gate.token:
	}
	defer func() {
		gate.token <- struct{}{}
		releaseUser()
	}()
	return c.ShowTerminal(ctx, identity.Terminal)
}

func (c Client) InspectBoundTerminal(ctx context.Context, target executionapi.ExecutionTarget, identity executionapi.ProcessIdentity) (Terminal, error) {
	term, err := c.ShowBoundTerminal(ctx, target, identity)
	if err != nil {
		return term, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "could not inspect fixed Orca terminal", Cause: err}
	}
	if identity.RuntimeID == "" || identity.Incarnation == "" || term.Handle != identity.Terminal ||
		term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return term, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal runtime/handle/incarnation changed or is incomplete"}
	}
	return term, nil
}

func (c Client) CloseTerminalChecked(ctx context.Context, handle, runtimeID string) (executionapi.StopReceipt, error) {
	if strings.TrimSpace(handle) == "" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "cannot close an empty terminal handle"}
	}
	out, callErr := c.Raw(ctx, positiveDuration(c.config.CommandTimeout, 30*time.Second), "terminal", "close", "--terminal", handle, "--json")
	env, parseErr := ParseEnvelope(out)
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

func (c Client) ShowTerminal(ctx context.Context, handle string) (Terminal, error) {
	out, callErr := c.Raw(ctx, positiveDuration(c.config.CommandTimeout, 30*time.Second), "terminal", "show", "--terminal", handle, "--json")
	env, e := ParseEnvelope(out)
	if e != nil || callErr != nil {
		return Terminal{}, errors.Join(callErr, e)
	}
	var res struct {
		Terminal Terminal `json:"terminal"`
	}
	if e := json.Unmarshal(env.Result, &res); e != nil {
		return Terminal{}, e
	}
	res.Terminal.RuntimeID = env.Meta.RuntimeID
	return res.Terminal, nil
}
