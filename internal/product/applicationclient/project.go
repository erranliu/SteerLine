package applicationclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

// Project captures the logical runtime identity once. Reconnecting the host
// never silently rebinds a mutation or its original request ID to a new runtime.
type Project struct {
	client   *Client
	identity httpcontract.Identity
}

func OpenProject(ctx context.Context, home, selector string) (*Project, error) {
	client, err := Open(ctx, home)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			client.Close()
		}
	}()
	var catalog contract.Catalog
	if err = client.Get(ctx, "/projects", &catalog); err != nil {
		return nil, err
	}
	var selected string
	for _, p := range catalog.Projects {
		if selector != "" && selector != p.ProjectID && selector != p.ConfigID {
			continue
		}
		if p.ProjectID == "" || p.Conflict != "" {
			continue
		}
		if selected != "" {
			return nil, errors.New("select one project with --project <project-id>")
		}
		selected = p.ProjectID
	}
	if selected == "" {
		return nil, errors.New("configured project is absent, unresolved or conflicted")
	}
	p := &Project{client: client}
	if err = client.Get(ctx, "/projects/"+selected+"/identity", &p.identity); err != nil {
		return nil, err
	}
	if p.identity.ProjectID != selected {
		return nil, errors.New("project identity differs")
	}
	success = true
	return p, nil
}
func (p *Project) Close() error                    { return p.client.Close() }
func (p *Project) Identity() httpcontract.Identity { return p.identity }
func (p *Project) get(ctx context.Context, path string, target any) error {
	return p.client.Get(ctx, "/projects/"+p.identity.ProjectID+path, target)
}
func (p *Project) post(ctx context.Context, path string, in, out any) error {
	return p.client.Post(ctx, "/projects/"+p.identity.ProjectID+path, in, out)
}
func (p *Project) ShowTask(ctx context.Context, id string) (out api.TaskPage, err error) {
	err = p.get(ctx, "/tasks/"+url.PathEscape(id), &out)
	return
}
func (p *Project) ListTasks(ctx context.Context, cursor string, limit int) (out api.TaskPage, err error) {
	err = p.get(ctx, fmt.Sprintf("/tasks?cursor=%s&limit=%d", url.QueryEscape(cursor), limit), &out)
	return
}
func (p *Project) TaskGraph(ctx context.Context, cursor string, limit int) (out api.GraphPage, err error) {
	err = p.get(ctx, fmt.Sprintf("/graph?cursor=%s&limit=%d", url.QueryEscape(cursor), limit), &out)
	return
}
func (p *Project) ReadProject(ctx context.Context, task string, previous api.ProjectSnapshot) api.ProjectSnapshot {
	var out api.ProjectSnapshot
	if err := p.get(ctx, "/snapshot?task_id="+url.QueryEscape(task), &out); err != nil {
		previous.NativeTaskError = err.Error()
		previous.Connection.Diagnostic = err.Error()
		previous.Connection.CanControl = false
		return previous
	}
	return out
}
func (p *Project) task(ctx context.Context, route string, body, out any) error {
	return p.post(ctx, route, map[string]any{"project_id": p.identity.ProjectID, "instance_id": p.identity.InstanceID, "payload": body}, out)
}
func (p *Project) ValidateTaskBatch(ctx context.Context, body []byte) (out api.TaskBatchValidation, err error) {
	err = p.task(ctx, "/task-batch-validation", json.RawMessage(body), &out)
	return
}
func (p *Project) CreateTaskBatch(ctx context.Context, in api.CreateInput) (out api.TaskBatchReceipt, err error) {
	body := struct {
		Payload                  json.RawMessage `json:"payload"`
		RequestID                string          `json:"request_id"`
		RequestedBy              string          `json:"requested_by"`
		ExpectedAuthorityVersion uint64          `json:"expected_authority_version"`
	}{json.RawMessage(in.Payload), in.RequestID, in.RequestedBy, in.ExpectedAuthorityVersion}
	err = p.task(ctx, "/task-batches", body, &out)
	return
}
func (p *Project) AddPredecessors(ctx context.Context, in api.TaskPredecessorAddInput) (out api.TaskPredecessorAddReceipt, err error) {
	err = p.task(ctx, "/tasks/"+in.TaskID+"/predecessors", in, &out)
	return
}
func (p *Project) TaskBatchReceipt(ctx context.Context, id string) (out api.TaskBatchReceipt, err error) {
	err = p.get(ctx, "/task-batches/"+id, &out)
	return
}
func (p *Project) PredecessorReceipt(ctx context.Context, task, id string) (out api.TaskPredecessorAddReceipt, err error) {
	err = p.get(ctx, "/tasks/"+task+"/predecessors/requests/"+id, &out)
	return
}
func (p *Project) submit(ctx context.Context, envelope httpcontract.CommandEnvelope) (out httpcontract.CommandResponse, err error) {
	err = p.post(ctx, "/commands", envelope, &out)
	return
}
func (p *Project) envelope(id, action, target string, version, revision uint64, payload any) httpcontract.CommandEnvelope {
	body, _ := json.Marshal(payload)
	return httpcontract.CommandEnvelope{RequestID: id, ProjectID: p.identity.ProjectID, InstanceID: p.identity.InstanceID, Action: action, TargetID: target, ExpectedVersion: version, ExpectedControlRevision: revision, Payload: body}
}
func (p *Project) DiscardTask(ctx context.Context, in api.DiscardInput) (api.TaskDiscardReceipt, error) {
	response, err := p.submit(ctx, p.envelope(in.RequestID, "discard_task", in.TaskID, in.ExpectedVersion, 0, map[string]string{"reason": in.Reason}))
	if err != nil {
		return api.TaskDiscardReceipt{}, err
	}
	if response.Discard == nil {
		return api.TaskDiscardReceipt{}, errors.New("original discard outcome unverified")
	}
	return *response.Discard, nil
}
func (p *Project) StartTaskPayload(ctx context.Context, task, profile string) (api.ControlPayload, error) {
	if profile == "" {
		return api.ControlPayload{}, errors.New("start_task requires -execution-profile")
	}
	var presentation httpcontract.ProjectPresentation
	if err := p.get(ctx, "/presentation", &presentation); err != nil {
		return api.ControlPayload{}, err
	}
	for _, candidate := range presentation.Presentation.Profiles {
		if candidate.ID == profile {
			return api.ControlPayload{TaskID: task, ExecutionProfileID: profile, ExecutionProfileDigest: candidate.Digest}, nil
		}
	}
	return api.ControlPayload{}, errors.New("execution profile is not available in this project")
}
func (p *Project) SubmitControl(ctx context.Context, in api.ControlInput) (api.ControlRequest, error) {
	if in.Action == api.StopService {
		return api.ControlRequest{}, errors.New("stop_service is retired; use steerline stop")
	}
	snapshot := p.ReadProject(ctx, in.Payload.TaskID, api.ProjectSnapshot{})
	page := snapshot.Connection.Snapshot.Page
	if page == nil {
		return api.ControlRequest{}, errors.New("current project snapshot unavailable")
	}
	revision := page.Control.Revision
	if in.ExpectedRevision != nil {
		revision = *in.ExpectedRevision
	}
	instance := p.identity.InstanceID
	if in.InstanceID != "" {
		instance = in.InstanceID
	}
	target, version, payload := "dispatch", revision, any(map[string]any{})
	switch in.Action {
	case api.StartTask, api.StartCandidate:
		if snapshot.SelectedNative == nil {
			return api.ControlRequest{}, errors.New("task snapshot unavailable")
		}
		target = in.Payload.TaskID
		version = snapshot.SelectedNative.Version
		payload = map[string]string{"execution_profile_id": in.Payload.ExecutionProfileID}
	case api.SetDispatchMode:
		payload = map[string]string{"mode": in.Payload.Mode}
	case api.OpenTerminal, api.RecheckEnvironment:
		if in.Payload.Environment == nil {
			return api.ControlRequest{}, errors.New("exact environment target required")
		}
		target = in.Payload.Environment.RunID
		version = in.Payload.Environment.Version
		payload = map[string]any{"target": in.Payload.Environment}
	}
	envelope := p.envelope(in.RequestID, string(in.Action), target, version, revision, payload)
	envelope.InstanceID = instance
	if in.Action == api.StartCandidate {
		envelope.ExpectedGraphRevision = in.Payload.ExpectedGraphRevision
	}
	response, err := p.submit(ctx, envelope)
	if err != nil {
		return api.ControlRequest{}, err
	}
	if response.ControlRequest == nil {
		return api.ControlRequest{}, errors.New("original control request unverified")
	}
	return *response.ControlRequest, nil
}
func (p *Project) response(ctx context.Context, id string) (out httpcontract.CommandResponse, err error) {
	err = p.get(ctx, "/commands/"+id, &out)
	return
}
func (p *Project) Receipt(ctx context.Context, id string) (*api.ControlReceipt, error) {
	out, err := p.response(ctx, id)
	if err != nil {
		return nil, err
	}
	if out.ControlReceipt == nil {
		return nil, errors.New("original control receipt pending")
	}
	return out.ControlReceipt, nil
}
func (p *Project) Command(ctx context.Context, id string) (api.CommandReceipt, error) {
	out, err := p.response(ctx, id)
	if err != nil {
		return api.CommandReceipt{}, err
	}
	if out.Command == nil {
		return api.CommandReceipt{}, errors.New("original command receipt unverified")
	}
	return *out.Command, nil
}
func (p *Project) WaitControl(ctx context.Context, id string, poll time.Duration) (*api.ControlReceipt, error) {
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		out, err := p.Receipt(ctx, id)
		if err == nil && out.Status != "applying" {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("original request %s remains unverified: %w", id, ctx.Err())
		case <-tick.C:
		}
	}
}
func (p *Project) WaitCommand(ctx context.Context, id string, wait time.Duration) (api.CommandReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		out, err := p.Command(ctx, id)
		if err == nil && (out.Status == "accepted" || out.Status == "rejected" || out.Status == "failed" || out.Status == "indeterminate") {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, fmt.Errorf("original command %s remains unverified: %w", id, ctx.Err())
		case <-tick.C:
		}
	}
}
func (p *Project) Answer(ctx context.Context, in api.AnswerInput) (api.CommandReceipt, bool, error) {
	var run api.CandidateSummary
	if err := p.get(ctx, "/runs/"+in.RunID, &run); err != nil {
		return api.CommandReceipt{}, false, err
	}
	if run.Decision == nil || in.DecisionID != "" && in.DecisionID != run.Decision.ID {
		return api.CommandReceipt{}, false, errors.New("original decision unavailable or changed")
	}
	payload := map[string]any{"run_id": run.RunID, "decision_id": run.Decision.ID, "option_id": in.Option, "note": in.Note, "expected_run_revision": run.ControlRevision, "source_attempt_id": run.AttemptID, "expected_recovery_version": run.RecoveryVersion}
	response, err := p.submit(ctx, p.envelope(in.CommandID, "answer_decision", run.Decision.ID, run.Decision.Version, 0, payload))
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	if response.Command == nil {
		return api.CommandReceipt{}, false, errors.New("original answer receipt unverified")
	}
	return *response.Command, true, nil
}
