package execution

import (
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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

func TestControlFailureCodeReportsUnsupportedAdapter(t *testing.T) {
	for name, err := range map[string]error{
		"backend":  &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "structured final evidence is unavailable"},
		"adapter":  executionbackend.UnsupportedAdapterOperation("observe", "structured final evidence is unavailable"),
		"native":   nativeErr("unsupported", "structured final evidence is unavailable"),
		"ordinary": &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "temporary connection failure"},
	} {
		want := "unsupported"
		if name == "ordinary" {
			want = "failed"
		}
		if got := controlFailureCode(err); got != want {
			t.Fatalf("%s capability failure code = %q, want %q", name, got, want)
		}
	}
}
