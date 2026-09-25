package steerline

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) processPendingCommands(ctx context.Context) error {
	if s.domainStore == nil {
		return nil
	}
	service := CommandService{Store: s.domainStore, Project: s.Config.identity}
	// Settle commands that might already have caused side effects before
	// considering newly queued work. Otherwise a pending retry can start a
	// worker before an older in-flight retry restores its blocker.
	for _, status := range []string{"in_flight", "pending"} {
		commands, err := s.domainStore.serviceCommands(ctx, s.Config.identity, status, 200)
		if err != nil {
			return err
		}
		for _, command := range commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			if command.Kind != "answer_decision" {
				if !validServiceCommandKind(command.Kind) {
					// Native task, dispatch and cleanup commands share the center's
					// commands table. Their own transaction/reconciler owns their status.
					continue
				}
				if command.Status == "in_flight" {
					if err = s.recoverInFlightServiceCommand(ctx, service, command); err != nil {
						return err
					}
					continue
				}
				if err = service.SetStatus(ctx, command, "in_flight", nil); err != nil {
					return err
				}
				command.Status = "in_flight"
				status, result := s.applyServiceCommand(ctx, command)
				if status == "" {
					status = "rejected"
				}
				if s.native && command.Kind == "retry_run" && status == "indeterminate" {
					// Keep the outer command in_flight until the center retry receipt
					// can be read. A later pass reconciles it without starting again.
					return fmt.Errorf("native retry outcome requires center reconciliation: %s", result)
				}
				if err = service.SetStatus(ctx, command, status, result); err != nil {
					return err
				}
				continue
			}
			var answer DecisionAnswer
			if err = decode(command.Payload, &answer); err != nil {
				result, _ := json.Marshal(map[string]string{"error": "invalid decision answer payload"})
				if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
					return e
				}
				continue
			}
			if command.Status == "pending" {
				if err = service.SetStatus(ctx, command, "in_flight", nil); err != nil {
					return err
				}
				command.Status = "in_flight"
			}
			if run := s.findDecision(command.TargetID); run != nil && run.Decision != nil && run.Decision.Status == "pending" {
				if err = s.requireDecisionReplyCapability(ctx, run); err != nil {
					code := "rejected"
					var backendErr *BackendError
					if errors.As(err, &backendErr) {
						code = string(backendErr.Code)
					}
					result, _ := json.Marshal(map[string]string{"code": code, "error": err.Error()})
					if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
						return e
					}
					continue
				}
			}
			record, _, acceptErr := service.AcceptAnswer(ctx, command, answer)
			if acceptErr != nil {
				code := "rejected"
				if errors.Is(acceptErr, ErrAnswerConflict) {
					code = "answer_conflict"
				}
				result, _ := json.Marshal(map[string]string{"code": code, "error": acceptErr.Error()})
				if err = service.SetStatus(ctx, command, "rejected", result); err != nil {
					return err
				}
				continue
			}
			run := s.findDecision(command.TargetID)
			if run != nil && run.Decision != nil && run.Decision.ID == command.TargetID {
				if record.WorkflowRunID != run.RunID || record.ExecutionID != run.ExecutionID || record.AttemptID != run.Decision.AttemptID {
					return fmt.Errorf("accepted decision %s no longer matches the runtime identity", command.TargetID)
				}
				run.Decision.Version = record.Version
				run.Decision.Status = record.Status
				if len(record.Answer) > 0 {
					var saved DecisionAnswer
					if err = decode(record.Answer, &saved); err != nil {
						return fmt.Errorf("decode accepted decision answer: %w", err)
					}
					run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = saved.OptionID, saved.Note, saved.AnsweredBy
					run.Decision.AnsweredAt = &saved.AnsweredAt
				}
				s.note(run, "steer", "decision", "answer_saved", "decision answer saved", decisionAnswerSummary(run), decisionAnswerMeta(run))
				if err = s.save(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type serviceCommandResult struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	CommandID         string `json:"command_id"`
	RunID             string `json:"run_id,omitempty"`
	AttemptID         string `json:"attempt_id,omitempty"`
	PreviousAttemptID string `json:"previous_attempt_id,omitempty"`
	ExpectedRevision  uint64 `json:"expected_revision,omitempty"`
	ResultRevision    uint64 `json:"result_revision,omitempty"`
	ResultSHA256      string `json:"result_sha256,omitempty"`
	Basis             string `json:"basis,omitempty"`
	OriginalRequest   string `json:"original_request_id,omitempty"`
	ResolvedBy        string `json:"resolved_by_request_id,omitempty"`
	RequestedBy       string `json:"requested_by,omitempty"`
	Resolution        string `json:"resolution,omitempty"`
}

func (s *Service) applyServiceCommand(ctx context.Context, command CommandRecord) (string, []byte) {
	reject := func(code, message string) (string, []byte) {
		return "rejected", mustJSON(serviceCommandResult{Code: code, Message: message, CommandID: command.CommandID, RunID: command.TargetID})
	}
	if s.State.Service.Status != "running" || !s.State.Service.Ready {
		return reject("stale_instance", "service is not ready to accept recovery changes")
	}
	if s.State.Control.Operation != nil {
		return reject("slot_busy", "another control operation is still applying")
	}
	var envelope ServiceCommandPayload
	if err := decode(command.Payload, &envelope); err != nil {
		return reject("invalid_request", "invalid recovery command envelope: "+err.Error())
	}
	if envelope.ServiceInstanceID != s.State.Service.InstanceID {
		return reject("stale_instance", "command targets a different service instance")
	}
	if envelope.ExpectedRevision != command.ExpectedVersion {
		return reject("invalid_request", "command envelope revision differs from its command row")
	}
	var body RecoveryCommandBody
	if err := decode(envelope.Body, &body); err != nil {
		return reject("invalid_request", "invalid recovery command payload: "+err.Error())
	}
	if command.Kind == "resolve_command" {
		if s.State.Control.Revision != envelope.ExpectedRevision {
			return reject("stale_revision", "control blockers changed; refresh before resolving")
		}
		result, err := s.resolveControlBlocker(ctx, command.TargetID, command.RequestedBy, body.ConfirmationBasis)
		if err != nil {
			return reject("recovery_rejected", err.Error())
		}
		return "accepted", mustJSON(serviceCommandResult{Code: "resolved", Message: "one indeterminate blocker was reconciled without replaying the original action", CommandID: command.CommandID, OriginalRequest: command.TargetID, ResolvedBy: command.CommandID, RequestedBy: command.RequestedBy, ExpectedRevision: envelope.ExpectedRevision, ResultRevision: s.State.Control.Revision, Resolution: fmt.Sprint(result["resolution"]), Basis: body.ConfirmationBasis})
	}
	run := s.recoveryTarget(command.TargetID)
	if run == nil {
		return reject("not_found", "target run was not found")
	}
	if run.ControlRevision != envelope.ExpectedRevision || run.AttemptID != envelope.RunAttemptID {
		return reject("stale_revision", "run or attempt identity changed; refresh the recovery report")
	}
	beforeRevision, beforeAttempt := run.ControlRevision, run.AttemptID
	switch command.Kind {
	case "retry_run":
		if !body.ConfirmedProcessStopped {
			return reject("confirmation_required", "operator must explicitly confirm that the original execution stopped")
		}
		if s.native {
			if run.WorkflowVersion != MinimalWorkflowVersion || s.State.Control.DispatchMode != "manual" || s.State.Control.DispatchPaused {
				return reject("recovery_rejected", "native retry requires a minimal-v1 run and unpaused manual dispatch")
			}
			startErr := s.startNativeRetryFor(ctx, run.RunID, command.RequestedBy)
			if startErr != nil {
				if receipt, applied, proofErr := s.nativeRetryProof(ctx, run, beforeAttempt, command.RequestedBy); proofErr == nil && applied {
					return "accepted", mustJSON(serviceCommandResult{Code: "retry_admitted", Message: "native retry was admitted; current step state requires reconciliation: " + startErr.Error(), CommandID: command.CommandID, RunID: run.RunID, AttemptID: receipt.AttemptID, PreviousAttemptID: beforeAttempt, RequestedBy: command.RequestedBy, ExpectedRevision: beforeRevision})
				} else if proofErr != nil {
					return "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: errors.Join(startErr, proofErr).Error(), CommandID: command.CommandID, RunID: run.RunID, PreviousAttemptID: beforeAttempt})
				}
				return reject("recovery_rejected", startErr.Error())
			}
			current := s.recoveryTarget(run.RunID)
			if current == nil || current.AttemptID == beforeAttempt {
				return "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: "native retry returned without a new fixed step attempt", CommandID: command.CommandID, RunID: run.RunID, PreviousAttemptID: beforeAttempt})
			}
			return "accepted", mustJSON(serviceCommandResult{Code: "worker_started", Message: "native retry created a new step attempt and saved its worker identity", CommandID: command.CommandID, RunID: current.RunID, AttemptID: current.AttemptID, PreviousAttemptID: beforeAttempt, RequestedBy: command.RequestedBy, ExpectedRevision: beforeRevision, ResultRevision: current.ControlRevision, Basis: body.ConfirmationBasis})
		}
		oldAttempt := run.AttemptID
		if err := s.startRetry(ctx, run.RunID); err != nil {
			if run.AttemptID != oldAttempt && run.ExecutionStatus == "unknown" {
				blocker := ControlBlocker{RequestID: command.CommandID, Action: command.Kind, TaskID: run.TaskID, RunID: run.RunID, AttemptID: run.AttemptID, Reason: "retry start outcome is indeterminate; no worker start will be replayed"}
				s.State.Control.Blockers = append(s.State.Control.Blockers, blocker)
				s.recalculateCurrentReservation("")
				if saveErr := s.save(); saveErr != nil {
					return "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: errors.Join(err, saveErr).Error(), CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID})
				}
				return "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: err.Error(), CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID})
			}
			if run.AttemptID != oldAttempt {
				return "failed", mustJSON(serviceCommandResult{Code: "failed", Message: err.Error(), CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID})
			}
			return reject("recovery_rejected", err.Error())
		}
		return "accepted", mustJSON(serviceCommandResult{Code: "worker_started", Message: "retry created a new attempt and saved its worker identity", CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID, PreviousAttemptID: beforeAttempt, RequestedBy: command.RequestedBy, ExpectedRevision: beforeRevision, ResultRevision: run.ControlRevision, Basis: body.ConfirmationBasis})
	case "mark_stopped":
		if !body.ConfirmedProcessStopped {
			return reject("confirmation_required", "operator must explicitly confirm that the original execution stopped")
		}
		if err := s.markStoppedCore(run.RunID, command.RequestedBy, body.ConfirmationBasis); err != nil {
			return reject("recovery_rejected", err.Error())
		}
		return "accepted", mustJSON(serviceCommandResult{Code: "stopped_confirmed", Message: "unknown execution was recorded as failed/blocked; no worker was started", CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID, RequestedBy: command.RequestedBy, ExpectedRevision: beforeRevision, ResultRevision: run.ControlRevision, Basis: body.ConfirmationBasis})
	case "accept_result":
		if !body.ConfirmedProcessStopped {
			return reject("confirmation_required", "operator must explicitly confirm that the original execution stopped")
		}
		data, err := base64.StdEncoding.DecodeString(body.ResultBase64)
		if err != nil {
			return reject("invalid_result", "result bytes are not valid base64")
		}
		if err = s.acceptResultBytes(ctx, run.RunID, command.CommandID, command.RequestedBy, body.ConfirmationBasis, data, body.ResultSHA256); err != nil {
			return reject("recovery_rejected", err.Error())
		}
		return "accepted", mustJSON(serviceCommandResult{Code: "result_accepted", Message: "frozen result and verified PR are bound to this run; PR merge verification remains pending", CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID, RequestedBy: command.RequestedBy, ExpectedRevision: beforeRevision, ResultRevision: run.ControlRevision, ResultSHA256: body.ResultSHA256, Basis: body.ConfirmationBasis})
	default:
		return reject("unsupported_command", "unsupported recovery command")
	}
}

func (s *Service) recoverInFlightServiceCommand(ctx context.Context, commands CommandService, command CommandRecord) error {
	if s.native && command.Kind == "retry_run" {
		var envelope ServiceCommandPayload
		if err := decode(command.Payload, &envelope); err != nil || !stableExecutionID(envelope.RunAttemptID) {
			reason := "saved native retry has no valid original attempt identity; its in-flight effect cannot be ruled out"
			s.State.Control.Blockers = appendUniqueBlocker(s.State.Control.Blockers, ControlBlocker{RequestID: command.CommandID, Action: command.Kind, RunID: command.TargetID, Reason: reason})
			if err := s.save(); err != nil {
				return err
			}
			return commands.SetStatus(ctx, command, "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: reason, CommandID: command.CommandID, RunID: command.TargetID}))
		}
		run := s.recoveryTarget(command.TargetID)
		if run == nil {
			reason := "native retry run is missing; original center attempt cannot be verified and no execution was replayed"
			s.State.Control.Blockers = appendUniqueBlocker(s.State.Control.Blockers, ControlBlocker{RequestID: command.CommandID, Action: command.Kind, RunID: command.TargetID, AttemptID: envelope.RunAttemptID, Reason: reason})
			if err := s.save(); err != nil {
				return err
			}
			return commands.SetStatus(ctx, command, "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: reason, CommandID: command.CommandID, RunID: command.TargetID}))
		}
		receipt, applied, err := s.nativeRetryProof(ctx, run, envelope.RunAttemptID, command.RequestedBy)
		if err != nil {
			return err
		}
		if !applied {
			return commands.SetStatus(ctx, command, "rejected", mustJSON(serviceCommandResult{Code: "not_applied", Message: "no center native retry transaction was committed; no worker was started", CommandID: command.CommandID, RunID: run.RunID}))
		}
		return commands.SetStatus(ctx, command, "accepted", mustJSON(serviceCommandResult{Code: "retry_admitted", Message: "reconciled the fixed native step attempt; current step state remains authoritative", CommandID: command.CommandID, RunID: run.RunID, AttemptID: receipt.AttemptID, PreviousAttemptID: envelope.RunAttemptID, RequestedBy: command.RequestedBy}))
	}
	if command.Kind == "resolve_command" {
		return commands.SetStatus(ctx, command, "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: "service restarted while resolving; blocker presence alone cannot prove this command's result, and no instruction was replayed", CommandID: command.CommandID, OriginalRequest: command.TargetID, RequestedBy: command.RequestedBy}))
	}
	if command.Kind == "accept_result" {
		var envelope ServiceCommandPayload
		var body RecoveryCommandBody
		if decode(command.Payload, &envelope) == nil && decode(envelope.Body, &body) == nil {
			if run := s.recoveryTarget(command.TargetID); run != nil && run.FinishTaskStatus == "waiting_pr_merge" && run.ResultPath != nil {
				expectedPath := filepath.Join(s.Config.RunDir, "runs", run.RunID, "recovery", command.CommandID, "result.json")
				if canonical(*run.ResultPath) == canonical(expectedPath) {
					data, readErr := os.ReadFile(expectedPath)
					if readErr == nil && operationDigest(data) == body.ResultSHA256 {
						result, parseErr := ParseResultBytes(data, run)
						if parseErr == nil && result.PRURL != nil {
							if _, verifyErr := s.verifyRecoveryPR(ctx, run, result); verifyErr == nil {
								return commands.SetStatus(ctx, command, "accepted", mustJSON(serviceCommandResult{Code: "result_accepted", Message: "reconciled the durable managed result and current provider PR", CommandID: command.CommandID, RunID: run.RunID, AttemptID: run.AttemptID, RequestedBy: command.RequestedBy}))
							}
						}
					}
				}
			}
		}
	}
	var envelope ServiceCommandPayload
	if decode(command.Payload, &envelope) != nil {
		envelope = ServiceCommandPayload{}
	}
	var body RecoveryCommandBody
	_ = decode(envelope.Body, &body)
	reason := "service restarted during an in-flight recovery command; no command was replayed"
	if run := s.recoveryTarget(command.TargetID); run != nil && run.FinishTaskStatus != "completed" {
		if run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || command.Kind == "retry_run" {
			run.ExecutionStatus = "unknown"
			s.blocked(run, reason)
			s.State.CurrentTask = ptr(run.TaskID)
		}
		s.State.Control.Blockers = appendUniqueBlocker(s.State.Control.Blockers, ControlBlocker{RequestID: command.CommandID, Action: command.Kind, TaskID: run.TaskID, RunID: run.RunID, AttemptID: run.AttemptID, Reason: reason})
	}
	s.recalculateCurrentReservation("")
	if err := s.save(); err != nil {
		return err
	}
	return commands.SetStatus(ctx, command, "indeterminate", mustJSON(serviceCommandResult{Code: "indeterminate", Message: reason, CommandID: command.CommandID, RunID: command.TargetID, RequestedBy: command.RequestedBy}))
}

func (s *Service) nativeRetryProof(ctx context.Context, run *Run, previousAttemptID, requestedBy string) (NativeStepRetryReceipt, bool, error) {
	if s.domainStore == nil || run == nil || !stableExecutionID(previousAttemptID) || !stableExecutionID(run.ExecutionID) {
		return NativeStepRetryReceipt{}, false, errors.New("native retry proof requires the fixed center run and original attempt")
	}
	requestID := nativeRetryRequestID(run.DomainTaskID, run.RunID, run.ExecutionID, previousAttemptID)
	command, err := s.domainStore.Command(ctx, s.Config.identity, requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return NativeStepRetryReceipt{}, false, nil
	}
	if err != nil {
		return NativeStepRetryReceipt{}, false, err
	}
	if command.ProjectIdentity != s.Config.identity || command.Kind != "retry_step" || command.TargetID != run.DomainTaskID || command.RequestedBy != strings.TrimSpace(requestedBy) || command.Status != "accepted" {
		return NativeStepRetryReceipt{}, false, errors.New("native retry center command identity or status conflicts with the recovery request")
	}
	var receipt NativeStepRetryReceipt
	if err = decode(command.Result, &receipt); err != nil || receipt.RequestID != requestID || receipt.TaskID != run.DomainTaskID || receipt.WorkflowRunID != run.RunID || receipt.ExecutionID != run.ExecutionID || receipt.PreviousAttempt != previousAttemptID || !stableExecutionID(receipt.AttemptID) {
		return NativeStepRetryReceipt{}, false, errors.New("native retry center receipt conflicts with the fixed workflow and attempt")
	}
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	if err != nil {
		return NativeStepRetryReceipt{}, false, err
	}
	for _, attempt := range snapshot.Attempts {
		if attempt.AttemptID == receipt.AttemptID && attempt.ExecutionID == receipt.ExecutionID && attempt.StepID == receipt.StepID {
			return receipt, true, nil
		}
	}
	return NativeStepRetryReceipt{}, false, errors.New("native retry receipt has no matching durable step attempt")
}

func appendUniqueBlocker(blockers []ControlBlocker, blocker ControlBlocker) []ControlBlocker {
	for _, existing := range blockers {
		if existing.RequestID == blocker.RequestID {
			return blockers
		}
	}
	return append(blockers, blocker)
}

func (s *Service) findDecision(id string) *Run {
	for _, run := range s.State.Runs {
		if run != nil && run.Decision != nil && run.Decision.ID == id {
			return run
		}
	}
	return nil
}

func (s *Service) processCommandsAndDecisions(ctx context.Context) error {
	if err := s.processPendingCommands(ctx); err != nil {
		return err
	}
	return s.applyDecisions(ctx)
}
