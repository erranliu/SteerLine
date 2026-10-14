package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// writeRequestRecord prepares private-file fixtures for package boundary tests.
// Production callers publish records only through fileRequestRecordOwner.
func writeRequestRecord(dir, path string, record requestRecord) error {
	data, err := marshalRequestRecord(record)
	if err != nil {
		return err
	}
	return writePrivateAtomic(dir, path, data)
}

func TestAPIRequestRecordSizeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, action   string
		textBytes      int
		reject, expand bool
	}{
		{"reported_unknown", "planning_publish", 600000, false, false},
		{"wire_limit_unknown", "planning_publish", maxCommandBytes - 243, false, false},
		{"mapped_control_near_budget", "start_task", 530000, false, false},
		{"mapped_control_over_budget", "start_task", 534000, true, false},
		{"normalized_decision_near_budget", "answer_decision", 530000, false, false},
		{"normalized_decision_over_budget", "answer_decision", 534000, true, false},
		{"normalizer_expansion_over_budget", "answer_decision", 10, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, reader, control := newAPIFixture(t)
			config := host.Config{Repository: "C:/repo", RepositoryIdentity: "C:/repo", Remote: "origin", Branch: "main", RunDir: server.options.RunDir, TaskAuthority: "native", ControlValidation: formalControlValidation{}}
			control.formalConfig = &config
			client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			text := strings.Repeat("x", tc.textBytes)
			envelope := CommandEnvelope{RequestID: strings.Repeat("e", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: tc.action, TargetID: strings.Repeat("a", 32), ExpectedVersion: 3, ExpectedControlRevision: 7}
			var payload any = map[string]any{"document": text}
			switch tc.action {
			case "planning_publish":
				payload = map[string]any{"text": text}
				envelope.TargetID = "8"
				envelope.ExpectedControlRevision = 0
			case "start_task":
				payload = map[string]any{"execution_profile_id": text}
				envelope.TargetID = "8"
			case "answer_decision":
				decisionID, attemptID := strings.Repeat("d", 32), strings.Repeat("c", 32)
				reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{{RunID: envelope.TargetID, AttemptID: attemptID, ControlRevision: 7, RecoveryVersion: 2, Decision: &api.DecisionSummary{ID: decisionID, Status: "pending", Version: 3}}}
				payload = map[string]any{"run_id": envelope.TargetID, "decision_id": decisionID, "option_id": " continue ", "note": text, "expected_run_revision": 7, "source_attempt_id": attemptID, "expected_recovery_version": 2}
				envelope.TargetID = decisionID
				if tc.expand {
					reader.onNormalizeDecision = func(in api.DecisionInput) (api.DecisionInput, error) {
						in.Answer.Note = strings.Repeat("x", maxCommandBytes+20000)
						return in, nil
					}
				}
			}
			envelope.Payload, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			wire, _ := json.Marshal(envelope)
			if len(wire) > maxCommandBytes {
				t.Fatalf("fixture wire exceeds POST limit: %d", len(wire))
			}
			before, err := os.ReadDir(server.commandsDir)
			if err != nil {
				t.Fatal(err)
			}
			_, postErr := client.SubmitCommand(context.Background(), envelope)
			path := filepath.Join(server.commandsDir, envelope.RequestID+".json")
			if tc.reject {
				var protocolErr *ClientError
				if !errors.As(postErr, &protocolErr) || protocolErr.Status != http.StatusRequestEntityTooLarge || protocolErr.Code != "request_record_too_large" {
					t.Fatalf("admission did not reject explicitly: %v", postErr)
				}
				after, err := os.ReadDir(server.commandsDir)
				if err != nil || len(after) != len(before) {
					t.Fatalf("oversize request changed transport records: %v", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("oversize record exists: %v", err)
				}
				if _, err := host.ReadControlRequest(config, envelope.RequestID); !os.IsNotExist(err) {
					t.Fatalf("oversize control authority changed: %v", err)
				}
				if _, ok := reader.commands[envelope.RequestID]; ok || control.submits != 0 || reader.decisionSubmits != 0 {
					t.Fatalf("oversize admission invoked authority: control=%d decision=%d", control.submits, reader.decisionSubmits)
				}
				if _, err := client.Command(context.Background(), envelope.RequestID); err == nil || !strings.Contains(err.Error(), "command_not_found") {
					t.Fatalf("oversize request was registered: %v", err)
				}
			} else {
				if tc.action == "planning_publish" {
					if postErr == nil || !strings.Contains(postErr.Error(), "unsupported") {
						t.Fatalf("unknown did not remain unsupported: %v", postErr)
					}
				} else if postErr != nil {
					t.Fatal(postErr)
				}
				record, err := readRequestRecord(path)
				if err != nil || !sameEnvelope(record.Envelope, envelope) {
					t.Fatalf("formal record reader lost original: %v", err)
				}
				data, err := os.ReadFile(path)
				if err != nil || len(data) > maxRequestRecordBytes || bytes.Contains(data, []byte(`"original":`)) {
					t.Fatalf("record storage unbounded/duplicated: size=%d err=%v", len(data), err)
				}
				if tc.action == "start_task" && (record.Control == nil || record.Control.Payload.ExecutionProfileID != text || control.submits != 1) {
					t.Fatal("complete control intent not durably bound")
				}
				if tc.action == "answer_decision" && (record.Decision == nil || record.Decision.Answer.Note != text || record.Decision.Answer.OptionID != "continue" || reader.decisionSubmits != 1) {
					t.Fatal("complete normalized decision not durably bound")
				}
				queried, err := client.Command(context.Background(), envelope.RequestID)
				if err != nil || queried.Original == nil || !sameEnvelope(*queried.Original, envelope) {
					t.Fatalf("GET lost original: %v", err)
				}
				_, repeatErr := client.SubmitCommand(context.Background(), envelope)
				if repeatErr != nil {
					t.Fatal(repeatErr)
				}
				unchanged, _ := os.ReadFile(path)
				if !bytes.Equal(data, unchanged) {
					t.Fatal("same ID replay changed original storage")
				}
				conflicting := envelope
				conflicting.ExpectedVersion++
				if _, err := client.SubmitCommand(context.Background(), conflicting); err == nil || !strings.Contains(err.Error(), "request_id_conflict") {
					t.Fatalf("conflicting replay accepted: %v", err)
				}
				// A later authority receipt can be much larger than its transport
				// budget; live DTO output must not copy it back into arbitration.
				if tc.action == "start_task" {
					control.receipts[envelope.RequestID] = &api.ControlReceipt{Version: 1, RequestID: envelope.RequestID, InstanceID: envelope.InstanceID, Action: envelope.Action, Status: "applied", Message: strings.Repeat("z", maxCommandBytes)}
					later, err := client.Command(context.Background(), envelope.RequestID)
					if err != nil || later.ControlReceipt == nil || len(later.ControlReceipt.Message) != maxCommandBytes {
						t.Fatalf("later typed receipt lost: %v", err)
					}
					if _, err = client.SubmitCommand(context.Background(), envelope); err != nil {
						t.Fatal(err)
					}
					if control.submits != 1 {
						t.Fatal("later receipt repeated business submit")
					}
					// Simulate authority loss via this fixture's owned request only.
					if err = os.Remove(filepath.Join(config.RunDir, "control", "requests", envelope.RequestID, "request.json")); err != nil {
						t.Fatal(err)
					}
					lost, err := client.Command(context.Background(), envelope.RequestID)
					if err != nil || lost.Status != "outcome_unknown" || lost.Original == nil || !sameEnvelope(*lost.Original, envelope) {
						t.Fatalf("authority-loss original recovery: %v", err)
					}
					if _, err = client.SubmitCommand(context.Background(), envelope); err != nil || control.submits != 1 {
						t.Fatalf("unknown outcome replayed: %v", err)
					}
				}
				t.Logf("wire=%d persisted=%d reader_bound=%d", len(wire), len(data), maxRequestRecordBytes)
			}
			if err := ValidateStateDirectory(server.options.RunDir, server.options.ProjectIdentity); err != nil {
				t.Fatalf("real preflight broken: %v", err)
			}
			// A subsequent supported mutation remains usable after every case.
			next := CommandEnvelope{RequestID: strings.Repeat("f", 32), ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID, Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
			if _, err := client.SubmitCommand(context.Background(), next); err != nil {
				t.Fatalf("subsequent mutation failed: %v", err)
			}
		})
	}
}

func TestAPIRequestRecordBoundedRejection(t *testing.T) {
	server, _, _, control := newAPIFixture(t)
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	control.submitErr = &api.Error{Kind: api.ReadFailed, Message: "permission denied: " + strings.Repeat("\x00", 20000)}
	payload, _ := json.Marshal(map[string]string{"execution_profile_id": strings.Repeat("x", 530000)})
	envelope := CommandEnvelope{RequestID: strings.Repeat("b", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "start_task", TargetID: "8", ExpectedVersion: 3, ExpectedControlRevision: 7, Payload: payload}
	if _, err := client.SubmitCommand(context.Background(), envelope); err == nil || !strings.Contains(err.Error(), "permission_denied") {
		t.Fatalf("expected definitive rejection: %v", err)
	}
	rejected, err := client.Command(context.Background(), envelope.RequestID)
	if err != nil || rejected.Status != "rejected" || rejected.Code != "permission_denied" || rejected.Original == nil || !sameEnvelope(*rejected.Original, envelope) || len(rejected.Diagnostic) > maxOutcomeTextBytes || !strings.HasSuffix(rejected.Diagnostic, " [truncated]") {
		t.Fatalf("bounded rejection GET failed: %v", err)
	}
	if _, err := readRequestRecord(filepath.Join(server.commandsDir, envelope.RequestID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateDirectory(server.options.RunDir, server.options.ProjectIdentity); err != nil {
		t.Fatal(err)
	}
	if control.submits != 0 || len(control.requests) != 0 {
		t.Fatal("rejected authority had a side effect")
	}
}

func TestAPIRequestRecordAtomicSizeGuard(t *testing.T) {
	server, _, _, _ := newAPIFixture(t)
	envelope := CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "planning_publish", TargetID: "draft", Payload: json.RawMessage(`{}`)}
	record := saveMappedRecord(t, server, envelope, nil, nil)
	path := filepath.Join(server.commandsDir, envelope.RequestID+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	record.Envelope.Payload = json.RawMessage(`{"text":"` + strings.Repeat("x", maxRequestRecordBytes) + `"}`)
	canonical, _ := json.Marshal(record.Envelope)
	record.Digest = digestBytes(canonical)
	if err := writeRequestRecord(server.commandsDir, path, record); err == nil {
		t.Fatal("atomic writer accepted oversize base")
	}
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(original, actual) {
		t.Fatal("oversize update changed old record")
	}
	record, err = readRequestRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	// Worst-case escaping must fit the reservation, with no typed DTO copies.
	record.Response = outcomeOf(CommandResponse{Status: "outcome_unknown", Code: strings.Repeat("\x00", 2000), Diagnostic: strings.Repeat("\x00", 2000), Original: &envelope, Command: &api.CommandReceipt{ResultError: strings.Repeat("x", maxRequestRecordBytes)}, SampledAt: time.Now().UTC()})
	if err := writeRequestRecord(server.commandsDir, path, record); err != nil {
		t.Fatal(err)
	}
	if _, err := readRequestRecord(path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStateDirectory(server.options.RunDir, server.options.ProjectIdentity); err != nil {
		t.Fatal(err)
	}
}
