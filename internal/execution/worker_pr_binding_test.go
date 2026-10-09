package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

// The old platform-specific PR test belongs to the shared business owner.
func TestWorkerPRBindingUsesProviderAndRetainsSession(t *testing.T) {
	state, branch := "open", "worker/task-8"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/org/repo/pulls/8" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"html_url": server.URL + "/org/repo/pulls/8", "number": 8, "state": state, "merged": false,
			"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "org/repo"}},
			"head": map[string]any{"ref": branch, "sha": strings.Repeat("a", 40), "repo": map[string]string{"full_name": "org/repo"}},
		})
	}))
	defer server.Close()
	url := server.URL + "/org/repo/pulls/8"
	client := publication.Client{Timeout: 5 * time.Second, Config: publication.Config{Kind: "gitea", BaseURL: server.URL, Repository: "org/repo"}}
	workflow := executionmodel.WorkflowRunRecord{WorkflowRunID: "wf-8", TaskID: "8", WorkflowVersion: executionmodel.Version, Status: "running", Branch: "main"}
	execution := executionmodel.ExecutionRecord{ExecutionID: "exec-8", WorkflowRunID: "wf-8", Workspace: executionmodel.WorkspaceEvidence{Branch: "worker/task-8"}, SessionID: "original-session", CurrentTurnID: "turn-8", LastStoppedTurnID: "turn-8"}
	bind := func() error {
		info, err := client.Query(context.Background(), url)
		if err != nil {
			return err
		}
		return BindWorkerPR(&workflow, &execution, info, url, "org/repo", false)
	}
	branch = "worker/other-task"
	if err := bind(); err == nil || workflow.PRURL != "" {
		t.Fatalf("unowned PR was bound: %v", err)
	}
	branch = "worker/task-8"
	if err := bind(); err != nil || workflow.PRURL != url || workflow.Status != "waiting_external" {
		t.Fatalf("provider-verified PR was not bound: %+v / %v", workflow, err)
	}
	state = "closed"
	if err := bind(); err != nil || workflow.PRState != "closed_unmerged" || workflow.Status != "waiting_input" || execution.CurrentTurnID != "turn-8" || execution.SessionID != "original-session" {
		t.Fatalf("closed unmerged PR lost the original session: %+v / %+v / %v", workflow, execution, err)
	}
}
