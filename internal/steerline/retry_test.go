package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func failedStartup(t *testing.T) (*fixture, *Service, *Run) {
	f := newFixture(t, task("t-1"), task("t-2"), task("t-3"))
	db := readFake(f.db)
	db.Behavior = "missing"
	writeFake(f.db, db)
	s := f.service(t)
	require(t, s.dispatch(context.Background()))
	finishWorker(t, s)
	s.State.Control.DispatchMode = "manual"
	require(t, s.save())
	return f, s, s.State.Runs[0]
}

func TestOnlineRetryPreservesHistoryAndNeverClaimsAnotherTask(t *testing.T) {
	f, s, r := failedStartup(t)
	oldLog := *r.LogPath
	oldResult := *r.ResultPath
	oldBytes, e := os.ReadFile(oldLog)
	require(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require(t, s.startRetry(ctx, r.RunID))
	finishWorker(t, s)
	if ctx.Err() != nil {
		t.Fatal("retry exceeded the test deadline")
	}
	db := readFake(f.db)
	if db.Claims["t-1"] != 1 || db.Starts["t-1"] != 2 || db.Claims["t-2"] != 0 || db.Claims["t-3"] != 0 {
		t.Fatalf("retry claimed new work: %+v", db.Claims)
	}
	if s.State.Service.Status == "stopped" || r.FinishTaskStatus != "blocked" {
		t.Fatal("online retry stopped the service or lost its result state")
	}
	current, e := os.ReadFile(oldLog)
	require(t, e)
	if string(current) != string(oldBytes) || *r.LogPath == oldLog || *r.ResultPath == oldResult {
		t.Fatal("old attempt overwritten")
	}
	if r.AttemptID == r.RunID || filepath.Base(filepath.Dir(*r.LogPath)) != r.AttemptID {
		t.Fatal("retry did not assign a distinct attempt identity")
	}
	previous, e := os.ReadFile(filepath.Join(filepath.Dir(*r.ResultPath), "previous-run.json"))
	require(t, e)
	var before Run
	require(t, json.Unmarshal(previous, &before))
	if before.LogPath == nil || *before.LogPath != oldLog || before.Error == nil {
		t.Fatal("prior attempt snapshot lost")
	}
}

func TestOnlineRetryContinuesTrackingOtherPRs(t *testing.T) {
	f, s, r := failedStartup(t)
	db := readFake(f.db)
	db.Behavior = ""
	writeFake(f.db, db)
	s.State.Control.DispatchMode = "auto"
	require(t, s.save())
	require(t, s.dispatch(context.Background()))
	finishWorker(t, s)
	f.pr("1", "merged")
	f.pr("2", "merged")
	s.State.Control.DispatchMode = "manual"
	require(t, s.save())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require(t, s.startRetry(ctx, r.RunID))
	finishWorker(t, s)
	require(t, s.pollPRs(context.Background()))
	if ctx.Err() != nil || r.FinishTaskStatus != "completed" {
		t.Fatal("selected retry did not complete")
	}
	db = readFake(f.db)
	if db.Claims["t-1"] != 1 || db.Claims["t-2"] != 1 || db.Claims["t-3"] != 0 || db.Tasks[1].Status != "closed" || s.State.Runs[1].FinishTaskStatus != "completed" {
		t.Fatal("online retry did not continue PR tracking for another run")
	}
}

func TestRetryRejectsDirtyAdvancedUnknownAndForeignRuns(t *testing.T) {
	for _, mode := range []string{"dirty", "advanced", "unknown", "foreign", "bound-pr"} {
		t.Run(mode, func(t *testing.T) {
			f, s, r := failedStartup(t)
			oldLog := *r.LogPath
			switch mode {
			case "dirty":
				require(t, os.WriteFile(filepath.Join(*r.WorktreePath, "keep.txt"), []byte("preserve"), 0600))
			case "advanced":
				fixtureGit(t, *r.WorktreePath, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "--quiet", "-m", "existing implementation")
			case "unknown":
				r.ExecutionStatus = "unknown"
			case "foreign":
				db := readFake(f.db)
				db.Tasks[0].Assignee = "another-agent"
				writeFake(f.db, db)
			case "bound-pr":
				r.PR.URL = ptr(f.server.URL + "/owner/repo/pulls/1")
			}
			if e := s.startRetry(context.Background(), r.RunID); e == nil {
				t.Fatal("unsafe retry accepted")
			}
			if readFake(f.db).Starts["t-1"] != 1 || *r.LogPath != oldLog {
				t.Fatal("rejected retry changed execution/history")
			}
		})
	}
}

func TestFullAccessMustBeExplicitInConfig(t *testing.T) {
	f := newFixture(t)
	data, _ := json.Marshal(f.c)
	var fields map[string]any
	require(t, json.Unmarshal(data, &fields))
	delete(fields, "sandbox")
	path := filepath.Join(f.c.RunDir, "config.json")
	data, _ = json.Marshal(fields)
	require(t, os.WriteFile(path, data, 0600))
	c, e := LoadConfig(path)
	require(t, e)
	if c.Sandbox != "workspace-write" {
		t.Fatal("default privileges widened")
	}
	fields["sandbox"] = "danger-full-access"
	data, _ = json.Marshal(fields)
	require(t, os.WriteFile(path, data, 0600))
	c, e = LoadConfig(path)
	require(t, e)
	if c.Sandbox != "danger-full-access" {
		t.Fatal("explicit permissions not honored")
	}
	fields["sandbox"] = "misspelled"
	data, _ = json.Marshal(fields)
	require(t, os.WriteFile(path, data, 0600))
	_, e = LoadConfig(path)
	if e == nil || !strings.Contains(e.Error(), "sandbox") {
		t.Fatal("unknown permissions accepted")
	}
}
