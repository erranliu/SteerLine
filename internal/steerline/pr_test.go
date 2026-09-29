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

func TestWorkerPRIdentityRequiresExecutionBranch(t *testing.T) {
	url := "https://git.example/o/r/pulls/8"
	head := strings.Repeat("a", 40)
	base := PRInfo{URL: url, Repository: "o/r", BaseBranch: "main", HeadRepository: "o/r", HeadBranch: "codex/task-8", HeadRevision: head, State: "open"}
	if err := validateWorkerPRIdentity(base, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PRInfo){
		"wrong repository":  func(p *PRInfo) { p.Repository = "o/other" },
		"wrong base":        func(p *PRInfo) { p.BaseBranch = "develop" },
		"fork head":         func(p *PRInfo) { p.HeadRepository = "other/r" },
		"other task branch": func(p *PRInfo) { p.HeadBranch = "codex/task-9" },
		"missing head":      func(p *PRInfo) { p.HeadRevision = "" },
	} {
		t.Run(name, func(t *testing.T) {
			pr := base
			mutate(&pr)
			if err := validateWorkerPRIdentity(pr, url, "o/r", "main", "codex/task-8"); err == nil {
				t.Fatal("accepted a PR outside the execution branch")
			}
		})
	}
	advanced := base
	advanced.HeadRevision = strings.Repeat("b", 40)
	advanced.State = "merged"
	if err := validateWorkerPRIdentity(advanced, url, "o/r", "main", "codex/task-8"); err != nil {
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
	client := PRClient{Config: PRConfig{Kind: "gitea", BaseURL: server.URL, Repository: "o/r"}, Runner: Runner{Timeout: time.Second}}
	info, err := client.Query(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWorkerPRIdentity(info, url, "o/r", "main", "codex/task-8"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeCorruptionNotOverwritten(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	require(t, s.Close())
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
