package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type clientDecision struct{ config Config }

func (r clientDecision) Eligibility(_ context.Context, run *api.CandidateSummary) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		return errors.New("Runner has no currently answerable question or resume request")
	}
	return nil
}
func (r clientDecision) Normalize(_ context.Context, in api.DecisionInput) (api.DecisionInput, error) {
	in.Answer.OptionID = strings.TrimSpace(in.Answer.OptionID)
	if in.CommandID == "" || in.Answer.RunID == "" || in.Answer.DecisionID == "" {
		return in, errors.New("answer requires original command, candidate and decision identities")
	}
	return in, nil
}

func sameRunnerAnswer(a, b api.DecisionAnswer) bool {
	a.AnsweredAt = time.Time{}
	b.AnsweredAt = time.Time{}
	return reflect.DeepEqual(a, b)
}

func (r clientDecision) Submit(ctx context.Context, in api.DecisionInput) (api.CommandReceipt, bool, error) {
	config := r.config
	hc, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	if old, err := host.ReadControlRequest(hc, in.CommandID); err == nil {
		payload, e := taskrunner.DecodeControlPayload(*old)
		if e != nil || payload.CallerAnswer == nil || old.RequestedBy != in.RequestedBy || !sameRunnerAnswer(*payload.CallerAnswer, in.Answer) {
			return api.CommandReceipt{}, false, taskError("idempotency_conflict", "answer command has different original input")
		}
		receipt, e := runnerAnswerReceipt(ctx, config, *old, nil)
		return receipt, false, e
	} else if !os.IsNotExist(err) {
		return api.CommandReceipt{}, false, err
	}
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	defer reader.Close()
	snapshot, err := reader.ReadRunnerSnapshot(ctx, config.Identity())
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	if snapshot.Checkpoint == nil || !snapshot.Checkpoint.Service.Ready || in.Answer.ServiceInstanceID != snapshot.Checkpoint.Service.InstanceID {
		return api.CommandReceipt{}, false, taskError("stale_instance", "answer targets another service instance")
	}
	candidate, err := reader.Candidate(ctx, config.Identity(), in.Answer.RunID)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	command, body, err := taskrunner.InputForAnswer(candidate, in.Answer)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	text := string(body)
	payload := api.ControlPayload{RunID: candidate.ID, ExpectedRunRevision: candidate.Revision, RunnerInput: &text, CallerAnswer: &in.Answer}
	revision := snapshot.Graph.Authority.AuthorityVersion
	request, err := host.SubmitControlTargeted(hc, "runner_"+command, payload, in.RequestedBy, in.CommandID, in.Answer.ServiceInstanceID, &revision)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	receipt, err := runnerAnswerReceipt(ctx, config, request, nil)
	return receipt, true, err
}

func (r clientDecision) Answer(ctx context.Context, in api.AnswerInput) (api.CommandReceipt, bool, error) {
	if in.CommandID == "" {
		in.CommandID = productclient.NewRequestID()
	}
	hc, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	if old, err := host.ReadControlRequest(hc, in.CommandID); err == nil {
		payload, e := taskrunner.DecodeControlPayload(*old)
		if e != nil || payload.CallerAnswer == nil {
			return api.CommandReceipt{}, false, taskError("idempotency_conflict", "command is not the original Runner answer")
		}
		answer := *payload.CallerAnswer
		if answer.RunID != in.RunID || in.DecisionID != "" && answer.DecisionID != in.DecisionID || answer.OptionID != strings.TrimSpace(in.Option) || answer.Note != in.Note || old.RequestedBy != in.RequestedBy || in.ExpectedVersion != 0 && answer.ExpectedDecisionVersion != in.ExpectedVersion {
			return api.CommandReceipt{}, false, taskError("idempotency_conflict", "answer has different original content")
		}
		receipt, e := runnerAnswerReceipt(ctx, r.config, *old, nil)
		return receipt, false, e
	} else if !os.IsNotExist(err) {
		return api.CommandReceipt{}, false, err
	}
	config := r.config
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, config.Identity(), in.RunID)
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	snapshot, err := reader.ReadRunnerSnapshot(ctx, config.Identity())
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	decision := taskrunner.Decision(candidate.Projection)
	if decision == nil || snapshot.Checkpoint == nil {
		return api.CommandReceipt{}, false, errors.New("original Runner question is unavailable")
	}
	if in.DecisionID != "" && in.DecisionID != decision.ID || in.ExpectedVersion != 0 && in.ExpectedVersion != decision.Version {
		return api.CommandReceipt{}, false, taskError("stale_version", "original question changed")
	}
	answer := api.DecisionAnswer{RunID: candidate.ID, DecisionID: decision.ID, OptionID: strings.TrimSpace(in.Option), Note: in.Note, AnsweredBy: in.RequestedBy, ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: candidate.Revision, SourceAttemptID: candidate.Projection.Execution.AttemptID, ExpectedRecoveryVersion: candidate.Projection.Revision, ServiceInstanceID: snapshot.Checkpoint.Service.InstanceID}
	return r.Submit(ctx, api.DecisionInput{CommandID: in.CommandID, RequestedBy: in.RequestedBy, Answer: answer})
}

func runnerAnswerReceipt(ctx context.Context, config Config, req api.ControlRequest, record *ports.CommandRecord) (api.CommandReceipt, error) {
	payload, err := taskrunner.DecodeControlPayload(req)
	if err != nil || payload.CallerAnswer == nil {
		return api.CommandReceipt{}, errors.New("original control has no public answer binding")
	}
	answer := *payload.CallerAnswer
	result := api.CommandReceipt{ProjectIdentity: config.Identity(), CommandID: req.RequestID, Kind: "answer_decision", TargetID: answer.DecisionID, ExpectedVersion: answer.ExpectedDecisionVersion, RequestedBy: req.RequestedBy, DecisionAnswer: &answer, Status: "pending", CreatedAt: req.CreatedAt, UpdatedAt: req.CreatedAt}
	if record != nil {
		var control api.ControlReceipt
		if len(record.Result) > 0 && json.Unmarshal(record.Result, &control) == nil && control.Status != "applied" {
			result.Status = control.Status
			result.ResultCode = control.Code
			result.ResultError = control.Message
			return result, nil
		}
	}
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return result, err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, config.Identity(), payload.RunID)
	if err == nil {
		for _, action := range candidate.Actions {
			if action.Request.RequestID == "sl-input-"+req.RequestID {
				result.UpdatedAt = time.Now().UTC()
				switch action.State {
				case "cancelled":
					result.Status = "rejected"
					result.ResultCode = "fresh_reply_required"
					result.ResultError = "unused input was withdrawn at the caller service boundary"
				case "unknown":
					result.Status = "indeterminate"
					result.ResultCode = "request_outcome_unknown"
				case "rejected":
					result.Status = "rejected"
					result.ResultCode = action.ErrorCode
				case "accepted":
					switch action.ActionStatus {
					case "completed":
						result.Status = "accepted"
					case "failed", "rejected", "cancelled":
						result.Status = "failed"
						result.ResultCode = "runner_input_failed"
					case "unknown":
						result.Status = "indeterminate"
						result.ResultCode = "request_outcome_unknown"
					}
				}
				return result, nil
			}
		}
	} else if !errors.Is(err, ports.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
		// A valid candidate is part of every accepted input; missing it is not a new submission permission.
		result.ResultCode = "runner_binding_unavailable"
	}
	hc, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return result, err
	}
	if receipt, err := host.ReadVerifiedControlReceipt(hc, req.RequestID); err == nil && receipt != nil && receipt.Status != "applied" {
		result.Status = receipt.Status
		result.ResultCode = receipt.Code
		result.ResultError = receipt.Message
	}
	return result, nil
}

func AnswererName() string {
	for _, key := range []string{"STEERLINE_ANSWERED_BY", "USERNAME", "USER"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "unknown"
}
