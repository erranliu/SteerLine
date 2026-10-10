package backend

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestExecutionRecordsKeepWireIdentity(t *testing.T) {
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV3, Target: ExecutionTarget{ID: "target"}, Agent: AgentSpec{Name: "codex"}, RunID: "run", AttemptID: "attempt", External: ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}}
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"backend":"orca","protocol":"orca_terminal_v3","target":{"id":"target"},"agent":{"name":"codex"},"run_id":"run","attempt_id":"attempt","external":{"kind":"orca","terminal":"terminal","runtime_id":"runtime","incarnation":"incarnation"}}`
	if string(data) != want {
		t.Fatalf("binding representation changed: %s", data)
	}
	var restored ExecutionRef
	if err := json.Unmarshal(data, &restored); err != nil || restored != ref {
		t.Fatalf("native binding identity changed: %+v / %v", restored, err)
	}
	for _, status := range []OperationStatus{OperationPending, OperationInFlight, OperationAccepted, OperationConfirmed, OperationRejected, OperationUnknown} {
		op := Operation{Version: 1, ID: "stop-attempt", RunID: ref.RunID, AttemptID: ref.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: "fixed", Status: status, Ref: &ref}
		data, err := json.Marshal(op)
		if err != nil {
			t.Fatal(err)
		}
		if status == OperationAccepted {
			wantOperation := `{"version":1,"id":"stop-attempt","run_id":"run","attempt_id":"attempt","backend":"orca","protocol":"orca_terminal_v3","target":{"id":"target"},"action":"stop","payload_digest":"fixed","status":"accepted","started_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z","ref":` + want + `}`
			if string(data) != wantOperation {
				t.Fatalf("operation representation changed: %s", data)
			}
		}
		var restored Operation
		if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(restored, op) {
			t.Fatalf("operation lost its status or fixed ref: %+v / %v", restored, err)
		}
	}
	question := Question{ID: "choice", Header: "Choice", Question: "Continue?", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}, AllowFreeText: true}
	data, err = json.Marshal(question)
	if err != nil || string(data) != `{"id":"choice","header":"Choice","question":"Continue?","options":[{"id":"yes","label":"Yes"}],"allow_free_text":true}` {
		t.Fatalf("persisted native question representation changed: %s / %v", data, err)
	}
	hostOption := DecisionOption{ID: "recheck_environment", Label: "Recheck", Receiver: "center", Action: "recheck_environment"}
	data, err = json.Marshal(hostOption)
	if err != nil || string(data) != `{"id":"recheck_environment","label":"Recheck","receiver":"center","action":"recheck_environment"}` {
		t.Fatalf("host action routing changed: %s / %v", data, err)
	}
}

func TestAdapterAndBackendErrorsPreserveCause(t *testing.T) {
	cause := errors.New("original transport failure")
	backend := &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "reconcile original operation", Cause: cause}
	adapter := &AdapterError{Kind: AdapterIdentityMismatch, Operation: "observe", Diagnostic: "original session changed", Cause: cause}
	for _, err := range []error{backend, adapter} {
		if !errors.Is(errors.Join(errors.New("wrapper"), err), cause) {
			t.Fatalf("error lost its original cause: %v", err)
		}
	}
	if backend.Error() != "outcome_unknown: reconcile original operation: original transport failure" || adapter.Error() != "identity_mismatch observe: original session changed: original transport failure" {
		t.Fatal("error diagnostic changed during relocation")
	}
	if !IsAdapterError(errors.Join(cause, adapter), AdapterIdentityMismatch) || IsAdapterError(adapter, AdapterRejected) {
		t.Fatal("wrapped adapter error lost its classification")
	}
	unsupported := UnsupportedAdapterOperation("resume", "not certified")
	if !IsAdapterError(unsupported, AdapterUnsupported) || unsupported.Error() != "capability_unsupported resume: not certified" {
		t.Fatal("unsupported operation was not rejected with its original reason")
	}
}

func TestPRCluesKeepOrderAndDeduplicate(t *testing.T) {
	want := []string{"https://github.example/o/r/pull/7", "http://git.example/o/r/pulls/9"}
	text := `Created https://github.example/o/r/pull/7. See <http://git.example/o/r/pulls/9>, repeated https://github.example/o/r/pull/7; https://github.example/o/r/issues/7 is not a PR.`
	if got := PRClues(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("PR clues changed: %v", got)
	}
	if got := PRClues("no publication evidence"); len(got) != 0 {
		t.Fatalf("ordinary output became a PR clue: %v", got)
	}
}

func TestPRCluesSeparateMarkdownLinks(t *testing.T) {
	want := []string{"https://github.example/o/r/pull/7", "http://git.example/o/r/pulls/9"}
	for _, text := range []string{
		`PR: [https://github.example/o/r/pull/7](https://github.example/o/r/pull/7) and [second](http://git.example/o/r/pulls/9)`,
		"PR: `https://github.example/o/r/pull/7` (`http://git.example/o/r/pulls/9`)",
		`[https://github.example/o/r/pull/7](http://git.example/o/r/pulls/9)`,
	} {
		if got := PRClues(text); !reflect.DeepEqual(got, want) {
			t.Fatalf("Markdown links were joined or lost: %q: %v", text, got)
		}
	}
	ipv6 := "http://[::1]:3000/o/r/pulls/7"
	if got := PRClues("[" + ipv6 + "](" + ipv6 + ")"); !reflect.DeepEqual(got, []string{ipv6}) {
		t.Fatalf("Markdown boundaries discarded an IPv6 authority: %v", got)
	}
}
