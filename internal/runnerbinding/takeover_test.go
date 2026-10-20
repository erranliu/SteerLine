package runnerbinding

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestTakeoverCallerBindingPolicies(t *testing.T) {
	dir := t.TempDir()
	old := runnercontract.BuildPin{StateDir: dir, Executable: filepath.Join(dir, "runner.exe"), SHA256: "old", Version: "old", ResourceDigest: "original-resources"}
	current := old
	current.SHA256 = "candidate"
	current.Version = "candidate"
	c := runnerstate.Candidate{ID: "candidate", StateDir: dir, RunnerBuild: old, RunID: "run", InputDigest: "input", PRURL: "original-pr", StopRequested: true}
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", Compatible: true, StateDir: dir, CandidateSHA256: current.SHA256, Items: []runnercontract.TakeoverItem{{ID: "run", Kind: "run", InputDigest: "input", Status: "read_only", Revision: 7}}}
	before, _ := json.Marshal(c)
	if err := ValidateBuildBindings(current, &proof, []runnerstate.Candidate{c}, nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRestartBindings(current, &proof, []runnerstate.Candidate{c}, nil); err != nil {
		t.Fatal(err)
	}
	if item, ok := Candidate(proof, c); !ok || item.Status != "read_only" {
		t.Fatal("read-only authority lost")
	}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Fatal("original facts rewritten")
	}
	for _, bad := range []string{"input", "directory", "proof_sha"} {
		t.Run(bad, func(t *testing.T) {
			other := c
			report := proof
			switch bad {
			case "input":
				other.InputDigest = "foreign"
			case "directory":
				other.StateDir = filepath.Join(dir, "foreign")
			case "proof_sha":
				report.CandidateSHA256 = "foreign"
			}
			if ValidateBuildBindings(current, &report, []runnerstate.Candidate{other}, nil) == nil {
				t.Fatal("foreign binding admitted")
			}
		})
	}
	unknown := c
	unknown.RunID = ""
	unknown.InputDigest = ""
	unknown.Submit = runnercontract.Request{Protocol: "steerrunner-cli/v1", RequestID: "lost-submit", Input: json.RawMessage(`{"n":1234567890123456789}`)}
	if ValidateBuildBindings(current, &proof, []runnerstate.Candidate{unknown}, nil) != nil {
		t.Fatal("project admission changed unbound observe-only policy")
	}
	if ValidateRestartBindings(current, &proof, []runnerstate.Candidate{unknown}, nil) == nil {
		t.Fatal("offline restart accepted missing original envelope proof")
	}
	proof.Items[0].RequestID = unknown.Submit.RequestID
	proof.Items[0].RequestDigest = runnercontract.RequestDigest("submit", "", unknown.Submit)
	if ValidateRestartBindings(current, &proof, []runnerstate.Candidate{unknown}, nil) != nil {
		t.Fatal("accepted original request not matched")
	}
	unknown.Submit.Input = json.RawMessage(`{"n":1234567890123456790}`)
	if _, ok := Candidate(proof, unknown); ok {
		t.Fatal("changed original envelope matched")
	}
}

func TestTakeoverCheckBindings(t *testing.T) {
	dir := t.TempDir()
	c := runnerstate.EnvironmentCheckRecord{StateDir: dir, CheckID: "check", Report: &runnercontract.Check{InputDigest: "original"}}
	proof := runnercontract.TakeoverReport{StateDir: dir, Items: []runnercontract.TakeoverItem{{ID: "check", Kind: "check", Status: "read_only", InputDigest: "original"}}}
	if !Check(proof, c) {
		t.Fatal("original check not matched")
	}
	c.Report.InputDigest = "foreign"
	if Check(proof, c) {
		t.Fatal("foreign input matched")
	}
	c.CheckID = ""
	c.Report = nil
	c.Operations = []runnerstate.EnvironmentCheckOperation{{Status: "outcome_unknown", Request: runnercontract.Request{Protocol: "steerrunner-cli/v1", RequestID: "lost-check", Input: json.RawMessage(`{"access":"sandbox"}`)}}}
	proof.Items[0].RequestID = c.Operations[0].Request.RequestID
	proof.Items[0].RequestDigest = runnercontract.RequestDigest("environment-check start", "", c.Operations[0].Request)
	if !Check(proof, c) {
		t.Fatal("original lost check request not matched")
	}
	c.StateDir = filepath.Join(dir, "foreign")
	if Check(proof, c) {
		t.Fatal("foreign check directory matched")
	}
	c.StateDir = dir
	c.Operations[0].Status = "rejected"
	proof.Items = nil
	if !Check(proof, c) {
		t.Fatal("never-created rejected check needs native state")
	}
}

func TestTakeoverCallerBuildIdentity(t *testing.T) {
	pin := runnercontract.BuildPin{StateDir: filepath.Join(t.TempDir(), "mixed-case"), SHA256: "abc"}
	proof := runnercontract.TakeoverReport{Contract: "steerrunner-takeover/v1", Compatible: true, StateDir: pin.StateDir, CandidateSHA256: pin.SHA256, Items: []runnercontract.TakeoverItem{}}
	if !Compatible(proof, pin) {
		t.Fatal("original build identity rejected")
	}
	proof.CandidateSHA256 = "ABC"
	if Compatible(proof, pin) {
		t.Fatal("caller exact SHA comparison changed")
	}
	proof.CandidateSHA256 = pin.SHA256
	proof.StateDir = strings.ToUpper(pin.StateDir)
	if Compatible(proof, pin) != (runtime.GOOS == "windows") {
		t.Fatal("caller OS path identity comparison changed")
	}
	// EqualFold treats final sigma and sigma as equivalent, while the caller's
	// original canonical-path rule only lowercased each cleaned path. Keep the
	// original resource/build identity even for these Unicode path names.
	finalSigma := filepath.Join(filepath.Dir(pin.StateDir), "\u03c2")
	sigma := filepath.Join(filepath.Dir(pin.StateDir), "\u03c3")
	if !strings.EqualFold(finalSigma, sigma) || SamePath(finalSigma, sigma) {
		t.Fatal("caller canonical path identity was broadened by Unicode folding")
	}
	original := pin
	original.StateDir = finalSigma
	changed := original
	changed.StateDir = sigma
	if SameBuild(original, changed) {
		t.Fatal("distinct original Runner directories became the same build")
	}
}
