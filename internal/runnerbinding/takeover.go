// Package runnerbinding matches immutable caller facts to public Runner proofs.
// It owns no transport, storage, resources or execution/recovery state machine.
package runnerbinding

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// ValidateBuildBindings is project admission after the selected transport has
// been verified. A never-bound submit keeps its original observe-only policy.
func ValidateBuildBindings(pin runnercontract.BuildPin, proof *runnercontract.TakeoverReport, candidates []runnerstate.Candidate) error {
	compatible := proof != nil && Compatible(*proof, pin)
	for _, candidate := range candidates {
		if SameBuild(candidate.RunnerBuild, pin) {
			continue
		}
		if !compatible || !SamePath(candidate.StateDir, pin.StateDir) {
			return errors.New("runner_build_changed: candidate takeover is unverified")
		}
		if candidate.RunID != "" {
			if _, ok := Candidate(*proof, candidate); !ok {
				return errors.New("candidate differs from original Run/input takeover facts")
			}
		}
	}
	return nil
}

// ValidateRestartBindings uses the ORIGINAL home/checkpoint pin. A report from
// the separately verified candidate may prove earlier builds without rewriting
// that pin. Unlike project admission, an unbound cross-build submit needs its
// accepted original request digest to establish offline restart eligibility.
func ValidateRestartBindings(pin runnercontract.BuildPin, proof *runnercontract.TakeoverReport, candidates []runnerstate.Candidate) error {
	for _, candidate := range candidates {
		bound := SameBuild(candidate.RunnerBuild, pin)
		if proof != nil {
			if _, ok := Candidate(*proof, candidate); ok {
				bound = true
			}
		}
		if !bound || !SamePath(candidate.StateDir, pin.StateDir) || candidate.RunID != "" && candidate.InputDigest == "" {
			return errors.New("restart candidate differs from the original Runner build or directory")
		}
	}
	return nil
}

func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.ToLower(a) == strings.ToLower(b)
	}
	return a == b
}

func SameBuild(a, b runnercontract.BuildPin) bool {
	return a.SHA256 == b.SHA256 && a.ResourceDigest == b.ResourceDigest && a.Version == b.Version && SamePath(a.StateDir, b.StateDir) && SamePath(a.Executable, b.Executable)
}

// Compatible retains the caller's original build identity comparison in
// addition to wire validation: SHA is exact and paths follow the host OS.
func Compatible(proof runnercontract.TakeoverReport, pin runnercontract.BuildPin) bool {
	return proof.Validate(pin.StateDir, pin.SHA256) == nil && proof.CandidateSHA256 == pin.SHA256 && SamePath(proof.StateDir, pin.StateDir)
}

// Candidate matches only original Run/input or the accepted submit envelope.
// Callers separately enforce directory/build admission and read-only authority.
func Candidate(proof runnercontract.TakeoverReport, c runnerstate.Candidate) (runnercontract.TakeoverItem, bool) {
	for _, item := range proof.Items {
		if item.Kind != "run" || item.Status != "compatible" && item.Status != "read_only" {
			continue
		}
		if c.RunID != "" {
			if item.ID == c.RunID && c.InputDigest != "" && item.InputDigest == c.InputDigest {
				return item, true
			}
		} else if item.RequestID == c.Submit.RequestID && item.RequestDigest == runnercontract.RequestDigest("submit", "", c.Submit) {
			return item, true
		}
	}
	return runnercontract.TakeoverItem{}, false
}
