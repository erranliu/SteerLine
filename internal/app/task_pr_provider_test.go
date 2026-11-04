package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

const taskPRTestRef = "STEERLINE_TASK_PR_TEST_KEY"
const taskPRTestCredential = "synthetic-task-pr-credential"

func TestTaskPRQueryChild(t *testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 {
		return
	}
	if os.Getenv(taskPRTestRef) != taskPRTestCredential || !slices.Equal(os.Args[i+1:], []string{"https://example.invalid/fixture/project/pull/17", "17", "fixture/project"}) {
		fmt.Fprintln(os.Stderr, taskPRTestCredential)
		os.Exit(2)
	}
	fmt.Print(`{"url":"https://example.invalid/fixture/project/pull/17","repository":"fixture/project","base_branch":"main","head_repository":"fixture/project","head_branch":"candidate-17","state":"open","merged":false}`)
	os.Exit(0)
}

func TestTaskPRQueryUsesFrozenProvider(t *testing.T) {
	ctx := context.Background()
	timeout := 5 * time.Second
	client := publication.QueryClient{Timeout: timeout, Command: commandRunner{ProcessHost: process.New(timeout), Timeout: timeout}}
	bind := func(t *testing.T, c Config, url string) ports.RunnerPRTarget {
		t.Helper()
		input := runnercontract.RunInput{Publication: c.runnerPublication(), Repository: runnercontract.Repository{BaseBranch: "main"}}
		raw, err := json.Marshal(input)
		if err != nil || strings.Contains(string(raw), taskPRTestCredential) {
			t.Fatal("could not freeze provider without credentials", err)
		}
		candidate := runnerstate.Candidate{RunID: "original", InputDigest: "fixed", Branch: "candidate-17", PRURL: url, Submit: runnercontract.Request{Input: raw},
			Projection: &runnercontract.Run{RunID: "original", InputDigest: "fixed", Execution: runnercontract.Execution{Branch: "candidate-17", Workspace: "workspace", SessionID: "session"},
				PR: &runnercontract.PRFact{URL: url, Provider: c.PR.Kind, Number: 17, Repository: "fixture/project", HeadRepository: "fixture/project", BaseBranch: "main", HeadBranch: "candidate-17"}}}
		target, err := ports.BindRunnerPR(candidate, "main")
		if err != nil || target.Provider.TokenEnv != taskPRTestRef {
			t.Fatal("failed to bind frozen provider", err)
		}
		return target
	}
	t.Run("command", func(t *testing.T) {
		t.Setenv(taskPRTestRef, taskPRTestCredential)
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
		cfg := Config{Branch: "main", PR: model.PRProviderConfig{Kind: "command", BaseURL: "https://example.invalid", Repository: "fixture/project", TokenEnv: taskPRTestRef,
			Query: backend.Command{Executable: filepath.Base(exe), Args: []string{"-test.run=^TestTaskPRQueryChild$", "--", "{pr_url}", "{number}", "{repository}"}}}}
		target := bind(t, cfg, "https://example.invalid/fixture/project/pull/17")
		fact, err := client.Query(ctx, target.Provider, target.URL)
		if err != nil || !target.Matches(fact) || fact.State != "open" {
			t.Fatal("wrong command provider response", err)
		}
		t.Setenv(taskPRTestRef, "")
		if _, err := client.Query(ctx, target.Provider, target.URL); err == nil || strings.Contains(err.Error(), taskPRTestCredential) {
			t.Fatal("command authentication failure was lost or leaked a credential")
		}
	})
	t.Run("gitea", func(t *testing.T) {
		t.Setenv(taskPRTestRef, taskPRTestCredential)
		var hits atomic.Int32
		var redirect atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/fixture/project/pulls/17" || r.Header.Get("Authorization") != "token "+taskPRTestCredential {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if redirect.Load() {
				http.Redirect(w, r, "/another-target", http.StatusFound)
				return
			}
			fmt.Fprintf(w, `{"html_url":"http://%s/fixture/project/pulls/17","number":17,"state":"closed","merged":false,"base":{"ref":"main","repo":{"full_name":"fixture/project"}},"head":{"ref":"candidate-17","repo":{"full_name":"fixture/project"}}}`, r.Host)
		}))
		defer server.Close()
		target := bind(t, Config{Branch: "main", PR: model.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "fixture/project", TokenEnv: taskPRTestRef}}, server.URL+"/fixture/project/pulls/17")
		fact, err := client.Query(ctx, target.Provider, target.URL)
		if err != nil || !target.Matches(fact) || fact.State != "closed_unmerged" {
			t.Fatal("wrong Gitea provider response", err)
		}
		t.Setenv(taskPRTestRef, "")
		if _, err := client.Query(ctx, target.Provider, target.URL); err == nil || hits.Load() != 1 {
			t.Fatal("missing Gitea credential did not prevent the request")
		}
		t.Setenv(taskPRTestRef, taskPRTestCredential)
		redirect.Store(true)
		if _, err := client.Query(ctx, target.Provider, target.URL); err == nil || hits.Load() != 2 {
			t.Fatal("Gitea query followed a provider redirect")
		}
	})
}
