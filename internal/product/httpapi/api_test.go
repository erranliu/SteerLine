package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
)

type apiReader struct {
	snapshot        api.ProjectSnapshot
	page            api.TaskPage
	graph           api.GraphPage
	activity        api.LogBatch
	commands        map[string]api.CommandReceipt
	commandErr      error
	listCursor      string
	listLimit       int
	decisionSubmits int
}

func (r *apiReader) Close() error { return nil }
func (r *apiReader) ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot {
	return r.snapshot
}
func (r *apiReader) RecentActivity(context.Context, string, string, string) (api.LogBatch, error) {
	return r.activity, nil
}
func (r *apiReader) Activity(_ context.Context, run, execution, attempt string, cursor api.ActivityCursor) (api.LogBatch, error) {
	if run != r.activity.Cursor.RunID || execution != r.activity.Cursor.ExecutionID || attempt != r.activity.Cursor.AttemptID {
		return api.LogBatch{}, errors.New("activity identity mismatch")
	}
	if cursor.Initialized && cursor.AfterEventID != r.activity.Cursor.AfterEventID {
		return api.LogBatch{}, errors.New("cursor_invalidated")
	}
	return r.activity, nil
}
func (r *apiReader) ShowTask(context.Context, string) (api.TaskPage, error) { return r.page, nil }
func (r *apiReader) ListTasks(_ context.Context, cursor string, limit int) (api.TaskPage, error) {
	r.listCursor, r.listLimit = cursor, limit
	return r.page, nil
}
func (r *apiReader) TaskGraph(context.Context, string, int) (api.GraphPage, error) {
	return r.graph, nil
}
func (r *apiReader) Conversation(context.Context, api.ConversationFilter, string, int) (api.ConversationPage, error) {
	return api.ConversationPage{Events: []api.ConversationEvent{}}, nil
}
func (r *apiReader) Body(_ context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	data := []byte("the final reply")
	if offset >= int64(len(data)) {
		return api.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: offset, TotalBytes: int64(len(data)), SHA256: ref.SHA256, Complete: true, Data: []byte{}}, nil
	}
	end := int(offset) + limit
	if end > len(data) {
		end = len(data)
	}
	chunk := data[offset:end]
	return api.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: int64(end), TotalBytes: int64(len(data)), SHA256: ref.SHA256, Completeness: api.BodyComplete, Complete: end == len(data), Data: chunk}, nil
}
func (r *apiReader) Command(_ context.Context, id string) (api.CommandReceipt, error) {
	if r.commandErr != nil {
		return api.CommandReceipt{}, r.commandErr
	}
	if value, ok := r.commands[id]; ok {
		return value, nil
	}
	return api.CommandReceipt{}, api.ErrRecordNotFound
}

type apiCommands struct{}

func (apiCommands) Close() error { return nil }
func (apiCommands) Validate(context.Context, []byte) (api.TaskBatchValidation, error) {
	return api.TaskBatchValidation{}, nil
}
func (apiCommands) Create(context.Context, api.CreateInput) (api.TaskBatchReceipt, error) {
	return api.TaskBatchReceipt{}, nil
}
func (apiCommands) Discard(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error) {
	return api.TaskDiscardReceipt{}, nil
}
func (apiCommands) AddPredecessors(context.Context, api.TaskPredecessorAddInput) (api.TaskPredecessorAddReceipt, error) {
	return api.TaskPredecessorAddReceipt{}, nil
}

type apiControl struct {
	requests  map[string]api.ControlRequest
	receipts  map[string]*api.ControlReceipt
	submits   int
	failRead  error
	submitErr error
}

func (c *apiControl) Submit(_ context.Context, input api.ControlInput) (api.ControlRequest, error) {
	if c.submitErr != nil {
		return api.ControlRequest{}, c.submitErr
	}
	c.submits++
	var payload json.RawMessage
	if input.Payload != (api.ControlPayload{}) {
		payload, _ = json.Marshal(input.Payload)
	}
	revision := uint64(0)
	if input.ExpectedRevision != nil {
		revision = *input.ExpectedRevision
	}
	request := api.ControlRequest{ProtocolVersion: 1, RequestID: input.RequestID, RepositoryIdentity: "C:/repo",
		InstanceID: input.InstanceID, Action: string(input.Action), ExpectedControlRevision: revision, Payload: payload,
		RequestedBy: input.RequestedBy, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	c.requests[input.RequestID] = request
	return request, nil
}
func (c *apiControl) SubmitInbox(ctx context.Context, input api.ControlInput) (api.ControlRequest, error) {
	return c.Submit(ctx, input)
}
func (c *apiControl) Request(_ context.Context, id string) (*api.ControlRequest, error) {
	if c.failRead != nil {
		return nil, c.failRead
	}
	request, ok := c.requests[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &request, nil
}
func (c *apiControl) Receipt(_ context.Context, id string) (*api.ControlReceipt, error) {
	receipt, ok := c.receipts[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return receipt, nil
}
func (c *apiControl) Stop(context.Context, string, time.Duration) (api.ServiceStopResult, error) {
	return api.ServiceStopResult{}, nil
}

type apiDecision struct {
	reader  *apiReader
	submits int
}

func (apiDecision) Eligibility(context.Context, *api.CandidateSummary) error { return nil }
func (d *apiDecision) Submit(_ context.Context, input api.DecisionInput) (api.CommandReceipt, bool, error) {
	d.submits++
	d.reader.decisionSubmits++
	now := time.Now().UTC()
	receipt := api.CommandReceipt{ProjectIdentity: "C:/repo", CommandID: input.CommandID, Kind: "answer_decision",
		TargetID: input.Answer.DecisionID, ExpectedVersion: input.Answer.ExpectedDecisionVersion, RequestedBy: input.RequestedBy,
		Status: "accepted", CreatedAt: now, UpdatedAt: now}
	d.reader.commands[input.CommandID] = receipt
	return receipt, true, nil
}
func (d *apiDecision) Answer(ctx context.Context, input api.AnswerInput) (api.CommandReceipt, bool, error) {
	return d.Submit(ctx, api.DecisionInput{CommandID: input.CommandID, RequestedBy: input.RequestedBy, Answer: api.DecisionAnswer{
		RunID: input.RunID, DecisionID: input.DecisionID, OptionID: input.Option, Note: input.Note,
		AnsweredBy: input.RequestedBy, ExpectedDecisionVersion: input.ExpectedVersion}})
}

func newAPIFixture(t *testing.T) (*Server, *productclient.Client, *apiReader, *apiControl) {
	t.Helper()
	ctx := context.Background()
	runDir := t.TempDir()
	now := time.Now().UTC()
	page := api.TaskPage{Protocol: "native-tasks/v1", ProjectID: "project", AuthorityVersion: 8, GraphRevision: 4, DatabaseRevision: 21, DatabaseID: "db-a", SampledAt: now, Tasks: []api.TaskDetail{}}
	reader := &apiReader{page: page, graph: api.GraphPage{Protocol: api.TaskGraphProtocol, ProjectID: "project", AuthorityVersion: 8, GraphRevision: 4, DatabaseRevision: 21, DatabaseID: "db-a", SampledAt: now, Nodes: []api.GraphNode{}, Edges: []api.GraphEdge{}},
		activity: api.LogBatch{Cursor: api.ActivityCursor{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32), Initialized: true}}, commands: map[string]api.CommandReceipt{}}
	capabilities := []string{"answer_decision", "open_terminal", "recheck_environment", "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	servicePage := &api.ServicePage{Service: api.ServiceSummary{InstanceID: "instance-a", Status: "running", Ready: true, ControlProtocol: 1, Capabilities: capabilities}, Control: api.ControlSummary{Revision: 7}, Runs: []*api.CandidateSummary{}}
	reader.snapshot = api.ProjectSnapshot{SelectedTaskID: "", FullReadAt: now,
		Connection:  api.ServiceConnection{State: api.ConnectionConnected, IdentityMatched: true, ConfigMatched: true, CanControl: true, CanStop: true, Snapshot: api.ObservationSnapshot{Page: servicePage, State: api.SnapshotValid, SampledAt: now}},
		Observation: api.ObservationSnapshot{Page: servicePage, State: api.SnapshotValid, SampledAt: now}}
	control := &apiControl{requests: map[string]api.ControlRequest{}, receipts: map[string]*api.ControlReceipt{}}
	decision := &apiDecision{reader: reader}
	ref := api.ProjectRef{ConfigPath: filepath.Join(runDir, "project.json"), Identity: "C:/repo", RunDir: runDir}
	client, err := productclient.Open(ctx, ref, func(context.Context, api.ProjectRef) (productclient.Resources, error) {
		return productclient.Resources{Project: ref, OpenReader: func(context.Context) (productclient.Reader, error) { return reader, nil },
			OpenCommands: func(context.Context, bool) (productclient.TaskCommands, error) { return apiCommands{}, nil },
			Control:      control, Decision: decision, DefaultRequestedBy: "tester",
			ResolveStartTask: func(_ context.Context, taskID, profileID string) (api.ControlPayload, error) {
				return api.ControlPayload{TaskID: taskID, ExecutionProfileID: profileID, ExecutionProfileDigest: strings.Repeat("a", 64)}, nil
			}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server, err := Prepare(ctx, client, Options{RunDir: runDir, ProjectIdentity: ref.Identity, InstanceID: "instance-a",
		Process:      ProcessIdentity{Kind: "os", PID: os.Getpid(), Created: "created-1", Executable: "steerline.exe"},
		Capabilities: capabilities})
	if err != nil {
		t.Fatal(err)
	}
	if err = server.SetReady("instance-a", true, ""); err != nil {
		t.Fatal(err)
	}
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Close(closeCtx); err != nil {
			t.Errorf("close test API: %v", err)
		}
	})
	return server, client, reader, control
}

func TestAPIIdentityAndAuthentication(t *testing.T) {
	server, _, _, _ := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if client.Identity().InstanceID != "instance-a" || !client.Identity().Ready || client.Identity().Protocol != Protocol {
		t.Fatalf("unexpected live API identity: %+v", client.Identity())
	}
	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/identity", nil)
	request.Host = server.listener.Addr().String()
	request.RemoteAddr = "127.0.0.1:49152"
	request.Header.Set("Authorization", "Bearer incorrect")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid credential status = %d", response.Code)
	}
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Origin", "https://example.test")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "origin_rejected") {
		t.Fatalf("browser origin was not rejected: %d %s", response.Code, response.Body.String())
	}
	request.Header.Del("Origin")
	request.Host = "localhost:9999"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("wrong Host status = %d", response.Code)
	}
	request.Host = server.listener.Addr().String()
	request.RemoteAddr = "192.0.2.9:49152"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "source_rejected") {
		t.Fatalf("non-loopback source was not rejected: %d %s", response.Code, response.Body.String())
	}
}

func TestAPIQueryParity(t *testing.T) {
	server, _, reader, _ := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListTasks(context.Background(), "cursor-a", 17)
	if err != nil {
		t.Fatal(err)
	}
	if page.DatabaseID != "db-a" || page.DatabaseRevision != 21 || page.GraphRevision != 4 || reader.listCursor != "cursor-a" || reader.listLimit != 17 {
		t.Fatalf("query dropped existing page identity or cursor: page=%+v cursor=%q limit=%d", page, reader.listCursor, reader.listLimit)
	}
	graph, err := client.Graph(context.Background(), "", 40)
	if err != nil || graph.DatabaseID != "db-a" || graph.GraphRevision != 4 {
		t.Fatalf("graph did not preserve the source snapshot: %+v %v", graph, err)
	}
	if _, err := client.ListTasks(context.Background(), "", 501); err == nil {
		t.Fatal("oversized task page limit was accepted")
	}
	prURL := "https://github.com/example/repo/pull/45"
	mergeCommit := strings.Repeat("e", 40)
	reader.snapshot.SelectedNative = &api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: "8", Version: 3}, Delivery: &api.DeliverySummary{
		PRURL: prURL, WorkflowRunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), MergedAt: time.Now().UTC()}}
	reader.snapshot.History = &api.HistorySummary{Decisions: []api.DecisionHistory{{WorkflowRunID: strings.Repeat("a", 32), DecisionID: strings.Repeat("d", 32), Status: "pending"}}}
	run := &api.CandidateSummary{RunID: strings.Repeat("a", 32), TaskID: "8", ExecutionID: strings.Repeat("b", 32),
		AttemptID: strings.Repeat("c", 32), PR: api.PRSummary{URL: &prURL, State: "merged", MergeCommit: &mergeCommit}}
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{run}
	reader.snapshot.Observation.Page = reader.snapshot.Connection.Snapshot.Page
	snapshot, err := client.Snapshot(context.Background(), "8")
	if err != nil || snapshot.SelectedNative == nil || snapshot.SelectedNative.Delivery == nil || snapshot.SelectedNative.Delivery.PRURL != prURL ||
		snapshot.Connection.Snapshot.Page == nil || snapshot.Connection.Snapshot.Page.Runs[0].PR.MergeCommit == nil || *snapshot.Connection.Snapshot.Page.Runs[0].PR.MergeCommit != mergeCommit ||
		snapshot.History == nil || snapshot.History.Decisions[0].DecisionID != strings.Repeat("d", 32) {
		t.Fatalf("HTTP snapshot changed product Decision/PR/delivery DTOs: %+v %v", snapshot, err)
	}
}

func TestAPIOriginalCommandReceipt(t *testing.T) {
	server, _, _, control := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	envelope := CommandEnvelope{RequestID: strings.Repeat("9", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a",
		Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
	first, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || first.ControlRequest == nil || first.Status != "submitted" || control.submits != 1 {
		t.Fatalf("original control request was not durably submitted: %+v %v submits=%d", first, err, control.submits)
	}
	pending, err := client.Command(context.Background(), envelope.RequestID)
	if err != nil || pending.Status != "submitted" || pending.ControlRequest == nil || pending.Original == nil {
		t.Fatalf("delayed control receipt was not represented as the original submitted request: %+v %v", pending, err)
	}
	control.receipts[envelope.RequestID] = &api.ControlReceipt{Version: 1, RequestID: envelope.RequestID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, Status: "applied", Code: "applied", Message: "control action applied"}
	applied, err := client.Command(context.Background(), envelope.RequestID)
	if err != nil || applied.Status != "applied" || applied.ControlReceipt == nil || applied.ControlReceipt.RequestID != envelope.RequestID {
		t.Fatalf("receipt query lost the original request binding: %+v %v", applied, err)
	}
	replayed, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || replayed.ControlReceipt == nil || replayed.ControlReceipt.RequestID != envelope.RequestID || control.submits != 1 {
		t.Fatalf("late duplicate did not reconcile to the original receipt: %+v %v submits=%d", replayed, err, control.submits)
	}
}

func TestAPIReconnectAndUnsupported(t *testing.T) {
	server, _, reader, control := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	requestID := strings.Repeat("1", 32)
	envelope := CommandEnvelope{RequestID: requestID, ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "pause_dispatch",
		TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
	first, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || first.Status != "submitted" || control.submits != 1 {
		t.Fatalf("first command was not submitted once: %+v %v submits=%d", first, err, control.submits)
	}
	second, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || second.RequestID != requestID || control.submits != 1 {
		t.Fatalf("same original envelope repeated a side effect: %+v %v submits=%d", second, err, control.submits)
	}
	var parallel sync.WaitGroup
	errorsFound := make(chan error, 8)
	for i := 0; i < 8; i++ {
		parallel.Add(1)
		go func() {
			defer parallel.Done()
			_, submitErr := client.SubmitCommand(context.Background(), envelope)
			errorsFound <- submitErr
		}()
	}
	parallel.Wait()
	close(errorsFound)
	for submitErr := range errorsFound {
		if submitErr != nil {
			t.Fatalf("late concurrent duplicate was not reconciled: %v", submitErr)
		}
	}
	if control.submits != 1 {
		t.Fatalf("late duplicate competition produced %d control submissions", control.submits)
	}
	conflict := envelope
	conflict.Action = "resume_dispatch"
	if _, err = client.SubmitCommand(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "request_id_conflict") || control.submits != 1 {
		t.Fatalf("same request ID with changed content was not rejected: %v submits=%d", err, control.submits)
	}
	response, err := client.Command(context.Background(), requestID)
	if err != nil || response.Status != "submitted" || response.ControlRequest == nil || response.Original == nil {
		t.Fatalf("original control request was not queryable: %+v %v", response, err)
	}
	unsupported := envelope
	unsupported.RequestID = strings.Repeat("2", 32)
	unsupported.Action = "planning_publish"
	if _, err = client.SubmitCommand(context.Background(), unsupported); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown action did not return unsupported: %v", err)
	}
	unsupportedReceipt, err := client.Command(context.Background(), unsupported.RequestID)
	if err != nil || unsupportedReceipt.Status != "rejected" || unsupportedReceipt.Code != "unsupported" {
		t.Fatalf("unsupported action was not retained as an unsupported receipt: %+v %v", unsupportedReceipt, err)
	}
	unregistered := envelope
	unregistered.RequestID = strings.Repeat("6", 32)
	canonical, _ := json.Marshal(unregistered)
	record := requestRecord{Version: 1, Digest: digestBytes(canonical), Envelope: unregistered, CreatedAt: time.Now().UTC()}
	recordData, _ := json.Marshal(record)
	if err = writePrivateExclusive(server.commandsDir, server.commandsDir+string(os.PathSeparator)+unregistered.RequestID+".json", append(recordData, '\n')); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Command(context.Background(), unregistered.RequestID); err == nil || !strings.Contains(err.Error(), "command_not_found") {
		t.Fatalf("request registered before dispatch was not authoritatively absent: %v", err)
	}
	if _, err = client.SubmitCommand(context.Background(), unregistered); err != nil || control.submits != 2 {
		t.Fatalf("same registered request did not resume through one product submission: err=%v submits=%d", err, control.submits)
	}
	permissionDenied := envelope
	permissionDenied.RequestID = strings.Repeat("3", 32)
	control.submitErr = &api.Error{Kind: api.ReadFailed, Message: "access is denied writing the original control request"}
	if _, err = client.SubmitCommand(context.Background(), permissionDenied); err == nil || !strings.Contains(err.Error(), "permission_denied") {
		t.Fatalf("control permission error was not reported explicitly: %v", err)
	}
	permissionReceipt, err := client.Command(context.Background(), permissionDenied.RequestID)
	if err != nil || permissionReceipt.Status != "rejected" || permissionReceipt.Code != "permission_denied" {
		t.Fatalf("permission rejection did not remain bound to its original request: %+v %v", permissionReceipt, err)
	}
	control.submitErr = nil
	reader.commandErr = &api.Error{Kind: api.ReadFailed, Message: "permission denied reading SQLite"}
	if _, err = client.Command(context.Background(), strings.Repeat("4", 32)); err == nil || !strings.Contains(err.Error(), "read_failed") || strings.Contains(err.Error(), "command_not_found") {
		t.Fatalf("SQLite permission/read failure was disguised as command_not_found: %v", err)
	}
	reader.commandErr = nil
	control.failRead = errors.New("control receipt reader is unavailable")
	if _, err = client.Command(context.Background(), strings.Repeat("5", 32)); err == nil || strings.Contains(err.Error(), "command_not_found") {
		t.Fatalf("control read failure was disguised as command_not_found: %v", err)
	}
}

func TestAPIStartTaskAndCandidateVersionMapping(t *testing.T) {
	server, _, reader, control := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	reader.snapshot.SelectedNative = &api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: "8", Version: 3}}
	reader.snapshot.NativeTasks = &api.TaskPage{ProjectID: "project", AuthorityVersion: 8, GraphRevision: 4, DatabaseID: "db-a"}
	profile := json.RawMessage(`{"execution_profile_id":"fixture"}`)
	startTask := CommandEnvelope{RequestID: strings.Repeat("4", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_task",
		TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, Payload: profile}
	if _, err = client.SubmitCommand(context.Background(), startTask); err != nil {
		t.Fatal(err)
	}
	var taskControl api.ControlPayload
	if err = json.Unmarshal(control.requests[startTask.RequestID].Payload, &taskControl); err != nil {
		t.Fatal(err)
	}
	if taskControl.ExpectedTaskVersion != 3 || taskControl.ExpectedGraphRevision != 0 {
		t.Fatalf("start_task changed its task version or received an unsupported graph field: %+v", taskControl)
	}
	startCandidate := startTask
	startCandidate.RequestID = strings.Repeat("5", 32)
	startCandidate.Action = "start_candidate"
	startCandidate.ExpectedGraphRevision = 4
	if _, err = client.SubmitCommand(context.Background(), startCandidate); err != nil {
		t.Fatal(err)
	}
	var candidateControl api.ControlPayload
	if err = json.Unmarshal(control.requests[startCandidate.RequestID].Payload, &candidateControl); err != nil {
		t.Fatal(err)
	}
	if candidateControl.ExpectedTaskVersion != 3 || candidateControl.ExpectedGraphRevision != 4 {
		t.Fatalf("start_candidate lost task/graph versions: %+v", candidateControl)
	}
}

func TestAPIAnswerDecisionReceiptAndUniqueRequest(t *testing.T) {
	server, _, reader, _ := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	runID, executionID, attemptID, decisionID := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{{RunID: runID, TaskID: "8", ExecutionID: executionID,
		AttemptID: attemptID, ControlRevision: 7, RecoveryVersion: 2, Decision: &api.DecisionSummary{ID: decisionID, Status: "pending", Version: 3}}}
	reader.snapshot.Observation.Page = reader.snapshot.Connection.Snapshot.Page
	payload, _ := json.Marshal(map[string]any{"run_id": runID, "decision_id": decisionID, "option_id": "continue", "note": "continue original work",
		"expected_run_revision": 7, "source_attempt_id": attemptID, "expected_recovery_version": 2})
	envelope := CommandEnvelope{RequestID: strings.Repeat("7", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a",
		Action: "answer_decision", TargetID: decisionID, ExpectedVersion: 3, Payload: payload}
	first, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || first.Command == nil || first.Status != "accepted" || reader.decisionSubmits != 1 {
		t.Fatalf("answer command did not keep its original ID and Decision target: %+v %v calls=%d", first, err, reader.decisionSubmits)
	}
	second, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || second.Command == nil || second.Command.CommandID != envelope.RequestID || reader.decisionSubmits != 1 {
		t.Fatalf("replaying the original answer created another command: %+v %v calls=%d", second, err, reader.decisionSubmits)
	}
	queried, err := client.Command(context.Background(), envelope.RequestID)
	if err != nil || queried.Command == nil || queried.Command.TargetID != decisionID || queried.Command.ExpectedVersion != 3 || queried.Status != "accepted" {
		t.Fatalf("original Decision receipt was not queryable: %+v %v", queried, err)
	}
}

func TestAPIServiceLifecycle(t *testing.T) {
	server, product, _, control := newAPIFixture(t)
	oldToken, runDir, projectIdentity := server.token, server.options.RunDir, server.options.ProjectIdentity
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := server.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	cancel()
	rotated, err := Prepare(context.Background(), product, Options{RunDir: runDir, ProjectIdentity: projectIdentity, InstanceID: "instance-b",
		Process:      ProcessIdentity{Kind: "os", PID: os.Getpid(), Created: "created-2", Executable: "steerline.exe"},
		Capabilities: []string{"pause_dispatch", "stop_service"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = rotated.SetReady("instance-b", false, "controlled startup is not ready"); err != nil {
		t.Fatal(err)
	}
	if err = rotated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = rotated.Close(ctx)
	})
	if rotated.token == oldToken || rotated.identity.InstanceID != "instance-b" {
		t.Fatal("service restart did not rotate the per-instance credential and identity")
	}
	client, err := Open(context.Background(), runDir, projectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if client.Identity().Ready || client.Identity().Diagnostic != "controlled startup is not ready" {
		t.Fatalf("not-ready service did not preserve its actual diagnosis: %+v", client.Identity())
	}
	if _, err = client.ListTasks(context.Background(), "", 10); err != nil {
		t.Fatalf("read-only query was blocked during service recovery: %v", err)
	}
	blocked := CommandEnvelope{RequestID: strings.Repeat("8", 32), ProjectID: ProjectID(projectIdentity), InstanceID: "instance-b",
		Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
	if _, err = client.SubmitCommand(context.Background(), blocked); err == nil || !strings.Contains(err.Error(), "service_not_ready") || control.submits != 0 {
		t.Fatalf("not-ready service accepted a business control action: %v submits=%d", err, control.submits)
	}
	blockedReceipt, err := client.Command(context.Background(), blocked.RequestID)
	if err != nil || blockedReceipt.Status != "rejected" || blockedReceipt.Code != "service_not_ready" {
		t.Fatalf("not-ready request was not reconciled as its original rejection: %+v %v", blockedReceipt, err)
	}
	stopEnvelope := CommandEnvelope{RequestID: strings.Repeat("9", 32), ProjectID: ProjectID(projectIdentity), InstanceID: "instance-b",
		Action: "stop_service", TargetID: "instance-b", Payload: json.RawMessage(`{}`)}
	stopReceipt, err := client.SubmitCommand(context.Background(), stopEnvelope)
	if err != nil || stopReceipt.ControlRequest == nil || stopReceipt.Status != "submitted" || control.submits != 1 {
		t.Fatalf("not-ready existing instance could not be stopped: %+v %v submits=%d", stopReceipt, err, control.submits)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://"+rotated.listener.Addr().String()+"/v1/identity", nil)
	request.Host = rotated.listener.Addr().String()
	request.RemoteAddr = "127.0.0.1:49152"
	request.Header.Set("Authorization", "Bearer "+oldToken)
	response := httptest.NewRecorder()
	rotated.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("retired token was accepted by the new instance: %d", response.Code)
	}
}
