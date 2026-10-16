package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	execution "github.com/erranliu/SteerLine/internal/taskrunner"
)

type formalControlValidation struct{}

func (formalControlValidation) ValidAction(action string) bool {
	return execution.ValidControlAction(action)
}
func (formalControlValidation) DecodePayload(req api.ControlRequest) (api.ControlPayload, error) {
	return execution.DecodeControlPayload(req)
}
func (formalControlValidation) ValidateRequest(req api.ControlRequest, instance, project string, revision uint64, at time.Time) (string, error) {
	return execution.ValidateControlRequest(req, instance, project, revision, at)
}
func (formalControlValidation) RequestDigest(req api.ControlRequest) string {
	return execution.RequestDigest(req)
}
func (formalControlValidation) TerminalStatus(status string) bool {
	return execution.TerminalControlStatus(status)
}

func saveMappedRecord(t *testing.T, server *Server, envelope CommandEnvelope, control *api.ControlInput, decision *api.DecisionInput) requestRecord {
	t.Helper()
	canonical, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	record := requestRecord{Version: 1, Digest: digestBytes(canonical), Envelope: envelope, Control: control, Decision: decision, CreatedAt: time.Now().UTC()}
	if err := writeRequestRecord(server.commandsDir, filepath.Join(server.commandsDir, envelope.RequestID+".json"), record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestUnifiedStartupRetiredStopReceiptRemainsReadOnly(t *testing.T) {
	server, _, _, control := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	revision := uint64(7)
	envelope := CommandEnvelope{RequestID: strings.Repeat("f", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "stop_service", TargetID: "instance-a", ExpectedControlRevision: revision, Payload: json.RawMessage(`{}`)}
	input := api.ControlInput{RequestID: envelope.RequestID, Action: api.StopService, InstanceID: envelope.InstanceID, ExpectedRevision: &revision, RequestedBy: "original-user"}
	saveMappedRecord(t, server, envelope, &input, nil)
	control.requests[envelope.RequestID] = api.ControlRequest{ProtocolVersion: 1, RequestID: envelope.RequestID, RepositoryIdentity: "C:/repo", InstanceID: envelope.InstanceID, Action: envelope.Action, ExpectedControlRevision: revision, RequestedBy: input.RequestedBy}
	response, err := client.Command(context.Background(), envelope.RequestID)
	if err != nil || response.ControlRequest == nil || response.ControlRequest.ExpectedControlRevision != revision {
		t.Fatal("old stop receipt unavailable", response, err)
	}
	if _, err = client.SubmitCommand(context.Background(), envelope); err == nil {
		t.Fatal("old action was reaccepted")
	}
	if control.submits != 0 {
		t.Fatal("receipt lookup submitted a stop")
	}
}

func TestAPIAuthoritativeControlBinding(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*api.ControlInput)
	}{
		{"same_formal_content", nil},
		{"target", func(in *api.ControlInput) { in.Payload.TaskID = "9" }},
		{"task_version", func(in *api.ControlInput) { in.Payload.ExpectedTaskVersion = 55 }},
		{"control_revision", func(in *api.ControlInput) { revision := uint64(99); in.ExpectedRevision = &revision }},
		{"instance", func(in *api.ControlInput) { in.InstanceID = "instance-b" }},
		{"profile", func(in *api.ControlInput) { in.Payload.ExecutionProfileID = "other" }},
		{"profile_digest", func(in *api.ControlInput) { in.Payload.ExecutionProfileDigest = strings.Repeat("b", 64) }},
		{"graph_revision", func(in *api.ControlInput) { in.Payload.ExpectedGraphRevision = 99 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server, _, _, control := newAPIFixture(t)
			config := host.Config{Repository: "C:/repo", RepositoryIdentity: "C:/repo", Remote: "origin", Branch: "main", RunDir: server.options.RunDir, TaskAuthority: "native", ControlValidation: formalControlValidation{}}
			control.formalConfig = &config
			envelope := CommandEnvelope{RequestID: strings.Repeat("e", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_candidate", TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, ExpectedGraphRevision: 4, Payload: json.RawMessage(`{"execution_profile_id":"fixture"}`)}
			mapped, err := mappedControlForTest(server.commands, context.Background(), envelope, currentActor())
			if err != nil {
				t.Fatal(err)
			}
			saveMappedRecord(t, server, envelope, &mapped, nil)
			actual := mapped
			if test.mutate != nil {
				test.mutate(&actual)
			}
			_, err = host.SubmitControlInbox(config, actual)
			if err != nil {
				t.Fatal(err)
			}
			// The real immutable publisher refuses the original after different content wins.
			var publishErr error
			if test.mutate != nil {
				_, publishErr = host.SubmitControlInbox(config, mapped)
			}
			publishedRecord, err := host.ReadControlRequest(config, envelope.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil && publishErr == nil {
				t.Fatal("formal publisher did not reject a content collision")
			}
			client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
			if err != nil {
				t.Fatal(err)
			}
			response, getErr := client.Command(context.Background(), envelope.RequestID)
			_, postErr := client.SubmitCommand(context.Background(), envelope)
			if test.mutate == nil {
				if publishErr != nil || getErr != nil || postErr != nil || response.ControlRequest == nil || !matchesControl(envelope, &requestRecord{Control: &mapped}, *response.ControlRequest) {
					t.Fatalf("same formal content failed reconciliation: %+v get=%v post=%v publisher=%v", response, getErr, postErr, publishErr)
				}
			} else if getErr == nil || postErr == nil || !strings.Contains(getErr.Error(), "identity_conflict") || !strings.Contains(postErr.Error(), "identity_conflict") {
				t.Fatalf("different authority was reused: %+v get=%v post=%v", response, getErr, postErr)
			}
			if control.submits != 0 {
				t.Fatalf("reconciliation invoked another backend action: %d", control.submits)
			}
			reread, err := host.ReadControlRequest(config, envelope.RequestID)
			if err != nil || !reflect.DeepEqual(*reread, *publishedRecord) {
				t.Fatalf("authority changed: %+v %v", reread, err)
			}
		})
	}
}

func TestAPIMappedControlPayloadBinding(t *testing.T) {
	server, _, _, _ := newAPIFixture(t)
	environment := api.HostActionTarget{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32), TurnID: strings.Repeat("d", 32), BlockerID: "blocker", Version: 3}
	for _, action := range []string{"open_terminal", "recheck_environment", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"} {
		t.Run(action, func(t *testing.T) {
			envelope := CommandEnvelope{RequestID: strings.Repeat("f", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: action, TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
			switch action {
			case "open_terminal", "recheck_environment":
				envelope.TargetID = environment.RunID
				envelope.ExpectedVersion = 3
				envelope.Payload, _ = json.Marshal(environmentPayload{Target: environment})
			case "set_dispatch_mode":
				envelope.Payload = json.RawMessage(`{"mode":"auto"}`)
			case "stop_service":
				envelope.TargetID = envelope.InstanceID
				envelope.ExpectedVersion = 0
			}
			input, err := mappedControlForTest(server.commands, context.Background(), envelope, currentActor())
			if err != nil {
				t.Fatal(err)
			}
			record := requestRecord{Envelope: envelope, Control: &input}
			request := api.ControlRequest{ProtocolVersion: 1, RequestID: input.RequestID, RepositoryIdentity: "C:/repo", InstanceID: input.InstanceID, Action: action, RequestedBy: input.RequestedBy}
			if input.ExpectedRevision != nil {
				request.ExpectedControlRevision = *input.ExpectedRevision
			}
			request.Payload, _ = json.Marshal(input.Payload)
			if !matchesControl(envelope, &record, request) {
				t.Fatal("same mapped action payload did not match")
			}
			changed := input.Payload
			switch action {
			case "open_terminal", "recheck_environment":
				target := *changed.Environment
				target.AttemptID = strings.Repeat("e", 32)
				changed.Environment = &target
			case "set_dispatch_mode":
				changed.Mode = "manual"
			default:
				changed.Mode = "foreign payload"
			}
			request.Payload, _ = json.Marshal(changed)
			if matchesControl(envelope, &record, request) {
				t.Fatal("different mapped action payload matched")
			}
		})
	}
}

func TestAPIAuthoritativeSQLiteBinding(t *testing.T) {
	for _, kind := range []string{"answer_decision", "discard_task"} {
		t.Run(kind, func(t *testing.T) {
			for _, mismatch := range []bool{false, true} {
				t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
					server, _, reader, _ := newAPIFixture(t)
					storePath := filepath.Join(t.TempDir(), "authority.db")
					store, err := sqlite.OpenSQLiteStore(storePath)
					if err != nil {
						t.Fatal(err)
					}
					defer store.Close()
					if err = store.InitializeProject(context.Background(), model.ProjectTaskAuthority{ProjectIdentity: "C:/repo", Authority: "native", AuthorityVersion: 1, Repository: "C:/repo", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}); err != nil {
						t.Fatal(err)
					}
					envelope := CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: kind, TargetID: "decision-original", ExpectedVersion: 3}
					actor := currentActor()
					var mapped *api.DecisionInput
					var body []byte
					if kind == "answer_decision" {
						recovery := uint64(2)
						payload := answerPayload{RunID: strings.Repeat("b", 32), DecisionID: envelope.TargetID, OptionID: "continue", Note: "original note", Answers: map[string][]string{"q": {"choice", "text", "remarks"}}, ExpectedRunRevision: 7, SourceAttemptID: strings.Repeat("c", 32), ExpectedRecoveryVersion: &recovery}
						envelope.Payload, _ = json.Marshal(payload)
						input := api.DecisionInput{CommandID: envelope.RequestID, RequestedBy: actor, Answer: api.DecisionAnswer{RunID: payload.RunID, DecisionID: payload.DecisionID, OptionID: payload.OptionID, Note: payload.Note, Answers: payload.Answers, AnsweredBy: actor, ExpectedDecisionVersion: 3, ExpectedRunRevision: 7, SourceAttemptID: payload.SourceAttemptID, ExpectedRecoveryVersion: 2, ServiceInstanceID: envelope.InstanceID}}
						mapped = &input
						actual := input.Answer
						if mismatch {
							actual.Answers = map[string][]string{"q": {"other choice", "different text", "different remarks"}}
							actual.ExpectedRecoveryVersion = 99
						}
						body, _ = json.Marshal(actual)
					} else {
						envelope.TargetID = "8"
						envelope.Payload = json.RawMessage(`{"reason":" original reason "}`)
						actual := api.DiscardInput{TaskID: "8", RequestID: envelope.RequestID, ExpectedVersion: 3, RequestedBy: actor, Reason: "original reason"}
						if mismatch {
							actual.Reason = "different reason"
						}
						body, _ = json.Marshal(actual)
					}
					mappedRecord := saveMappedRecord(t, server, envelope, nil, mapped)
					if kind == "discard_task" {
						mappedRecord.Discard = &api.DiscardInput{TaskID: envelope.TargetID, RequestID: envelope.RequestID, ExpectedVersion: envelope.ExpectedVersion, RequestedBy: actor, Reason: "original reason"}
						if err := writeRequestRecord(server.commandsDir, filepath.Join(server.commandsDir, envelope.RequestID+".json"), mappedRecord); err != nil {
							t.Fatal(err)
						}
					}
					record := storageports.CommandRecord{ProjectIdentity: "C:/repo", CommandID: envelope.RequestID, IdempotencyKey: envelope.RequestID, Kind: kind, TargetID: envelope.TargetID, ExpectedVersion: 3, RequestedBy: actor, Status: "accepted", Payload: body, CreatedAt: time.Now().UTC()}
					if kind == "answer_decision" {
						// Controlled persisted receipt fixture: reconciliation reads the
						// actual commands table through LookupCommand and its DTO mapper.
						database, openErr := sql.Open("sqlite", storePath)
						if openErr != nil {
							t.Fatal(openErr)
						}
						at := record.CreatedAt.Format(time.RFC3339Nano)
						_, err = database.Exec(`INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, record.ProjectIdentity, record.CommandID, record.Kind, record.TargetID, record.ExpectedVersion, record.IdempotencyKey, record.RequestedBy, record.Payload, record.Status, at, at)
						closeErr := database.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("persist answer receipt: %v close=%v", err, closeErr)
						}
					} else {
						_, created, enqueueErr := store.EnqueueCommand(context.Background(), record)
						if enqueueErr != nil || !created {
							t.Fatalf("persist discard authority: %v created=%v", enqueueErr, created)
						}
					}
					loaded, found, err := store.LookupCommand(context.Background(), "C:/repo", envelope.RequestID)
					if err != nil || !found || !reflect.DeepEqual(loaded.Payload, record.Payload) {
						t.Fatalf("read authority: %v found=%v", err, found)
					}
					reader.commands[envelope.RequestID] = query.CommandReceipt(loaded)
					client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
					if err != nil {
						t.Fatal(err)
					}
					_, getErr := client.Command(context.Background(), envelope.RequestID)
					_, postErr := client.SubmitCommand(context.Background(), envelope)
					if mismatch {
						if getErr == nil || postErr == nil || !strings.Contains(getErr.Error(), "identity_conflict") || !strings.Contains(postErr.Error(), "identity_conflict") {
							t.Fatalf("different SQLite payload was reused: get=%v post=%v", getErr, postErr)
						}
					} else if getErr != nil || postErr != nil {
						t.Fatalf("same SQLite content did not reconcile: get=%v post=%v", getErr, postErr)
					}
					if reader.decisionSubmits != 0 {
						t.Fatal("reconciliation repeated the decision action")
					}
				})
			}
		})
	}
}

type heldReader struct {
	started chan struct{}
	release chan struct{}
	body    io.Reader
	first   bool
}

func (r *heldReader) Read(body []byte) (int, error) {
	if !r.first {
		r.first = true
		close(r.started)
		<-r.release
	}
	return r.body.Read(body)
}

func TestAPIPrivateWritersConcurrentPreflight(t *testing.T) {
	for _, exclusive := range []bool{false, true} {
		t.Run(fmt.Sprint(exclusive), func(t *testing.T) {
			runDir := t.TempDir()
			root := filepath.Join(runDir, "local-api")
			directory := filepath.Join(root, "credentials")
			if err := ensurePrivateDirectory(root); err != nil {
				t.Fatal(err)
			}
			if err := ensurePrivateDirectory(directory); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(directory, strings.Repeat("a", 32)+".token")
			held := &heldReader{started: make(chan struct{}), release: make(chan struct{}), body: strings.NewReader(strings.Repeat("b", 64))}
			done := make(chan error, 1)
			go func() { done <- writePrivateContents(directory, destination, held, exclusive) }()
			select {
			case <-held.started:
			case <-time.After(5 * time.Second):
				t.Fatal("writer did not reach its open temporary file")
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				close(held.release)
				t.Fatal(err)
			}
			if len(entries) != 1 || !temporaryFileName(entries[0].Name()) {
				close(held.release)
				t.Fatalf("writer has no live temporary entry: %+v", entries)
			}
			// This calls the real preflight while the actual atomic/exclusive writer is
			// still holding its file open, before write/sync/close/publication.
			preflightErr := ValidateStateDirectory(runDir, "C:/repo")
			close(held.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if preflightErr != nil {
				t.Fatalf("legitimate in-progress writer failed concurrent preflight: %v", preflightErr)
			}
			if err := ValidateStateDirectory(runDir, "C:/repo"); err != nil {
				t.Fatalf("published private file failed preflight: %v", err)
			}
		})
	}
}

func TestAPIFirstRegistrationBindsMappedControlIntent(t *testing.T) {
	server, _, reader, _ := newAPIFixture(t)
	envelope := CommandEnvelope{RequestID: strings.Repeat("d", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_task", TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, Payload: json.RawMessage(`{"execution_profile_id":"fixture"}`)}
	path := filepath.Join(server.commandsDir, envelope.RequestID+".json")
	resolutions := 0
	reader.onResolveStart = func(task, profile string) {
		resolutions++
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("first arbitration record was visible before its original mapped content: %v", err)
		}
	}
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubmitCommand(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	record, err := readRequestRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if resolutions != 1 || record.Control == nil || record.Control.Payload.ExecutionProfileDigest != strings.Repeat("a", 64) {
		t.Fatalf("original mapping was not bound exactly once: resolutions=%d record=%+v", resolutions, record)
	}
	reader.onResolveStart = func(string, string) { t.Error("reconciliation re-resolved the original profile") }
	if _, err := client.Command(context.Background(), envelope.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubmitCommand(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
}

func TestAPIFormalConsumerWinsRegistrationWindow(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(fmt.Sprint(different), func(t *testing.T) {
			server, _, _, control := newAPIFixture(t)
			config := host.Config{Repository: "C:/repo", RepositoryIdentity: "C:/repo", Remote: "origin", Branch: "main", RunDir: server.options.RunDir, TaskAuthority: "native", ControlValidation: formalControlValidation{}}
			control.formalConfig = &config
			envelope := CommandEnvelope{RequestID: strings.Repeat("c", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_task", TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, Payload: json.RawMessage(`{"execution_profile_id":"fixture"}`)}
			control.beforeFormalSubmit = func(input api.ControlInput) {
				record, err := readRequestRecord(filepath.Join(server.commandsDir, envelope.RequestID+".json"))
				if err != nil || record.Control == nil {
					t.Errorf("formal consumer observed incomplete registration: %v", err)
				}
				if different {
					input.Payload.TaskID = "9"
					input.Payload.ExpectedTaskVersion = 55
					revision := uint64(99)
					input.ExpectedRevision = &revision
				}
				if _, err := host.SubmitControlInbox(config, input); err != nil {
					t.Errorf("formal consumer publication: %v", err)
				}
			}
			client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
			if err != nil {
				t.Fatal(err)
			}
			_, postErr := client.SubmitCommand(context.Background(), envelope)
			_, getErr := client.Command(context.Background(), envelope.RequestID)
			_, repeatErr := client.SubmitCommand(context.Background(), envelope)
			if different {
				if postErr == nil || getErr == nil || repeatErr == nil || !strings.Contains(postErr.Error(), "identity_conflict") || !strings.Contains(getErr.Error(), "identity_conflict") || !strings.Contains(repeatErr.Error(), "identity_conflict") {
					t.Fatalf("different formal winner was reused: post=%v get=%v repeat=%v", postErr, getErr, repeatErr)
				}
			} else if postErr != nil || getErr != nil || repeatErr != nil {
				t.Fatalf("same formal winner did not reconcile: post=%v get=%v repeat=%v", postErr, getErr, repeatErr)
			}
			if control.submits != 1 {
				t.Fatalf("registered request repeated backend submit: %d", control.submits)
			}
		})
	}
}

func TestAPIStartTaskBindsServerResolvedGraphToOriginalReceipt(t *testing.T) {
	for _, graph := range []uint64{0, 4, 7} {
		t.Run(fmt.Sprint(graph), func(t *testing.T) {
			envelope := CommandEnvelope{RequestID: strings.Repeat("c", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_task", TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, ExpectedGraphRevision: graph, Payload: json.RawMessage(`{"execution_profile_id":"fixture"}`)}
			revision := uint64(7)
			input := api.ControlInput{RequestID: envelope.RequestID, InstanceID: envelope.InstanceID, Action: api.StartTask, ExpectedRevision: &revision, RequestedBy: "tester", Payload: api.ControlPayload{TaskID: "8", ExpectedTaskVersion: 3, ExpectedGraphRevision: 4, ExecutionProfileID: "fixture", ExecutionProfileDigest: "frozen-profile"}}
			record := requestRecord{Envelope: envelope, Control: &input}
			request := registryTestControlRequest(input)
			if matchesControl(envelope, &record, request) != (graph == 0 || graph == 4) {
				t.Fatal("resolved graph does not bind to the original mapped intent")
			}
			changed := input.Payload
			changed.ExpectedGraphRevision = 9
			request.Payload, _ = json.Marshal(changed)
			if matchesControl(envelope, &record, request) {
				t.Fatal("different published graph reused original receipt")
			}
		})
	}
}

func mappedControlForTest(s *commandRegistry, ctx context.Context, envelope CommandEnvelope, requestedBy string) (api.ControlInput, error) {
	// Retired stop_service can still be mapped for matching historical receipts.
	// Submit rejects it through parseActionEnvelope.
	var parsed actionPayload
	if envelope.Action != "stop_service" {
		var err error
		parsed, err = parseActionEnvelope(envelope)
		if err != nil {
			return api.ControlInput{}, err
		}
	}
	return s.mappedControlInput(ctx, envelope, parsed, requestedBy)
}
