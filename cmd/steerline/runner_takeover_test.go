package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRunnerTakeoverPreflight(t *testing.T) {
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "preflight-fixture")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(image)
	sha := hex.EncodeToString(sum[:])
	dir := t.TempDir()
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: dir, CandidateSHA256: sha, Compatible: true, Items: []runnercontract.TakeoverItem{{ID: "run", Kind: "run", Status: "read_only", InputDigest: "original"}}}
	for _, change := range []string{"valid", "duplicate", "directory", "sha", "unsupported"} {
		t.Run(change, func(t *testing.T) {
			value := proof
			value.Items = append([]runnercontract.TakeoverItem(nil), proof.Items...)
			switch change {
			case "duplicate":
				value.Items = append(value.Items, value.Items[0])
			case "directory":
				value.StateDir = filepath.Join(dir, "foreign")
			case "sha":
				value.CandidateSHA256 = strings.Repeat("b", 64)
			case "unsupported":
				value.Items[0].Status = "unsupported"
			}
			raw, _ := json.Marshal(value)
			if err := os.WriteFile(filepath.Join(dir, "idle-takeover-fixture.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			previousOut, previousArgs := os.Stdout, os.Args
			os.Stdout = writer
			os.Args = []string{"steerline", "verify-runner-takeover", "--runner-exe", executable, "--runner-sha256", sha, "--state-dir", dir, "--json"}
			defer func() { os.Stdout = previousOut; os.Args = previousArgs; reader.Close(); writer.Close() }()
			err = run()
			os.Stdout = previousOut
			os.Args = previousArgs
			writer.Close()
			if (err == nil) != (change == "valid") {
				t.Fatal("preflight admission differs", err)
			}
			if change == "valid" {
				var result runnercontract.TakeoverReport
				if err := json.NewDecoder(reader).Decode(&result); err != nil || result.CandidateSHA256 != sha || result.Items[0].Status != "read_only" {
					t.Fatal(result, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "idle-fixture.json")); !os.IsNotExist(err) {
				t.Fatal("preflight started service")
			}
		})
	}
}
