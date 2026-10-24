package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProgressPublicCLIIdentityAndIntegrity(t *testing.T) {
	exe, _ := os.Executable()
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	call := filepath.Join(t.TempDir(), "call")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", call)
	c := Client{Executable: exe, StateDir: t.TempDir(), Timeout: 5 * time.Second}
	bytes := []byte(`{"test.one":true}`)
	sum := sha256.Sum256(bytes)
	encoded := base64.StdEncoding.EncodeToString(bytes)
	now := time.Now().UTC()
	good := runnercontract.ProgressFile{Contract: runnercontract.ProgressContract, RunID: "run", Version: 2, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(bytes), SHA256: hex.EncodeToString(sum[:]), CollectionStatus: "observing"}
	for _, tt := range []struct {
		name   string
		change func(*runnercontract.ProgressFile)
		ok     bool
	}{
		{"valid", func(*runnercontract.ProgressFile) {}, true},
		{"foreign", func(p *runnercontract.ProgressFile) { p.RunID = "other" }, false},
		{"digest", func(p *runnercontract.ProgressFile) { p.SHA256 = strings.Repeat("0", 64) }, false},
		{"size", func(p *runnercontract.ProgressFile) { p.SizeBytes++ }, false},
		{"missing-body", func(p *runnercontract.ProgressFile) { p.ContentBase64 = nil }, false},
		{"status", func(p *runnercontract.ProgressFile) { p.CollectionStatus = "passed" }, false},
		{"zero-with-body", func(p *runnercontract.ProgressFile) { p.Version = 0 }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := good
			tt.change(&p)
			raw, _ := json.Marshal(p)
			t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
			if _, err := c.ProgressFile(context.Background(), "run"); (err == nil) != tt.ok {
				t.Fatal(err)
			}
		})
	}
	argv, _ := os.ReadFile(call)
	if !strings.Contains(string(argv), "progress-file\n--run-id\nrun") {
		t.Fatal(string(argv))
	}
	p := runnercontract.ProgressFile{Contract: runnercontract.ProgressContract, RunID: "run", Encoding: "base64", CollectionStatus: "pending"}
	if _, err := ValidateProgressFile(p, "run"); err != nil {
		t.Fatal("version zero treated as empty snapshot", err)
	}
}
