package execution

import (
	"context"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/testkit/fixtures"
	"time"
)

var taskPayloadDigest = fixtures.TaskPayloadDigest

func taskDocumentsForPorts(target func(context.Context) (string, error), repository string, git executionbackend.Command, timeout time.Duration, host executionbackend.ManagedProcessHost) fixtures.TaskDocuments {
	runner := Runner{Dir: repository, Timeout: timeout, ProcessHost: host}
	return fixtures.TaskDocuments{Target: target, Output: func(ctx context.Context, args ...string) ([]byte, error) { return runner.Output(ctx, git, args...) }}
}
