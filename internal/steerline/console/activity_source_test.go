package console

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestConsoleSourcePrefersCommittedDatabaseProjection(t *testing.T) {
	repo, dir := t.TempDir(), t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip(err)
	}
	if out, err := exec.Command(git, "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	c := steerline.Config{Repository: repo, RunDir: dir, RuntimeFile: "runtime.json", Git: steerline.Command{Executable: git}, CommandTimeout: "5s"}
	resolved := c
	if err = resolved.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := steerline.OpenSQLiteStore(filepath.Join(dir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Transact(context.Background(), func(tx steerline.DomainTx) error {
		return tx.PutProjectTaskAuthority(steerline.ProjectTaskAuthority{ProjectIdentity: resolved.Identity(), Authority: "native", AuthorityVersion: 1, Repository: "fixture", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true})
	}); err != nil {
		t.Fatal(err)
	}
	state := steerline.Runtime{SchemaVersion: 5, Repository: steerline.Repository{Identity: resolved.Identity()}, HeartbeatAt: time.Now().UTC()}
	state, err = db.SaveRuntime(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	old := state
	old.HeartbeatAt = old.HeartbeatAt.Add(-time.Minute)
	bytes, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, c.RuntimeFile), bytes, 0600); err != nil {
		t.Fatal(err)
	}
	obs := newFileSource(resolved).SnapshotFromObservation(steerline.ReadObservationSnapshot(filepath.Join(dir, c.RuntimeFile), c.Heartbeat, nil))
	if obs.Runtime == nil || obs.Runtime.Revision != state.Revision || obs.Runtime.ProjectionError == "" || !obs.Runtime.HeartbeatAt.Equal(state.HeartbeatAt) {
		t.Fatal("equal-revision stale file masked fresh database heartbeat")
	}
}

func TestConsoleSourceFallbackNormalizerAndLateActivitySwitch(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte(`{"type":"session","version":1,"session_id":"p"}`+"\n"+`not-json`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := newFileSource(steerline.Config{})
	fallback, err := source.ReadLog(logPath, steerline.JournalCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback.Activity) != 2 || fallback.Activity[0].Source != "pi" || fallback.Activity[1].Kind != "unknown" || fallback.Activity[0].V != steerline.ActivityVersion || fallback.Activity[0].Seq != 1 || fallback.Activity[1].Seq != 2 {
		t.Fatalf("fallback did not detect pi from first complete raw line or preserve invalid content: %+v", fallback)
	}
	if _, err := json.Marshal(fallback.Activity[1]); err != nil {
		t.Fatalf("unknown event with invalid raw bytes could not be emitted as JSON: %v", err)
	}
	activityPath := steerline.ActivityPath(logPath)
	activity := `{"v":1,"seq":1,"ts":"2026-09-23T00:00:00Z","source":"steer","run_id":"r","kind":"claim"}` + "\n"
	if err = os.WriteFile(activityPath, []byte(activity), 0600); err != nil {
		t.Fatal(err)
	}
	switched, err := source.ReadLog(logPath, fallback.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !switched.Reset || switched.Cursor.Path != activityPath || len(switched.Activity) != 1 || switched.Activity[0].Kind != "claim" {
		t.Fatalf("activity arrival did not atomically switch view generation: %+v", switched)
	}
	if strings.Contains(switched.Activity[0].Kind, "unknown") {
		t.Fatal("raw fallback leaked after activity appeared")
	}
}

func TestConsoleSourceDiagnosesBadActivityAndKeepsFollowingValidLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	activityPath := steerline.ActivityPath(logPath)
	if err := os.WriteFile(activityPath, []byte("{bad json}\nnull\n"+`{"v":1,"seq":1,"kind":"claim"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := newFileSource(steerline.Config{}).ReadLog(logPath, steerline.JournalCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Diagnostics) != 2 || len(batch.Activity) != 1 || batch.Activity[0].Kind != "claim" {
		t.Fatalf("bad activity line blocked the next valid record: %+v", batch)
	}
}

func TestConsoleSourceInvalidSequenceCannotEraseValidEvents(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	body := "{\"seq\":10,\"kind\":\"claim\"}\n{\"seq\":1}\n{\"seq\":11,\"kind\":\"message\"}\n"
	if err := os.WriteFile(steerline.ActivityPath(logPath), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := newFileSource(steerline.Config{}).ReadLog(logPath, steerline.JournalCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activity) != 2 || batch.Activity[0].Seq != 10 || batch.Activity[1].Seq != 11 || len(batch.Diagnostics) != 1 {
		t.Fatalf("bad record changed sequence state: events=%+v diagnostics=%v", batch.Activity, batch.Diagnostics)
	}
}

func TestConsoleSourceFreshReplayResetsNormalizer(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	source := newFileSource(steerline.Config{})
	if err := os.WriteFile(logPath, []byte("{\"type\":\"thread.started\",\"thread_id\":\"old\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadLog(logPath, steerline.JournalCursor{}); err != nil {
		t.Fatal(err)
	}
	// A new visit starts at zero even if the same inode was rewritten while away.
	if err := os.WriteFile(logPath, []byte("{\"type\":\"session\",\"version\":1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := source.ReadLog(logPath, steerline.JournalCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activity) != 1 || batch.Activity[0].Source != "pi" || batch.Activity[0].Kind != "session" {
		t.Fatalf("fresh replay reused another reader's normalizer: %+v", batch.Activity)
	}
}
