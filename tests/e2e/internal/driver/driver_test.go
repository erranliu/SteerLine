package driver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

// The subprocess exercises Client.Tasks through the real CLI transport without
// creating a project, database, service or Runner instance.
func TestMain(m *testing.M) {
	if protocol.RunRunnerServiceFixture() {
		return
	}
	if os.Getenv("GO_TEST_TASK_PAGE_FIXTURE") == "1" {
		if len(os.Args) != 5 || os.Args[1] != "task" || os.Args[2] != "list" || os.Args[3] != "-config" || os.Args[4] != "fixture-config" {
			os.Exit(2)
		}
		fmt.Print(`{"protocol":"native-tasks/v1","project_id":"project","tasks":[{"task_id":"1","status":"closed","workflow_runs":[{"workflow_run_id":"candidate","execution_id":"run","candidate_number":2}],"delivery":{"workflow_run_id":"candidate","execution_id":"run","pr_url":"https://example.invalid/pr/7"}}]}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestTaskListDriverRetainsPublicCandidateAndDeliveryIdentity(t *testing.T) {
	t.Setenv("GO_TEST_TASK_PAGE_FIXTURE", "1")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Binary: binary, Config: "fixture-config", Home: t.TempDir()}
	page, err := client.Tasks(context.Background())
	if err != nil || page.Protocol != "native-tasks/v1" || len(page.Tasks) != 1 {
		t.Fatal("public task page was lost", page, err)
	}
	task := page.Tasks[0]
	if len(task.WorkflowRuns) != 1 || task.WorkflowRuns[0].WorkflowRunID != "candidate" || task.WorkflowRuns[0].ExecutionID != "run" || task.WorkflowRuns[0].CandidateNumber != 2 || task.Delivery == nil || task.Delivery.WorkflowRunID != "candidate" || task.Delivery.ExecutionID != "run" {
		t.Fatal("CLI task decoder used storage facts instead of public DTOs", task)
	}
}

func TestProjectEnvironmentRejectsInheritedRuntimeAndTestSwitches(t *testing.T) {
	got := strings.Join(Environment([]string{"Path=tools", "STEERLINE_HOME=production", "STEERLINE_REAL_CODEX=1", "STEERLINE_WORKER_FLOW_MOCK=1", "CODEX_HOME=real", "TEMP=old", "GOFLAGS=-tags=e2e"}, "isolated", "state"), "\n")
	for _, bad := range []string{"production", "REAL_CODEX", "WORKER_FLOW_MOCK", "CODEX_HOME=real", "TEMP=old", "-tags=e2e"} {
		if strings.Contains(got, bad) {
			t.Fatalf("inherited %s: %s", bad, got)
		}
	}
	for _, want := range []string{"Path=tools", "STEERLINE_HOME=isolated", "STEERLINE_E2E_CONTROLLED_STATE=state"} {
		if !strings.Contains(got, want) {
			t.Fatal(got)
		}
	}
}
