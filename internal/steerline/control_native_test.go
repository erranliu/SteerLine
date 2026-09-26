package steerline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestControlRequestPublicationIsImmutableAcrossClientProcesses(t *testing.T) {
	runDir := t.TempDir()
	created := now()
	request := ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: filepath.Join(t.TempDir(), ".git"), InstanceID: newID(), Action: "pause_dispatch", RequestedBy: "process-race", CreatedAt: created, ExpiresAt: created.Add(time.Minute), ExpectedControlRevision: 1}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(requestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{exec.Command(executable, "--fake", "publish-control", runDir, requestPath), exec.Command(executable, "--fake", "publish-control", runDir, requestPath)}
	var group sync.WaitGroup
	errorsFound := make([]error, len(commands))
	for i, command := range commands {
		group.Add(1)
		go func(i int, command *exec.Cmd) {
			defer group.Done()
			errorsFound[i] = command.Run()
		}(i, command)
	}
	group.Wait()
	for i, err := range errorsFound {
		if err != nil {
			t.Fatalf("publisher process %d failed: %v", i, err)
		}
	}
	stored, err := readControlRequest(controlRequestPath(runDir, request.RequestID))
	if err != nil || requestDigest(stored) != requestDigest(request) {
		t.Fatalf("published request differs from the immutable client payload: stored=%+v err=%v", stored, err)
	}
	changed := request
	changed.Action = "resume_dispatch"
	changedPath := filepath.Join(t.TempDir(), "changed.json")
	data, err = json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(changedPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = publishControlRequestFromFileForTest(runDir, changedPath); err == nil {
		t.Fatal("different request replaced an existing ID")
	}
	if err = publishControlRequest(runDir, request); err != nil {
		t.Fatalf("same ID and content should be idempotent: %v", err)
	}
}

func publishControlRequestFromFileForTest(runDir, path string) error {
	request, err := readControlRequest(path)
	if err != nil {
		return err
	}
	return publishControlRequest(runDir, request)
}

func TestNativeControlPayloadRejectsUnknownFields(t *testing.T) {
	for _, request := range []ControlRequest{
		{Action: "start_task", Payload: json.RawMessage(`{"task_id":"1","extra":true}`)},
		{Action: "set_dispatch_mode", Payload: json.RawMessage(`{"mode":"manual","extra":true}`)},
		{Action: "pause_dispatch", Payload: json.RawMessage(`{"unexpected":true}`)},
	} {
		if _, err := decodeControlPayload(request); err == nil {
			t.Fatalf("control payload with unknown fields was accepted: %+v", request)
		}
	}
	if _, err := decodeControlPayload(ControlRequest{Action: "pause_dispatch", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("empty object should be a valid payload for pause_dispatch: %v", err)
	}
	if _, err := decodeControlPayload(ControlRequest{Action: "set_dispatch_mode", Payload: json.RawMessage(`{"mode":"auto"}`)}); err != nil {
		t.Fatalf("valid native dispatch mode was rejected: %v", err)
	}
}
