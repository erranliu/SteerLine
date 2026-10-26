package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestProjectEnvironmentExecutionResultChannel(t *testing.T) {
	exe, _ := os.Executable()
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	call := filepath.Join(t.TempDir(), "call")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", call)
	client := Client{Executable: exe, StateDir: t.TempDir(), Timeout: 5 * time.Second}
	body := []byte(`{"completed":false}`)
	encoded := base64.StdEncoding.EncodeToString(body)
	now := time.Now().UTC()
	// Known SHA keeps this independent of the business report parser.
	p := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: "run", Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), CollectionStatus: "closed"}
	// Use the existing transport hash to build the peer fixture.
	sum := sha256.Sum256(body)
	p.SHA256 = hex.EncodeToString(sum[:])
	for _, tt := range []struct {
		name   string
		change func(*runnercontract.ResultFile)
		valid  bool
	}{
		{"result", func(*runnercontract.ResultFile) {}, true},
		{"progress-channel", func(p *runnercontract.ResultFile) { p.Contract = runnercontract.ProgressContract }, false},
		{"foreign-run", func(p *runnercontract.ResultFile) { p.RunID = "other" }, false},
		{"bad-digest", func(p *runnercontract.ResultFile) { p.SHA256 = strings.Repeat("0", 64) }, false},
		{"missing", func(p *runnercontract.ResultFile) { p.ContentBase64 = nil }, false},
		{"oversize", func(p *runnercontract.ResultFile) { p.SizeBytes = 65537 }, false},
		{"continuous-version", func(p *runnercontract.ResultFile) { p.Version = 2 }, false},
		{"observing", func(p *runnercontract.ResultFile) { p.CollectionStatus = "observing" }, false},
		{"pending-with-bytes", func(p *runnercontract.ResultFile) { p.CollectionStatus = "pending" }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := p
			tt.change(&v)
			raw, _ := json.Marshal(v)
			t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
			if _, e := client.ResultFile(context.Background(), "run"); (e == nil) != tt.valid {
				t.Fatal(e)
			}
		})
	}
	args, _ := os.ReadFile(call)
	if !strings.Contains(string(args), "result-file\n--run-id\nrun") {
		t.Fatal(string(args))
	}
	if _, e := ValidateProgressFile(p, "run"); e == nil {
		t.Fatal("result accepted in progress channel")
	}
}
func TestProjectEnvironmentExecutionCapabilityGate(t *testing.T) {
	c := &Catalog{}
	if c.ValidateResult() == nil {
		t.Fatal("missing capability accepted")
	}
	readsFile := false
	c.Capabilities.ResultFile = &ResultCapability{ProgressCapability: ProgressCapability{Contract: runnercontract.ResultContract, Command: "result-file --run-id R --json", SubmitProperty: "result_file", EnvironmentVariable: "STEERLINE_RESULT_FILE", MaxBytes: 65536, Encoding: "base64", Digest: "sha256"}, Collection: "end_only", QueryReadsFile: &readsFile}
	raw, e := os.ReadFile("testdata/result-file.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	c.Capabilities.ResultFileSchema = raw
	if e := c.ValidateResult(); e != nil {
		t.Fatal(e)
	}
	for _, schema := range []string{strings.ReplaceAll(string(raw), `"maximum": 1`, `"maximum": 2`), strings.ReplaceAll(string(raw), `"pending"`, `"observing"`)} {
		c.Capabilities.ResultFileSchema = json.RawMessage(schema)
		if c.ValidateResult() == nil {
			t.Fatal("continuous result schema accepted")
		}
	}
	c.Capabilities.ResultFileSchema = raw
	c.Capabilities.ResultFile.Collection = "continuous"
	if c.ValidateResult() == nil {
		t.Fatal("continuous result capability accepted")
	}
	c.Capabilities.ResultFile.Collection = "end_only"
	readsFile = true
	if c.ValidateResult() == nil {
		t.Fatal("file-reading query capability accepted")
	}
	readsFile = false
	c.Capabilities.ResultFile.EnvironmentVariable = "STEERLINE_PROGRESS_FILE"
	if c.ValidateResult() == nil {
		t.Fatal("channel variable alias accepted")
	}
}

func TestProjectEnvironmentExecutionFinalResultStates(t *testing.T) {
	for _, tt := range []struct {
		name, status, diagnostic string
		version                  uint64
		valid                    bool
	}{
		{"disabled", "disabled", "", 0, true},
		{"pending", "pending", "", 0, true},
		{"pending-initialization-diagnostic", "pending", "initialization_unverified", 0, true},
		{"failed-seal", "closed", "file_missing", 0, true},
		{"failure-without-reason", "closed", "", 0, false},
		{"observing", "observing", "", 0, false},
		{"detached", "detached", "", 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: "run", Encoding: "base64", CollectionStatus: tt.status, Diagnostic: tt.diagnostic, Version: tt.version}
			if _, e := ValidateResultFile(p, "run"); (e == nil) != tt.valid {
				t.Fatal(e)
			}
		})
	}
	encoded := ""
	now := time.Now().UTC()
	empty := runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: "run", Encoding: "base64", CollectionStatus: "closed", Version: 1, ObservedAt: &now, ContentBase64: &encoded, SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
	if raw, e := ValidateResultFile(empty, "run"); e != nil || len(raw) != 0 {
		t.Fatal("real empty file rejected", e)
	}
	empty.Version = 0
	empty.Diagnostic = "file_missing"
	if _, e := ValidateResultFile(empty, "run"); e == nil {
		t.Fatal("failed seal with bytes accepted")
	}
}
