// Package orca contains the public CLI and runtime query wire contracts. It knows no tasks,
// business steps, agent-private logs or orchestration lifecycle.
package orca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Config struct {
	CommandTimeout string
	Target         b.ExecutionTarget
	Output         func(context.Context, time.Duration, ...string) ([]byte, error)
}

const WorktreeTrashDirectory = ".orca-worktree-trash"

var ErrRepoUnavailable = errors.New("fixed Orca repository is unavailable")

func FindRepo(ctx context.Context, call func(context.Context, ...string) (json.RawMessage, error), repository string) (string, error) {
	raw, err := call(ctx, "repo", "list", "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Repos []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"repos"`
	}
	if json.Unmarshal(raw, &listing) != nil {
		return "", ErrRepoUnavailable
	}
	for _, repo := range listing.Repos {
		if strings.EqualFold(strings.ReplaceAll(repo.Path, "\\", "/"), strings.ReplaceAll(repository, "\\", "/")) {
			return repo.ID, nil
		}
	}
	return "", ErrRepoUnavailable
}

type Client struct{ config Config }

func New(config Config) Client { return Client{config: config} }

type Result struct {
	Payload       json.RawMessage
	RuntimeID     string
	ErrorCode     string
	CallUncertain bool
}
type Envelope struct {
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

func ParseEnvelope(raw []byte) (Envelope, error) {
	var value Envelope
	err := json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &value)
	if err != nil {
		return value, err
	}
	if !value.OK {
		if value.Error != nil {
			return value, errors.New(value.Error.Message)
		}
		return value, errors.New("Orca CLI rejected request")
	}
	return value, nil
}
func (c Client) Raw(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	return c.config.Output(ctx, timeout, args...)
}
func (c Client) MetaCall(ctx context.Context, args ...string) (Result, error) {
	timeout, err := time.ParseDuration(c.config.CommandTimeout)
	if err != nil || timeout <= 0 {
		timeout = 40 * time.Second
	}
	raw, callErr := c.Raw(ctx, timeout, args...)
	envelope, parseErr := ParseEnvelope(raw)
	var exit *exec.ExitError
	uncertain := callErr != nil && (!errors.As(callErr, &exit) || !exit.Exited())
	result := Result{Payload: envelope.Result, RuntimeID: envelope.Meta.RuntimeID, CallUncertain: uncertain}
	if envelope.Error != nil {
		result.ErrorCode = envelope.Error.Code
	}
	return result, errors.Join(callErr, parseErr)
}

type Terminal struct {
	Handle        string          `json:"handle"`
	RuntimeID     string          `json:"-"`
	IncarnationID string          `json:"incarnationId"`
	WorktreeID    string          `json:"worktreeId"`
	WorktreePath  string          `json:"worktreePath"`
	Branch        string          `json:"branch"`
	AgentIdentity string          `json:"agentIdentity"`
	Connected     bool            `json:"connected"`
	Orphaned      bool            `json:"orphaned"`
	Status        string          `json:"status"`
	AgentWait     json.RawMessage `json:"agentWait"`
}

func (t Terminal) Exited() bool { return t.Status == "exited" }

type Worktree struct {
	ID          string `json:"id"`
	RepoID      string `json:"repoId"`
	Path        string `json:"path"`
	Head        string `json:"head"`
	Branch      string `json:"branch"`
	BaseRef     string `json:"baseRef"`
	DisplayName string `json:"displayName"`
}

func (c Client) ShowTerminal(ctx context.Context, handle string) (Terminal, error) {
	if handle == "" {
		return Terminal{}, errors.New("missing original Orca terminal")
	}
	r, err := c.MetaCall(ctx, "terminal", "show", "--terminal", handle, "--json")
	if err != nil {
		return Terminal{}, err
	}
	var value struct {
		Terminal Terminal `json:"terminal"`
	}
	if err = json.Unmarshal(r.Payload, &value); err != nil {
		return Terminal{}, err
	}
	value.Terminal.RuntimeID = r.RuntimeID
	return value.Terminal, nil
}
func (c Client) InspectBoundTerminal(ctx context.Context, _ b.ExecutionTarget, p b.ProcessIdentity) (Terminal, error) {
	t, err := c.ShowTerminal(ctx, p.Terminal)
	if err != nil {
		return t, err
	}
	if p.RuntimeID == "" || p.Incarnation == "" || t.RuntimeID != p.RuntimeID || t.Handle != p.Terminal || t.IncarnationID != p.Incarnation {
		return t, &b.BackendError{Code: b.BackendIdentityMismatch, Diagnostic: "original Orca runtime/terminal/incarnation changed"}
	}
	return t, nil
}
func ValidateWorkspaceTerminal(t Terminal, path, branch string) error {
	if t.WorktreePath != path || strings.TrimPrefix(t.Branch, "refs/heads/") != branch {
		return errors.New("original Orca workspace/branch changed")
	}
	return nil
}
func (c Client) CloseTerminalChecked(ctx context.Context, handle, runtime string) (b.StopReceipt, error) {
	if handle == "" || runtime == "" {
		return b.StopReceipt{}, errors.New("missing exact terminal close identity")
	}
	r, err := c.MetaCall(ctx, "terminal", "close", "--terminal", handle, "--json")
	if err != nil {
		return b.StopReceipt{}, err
	}
	var value struct {
		Close struct {
			Handle    string `json:"handle"`
			PtyKilled bool   `json:"ptyKilled"`
		} `json:"close"`
	}
	if json.Unmarshal(r.Payload, &value) != nil || r.RuntimeID != runtime || value.Close.Handle != handle {
		return b.StopReceipt{}, errors.New("original terminal close is unverified")
	}
	if value.Close.PtyKilled {
		return b.StopReceipt{Disposition: "confirmed"}, nil
	}
	return b.StopReceipt{Disposition: "accepted"}, nil
}

func (c Client) WaitIdle(ctx context.Context, p b.ProcessIdentity, timeout time.Duration) (bool, error) {
	r, err := c.MetaCall(ctx, "terminal", "wait", "--terminal", p.Terminal, "--for", "tui-idle", "--timeout-ms", strconv.FormatInt(timeout.Milliseconds(), 10), "--json")
	if r.ErrorCode == "timeout" && !r.CallUncertain {
		return false, nil
	}
	var value struct {
		Wait struct {
			Satisfied     bool   `json:"satisfied"`
			BlockedReason string `json:"blockedReason"`
		} `json:"wait"`
	}
	if json.Unmarshal(r.Payload, &value) != nil || r.RuntimeID != p.RuntimeID || r.CallUncertain || r.ErrorCode != "" {
		return false, errors.Join(err, errors.New("original terminal wait is unverified"))
	}
	return value.Wait.Satisfied && value.Wait.BlockedReason == "", nil
}
