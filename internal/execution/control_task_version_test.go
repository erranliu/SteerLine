package execution

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type controlVersionWork struct{ tasks []taskmodel.TaskRecord }

func (controlVersionWork) VerifyContractFiles(context.Context, taskmodel.TaskContract) error {
	return nil
}
func (w controlVersionWork) Ready(context.Context, string) ([]taskmodel.TaskRecord, error) {
	return append([]taskmodel.TaskRecord(nil), w.tasks...), nil
}
func (controlVersionWork) CanExecuteType(value string) bool { return value == "task" }
func (controlVersionWork) DefaultRequestedBy() string       { return "test" }
func (controlVersionWork) Fixed(context.Context, string, string, uint64, bool) (taskmodel.FixedWork, error) {
	return taskmodel.FixedWork{}, nil
}
func (controlVersionWork) Claim(taskmodel.FixedWork, string, time.Time) (taskmodel.TaskRecord, error) {
	return taskmodel.TaskRecord{}, nil
}

func TestStartTaskRechecksDisplayedTaskVersion(t *testing.T) {
	service := &Service{Config: Config{identity: "project"}, taskWork: controlVersionWork{tasks: []taskmodel.TaskRecord{{TaskID: "8", Type: "task", Version: 5}}}}
	if err := service.validateReadyTargetVersion(context.Background(), "8", 4); err == nil || !strings.Contains(err.Error(), "stale_version") {
		t.Fatalf("start_task silently retargeted an old task version: %v", err)
	} else if code, ok := controlRejectionCode(err); !ok || code != "stale_version" {
		t.Fatalf("stale task version lost its control receipt code: %q %t", code, ok)
	}
	if err := service.validateReadyTargetVersion(context.Background(), "8", 5); err != nil {
		t.Fatalf("current task version was rejected: %v", err)
	}
	if err := service.validateReadyTargetVersion(context.Background(), "8", 0); err != nil {
		t.Fatalf("existing control callers without an explicit version lost their established behavior: %v", err)
	}
}

func TestStartTaskControlPayloadAcceptsExpectedTaskVersion(t *testing.T) {
	request := api.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: strings.Repeat("1", 32),
		RepositoryIdentity: "project", InstanceID: strings.Repeat("2", 32), Action: "start_task",
		Payload:     []byte(`{"task_id":"8","expected_task_version":5,"execution_profile_id":"fixture","execution_profile_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		RequestedBy: "test", CreatedAt: now(), ExpiresAt: now().Add(time.Minute)}
	payload, err := DecodeControlPayload(request)
	if err != nil || payload.TaskID != "8" || payload.ExpectedTaskVersion != 5 {
		t.Fatalf("start_task version envelope was not decoded by the existing control contract: %+v %v", payload, err)
	}
}
