package execution

import (
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
)

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
