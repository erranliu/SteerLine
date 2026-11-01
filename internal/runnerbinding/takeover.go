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

// ValidateBuildBindings binds records to their namespace, Run and input. Build
// hashes describe provenance; they do not grant per-Run execution authority.
func ValidateBuildBindings(pin runnercontract.BuildPin, candidates []runnerstate.Candidate) error {
	for _, c := range candidates {
		if !SamePath(c.StateDir, pin.StateDir) || !SamePath(c.RunnerBuild.StateDir, pin.StateDir) || c.RunID != "" && c.InputDigest == "" {
			return errors.New("candidate Runner namespace or original input differs")
		}
	}
	return nil
}
func ValidateRestartBindings(pin runnercontract.BuildPin, candidates []runnerstate.Candidate) error {
	return ValidateBuildBindings(pin, candidates)
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
