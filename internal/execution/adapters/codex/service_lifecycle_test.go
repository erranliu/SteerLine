package codexworker

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

func TestServiceStopWorkerHelper(t *testing.T) {
	if os.Getenv("STEERLINE_TEST_STOP_HELPER") != "1" {
		t.Skip("owned process helper")
	}
	for {
		time.Sleep(time.Minute)
	}
}

func TestServiceStopWorkerFacts(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServiceStopWorkerHelper$")
	cmd.Env = append(os.Environ(), "STEERLINE_TEST_STOP_HELPER=1")
	owned, err := resourceprocess.StartManaged(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	defer cmd.Wait()
	identity := owned.Identity()
	if owned.WritersAbsent() {
		t.Fatal("live owned job was reported empty")
	}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{External: identity},
		Session:             executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt"},
		ConfigurationDigest: "config", ResourceGeneration: "generation"}
	l := ExecutionLifecycle{setup: executionbackend.LifecycleSetup{OwnedProcess: owned}, Backend: Backend{Process: ProcessPort{Inspect: resourceprocess.InspectProcess}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	facts, err := l.ShutdownLifecycle(ctx, snapshot, owned)
	if err != nil || !facts.WorkerAbsent || !facts.WritersAbsent || !facts.Stopped || facts.TurnCompleted {
		t.Fatalf("owned worker release facts: %+v, %v", facts, err)
	}
	if l.PolicyCapabilities().ServiceStopBehavior != executionbackend.StopExecutionOnServiceStop {
		t.Fatal("local worker did not declare explicit stop behavior")
	}
}
