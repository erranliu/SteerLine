package controlled

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

// This public peer scripts a sealed adaptation result so recovery tests can
// pass the real caller admission gate. It proves no native environment or PR.
func scriptedEnvironment(input runnercontract.RunInput, run *runnercontract.Run) (runnercontract.ResultFile, error) {
	initial, err := base64.StdEncoding.Strict().DecodeString(input.ResultFile.InitialContentBase64)
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	var report env.Report
	if err = json.Unmarshal(initial, &report); err != nil {
		return runnercontract.ResultFile{}, err
	}
	raw, err := Git(context.Background(), input.Repository.LocalPath, "show", input.Repository.BaseRevision+":"+testmanagement.CataloguePath)
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	catalogue, err := testmanagement.ParseCatalogue([]byte(raw))
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	report.L2 = []env.Item{}
	for _, definition := range catalogue.Tests {
		if definition.Level == "L2" {
			report.L2 = append(report.L2, env.Item{ID: definition.ID, Status: "passed", Message: "scripted controlled peer"})
		}
	}
	if len(report.L2) == 0 {
		return runnercontract.ResultFile{}, fmt.Errorf("controlled environment requires a versioned L2 selection")
	}
	report.Completed = true
	report.Checks = []env.Check{{ID: "terminal.tty", Status: "passed", Message: "scripted fixture only; no native validation"}}
	report.PR = env.PR{URL: "https://example.invalid/environment/" + run.RunID, Base: input.Publication.BaseBranch, Head: run.Execution.Branch, HeadRevision: strings.Repeat("f", 40), CleanupCompleted: true}
	body, err := json.Marshal(report)
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	hash := sha256.Sum256(body)
	encoded := base64.StdEncoding.EncodeToString(body)
	now := time.Now().UTC()
	run.Status, run.Phase, run.StopStatus, run.NewInputsAllowed = "completed", "publish", "stopped", false
	run.Controls = runnercontract.Controls{}
	run.PR = &runnercontract.PRFact{State: "open", URL: report.PR.URL, BaseBranch: report.PR.Base, HeadBranch: report.PR.Head, Provider: input.Publication.Provider, Repository: input.Publication.Repository, HeadRepository: input.Publication.Repository, HeadRevision: report.PR.HeadRevision}
	return runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: run.RunID, Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: hex.EncodeToString(hash[:]), CollectionStatus: "closed"}, nil
}

func advertiseResult(caps map[string]any) {
	// Derive the final-only schema from this peer's existing public file shape.
	raw, _ := json.Marshal(caps["progress_file_schema"])
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	schema["$id"] = runnercontract.ResultContract
	properties := schema["properties"].(map[string]any)
	properties["contract"] = map[string]any{"const": runnercontract.ResultContract}
	properties["version"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 1}
	properties["collection_status"] = map[string]any{"type": "string", "enum": []string{"disabled", "pending", "closed"}}
	caps["result_file_schema"] = schema
	caps["result_file"] = map[string]any{"contract": runnercontract.ResultContract, "command": "result-file --run-id R --json", "submit_property": "result_file", "environment_variable": "STEERLINE_RESULT_FILE", "max_bytes": 65536, "encoding": "base64", "digest": "sha256", "collection": "end_only", "query_reads_file": false}
}
