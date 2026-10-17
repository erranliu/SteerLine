package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type candidateLookupStore struct {
	Store
	err error
}

type deliveryReplayStore struct {
	Store
	graph  storageports.TaskGraphSnapshot
	result taskmodel.ResultReceipt
}

func (s deliveryReplayStore) TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error) {
	return s.graph, nil
}

func (s deliveryReplayStore) TaskResult(context.Context, string, string) (taskmodel.ResultReceipt, bool, error) {
	return s.result, true, nil
}

func TestRunnerDeliveryReplayKeepsOriginalTaskReceipt(t *testing.T) {
	delivery := taskmodel.TaskDeliveryRecord{ProjectIdentity: "project", TaskID: "1", SelectedWorkflowRunID: "original", Evidence: []byte(`{"fixed":"evidence"}`)}
	store := deliveryReplayStore{
		graph: storageports.TaskGraphSnapshot{
			Authority:  taskmodel.ProjectTaskAuthority{AuthorityVersion: 99, GraphRevision: 100},
			ByID:       map[string]taskmodel.TaskRecord{"1": {TaskID: "1", Status: "closed", Version: 8}},
			Deliveries: map[string]taskmodel.TaskDeliveryRecord{"1": delivery},
		},
		result: taskmodel.ResultReceipt{Input: taskmodel.Result{ExpectedTaskVersion: 7}, TaskVersion: 8, GraphRevision: 12, Status: "closed"},
	}
	// No Runner or write implementation is available: replay must use the saved
	// task receipt even after the live graph has advanced.
	service := Service{Project: "project", Store: store}
	got, err := service.Deliver(context.Background(), "1", "original", 7)
	want := taskmodel.TaskDeliveryReceipt{TaskID: "1", WorkflowRunID: "original", TaskVersion: 8, GraphRevision: 12, Status: "closed", Delivery: delivery}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("replay changed original receipt", got, err)
	}
	for _, tc := range []struct {
		candidate string
		version   uint64
		code      string
	}{
		{"original", 8, "stale_version"},
		{"other", 7, "inconsistent_delivery"},
	} {
		got, err := service.Deliver(context.Background(), "1", tc.candidate, tc.version)
		var rejected *taskmodel.Error
		if !errors.As(err, &rejected) || rejected.Code != tc.code || got.TaskID != "" {
			t.Fatal("changed retry reused saved receipt", got, err)
		}
	}
}

func (s candidateLookupStore) Candidate(context.Context, string, string) (runnerbridge.Candidate, error) {
	return runnerbridge.Candidate{}, s.err
}

func TestRunnerStartClassifiesCandidateLookupErrors(t *testing.T) {
	lookupFailure := errors.New("candidate storage unavailable")
	admissionFailure := errors.New("admission gate reached")
	for _, tc := range []struct {
		name     string
		err      error
		admitted bool
	}{
		{"wrapped_not_found", fmt.Errorf("candidate lookup: %w", storageports.ErrNotFound), true},
		{"storage_failure", lookupFailure, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			admitted := false
			service := Service{Store: candidateLookupStore{err: tc.err}, StateDir: t.TempDir(), Admit: func(context.Context) error {
				admitted = true
				return admissionFailure
			}}
			_, err := service.Start(context.Background(), Start{RequestID: "original", TaskVersion: 1})
			matched := errors.Is(err, tc.err)
			if tc.admitted {
				var unavailable *taskmodel.Error
				matched = errors.As(err, &unavailable) && unavailable.Code == "runner_unavailable" && unavailable.Message == admissionFailure.Error()
			}
			if !matched || admitted != tc.admitted {
				t.Fatalf("error=%v admitted=%v; expected original lookup error or runner_unavailable admission failure; admitted=%v", err, admitted, tc.admitted)
			}
		})
	}
}
