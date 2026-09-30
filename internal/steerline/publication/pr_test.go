package publication

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	base := contracts.PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "codex/task-8", HeadRevision: head, State: "open"}
	if err := ValidateWorkerPRIdentity(base, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*contracts.PRInfo){
		"wrong repository":  func(p *contracts.PRInfo) { p.Repository = "o/other" },
		"wrong base":        func(p *contracts.PRInfo) { p.BaseBranch = "develop" },
		"fork head":         func(p *contracts.PRInfo) { p.HeadRepository = "other/r" },
		"other task branch": func(p *contracts.PRInfo) { p.HeadBranch = "codex/task-9" },
		"missing head":      func(p *contracts.PRInfo) { p.HeadRevision = "" },
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

func TestPRCloseUsesExactGiteaIdentityAndReturnsClosedReceipt(t *testing.T) {
	t.Setenv("STEERLINE_TEST_PR_TOKEN", "fixture-token")
	var server *httptest.Server
	var patched int
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/owner/repo/pulls/7" || r.Header.Get("Authorization") != "token fixture-token" {
			http.Error(w, "identity mismatch", http.StatusNotFound)
			return
		}
		state := "open"
		if r.Method == http.MethodPatch {
			var body struct {
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.State != "closed" {
				http.Error(w, "expected closed state", http.StatusBadRequest)
				return
			}
			patched++
			state = "closed"
		} else if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"html_url": server.URL + "/owner/repo/pulls/7", "number": 7, "state": state, "merged": false,
			"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "owner/repo"}},
			"head": map[string]string{"sha": strings.Repeat("a", 40)},
		})
	}))
	defer server.Close()
	client := Client{Config: Config{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo", TokenEnv: "STEERLINE_TEST_PR_TOKEN"}, Timeout: time.Second}
	info, err := client.Close(context.Background(), server.URL+"/owner/repo/pulls/7")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != "closed_unmerged" || info.Merged || info.HeadRevision != strings.Repeat("a", 40) || patched != 1 {
		t.Fatalf("Gitea close did not return the fixed unmerged PR receipt: %+v patches=%d", info, patched)
	}
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicationRequestDigestBindsCandidate(t *testing.T) {
	base := Request{Version: 1, RunID: strings.Repeat("a", 32), ExecutionID: "exec-1", TurnID: "00000000-0000-0000-0000-000000000001", TaskID: "42", Workspace: executionapi.WorkspaceIntent{Path: t.TempDir(), Branch: "codex/42-123456789abc", BaseCommit: strings.Repeat("b", 40)}, GitCommonDir: t.TempDir(), Remote: "origin", BaseBranch: "main", Provider: ProviderIdentity(Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: contracts.Command{Executable: "gh"}})}
	base.StartingHead, base.IndexTree = strings.Repeat("c", 40), strings.Repeat("d", 40)
	base.Digest = requestDigest(base)
	if err := base.ValidateCandidate(); err != nil {
		t.Fatal(err)
	}
	base.Remote = "other"
	if err := base.ValidateCandidate(); err == nil {
		t.Fatal("accepted a changed publication request with the old digest")
	}
	provider := Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: contracts.Command{Executable: "gh", Args: []string{"api", "repos/{repository}/pulls/{number}"}}}
	identity := ProviderIdentity(provider)
	provider.Query.Args[1] = "repos/another/repository"
	if identity.Matches(provider) {
		t.Fatal("provider identity accepted changed PR query arguments")
	}
}

type failingCommandRunner struct{ err error }

func (r failingCommandRunner) Output(context.Context, contracts.Command, ...string) ([]byte, error) {
	return nil, r.err
}
func TestCommandProviderErrorsRedactReferencedCredential(t *testing.T) {
	const name = "STEERLINE_TEST_PUBLICATION_TOKEN"
	const secret = "credential-value-that-must-not-escape"
	t.Setenv(name, secret)
	client := NewClient(Config{TokenEnv: name}, failingCommandRunner{err: errors.New("command echoed " + secret)}, time.Second)
	_, err := client.command(context.Background(), contracts.Command{Executable: "gh"})
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
		func() error { _, err := client.Close(context.Background(), url); return err },
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
