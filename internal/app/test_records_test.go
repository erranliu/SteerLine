package app

import (
	"context"
	"encoding/json"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	api "github.com/erranliu/SteerLine/internal/product/api"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTRQueryOnlyCommittedGitHistoryAndDeletion(t *testing.T) {
	repo := t.TempDir()
	pmGit(t, repo, "init", "--initial-branch=main")
	pmGit(t, repo, "config", "user.name", "History test")
	pmGit(t, repo, "config", "user.email", "history@example.invalid")
	write := func(name, body string) {
		t.Helper()
		target := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() string {
		t.Helper()
		pmGit(t, repo, "add", ".")
		pmGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-qm", "test")
		return pmGit(t, repo, "rev-parse", "HEAD")
	}
	write("code.txt", "one")
	tested := commit()
	m := newProjectMaintenance(Config{Repository: repo, Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute)}, nil)
	m.observation = pm.Observation{Commit: tested}
	p := &testManagementPort{maintenance: m, service: &testmanagement.GenerationService{Project: repo}}
	ctx := context.Background()
	path := testmanagement.RecordsPath + "/" + tested + ".json"
	body := `{"runs":[{"at":"2026-10-23T00:00:00Z","results":[{"test_id":"old.rules","level":"L3","environment":"controlled","status":"failed","note":"actual failure"}]}]}`
	write(path, body)
	page, err := p.recordSnapshot(ctx)
	if err != nil || len(page.Records) != 0 {
		t.Fatal("uncommitted record visible", page, err)
	}
	main := commit()
	m.observation = pm.Observation{Commit: main}
	page, err = p.recordSnapshot(ctx)
	if err != nil || len(page.Records) != 1 || page.Records[0].Commit != tested {
		t.Fatal(page, err)
	}
	write(path, "broken worktree JSON")
	detail, err := p.TestRecord(ctx, page.Records[0].ID)
	if err != nil || !strings.Contains(string(detail), "actual failure") {
		t.Fatal("read worktree", string(detail), err)
	}
	catalog, err := p.TestCatalogue(ctx)
	if err != nil || !strings.Contains(string(catalog), `"id":"old.rules"`) || !strings.Contains(string(catalog), `"current":false`) {
		t.Fatal("missing catalogue blocked history", string(catalog), err)
	}
	write(path, `{"runs":[]}`)
	m.observation = pm.Observation{Commit: commit()}
	if _, err = p.TestRecords(ctx, api.TestRecordFilter{}); err == nil {
		t.Fatal("bad Git JSON silently hidden")
	}
	if err = os.Remove(filepath.Join(repo, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
	m.observation = pm.Observation{Commit: commit()}
	raw, err := p.TestRecords(ctx, api.TestRecordFilter{})
	var empty testmanagement.RecordPage
	if err != nil || json.Unmarshal(raw, &empty) != nil || len(empty.Records) != 0 {
		t.Fatal("deleted history retained", string(raw), err)
	}
	if _, err = p.TestRecord(ctx, page.Records[0].ID); err == nil {
		t.Fatal("deleted detail retained")
	}
}
