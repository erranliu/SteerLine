package viewer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func TestFileSourceRetainsLastGoodSnapshotWithFailureTimestamp(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime.json")
	rt := steerline.Runtime{SchemaVersion: 3, HeartbeatAt: time.Now().UTC(), Runs: []*steerline.Run{{RunID: "r", TaskID: "t"}}}
	b, err := json.Marshal(rt)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(runtimePath, b, 0600); err != nil {
		t.Fatal(err)
	}
	source := NewFileSource(steerline.Config{RunDir: dir, RuntimeFile: "runtime.json", Heartbeat: "1s"})
	first := source.Snapshot()
	if first.Runtime == nil || first.State != steerline.SnapshotValid {
		t.Fatalf("initial snapshot: %+v", first)
	}
	if err = os.WriteFile(runtimePath, []byte(`{"schema_version":3,`), 0600); err != nil {
		t.Fatal(err)
	}
	failed := source.Snapshot()
	if failed.Runtime == nil || failed.State != steerline.SnapshotReadFailed || failed.Error == "" || !failed.SampledAt.Equal(first.SampledAt) {
		t.Fatalf("last good snapshot/time/error not retained: %+v", failed)
	}
}

func TestFileSourceFallbackNormalizerAndLateActivitySwitch(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte(`{"type":"session","version":1,"session_id":"p"}`+"\n"+`not-json`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := NewFileSource(steerline.Config{})
	fallback, err := source.ReadLog(logPath, steerline.JournalCursor{}, false)
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
	switched, err := source.ReadLog(logPath, fallback.Cursor, false)
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

func TestFileSourceDiagnosesBadActivityAndKeepsFollowingValidLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	activityPath := steerline.ActivityPath(logPath)
	if err := os.WriteFile(activityPath, []byte("{bad json}\nnull\n"+`{"v":1,"seq":1,"kind":"claim"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := NewFileSource(steerline.Config{}).ReadLog(logPath, steerline.JournalCursor{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Diagnostics) != 2 || len(batch.Activity) != 1 || batch.Activity[0].Kind != "claim" {
		t.Fatalf("bad activity line blocked the next valid record: %+v", batch)
	}
}

func TestFileSourceInvalidSequenceCannotEraseValidEvents(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	body := "{\"seq\":10,\"kind\":\"claim\"}\n{\"seq\":1}\n{\"seq\":11,\"kind\":\"message\"}\n"
	if err := os.WriteFile(steerline.ActivityPath(logPath), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := NewFileSource(steerline.Config{}).ReadLog(logPath, steerline.JournalCursor{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activity) != 2 || batch.Activity[0].Seq != 10 || batch.Activity[1].Seq != 11 || len(batch.Diagnostics) != 1 {
		t.Fatalf("bad record changed sequence state: events=%+v diagnostics=%v", batch.Activity, batch.Diagnostics)
	}
}

func TestFileSourceFreshReplayResetsNormalizer(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	source := NewFileSource(steerline.Config{})
	if err := os.WriteFile(logPath, []byte("{\"type\":\"thread.started\",\"thread_id\":\"old\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadLog(logPath, steerline.JournalCursor{}, false); err != nil {
		t.Fatal(err)
	}
	// A new visit starts at zero even if the same inode was rewritten while away.
	if err := os.WriteFile(logPath, []byte("{\"type\":\"session\",\"version\":1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := source.ReadLog(logPath, steerline.JournalCursor{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activity) != 1 || batch.Activity[0].Source != "pi" || batch.Activity[0].Kind != "session" {
		t.Fatalf("fresh replay reused another reader's normalizer: %+v", batch.Activity)
	}
}
