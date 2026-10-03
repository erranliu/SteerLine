package steerlineclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

func TestConsoleSourceFallbackNormalizerAndLateActivitySwitch(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte(`{"type":"session","version":1,"session_id":"p"}`+"\n"+`not-json`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := NewLogReader(testNormalizer)
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
	batch, err := NewLogReader(testNormalizer).ReadLog(logPath, steerline.JournalCursor{})
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
	batch, err := NewLogReader(testNormalizer).ReadLog(logPath, steerline.JournalCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Activity) != 2 || batch.Activity[0].Seq != 10 || batch.Activity[1].Seq != 11 || len(batch.Diagnostics) != 1 {
		t.Fatalf("bad record changed sequence state: events=%+v diagnostics=%v", batch.Activity, batch.Diagnostics)
	}
}

func TestConsoleSourceFreshReplayResetsNormalizer(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	source := NewLogReader(testNormalizer)
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

func testNormalizer(first []byte) api.LogNormalizer {
	return steerline.DetectNormalizer(first, steerline.NewPiNormalizer(), codexworker.NewNormalizer())
}
