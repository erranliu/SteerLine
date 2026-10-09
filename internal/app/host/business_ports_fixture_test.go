package host

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/testkit/fixtures"
)

type neutralBackend = fixtures.Backend

func newNeutralBackend(c execution.Config) neutralBackend {
	return neutralBackend{Inspect: func(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
		return inspectTestWorkspace(ctx, c, ref)
	}}
}

type neutralLifecycle = fixtures.Lifecycle
type neutralConversationSources = fixtures.ConversationSources
