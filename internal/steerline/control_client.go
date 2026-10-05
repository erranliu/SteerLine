package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func ControlSnapshot(c execution.Config) (*Runtime, error) {
	return controlSnapshot(c, false)
}

func controlSnapshot(c execution.Config, allowInitializing bool) (*Runtime, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
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

func ReadTaskSnapshot(c execution.Config) (*TaskSnapshot, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return nil, err
	}
	if c.TaskReader == nil {
		return nil, errors.New("application task reader is required")
	}
	graph, err := c.TaskReader.TaskGraphSnapshot(context.Background(), c.Identity())
	if err != nil {
		return nil, err
	}
	authority := graph.Authority
	ready, err := (productquery.Service{Project: c.Identity(), Tasks: c.TaskReader}).ReadyNativeTasks(context.Background())
	if err != nil {
		return nil, err
	}
	queried := now()
	snapshot := &TaskSnapshot{Version: 1, RepositoryIdentity: c.Identity(), QueriedAt: queried, LastSuccessAt: &queried, Tasks: []TaskCandidate{}}
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

func SubmitControl(c execution.Config, action string, payload any, requestedBy string) (productapi.ControlRequest, error) {
	return SubmitControlWithID(c, action, payload, requestedBy, newID())
}

// SubmitControlWithID publishes a control action under a caller-allocated ID.
// The ID is immutable: reusing it with different content is rejected by the
// request inbox rather than silently replacing the original action.
func SubmitControlWithID(c execution.Config, action string, payload any, requestedBy, requestID string) (productapi.ControlRequest, error) {
	return SubmitControlTargeted(c, action, payload, requestedBy, requestID, "", nil)
}

// SubmitControlTargeted binds a UI action to the service instance and optional
// control revision it displayed. A restart or intervening control change is
// reported to the caller instead of silently retargeting the action.
func SubmitControlTargeted(c execution.Config, action string, payload any, requestedBy, requestID, expectedInstance string, expectedRevision *uint64) (productapi.ControlRequest, error) {
	if !controlIDPattern.MatchString(requestID) {
		return productapi.ControlRequest{}, errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	rt, err := controlSnapshot(c, action == "stop_service")
	if err != nil {
		return productapi.ControlRequest{}, err
	}
	if expectedInstance != "" && rt.Service.InstanceID != expectedInstance {
		return productapi.ControlRequest{}, errors.New("stale_instance: service instance changed; refresh before submitting")
	}
	if expectedRevision != nil && action != "stop_service" && rt.Control.Revision != *expectedRevision {
		return productapi.ControlRequest{}, errors.New("stale_revision: control state changed; refresh before submitting")
	}
	return submitControlSnapshotWithID(c, rt, action, payload, requestedBy, requestID)
}

// The caller has validated this snapshot. Keep its instance/revision unchanged:
// a restart between observation and publication must reject, never retarget.
func submitControlSnapshot(c execution.Config, rt *Runtime, action string, payload any, requestedBy string) (productapi.ControlRequest, error) {
	return submitControlSnapshotWithID(c, rt, action, payload, requestedBy, newID())
}

func submitControlSnapshotWithID(c execution.Config, rt *Runtime, action string, payload any, requestedBy, requestID string) (productapi.ControlRequest, error) {
	if !execution.ValidControlAction(action) {
		return productapi.ControlRequest{}, errors.New("unsupported control action")
	}
	if !controlIDPattern.MatchString(requestID) {
		return productapi.ControlRequest{}, errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	if !serviceHasCapability(rt, action) {
		return productapi.ControlRequest{}, fmt.Errorf("service does not advertise %s capability", action)
	}
	var raw json.RawMessage
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return productapi.ControlRequest{}, e
		}
		raw = b
	}
	if requestedBy == "" {
		requestedBy = DefaultRequestedBy()
	}
	created := now()
	req := productapi.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: requestID, RepositoryIdentity: rt.Repository.Identity, InstanceID: rt.Service.InstanceID, Action: action, Payload: raw, RequestedBy: requestedBy, CreatedAt: created, ExpiresAt: created.Add(2 * time.Minute)}
	if _, err := execution.DecodeControlPayload(req); err != nil {
		return productapi.ControlRequest{}, err
	}
	if action != "stop_service" {
		req.ExpectedControlRevision = rt.Control.Revision
	}
	if err := publishControlRequest(c.RunDir, req); err != nil {
		return productapi.ControlRequest{}, err
	}
	return req, nil
}

// ReadControlRequest reads the immutable request matching an ID. It is useful
// when a client reopens and needs to distinguish an unpublished request from a
// request whose receipt has not arrived yet.
func ReadControlRequest(c execution.Config, id string) (*productapi.ControlRequest, error) {
	req, _, err := readControlRequestForClient(c, id)
	return req, err
}

func readControlRequestForClient(c execution.Config, id string) (*productapi.ControlRequest, [sha256.Size]byte, error) {
	if !controlIDPattern.MatchString(id) {
		return nil, [sha256.Size]byte{}, errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	raw, err := readBoundedFile(controlRequestPath(c.RunDir, id), maxControlRequestBytes)
	if err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	var req productapi.ControlRequest
	if len(raw) == 0 {
		return nil, [sha256.Size]byte{}, errors.New("invalid control request size")
	}
	if err = decode(raw, &req); err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	if req.RequestID != id {
		return nil, [sha256.Size]byte{}, errors.New("request ID does not match path")
	}
	_, createdOffset := req.CreatedAt.Zone()
	_, expiryOffset := req.ExpiresAt.Zone()
	if req.ProtocolVersion != ControlProtocolVersion || !execution.ValidControlAction(req.Action) || req.InstanceID == "" || req.RepositoryIdentity == "" || strings.TrimSpace(req.RequestedBy) == "" || req.CreatedAt.IsZero() || req.ExpiresAt.IsZero() || createdOffset != 0 || expiryOffset != 0 || !req.ExpiresAt.After(req.CreatedAt) || req.ExpiresAt.Sub(req.CreatedAt) > 2*time.Minute+time.Second {
		return nil, [sha256.Size]byte{}, errors.New("control request has invalid protocol fields or target identity")
	}
	if _, err = execution.DecodeControlPayload(req); err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	return &req, sha256.Sum256(raw), nil
}

func ReadControlReceipt(c execution.Config, id string) (*productapi.ControlReceipt, error) {
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
	var receipt productapi.ControlReceipt
	if err = decode(b, &receipt); err != nil {
		return nil, err
	}
	if receipt.RequestID != id {
		return nil, errors.New("receipt request ID does not match path")
	}
	if receipt.Version != ControlProtocolVersion || (receipt.Status != "applying" && !execution.TerminalControlStatus(receipt.Status)) {
		return nil, errors.New("invalid control receipt version or status")
	}
	return &receipt, nil
}

// ReadVerifiedControlReceipt binds a receipt back to its immutable request.
// A caller must not present or act on a terminal result that names a different
// action, instance or request digest under the same path ID.
func ReadVerifiedControlReceipt(c execution.Config, id string) (*productapi.ControlReceipt, error) {
	req, requestFileDigest, err := readControlRequestForClient(c, id)
	if err != nil {
		return nil, err
	}
	receipt, err := ReadControlReceipt(c, id)
	if err != nil {
		return nil, err
	}
	if receipt.RequestID != req.RequestID || receipt.InstanceID != req.InstanceID || receipt.Action != req.Action || receipt.RequestDigest != fmt.Sprintf("%x", requestFileDigest) {
		return nil, errors.New("control receipt does not match its immutable request")
	}
	return receipt, nil
}

func WaitControlReceipt(ctx context.Context, c execution.Config, id string, poll time.Duration) (*productapi.ControlReceipt, error) {
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		if r, err := ReadVerifiedControlReceipt(c, id); err == nil {
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
