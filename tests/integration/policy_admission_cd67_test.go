package integration

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestProjectPolicyClaimRejectsStaticConflictsBeforeNativeQuery(t *testing.T) {
	c, _, store := nativeAdmissionFixture(t)
	commands := appExecutionCommands(t, store, c)
	codex := nativeProfileSettings(c)
	codex.Executable = filepath.Join(t.TempDir(), "must-never-be-called.exe")
	setNativeProfile(t, &c, codex)
	for _, mutate := range []func(*steerlineapp.Config){
		func(c *steerlineapp.Config) {
			settings := nativeProfileSettings(*c)
			settings.Model = " invalid "
			setNativeProfile(t, c, settings)
		},
		func(c *steerlineapp.Config) {
			settings := nativeProfileSettings(*c)
			settings.ReasoningEffort = "invalid"
			setNativeProfile(t, c, settings)
		},
		func(c *steerlineapp.Config) {
			settings := nativeProfileSettings(*c)
			settings.ReasoningEffort = "unrecognized"
			setNativeProfile(t, c, settings)
		},
		func(c *steerlineapp.Config) {
			settings := nativeProfileSettings(*c)
			settings.Model = "invalid\nmodel"
			setNativeProfile(t, c, settings)
		},
		func(c *steerlineapp.Config) { c.Host = "orca_supervised" },
	} {
		candidate := c
		mutate(&candidate)
		_, err := commands.ClaimNativeTask(context.Background(), candidate.ExecutionConfig(), execution.TaskClaimOptions{TaskID: "59"})
		if err == nil || strings.Contains(err.Error(), "must-never-be-called") {
			t.Fatalf("static policy conflict reached executable lookup: %v", err)
		}
	}
}

func TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests(t *testing.T) {
	for _, name := range []string{"local-native.json", "orca-native.json"} {
		raw, err := exec.Command("git", "show", "172cea6c6530b6693b742324a9c1d4f9aeb17b6d:docs/validation/execution-isolation-06-policy/"+name).Output()
		require(t, err)
		var report struct {
			Profile   executionmodel.FixedExecutionProfile `json:"profile"`
			Execution executionmodel.ExecutionRecord       `json:"execution"`
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			// Historical native-shaped receipts are no longer a supported wire format.
			if !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("historical native report failed for an unrelated reason: %v", err)
			}
			continue
		}
		if historicalProfileValid(report.Profile) {
			t.Fatalf("old native report was accepted as current format: %s", name)
		}
		if historicalResourceError(execution.Config{}, report.Execution.PolicyResources, report.Profile.Digest, report.Execution.Workspace.Worktree, &report.Execution) == nil {
			t.Fatalf("historical native permission receipt was accepted as current evidence: %s", name)
		}
	}
}
