package publication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestPRRequiresExactIdentityAndMergeEvidence(t *testing.T) {
	for _, bad := range []string{"url", "repository", "missing-merged", "missing-time", "missing-commit", "closed-only"} {
		t.Run(bad, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := map[string]any{"number": 1, "html_url": server.URL + "/o/r/pulls/1", "state": "closed", "merged": true, "merged_at": time.Now().UTC(), "merge_commit_sha": strings.Repeat("c", 40), "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "o/r"}}}
				switch bad {
				case "url":
					p["html_url"] = server.URL + "/o/r/pulls/2"
				case "repository":
					p["base"] = map[string]any{"ref": "main", "repo": map[string]string{"full_name": "o/else"}}
				case "missing-merged":
					delete(p, "merged")
				case "missing-time":
					p["merged_at"] = nil
				case "missing-commit":
					p["merge_commit_sha"] = nil
				case "closed-only":
					p["merged"] = false
				}
				json.NewEncoder(w).Encode(p)
			}))
			defer server.Close()
			client := Client{Config: Config{Kind: "gitea", BaseURL: server.URL, Repository: "o/r"}, Timeout: time.Second}
			info, e := client.Query(context.Background(), server.URL+"/o/r/pulls/1")
			if e == nil && info.State != "closed_unmerged" {
				t.Fatal("accepted invalid merged evidence")
			}
		})
	}
}

func TestPRURLsAndRemotes(t *testing.T) {
	c := Config{Kind: "gitea", BaseURL: "http://host:3000", Repository: "owner/repo"}
	require(t, c.ValidateRemote("http://host:3000/owner/repo.git"))
	require(t, c.ValidateRemote("ssh://git@host:2222/owner/repo.git"))
	for _, raw := range []string{"http://evil/owner/repo/pulls/1", "http://host:3000/other/repo/pulls/1", "http://host:3000/owner/repo/pulls/1?x=y", "http://user:secret@host:3000/owner/repo/pulls/1", "http://host:3000/owner/repo/pulls/01", "http://host:3000/owner/repo/pulls/%31"} {
		if _, e := c.PRNumber(raw); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestWorkerPRIdentityRequiresExecutionBranch(t *testing.T) {
	url := "https://git.example/o/r/pulls/8"
	head := strings.Repeat("a", 40)
	base := executionmodel.PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "codex/task-8", HeadRevision: head, State: "open"}
	if err := ValidateWorkerPRIdentity(base, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*executionmodel.PRInfo){
		"wrong repository":  func(p *executionmodel.PRInfo) { p.Repository = "o/other" },
		"wrong base":        func(p *executionmodel.PRInfo) { p.BaseBranch = "develop" },
		"fork head":         func(p *executionmodel.PRInfo) { p.HeadRepository = "other/r" },
		"other task branch": func(p *executionmodel.PRInfo) { p.HeadBranch = "codex/task-9" },
		"missing head":      func(p *executionmodel.PRInfo) { p.HeadRevision = "" },
	} {
		t.Run(name, func(t *testing.T) {
			pr := base
			mutate(&pr)
			if err := ValidateWorkerPRIdentity(pr, url, "o/r", "main", "codex/task-8"); err == nil {
				t.Fatal("accepted a PR outside the execution branch")
			}
		})
	}
	advanced := base
	advanced.HeadRevision = strings.Repeat("b", 40)
	advanced.State = "merged"
	if err := ValidateWorkerPRIdentity(advanced, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatalf("a later commit on the same PR must remain associated: %v", err)
	}
}

func TestWorkerPublicationReportsAllMismatchedFields(t *testing.T) {
	url := "https://git.example/o/r/pulls/8"
	head := strings.Repeat("a", 40)
	info := executionmodel.PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "codex/101-short", HeadRevision: head, State: "open"}
	if err := ValidateWorkerPublication(info, url, "o/r", "main", "codex/101-short", head); err != nil {
		t.Fatal(err)
	}
	info.HeadBranch = "codex/101-full-run-id"
	info.Repository = "o/other"
	info.BaseBranch = "develop"
	info.HeadRepository = "fork/r"
	info.HeadRevision = strings.Repeat("b", 40)
	info.URL = "https://git.example/o/other/pulls/9"
	info.State = "closed_unmerged"
	err := ValidateWorkerPublication(info, url, "o/r", "main", "codex/101-short", head)
	if !errors.Is(err, ErrPRIdentity) {
		t.Fatalf("lost PR identity error: %v", err)
	}
	for _, want := range []string{
		`url: expected="` + url + `", actual="` + info.URL + `"`,
		`repository: expected="o/r", actual="o/other"`,
		`base_branch: expected="main", actual="develop"`,
		`head_repository: expected="o/r", actual="fork/r"`,
		`head_branch: expected="codex/101-short", actual="codex/101-full-run-id"`,
		`head_revision: expected="` + head + `", actual="` + info.HeadRevision + `"`,
		`state: expected=open|merged, actual="closed_unmerged"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing mismatch %q: %v", want, err)
		}
	}
}

func TestGiteaQueryReadsPRHeadIdentity(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"html_url": server.URL + "/o/r/pulls/8", "number": 8, "state": "open", "merged": false,
			"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "o/r"}},
			"head": map[string]any{"ref": "codex/task-8", "sha": strings.Repeat("a", 40), "repo": map[string]string{"full_name": "o/r"}},
		})
	}))
	defer server.Close()
	url := server.URL + "/o/r/pulls/8"
	client := Client{Config: Config{Kind: "gitea", BaseURL: server.URL, Repository: "o/r"}, Timeout: time.Second}
	info, err := client.Query(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkerPRIdentity(info, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatal(err)
	}
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type failingCommandRunner struct{ err error }

func (r failingCommandRunner) Output(context.Context, executionbackend.Command, ...string) ([]byte, error) {
	return nil, r.err
}

func TestCommandProviderErrorsRedactReferencedCredential(t *testing.T) {
	const name = "STEERLINE_TEST_PUBLICATION_TOKEN"
	const secret = "credential-value-that-must-not-escape"
	t.Setenv(name, secret)
	client := NewClient(Config{TokenEnv: name}, failingCommandRunner{err: errors.New("command echoed " + secret)}, time.Second)
	_, err := client.command(context.Background(), executionbackend.Command{Executable: "gh"})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("provider command error leaked its credential: %v", err)
	}
}

func TestUnsupportedProviderDoesNoNetworkOperation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewClient(Config{Kind: "unknown", BaseURL: server.URL, Repository: "owner/repo"}, failingCommandRunner{err: errors.New("unexpected command")}, time.Second)
	url := server.URL + "/owner/repo/pulls/1"
	for _, call := range []func() error{
		func() error { _, err := client.Query(context.Background(), url); return err },
		func() error { return client.Check(context.Background()) },
	} {
		if err := call(); err == nil {
			t.Fatal("unknown PR provider was accepted")
		}
	}
	if requests != 0 {
		t.Fatalf("unsupported provider performed %d HTTP requests", requests)
	}
}
