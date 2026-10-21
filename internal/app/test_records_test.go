package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	api "github.com/erranliu/SteerLine/internal/product/api"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type trAppStore struct {
	testManagementStore
	definitions []tm.Definition
	writes      int
}

func (s *trAppStore) TestRecordDefinitions(context.Context, string) ([]tm.Definition, error) {
	return s.definitions, nil
}
func (s *trAppStore) SaveTestRecord(_ context.Context, r tm.TestRecord) (tm.TestRecord, error) {
	s.writes++
	return r, nil
}
func TestTRQueryCatalogueUsesObservedCommitAndKeepsHistoricalDefinitions(t *testing.T) {
	root := t.TempDir()
	pmGit(t, root, "init", "--initial-branch=main")
	pmGit(t, root, "config", "user.name", "fixture")
	pmGit(t, root, "config", "user.email", "fixture@example.invalid")
	path := filepath.Join(root, filepath.FromSlash(tm.CataloguePath))
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	raw := `{"tests":[{"id":"current","name":"Main frozen","level":"L1","environment":"isolated","command":"test","check":"assert"}]}`
	if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	pmGit(t, root, "add", ".")
	pmGit(t, root, "-c", "commit.gpgsign=false", "commit", "-qm", "catalogue")
	commit := pmGit(t, root, "rev-parse", "HEAD")
	_ = os.WriteFile(path, []byte("bad local edit"), 0600)
	proc := resourceprocess.New(time.Minute)
	maintenance := &projectMaintenance{config: Config{Repository: root, Git: backend.Command{Executable: "git"}, ProcessHost: proc}, observation: pm.Observation{Commit: commit}}
	store := &trAppStore{definitions: []tm.Definition{{ID: "historic", Name: "Historic"}}}
	port := &testManagementPort{service: &testmanagement.GenerationService{Project: "project"}, store: store, maintenance: maintenance}
	out, e := port.TestCatalogue(context.Background())
	if e != nil || !strings.Contains(string(out), "Main frozen") || !strings.Contains(string(out), "Historic") || strings.Contains(string(out), "bad local") {
		t.Fatal(string(out), e)
	}
	maintenance.observation.Commit = ""
	out, e = port.TestCatalogue(context.Background())
	if e != nil || !strings.Contains(string(out), "Historic") || !strings.Contains(string(out), "diagnostic") {
		t.Fatal("unavailable catalogue lost history", string(out), e)
	}
	if store.writes != 0 {
		t.Fatal("catalogue read mutated records")
	}
}
func TestTRRegisterRejectsForeignRepositoryBeforeWrite(t *testing.T) {
	project := filepath.Join(t.TempDir(), ".git")
	d := tm.Definition{ID: "rules", Name: "Rules", Level: "L1", Environment: "isolated", Command: "test", Check: "assert"}
	r := tm.Report{ID: strings.Repeat("a", 32), Sequence: 1, StartedAt: time.Now(), FinishedAt: time.Now(), Definition: d, Repository: filepath.Join(t.TempDir(), "foreign.git"), Command: "test", Selection: []string{d.ID}, Environment: d.Environment, Definitions: map[string]string{d.ID: testmanagement.Digest(d)}, Results: []tm.Result{{TestID: d.ID, Status: "passed", Log: "summary.json"}}, Evidence: tm.ReportEvidence{Path: "summary.json"}}
	store := &trAppStore{}
	port := &testManagementPort{service: &testmanagement.GenerationService{Project: project}, store: store, records: &testmanagement.RecordService{Project: project, Store: store}}
	raw, _ := json.Marshal(r)
	if _, e := port.RegisterTestRecord(context.Background(), api.TestRecordRegistration{Report: raw}); e == nil || store.writes != 0 {
		t.Fatal("foreign repository accepted", e, store.writes)
	}
	r.Repository = project
	raw, _ = json.Marshal(r)
	if _, e := port.RegisterTestRecord(context.Background(), api.TestRecordRegistration{Report: raw}); e != nil || store.writes != 1 {
		t.Fatal(e, store.writes)
	}
}
