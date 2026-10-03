package console

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerlineclient"
)

// ProjectRef is the identity of a console view. ConfigPath and RunDir are both
// part of the key so two configs for one repository keep separate view state.
type ProjectRef struct {
	Key             string
	Name            string
	ConfigPath      string
	Config          steerline.Config
	ApplicationHome string
	ApplicationBin  string
	Identity        string
	RunDir          string
	Error           string
	Conflict        string
}

type ProjectConfigFile struct {
	Name string
	Path string
	Err  error
}

// ConfigSource is supplied by the outer application. The console has no
// default loader or implementation directory of its own.
type ConfigSource interface {
	ConfigFiles() ([]ProjectConfigFile, error)
	LoadConfig(string) (steerline.Config, error)
	ApplicationPaths() (home, bin, etc string)
}

type ProjectSnapshot = api.ProjectSnapshot
type LogBatch = api.LogBatch

type QuerySource interface {
	ReadProject(context.Context, ProjectRef, string, ProjectSnapshot) ProjectSnapshot
	ReadActivity(context.Context, ProjectRef, string, string, string, string, steerline.JournalCursor) (LogBatch, error)
	ReadConversation(context.Context, ProjectRef, steerline.ConversationFilter, string) (steerline.ConversationPage, error)
	ReadBody(context.Context, ProjectRef, steerline.ConversationBodyRef, int64, int) (steerline.ConversationBodyPage, error)
	ReadCapabilities(context.Context, ProjectRef, executionapi.BackendKind, executionapi.ExecutionProtocol, string, string, string, string, string) ([]steerline.ConversationCapabilityReport, error)
}

type ControlClient interface {
	Submit(context.Context, ProjectRef, string, any, string, string, *uint64) (steerline.ControlRequest, error)
	Receipt(context.Context, ProjectRef, string) (*steerline.ControlReceipt, error)
	Request(context.Context, ProjectRef, string) (*steerline.ControlRequest, error)
}

type DecisionCommandClient interface {
	DecisionEligibility(context.Context, ProjectRef, *steerline.Run) error
	SubmitDecision(context.Context, ProjectRef, steerline.DecisionAnswer, string, string) (steerline.CommandRecord, bool, error)
	DecisionCommand(context.Context, ProjectRef, string) (steerline.CommandRecord, error)
}

// These bridges only map the not-yet-migrated TUI to the product client.
// #72 deletes them and ProjectRef.Config together.
type FileQuerySource struct{ Factory api.Factory }
type FileControlClient struct{ Factory api.Factory }

func productRef(ref ProjectRef) api.ProjectRef {
	return api.ProjectRef{Key: ref.Key, Name: ref.Name, ConfigPath: ref.ConfigPath, Identity: ref.Identity, RunDir: ref.RunDir}
}
func withClient[T any](ctx context.Context, ref ProjectRef, factory api.Factory, fn func(api.Client) (T, error)) (result T, err error) {
	if ref.Error != "" {
		return result, errors.New(ref.Error)
	}
	client, err := steerlineclient.Open(ctx, productRef(ref), factory)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	return fn(client)
}
func (f FileQuerySource) ReadProject(ctx context.Context, ref ProjectRef, selected string, previous ProjectSnapshot) ProjectSnapshot {
	result, err := withClient(ctx, ref, f.Factory, func(c api.Client) (ProjectSnapshot, error) {
		return c.ReadProject(ctx, selected, previous), nil
	})
	if err != nil {
		return ProjectSnapshot{SelectedTaskID: selected, TaskError: err.Error(), DomainError: err.Error()}
	}
	return result
}
func (f FileQuerySource) ReadActivity(ctx context.Context, ref ProjectRef, runID, executionID, attemptID, path string, cursor steerline.JournalCursor) (LogBatch, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (LogBatch, error) {
		return c.Activity(ctx, runID, executionID, attemptID, path, cursor)
	})
}
func (f FileQuerySource) ReadConversation(ctx context.Context, ref ProjectRef, filter steerline.ConversationFilter, cursor string) (steerline.ConversationPage, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (steerline.ConversationPage, error) {
		return c.Conversation(ctx, filter, cursor)
	})
}
func (f FileQuerySource) ReadBody(ctx context.Context, ref ProjectRef, body steerline.ConversationBodyRef, offset int64, limit int) (steerline.ConversationBodyPage, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (steerline.ConversationBodyPage, error) {
		return c.Body(ctx, body, offset, limit)
	})
}
func (f FileQuerySource) ReadCapabilities(ctx context.Context, ref ProjectRef, backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, targetID, executionID, attemptID, source, sourceIdentity string) ([]steerline.ConversationCapabilityReport, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) ([]steerline.ConversationCapabilityReport, error) {
		return c.Capabilities(ctx, api.CapabilityFilter{Backend: backend, Protocol: protocol, TargetID: targetID, ExecutionID: executionID, AttemptID: attemptID, Source: source, SourceIdentity: sourceIdentity})
	})
}
func (f FileControlClient) Submit(ctx context.Context, ref ProjectRef, action string, payload any, id, instance string, revision *uint64) (steerline.ControlRequest, error) {
	var p api.ControlPayload
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return steerline.ControlRequest{}, err
		}
		if err = contracts.DecodeJSON(raw, &p); err != nil {
			return steerline.ControlRequest{}, err
		}
	}
	return withClient(ctx, ref, f.Factory, func(c api.Client) (steerline.ControlRequest, error) {
		return c.SubmitControl(ctx, api.ControlInput{Action: api.ControlAction(action), Payload: p, RequestID: id, RequestedBy: "", InstanceID: instance, ExpectedRevision: revision})
	})
}
func (f FileControlClient) Receipt(ctx context.Context, ref ProjectRef, id string) (*steerline.ControlReceipt, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (*steerline.ControlReceipt, error) { return c.Receipt(ctx, id) })
}
func (f FileControlClient) Request(ctx context.Context, ref ProjectRef, id string) (*steerline.ControlRequest, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (*steerline.ControlRequest, error) { return c.Request(ctx, id) })
}
func (f FileControlClient) DecisionEligibility(ctx context.Context, ref ProjectRef, run *steerline.Run) error {
	_, err := withClient(ctx, ref, f.Factory, func(c api.Client) (struct{}, error) { return struct{}{}, c.DecisionEligibility(ctx, run) })
	return err
}
func (f FileControlClient) SubmitDecision(ctx context.Context, ref ProjectRef, answer steerline.DecisionAnswer, by, id string) (steerline.CommandRecord, bool, error) {
	type result struct {
		Record steerline.CommandRecord
		Fresh  bool
	}
	r, err := withClient(ctx, ref, f.Factory, func(c api.Client) (result, error) {
		v, fresh, err := c.SubmitDecision(ctx, api.DecisionInput{Answer: answer, RequestedBy: by, CommandID: id})
		return result{v, fresh}, err
	})
	return r.Record, r.Fresh, err
}
func (f FileControlClient) DecisionCommand(ctx context.Context, ref ProjectRef, id string) (steerline.CommandRecord, error) {
	return withClient(ctx, ref, f.Factory, func(c api.Client) (steerline.CommandRecord, error) { return c.Command(ctx, id) })
}
func candidateFreshness(ref ProjectRef, snapshot ProjectSnapshot, now time.Time) (bool, string) {
	return steerlineclient.CandidateFreshness(ref.Identity, snapshot, now)
}

func absoluteViewKey(configPath, runDir string) (string, error) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(runDir) == "" {
		return viewCanonical(configPath) + "\x00", nil
	}
	runDir, err = filepath.Abs(runDir)
	if err != nil {
		return "", err
	}
	return viewCanonical(configPath) + "\x00" + viewCanonical(runDir), nil
}

func viewCanonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
