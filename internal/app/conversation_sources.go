package app

import (
	"context"
	"os/exec"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
)

type conversationSources struct {
	local func() conversationtypes.ConversationSourcePort
	orca  conversationtypes.ConversationSourcePort
}

func newConversationSources() conversationtypes.ConversationSourcePort {
	return conversationSources{local: codexworker.NewConversationAdapter, orca: orcaworker.NewConversationAdapter()}
}

func isNativeOrcaSource(source conversationtypes.ConversationRunSource) bool {
	return source.Base.Backend == executionbackend.BackendOrca && source.Base.Protocol == executionbackend.ProtocolOrcaTerminalV2
}

func (ports conversationSources) DiscoverHistory(ctx context.Context, discovery conversationtypes.ConversationDiscovery) (conversationtypes.ConversationDiscoveryResult, error) {
	local, orca := discovery, discovery
	local.Runs = nil
	orca.Runs = nil
	for _, run := range discovery.Runs {
		if isNativeOrcaSource(run) {
			orca.Runs = append(orca.Runs, run)
		} else {
			local.Runs = append(local.Runs, run)
		}
	}
	var result conversationtypes.ConversationDiscoveryResult
	if len(local.Runs) > 0 {
		var err error
		result, err = ports.local().DiscoverHistory(ctx, local)
		if err != nil {
			return result, err
		}
	}
	native, err := ports.orca.DiscoverHistory(ctx, orca)
	if err != nil {
		return result, err
	}
	result.Sources = append(result.Sources, native.Sources...)
	result.Writes = append(result.Writes, native.Writes...)
	return result, nil
}

func (ports conversationSources) ReadHistory(ctx context.Context, request conversationtypes.ConversationHistoryReadRequest, start func(*exec.Cmd) (executionbackend.StopHandle, error)) (conversationtypes.ConversationHistoryBatch, error) {
	if request.Source.Event.Backend == executionbackend.BackendOrca && request.Source.Event.Protocol == executionbackend.ProtocolOrcaTerminalV2 {
		return ports.orca.ReadHistory(ctx, request, start)
	}
	return ports.local().ReadHistory(ctx, request, start)
}
