package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMCommittedPlanSnapshot(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "TM test")
	git("config", "user.email", "tm@example.invalid")
	write := func(p, text string) {
		t.Helper()
		name := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/task.plan.md", "# Plan\nTest rules.")
	write("docs/task.tests.json", `{"tests":["rules"]}`)
	write(testmanagement.CataloguePath, `{"tests":[{"id":"rules","name":"规则","level":"L1","environment":"isolated","command":"rules","check":"断言通过"}]}`)
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	commit := git("rev-parse", "HEAD")
	c := Config{Repository: root, Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute)}
	work := taskmodel.FixedWork{Contract: taskmodel.TaskContract{PlanRef: taskmodel.FileReference{Path: "docs/task.plan.md", Commit: commit}}}
	write("docs/task.tests.json", `{"tests":["not-in-commit"]}`)
	raw, refs := c.prepareTestPlan(context.Background(), work)
	var snap testmanagement.Snapshot
	_ = json.Unmarshal(raw, &snap)
	if snap.Status != "ready" || len(snap.Definitions) != 1 || snap.Definitions[0].ID != "rules" || len(refs) != 3 {
		t.Fatalf("moving files used: %s %+v", raw, refs)
	}
	work.Contract.PlanRef.Path = "docs/missing.plan.md"
	raw, refs = c.prepareTestPlan(context.Background(), work)
	_ = json.Unmarshal(raw, &snap)
	if snap.Status != "missing" || len(refs) != 0 {
		t.Fatalf("missing reference sent to Runner: %s %+v", raw, refs)
	}
	write("docs/task.tests.json", `{"tests":["unknown"]}`)
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-qm", "invalid selection")
	work.Contract.PlanRef.Path = "docs/task.plan.md"
	work.Contract.PlanRef.Commit = git("rev-parse", "HEAD")
	raw, _ = c.prepareTestPlan(context.Background(), work)
	_ = json.Unmarshal(raw, &snap)
	if snap.Status != "invalid" {
		t.Fatal("bad JSON selection not diagnosed", string(raw))
	}
}
