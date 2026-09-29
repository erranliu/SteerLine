package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Start requests are admitted independently of worker startup. The task claim
// transaction is the only authority for changing a task to in_progress; a
// saved request alone is shown as starting, never as a claimed task.
func runStartAdmissions(ctx context.Context, config Config, store *SQLiteStore, instanceID string) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	inFlight := map[string]bool{}
	seen := map[string]bool{}
	done := make(chan string)
	var workers sync.WaitGroup
	scan := func() {
		if ctx.Err() != nil {
			return
		}
		ids, err := controlRequestIDs(config.RunDir)
		if err != nil {
			return
		}
		for _, id := range ids {
			if inFlight[id] || seen[id] {
				continue
			}
			if _, err := ReadControlReceipt(config, id); err == nil {
				seen[id] = true
				continue
			} else if !os.IsNotExist(err) {
				continue
			}
			req, err := readControlRequest(controlRequestPath(config.RunDir, id))
			if err != nil || req.RequestID != id {
				continue
			}
			if req.Action != "start_task" {
				seen[id] = true
				continue
			}
			inFlight[id] = true
			workers.Add(1)
			go func(request ControlRequest) {
				defer workers.Done()
				_ = admitStartRequest(ctx, config, store, instanceID, request)
				select {
				case done <- request.RequestID:
				case <-ctx.Done():
				}
			}(req)
		}
	}
	scan()
	for {
		select {
		case <-ctx.Done():
			workers.Wait()
			return
		case id := <-done:
			delete(inFlight, id)
		case <-ticker.C:
			scan()
		}
	}
}

func controlRequestIDs(runDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(runDir, "control", "requests"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && controlIDPattern.MatchString(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func admitStartRequest(ctx context.Context, config Config, store *SQLiteStore, instanceID string, req ControlRequest) error {
	if ctx.Err() != nil {
		return nil
	}
	path := controlRequestPath(config.RunDir, req.RequestID)
	digest := rawRequestDigest(path)
	receipt := ControlReceipt{Version: ControlProtocolVersion, RequestID: req.RequestID, RequestDigest: digest, InstanceID: req.InstanceID, Action: req.Action, RequestedBy: req.RequestedBy, SubmittedAt: req.CreatedAt}
	finish := func(status, code, message string, claim *TaskClaimReceipt) error {
		receipt.Status, receipt.Code, receipt.Message = status, code, message
		completed := now()
		receipt.CompletedAt = &completed
		if claim != nil {
			receipt.AcceptedAt = &completed
			receipt.RunID, receipt.AttemptID = claim.WorkflowRunID, claim.AttemptID
		}
		return writeControlReceipt(config.RunDir, receipt)
	}
	if claim, found, err := savedStartClaim(ctx, store, config.identity, req); err != nil {
		return finish("indeterminate", "indeterminate", err.Error(), nil)
	} else if found {
		return finish("accepted", "claim_reserved", "任务已领取，正在准备执行", &claim)
	}
	if code, err := validateControlRequest(req, instanceID, config.identity, 0, now()); err != nil {
		return finish("rejected", code, err.Error(), nil)
	}
	payload, err := decodeControlPayload(req)
	if err != nil {
		return finish("rejected", "invalid_request", err.Error(), nil)
	}
	projection, _, err := store.LoadProjection(ctx, config.identity)
	if err != nil {
		return finish("failed", "read_failed", err.Error(), nil)
	}
	var runtime Runtime
	if json.Unmarshal(projection, &runtime) != nil || runtime.Service.InstanceID != instanceID || runtime.Service.ConfigDigest != config.LoadedConfigDigest() {
		return finish("rejected", "stale_instance", "服务实例或执行配置已变化，请刷新后重试", nil)
	}
	if !runtime.Service.Ready || runtime.Service.Status != "running" && runtime.Service.Status != "error" || runtime.Service.Mode != "scheduler" {
		return nil // A transient service checkpoint must not consume the request.
	}
	if runtime.Control.Operation != nil {
		return nil // Let a mode or stop checkpoint settle before admitting new work.
	}
	profileID := payload.ExecutionProfileID
	if profileID == "" {
		if len(config.ExecutionProfiles) != 0 {
			return finish("rejected", "invalid_request", "启动任务必须选择执行方案", nil)
		}
		profileID = ProjectExecutionProfileID
	}
	selected, profile, err := config.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest)
	if err != nil {
		return finish("rejected", "stale_profile", err.Error(), nil)
	}
	graph, err := store.taskGraphSnapshot(ctx, config.identity)
	if err != nil {
		return finish("failed", "read_failed", err.Error(), nil)
	}
	task, ok := graph.ByID[payload.TaskID]
	if !ok || task.Status != "open" {
		return finish("rejected", "task_not_ready", "任务已不是可开始状态", nil)
	}
	claim, err := (CommandService{Store: store, Project: config.identity}).ClaimNativeTask(ctx, selected, TaskClaimOptions{
		TaskID: payload.TaskID, RequestID: req.RequestID, ExpectedTaskVersion: task.Version,
		ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, RequestedBy: req.RequestedBy,
		Explicit: true, ExecutionProfile: &profile, ExpiresAt: req.ExpiresAt,
	})
	if err == nil {
		return finish("accepted", "claim_reserved", "任务已领取，正在准备执行", &claim)
	}
	if saved, found, lookupErr := savedStartClaim(context.Background(), store, config.identity, req); lookupErr == nil && found {
		return finish("accepted", "claim_reserved", "任务已领取，正在准备执行", &saved)
	} else if lookupErr != nil {
		return finish("indeterminate", "indeterminate", "领取结果待核实："+errors.Join(err, lookupErr).Error(), nil)
	}
	var native *NativeTaskError
	if errors.As(err, &native) {
		return finish("rejected", native.Code, err.Error(), nil)
	}
	return finish("failed", "failed", err.Error(), nil)
}

func savedStartClaim(ctx context.Context, store *SQLiteStore, project string, req ControlRequest) (TaskClaimReceipt, bool, error) {
	command, err := store.CommandByIdempotency(ctx, project, req.RequestID)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskClaimReceipt{}, false, nil
	}
	if err != nil {
		return TaskClaimReceipt{}, false, err
	}
	var payload ControlPayload
	if decode(req.Payload, &payload) != nil || command.Kind != "claim_task" || command.TargetID != payload.TaskID || command.RequestedBy != req.RequestedBy || command.Status != "accepted" {
		return TaskClaimReceipt{}, false, errors.New("saved claim does not match the immutable start request")
	}
	var saved struct {
		TaskID        string `json:"task_id"`
		ProfileID     string `json:"execution_profile_id"`
		ProfileDigest string `json:"execution_profile_digest"`
		Explicit      bool   `json:"explicit"`
	}
	profileID := payload.ExecutionProfileID
	if profileID == "" {
		profileID = ProjectExecutionProfileID
	}
	if json.Unmarshal(command.Payload, &saved) != nil {
		return TaskClaimReceipt{}, false, errors.New("saved claim payload is unreadable")
	}
	profileMatches := saved.ProfileID == profileID && (payload.ExecutionProfileID == "" && evidenceDigest(saved.ProfileDigest) || saved.ProfileDigest == payload.ExecutionProfileDigest)
	if saved.TaskID != payload.TaskID || !profileMatches || !saved.Explicit {
		return TaskClaimReceipt{}, false, errors.New("saved claim profile differs from the immutable start request")
	}
	var claim TaskClaimReceipt
	if json.Unmarshal(command.Result, &claim) != nil || claim.RequestID != req.RequestID || claim.TaskID != payload.TaskID || claim.Status != "claim_reserved" {
		return TaskClaimReceipt{}, false, errors.New("saved start claim has no matching reservation receipt")
	}
	return claim, true, nil
}
