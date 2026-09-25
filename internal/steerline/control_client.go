package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ControlSnapshot(c Config) (*Runtime, error) {
	return controlSnapshot(c, false)
}

func controlSnapshot(c Config, allowInitializing bool) (*Runtime, error) {
	connection := ConnectService(context.Background(), c)
	if connection.Snapshot.Runtime == nil {
		if connection.Diagnostic != "" {
			return nil, errors.New(connection.Diagnostic)
		}
		return nil, errors.New("no runtime snapshot")
	}
	if allowInitializing {
		if !connection.CanStop {
			return nil, fmt.Errorf("service cannot accept an explicit stop in state %q: %s", connection.State, connection.Diagnostic)
		}
	} else if !connection.CanControl {
		return nil, fmt.Errorf("service is not ready for control in state %q: %s", connection.State, connection.Diagnostic)
	}
	rt := connection.Snapshot.Runtime
	if rt.Service.ControlProtocol != ControlProtocolVersion {
		return nil, errors.New("service does not support control protocol v1")
	}
	if allowInitializing && !serviceHasCapability(rt, "stop_service") {
		return nil, errors.New("service does not advertise stop_service capability")
	}
	return rt, nil
}

func serviceHasCapability(rt *Runtime, action string) bool {
	if rt == nil {
		return false
	}
	for _, capability := range rt.Service.Capabilities {
		if capability == action {
			return true
		}
	}
	return false
}

func ReadTaskSnapshot(c Config) (*TaskSnapshot, error) {
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return nil, err
	}
	if c.taskAuthority() == "native" {
		store, err := OpenNativeTaskProject(context.Background(), &c)
		if err != nil {
			return nil, err
		}
		defer store.Close()
		authority, err := store.nativeAuthority(context.Background(), c.identity)
		if err != nil {
			return nil, err
		}
		ready, err := (QueryService{Store: store, Project: c.identity}).ReadyNativeTasks(context.Background())
		if err != nil {
			return nil, err
		}
		queried := now()
		snapshot := &TaskSnapshot{Version: 1, RepositoryIdentity: c.identity, QueriedAt: queried, LastSuccessAt: &queried, Tasks: []TaskCandidate{}}
		if authority.DispatchPaused {
			snapshot.BlockReason = "dispatch is paused"
		}
		for _, task := range ready {
			if !contains(c.ExecutableTypes, task.Type) {
				continue
			}
			snapshot.Tasks = append(snapshot.Tasks, TaskCandidate{TaskID: task.TaskID, Title: task.Title, Type: task.Type})
		}
		return snapshot, nil
	}
	rt, err := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
	if err != nil {
		return nil, err
	}
	if rt == nil || rt.SchemaVersion != 5 {
		return nil, errors.New("no runtime v5 snapshot for task candidates")
	}
	if canonical(rt.Repository.Identity) != canonical(c.identity) {
		return nil, errors.New("runtime repository identity does not match configuration")
	}
	if rt.Repository.Remote != c.Remote || rt.Repository.Branch != c.Branch {
		return nil, errors.New("runtime remote/branch does not match configuration")
	}
	b, err := readSnapshot(filepath.Join(c.RunDir, "tasks.json"))
	if err != nil {
		return nil, err
	}
	var snapshot TaskSnapshot
	if err = decode(b, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != 1 || snapshot.InstanceID != rt.Service.InstanceID || canonical(snapshot.RepositoryIdentity) != canonical(rt.Repository.Identity) {
		return nil, errors.New("task snapshot targets a different service instance or repository")
	}
	return &snapshot, nil
}

func SubmitControl(c Config, action string, payload any, requestedBy string) (ControlRequest, error) {
	rt, err := controlSnapshot(c, action == "stop_service")
	if err != nil {
		return ControlRequest{}, err
	}
	return submitControlSnapshot(c, rt, action, payload, requestedBy)
}

// The caller has validated this snapshot. Keep its instance/revision unchanged:
// a restart between observation and publication must reject, never retarget.
func submitControlSnapshot(c Config, rt *Runtime, action string, payload any, requestedBy string) (ControlRequest, error) {
	if !validControlAction(action) {
		return ControlRequest{}, errors.New("unsupported control action")
	}
	if !serviceHasCapability(rt, action) {
		return ControlRequest{}, fmt.Errorf("service does not advertise %s capability", action)
	}
	var raw json.RawMessage
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return ControlRequest{}, e
		}
		raw = b
	}
	if requestedBy == "" {
		requestedBy = DefaultRequestedBy()
	}
	created := now()
	req := ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: rt.Repository.Identity, InstanceID: rt.Service.InstanceID, Action: action, Payload: raw, RequestedBy: requestedBy, CreatedAt: created, ExpiresAt: created.Add(2 * time.Minute)}
	if _, err := decodeControlPayload(req); err != nil {
		return ControlRequest{}, err
	}
	if action != "stop_service" {
		req.ExpectedControlRevision = rt.Control.Revision
	}
	if err := publishControlRequest(c.RunDir, req); err != nil {
		return ControlRequest{}, err
	}
	return req, nil
}

func ReadControlReceipt(c Config, id string) (*ControlReceipt, error) {
	if !controlIDPattern.MatchString(id) {
		return nil, errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	b, err := readSnapshot(controlReceiptPath(c.RunDir, id))
	if err != nil {
		return nil, err
	}
	if len(b) > maxControlRequestBytes {
		return nil, errors.New("control receipt exceeds size limit")
	}
	var receipt ControlReceipt
	if err = decode(b, &receipt); err != nil {
		return nil, err
	}
	if receipt.RequestID != id {
		return nil, errors.New("receipt request ID does not match path")
	}
	if receipt.Version != ControlProtocolVersion || (receipt.Status != "applying" && !terminalControlStatus(receipt.Status)) {
		return nil, errors.New("invalid control receipt version or status")
	}
	return &receipt, nil
}

func WaitControlReceipt(ctx context.Context, c Config, id string, poll time.Duration) (*ControlReceipt, error) {
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		if r, err := ReadControlReceipt(c, id); err == nil {
			if r.Status != "applying" {
				return r, nil
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}
