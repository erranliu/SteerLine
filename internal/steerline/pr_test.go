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
