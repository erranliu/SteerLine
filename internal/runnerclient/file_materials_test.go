package runnerclient

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestFileMaterialManifest(t *testing.T) {
	body := "{\"original\":true}\r\n"
	expected := []runnercontract.Document{{Kind: "file", Path: "roadmap-inputs/graph.json", Content: &body}}
	location := filepath.Join(t.TempDir(), "graph.json")
	valid := runnercontract.Material{Kind: "file", Path: expected[0].Path, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte(body))), SnapshotPath: location, ExecutionLocation: location}
	if err := VerifyFileMaterials(expected, []runnercontract.Material{valid}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*runnercontract.Material){
		func(m *runnercontract.Material) { m.Path, m.Digest = "unknown", "" },
		func(m *runnercontract.Material) { m.Digest = "tampered" },
		func(m *runnercontract.Material) { m.Kind = "inline" },
		func(m *runnercontract.Material) { m.ExecutionLocation = "relative" },
		func(m *runnercontract.Material) { m.SnapshotPath = filepath.Join(t.TempDir(), "different") },
		func(m *runnercontract.Material) { m.Commit = "fabricated" },
	} {
		changed := valid
		change(&changed)
		if err := VerifyFileMaterials(expected, []runnercontract.Material{changed}); err == nil {
			t.Fatal("invalid original file binding accepted", changed)
		}
	}
	if VerifyFileMaterials(expected, nil) == nil || VerifyFileMaterials(expected, []runnercontract.Material{valid, valid}) == nil {
		t.Fatal("missing or duplicate file manifest accepted")
	}
}
