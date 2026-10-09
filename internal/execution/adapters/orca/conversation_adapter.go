package orcaworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

const outputSource = "orca-output-material"

type ConversationAdapter struct{}

func NewConversationAdapter() conversationtypes.ConversationSourcePort { return &ConversationAdapter{} }

func (*ConversationAdapter) DiscoverHistory(ctx context.Context, discovery conversationtypes.ConversationDiscovery) (conversationtypes.ConversationDiscoveryResult, error) {
	result := conversationtypes.ConversationDiscoveryResult{}
	for _, run := range discovery.Runs {
		base := run.Base
		if base.Backend != executionbackend.BackendOrca || base.Protocol != executionbackend.ProtocolOrcaTerminalV2 || base.ProjectID != discovery.ProjectID {
			continue
		}
		paths, err := filepath.Glob(filepath.Join(run.RunDirectory, "turns", "*", "orca-output.*.json"))
		if err != nil {
			return result, err
		}
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			binding, err := readOutputBinding(path)
			if err != nil {
				return result, err
			}
			if binding.ProjectID != base.ProjectID || binding.WorkflowID != base.WorkflowID || binding.ExecutionID != base.ExecutionID || !samePath(binding.Workspace, run.Workspace) {
				return result, errors.New("Orca output binding changed its original execution/workspace")
			}
			event := base
			event.AttemptID = binding.AttemptID
			event.Source, event.SourceRole, event.SourceIdentity = outputSource, executionmodel.ConversationSourceCanonical, outputBindingIdentity(binding)
			event.SourceSegment = event.SourceIdentity
			event.TurnID, event.TurnQuality = binding.TurnID, executionmodel.IdentityDerived
			event.SessionID, event.SessionQuality = SessionID(binding.Process), executionmodel.IdentityDerived
			result.Sources = append(result.Sources, conversationtypes.ConversationHistorySource{Identity: event.SourceIdentity, BindingPath: path, Workspace: binding.Workspace, Event: event})
		}
	}
	return result, nil
}

func readOutputBinding(path string) (outputBinding, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return outputBinding{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return outputBinding{}, errors.New("Orca output binding is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return outputBinding{}, err
	}
	var binding outputBinding
	if err = executionbackend.DecodeStrictJSON(raw, &binding, "Orca output binding has trailing data"); err != nil {
		return binding, err
	}
	if binding.Version != 1 || binding.ProjectID == "" || binding.ExecutionID == "" || binding.AttemptID == "" || !filepath.IsAbs(binding.Workspace) || binding.Command.Executable == "" || binding.Process.RuntimeID == "" || binding.Process.Incarnation == "" || binding.Process.Dispatch == "" || binding.Process.Task == "" || binding.Process.OrchestrationRun == "" {
		return binding, errors.New("Orca output binding lacks its frozen original identity")
	}
	if _, err = outputRelativePath(binding.WorkflowID, binding.TurnID); err != nil {
		return binding, err
	}
	return binding, nil
}

func outputBindingIdentity(binding outputBinding) string {
	return "orca-output:" + digestValue(binding)
}

func (*ConversationAdapter) ReadHistory(ctx context.Context, request conversationtypes.ConversationHistoryReadRequest, start func(*exec.Cmd) (executionbackend.StopHandle, error)) (conversationtypes.ConversationHistoryBatch, error) {
	binding, err := readOutputBinding(request.Source.BindingPath)
	if err != nil {
		return conversationtypes.ConversationHistoryBatch{}, err
	}
	if outputBindingIdentity(binding) != request.Source.Identity || binding.AttemptID != request.Source.Event.AttemptID || binding.TurnID != request.Source.Event.TurnID || binding.ProjectID != request.Source.Event.ProjectID || binding.WorkflowID != request.Source.Event.WorkflowID || binding.ExecutionID != request.Source.Event.ExecutionID || !samePath(binding.Workspace, request.Source.Workspace) || request.Source.Event.Backend != executionbackend.BackendOrca || request.Source.Event.Protocol != executionbackend.ProtocolOrcaTerminalV2 {
		return conversationtypes.ConversationHistoryBatch{}, errors.New("Orca output source changed its original binding")
	}
	// Platform reads use only the Orca executable frozen by the native launch.
	client := New(Config{Repository: binding.Workspace, Output: func(ctx context.Context, _ time.Duration, args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, binding.Command.Executable, append(append([]string(nil), binding.Command.Args...), args...)...)
		command.Dir = binding.Workspace
		if start == nil {
			return command.Output()
		}
		return managedConversationOutput(command, start)
	}})
	material, err := readSettledOutput(request.Source.BindingPath, binding)
	if err != nil {
		return conversationtypes.ConversationHistoryBatch{}, err
	}
	if material == nil {
		material, err = readOutputMaterial(ctx, client.MetaCall, binding)
		if err != nil {
			return conversationtypes.ConversationHistoryBatch{}, err
		}
		if material != nil {
			if err = freezeSettledOutput(request.Source.BindingPath, binding, *material); err != nil {
				return conversationtypes.ConversationHistoryBatch{}, err
			}
		}
	}
	if material == nil {
		return conversationtypes.ConversationHistoryBatch{Cursor: request.Cursor}, nil
	}
	digest := digestValue(*material)
	if request.Cursor != "" && request.Cursor != digest {
		return conversationtypes.ConversationHistoryBatch{}, errors.New("settled worker output material was modified")
	}
	if request.Cursor == digest {
		return conversationtypes.ConversationHistoryBatch{Cursor: digest, Sequence: 2, Settled: true}, nil
	}
	event := request.Source.Event
	event.Version, event.ReceivedAt = executionmodel.ConversationVersion, time.Now().UTC()
	event.SourceEventQuality, event.ItemQuality = executionmodel.IdentityDerived, executionmodel.IdentityDerived
	event.ItemID = "response"
	event.ItemKey = event.ItemIdentity()
	event.SourceEventID, event.SourceSeq = "response:"+digest, 1
	event.Type, event.Phase = executionmodel.ConversationMessageComplete, executionmodel.MessagePhaseFinalAnswer
	event.FinalAvailability = executionmodel.FinalAvailable
	event.Evidence = map[string]string{"phase_origin": "explicit_worker_output_material", "settlement": "original_orca_dispatch", "dispatch_id": binding.Process.Dispatch}
	event.Body = &executionmodel.ConversationBodyRef{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, ItemKey: event.ItemKey, Revision: 1, Source: outputSource, Completeness: executionmodel.BodyComplete}
	body := executionmodel.ConversationWrite{Event: event, Body: []byte(material.Body), BodyMode: "complete", BodySource: outputSource}
	complete := event
	complete.Body = nil
	complete.ItemKey = ""
	complete.ItemID = ""
	complete.ItemQuality = executionmodel.IdentityUnknown
	complete.SourceEventID, complete.SourceSeq = "settled:"+binding.Process.Dispatch, 2
	complete.Type, complete.Status = executionmodel.ConversationTurnCompleted, material.Outcome
	complete.Phase = executionmodel.MessagePhaseUnknown
	if material.Outcome == "failed" {
		complete.Type = executionmodel.ConversationTurnFailed
	}
	report := &executionmodel.ConversationCapabilityReport{Version: 1, Backend: event.Backend, Protocol: event.Protocol, Agent: event.Agent, Target: executionbackend.ExecutionTarget{ID: event.TargetID}, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: outputSource, SourceIdentity: request.Source.Identity, CheckedAt: event.ReceivedAt, Capabilities: []executionmodel.ConversationCapability{{Name: "final_content", Status: executionbackend.CapabilityVerified, Evidence: "complete explicit output material and original Orca Dispatch settlement"}, {Name: "message_phase", Status: executionbackend.CapabilityVerified, Evidence: "phase is declared by the generic worker output protocol, not inferred from provider transcript"}}}
	return conversationtypes.ConversationHistoryBatch{Writes: []executionmodel.ConversationWrite{body, {Event: complete}}, Cursor: digest, Sequence: 2, Settled: true, Capabilities: report}, nil
}

func managedConversationOutput(command *exec.Cmd, start func(*exec.Cmd) (executionbackend.StopHandle, error)) ([]byte, error) {
	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output
	handle, err := start(command)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	err = command.Wait()
	return []byte(output.String()), err
}
