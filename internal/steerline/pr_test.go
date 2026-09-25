package steerline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPRRequiresExactIdentityAndMergeEvidence(t *testing.T) {
	for _, bad := range []string{"url", "repository", "missing-merged", "missing-time", "missing-commit", "closed-only"} {
		t.Run(bad, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := map[string]any{"number": 1, "html_url": server.URL + "/o/r/pulls/1", "state": "closed", "merged": true, "merged_at": now(), "merge_commit_sha": strings.Repeat("c", 40), "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "o/r"}}}
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
			client := PRClient{Config: PRConfig{Kind: "gitea", BaseURL: server.URL, Repository: "o/r"}, Runner: Runner{Timeout: time.Second}}
			info, e := client.Query(context.Background(), server.URL+"/o/r/pulls/1")
			if e == nil && info.State != "closed_unmerged" {
				t.Fatal("accepted invalid merged evidence")
			}
		})
	}
}
func TestPRURLsAndRemotes(t *testing.T) {
	c := PRConfig{Kind: "gitea", BaseURL: "http://host:3000", Repository: "owner/repo"}
	require(t, c.ValidateRemote("http://host:3000/owner/repo.git"))
	require(t, c.ValidateRemote("ssh://git@host:2222/owner/repo.git"))
	for _, raw := range []string{"http://evil/owner/repo/pulls/1", "http://host:3000/other/repo/pulls/1", "http://host:3000/owner/repo/pulls/1?x=y", "http://user:secret@host:3000/owner/repo/pulls/1", "http://host:3000/owner/repo/pulls/01", "http://host:3000/owner/repo/pulls/%31"} {
		if _, e := c.PRNumber(raw); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestReadResultToleratesUTF8BOM(t *testing.T) {
	f := newFixture(t)
	r := &Run{TaskID: "t-1", RunID: "run-1"}
	path := filepath.Join(f.c.RunDir, "bom-result.json")
	body := `{"task_id":"t-1","run_id":"run-1","outcome":"blocked","pr_url":null,"worktree_path":null,"base_commit":null,"summary":null,"error":null}`
	require(t, os.WriteFile(path, append([]byte{0xef, 0xbb, 0xbf}, []byte(body)...), 0600))
	result, e := ReadResult(path, r)
	require(t, e)
	if result.Outcome != "blocked" || result.TaskID != "t-1" || result.RunID != "run-1" {
		t.Fatalf("unexpected result %+v", result)
	}
}
func TestResultRequiredFieldsAndDuplicatePR(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	r := &Run{TaskID: "t-1", RunID: "run-1"}
	path := filepath.Join(f.c.RunDir, "result.json")
	require(t, os.WriteFile(path, []byte(`{"task_id":"t-1","run_id":"run-1","outcome":"blocked"}`), 0600))
	if _, e := ReadResult(path, r); e == nil {
		t.Fatal("missing fields accepted")
	}
	url := f.server.URL + "/owner/repo/pulls/1"
	r.WorkspaceIntent = &WorkspaceIntent{Path: filepath.Join(f.c.RunDir, "worktrees", "test"), Branch: "codex/test", BaseCommit: strings.Repeat("a", 40)}
	r.WorktreePath = &r.WorkspaceIntent.Path
	r.WorktreeBranch = &r.WorkspaceIntent.Branch
	r.BaseCommit = &r.WorkspaceIntent.BaseCommit
	s.State.Runs = []*Run{{RunID: "other", PR: PRState{URL: &url}}}
	if e := s.bind(r, Result{Outcome: "pr_submitted", PRURL: &url, WorktreePath: r.WorktreePath, BaseCommit: r.BaseCommit}); e == nil || !strings.Contains(e.Error(), "already bound") {
		t.Fatal("duplicate PR accepted")
	}
}
func TestRuntimeCorruptionNotOverwritten(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.c.RunDir, f.c.RuntimeFile)
	require(t, os.WriteFile(path, []byte("truncated{"), 0600))
	if _, e := NewService(f.c); e == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "truncated{" {
		t.Fatal("corrupt evidence overwritten")
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
	client := PRClient{Config: PRConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo", TokenEnv: "STEERLINE_TEST_PR_TOKEN"}, Runner: Runner{Timeout: time.Second}}
	info, err := client.Close(context.Background(), server.URL+"/owner/repo/pulls/7")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != "closed_unmerged" || info.Merged || info.HeadRevision != strings.Repeat("a", 40) || patched != 1 {
		t.Fatalf("Gitea close did not return the fixed unmerged PR receipt: %+v patches=%d", info, patched)
	}
}
