//go:build windows

package httpapi

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestRecordReaderAllowsOutcomeReplacement(t *testing.T) {
	root := t.TempDir()
	if err := ensurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	owner := newFileRequestRecordOwner(root)
	envelope := CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: ProjectID("repo"), InstanceID: "original", Action: "pause_dispatch", TargetID: "dispatch", Payload: json.RawMessage(`{}`)}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	record := requestRecord{Version: 1, Digest: digestBytes(canonical), Envelope: envelope, CreatedAt: time.Now().UTC(), Response: &requestOutcome{Status: "accepted"}}
	if err = owner.Register(record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, envelope.RequestID+".json")
	reader, err := openRequestRecordSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	record.Response = &requestOutcome{Status: "applied"}
	if err = owner.Write(record); err != nil {
		t.Fatal("an original receipt reader blocked atomic outcome persistence", err)
	}
	previous, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(previous), `"status":"accepted"`) {
		t.Fatal("held reader lost its original complete receipt", err)
	}
	latest, err := owner.Read(envelope.RequestID)
	if err != nil || latest.Response == nil || latest.Response.Status != "applied" || !sameEnvelope(latest.Envelope, envelope) || latest.Digest != record.Digest {
		t.Fatal("replacement changed the immutable envelope or lost the outcome", err)
	}
	// An unrelated handle that excludes deletion must still fail; no copy/delete
	// fallback may turn a failed atomic replacement into a partial publication.
	blocker, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	record.Response = &requestOutcome{Status: "rejected", Code: "example"}
	if err = owner.Write(record); err == nil {
		t.Fatal("persistent replacement exclusion was hidden")
	}
	latest, err = owner.Read(envelope.RequestID)
	if err != nil || latest.Response == nil || latest.Response.Status != "applied" {
		t.Fatal("failed replacement changed the last complete receipt", err)
	}
}
